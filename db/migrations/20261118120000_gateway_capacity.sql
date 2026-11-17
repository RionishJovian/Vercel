-- Paid additional gateways (owner decision 2026-11-13).
--
-- One gateway is included with every client account. Approved Store router-slot
-- purchases and explicit platform grants add gateway capacity to the same account
-- and site. This migration makes one database function the authority for that
-- capacity and replaces the hard-coded "one router per business" check.
--
-- Additive only: existing devices, device_allowances rows, device_requests and
-- store_addon_requests are not rewritten. Legacy router grants recorded in
-- device_allowances.routers keep counting (above the included unit).
--
-- Downstream switches are not gateways. Their behaviour is unchanged: when
-- account-pass enforcement is on, a switch is still refused once the business has
-- any gateway or switch (the previous rule), and legacy mode keeps its
-- application-level device_allowances check.

CREATE TABLE gateway_capacity_grants (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
 units integer NOT NULL CHECK (units BETWEEN 1 AND 100),
 source text NOT NULL CHECK (source IN ('store_purchase','operator_grant')),
 -- Exactly-once fulfilment: one grant per approved purchase request.
 store_request_id uuid UNIQUE REFERENCES store_addon_requests(id) ON DELETE RESTRICT,
 -- Product and term snapshot taken from the purchase when it is fulfilled.
 addon_key text,
 product_name_en text,
 period text CHECK (period IN ('one_time','monthly','annual')),
 price_mmk integer CHECK (price_mmk >= 0),
 -- Renewal, expiry, refund and transfer rules have not been decided. A grant
 -- therefore never lapses by itself; 'not_enforced' records that explicitly.
 term_policy text NOT NULL DEFAULT 'not_enforced' CHECK (term_policy = 'not_enforced'),
 reason text NOT NULL DEFAULT '' CHECK (length(reason) <= 500),
 granted_by uuid,
 granted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 revoked_at timestamptz,
 revoked_by uuid,
 revoke_reason text CHECK (revoke_reason IS NULL OR length(revoke_reason) BETWEEN 1 AND 500),
 CONSTRAINT gateway_grant_source_link CHECK ((source = 'store_purchase') = (store_request_id IS NOT NULL)),
 CONSTRAINT gateway_grant_operator_reason CHECK (source <> 'operator_grant' OR length(reason) >= 3),
 CONSTRAINT gateway_grant_revocation CHECK ((revoked_at IS NULL) = (revoked_by IS NULL) AND (revoked_at IS NULL) = (revoke_reason IS NULL))
);
CREATE INDEX gateway_capacity_grants_business ON gateway_capacity_grants(business_id) WHERE revoked_at IS NULL;

-- A store grant must come from an approved router-slot purchase of the same
-- business, and its snapshot is copied from that purchase, never from the caller.
-- Grants are append-only; the only permitted change is a single revocation.
CREATE FUNCTION guard_gateway_capacity_grant() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE r record;
BEGIN
 IF TG_OP = 'INSERT' THEN
  IF NEW.source = 'store_purchase' THEN
   SELECT q.business_id, q.status, q.price_mmk, q.period, a.kind, a.addon_key, a.name_en INTO r
     FROM public.store_addon_requests q JOIN public.store_addons a ON a.id = q.addon_id
    WHERE q.id = NEW.store_request_id FOR SHARE OF q;
   IF NOT FOUND OR r.business_id <> NEW.business_id OR r.status <> 'approved' OR r.kind <> 'router_slot' THEN
    RAISE EXCEPTION 'only an approved router-slot purchase of this business can add gateway capacity' USING ERRCODE = '23514', CONSTRAINT = 'gateway_grant_purchase';
   END IF;
   NEW.units := 1;
   NEW.addon_key := r.addon_key; NEW.product_name_en := r.name_en; NEW.period := r.period; NEW.price_mmk := r.price_mmk;
  END IF;
  NEW.revoked_at := NULL; NEW.revoked_by := NULL; NEW.revoke_reason := NULL;
  RETURN NEW;
 END IF;
 IF OLD.revoked_at IS NOT NULL
    OR (NEW.id, NEW.business_id, NEW.units, NEW.source, NEW.store_request_id, NEW.addon_key, NEW.product_name_en, NEW.period, NEW.price_mmk, NEW.term_policy, NEW.reason, NEW.granted_by, NEW.granted_at)
       IS DISTINCT FROM
       (OLD.id, OLD.business_id, OLD.units, OLD.source, OLD.store_request_id, OLD.addon_key, OLD.product_name_en, OLD.period, OLD.price_mmk, OLD.term_policy, OLD.reason, OLD.granted_by, OLD.granted_at) THEN
  RAISE EXCEPTION 'gateway capacity grants are append-only' USING ERRCODE = '23514', CONSTRAINT = 'gateway_grant_immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER gateway_capacity_grant_guard BEFORE INSERT OR UPDATE ON gateway_capacity_grants FOR EACH ROW EXECUTE FUNCTION guard_gateway_capacity_grant();

ALTER TABLE gateway_capacity_grants ENABLE ROW LEVEL SECURITY;
ALTER TABLE gateway_capacity_grants FORCE ROW LEVEL SECURITY;
CREATE POLICY gateway_capacity_grants_tenant_read ON gateway_capacity_grants FOR SELECT TO authenticated
 USING (business_id = NULLIF(current_setting('app.current_business_id', true), '')::uuid);
REVOKE ALL ON gateway_capacity_grants FROM PUBLIC, authenticated;
GRANT SELECT ON gateway_capacity_grants TO authenticated;

-- The single source of truth for how many gateways a business may register:
-- one included + legacy router grants above the included unit + active grants.
CREATE FUNCTION business_gateway_capacity(b uuid) RETURNS integer LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,public AS $$
 SELECT (1
   + COALESCE((SELECT GREATEST(a.routers - 1, 0) FROM public.device_allowances a WHERE a.business_id = b), 0)
   + COALESCE((SELECT sum(g.units) FROM public.gateway_capacity_grants g WHERE g.business_id = b AND g.revoked_at IS NULL), 0))::integer
$$;
REVOKE ALL ON FUNCTION business_gateway_capacity(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION business_gateway_capacity(uuid) TO authenticated;

-- Replaces the 2026-10-24 rule. Sites keep "one per business" under enforcement.
-- Gateways are limited by business_gateway_capacity() in every mode, serialized
-- per business so two requests can never both take the last slot. Switches keep
-- the previous rule. Platform operator workspaces stay exempt.
CREATE OR REPLACE FUNCTION enforce_business_app_resource_cap() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE used integer; allowed integer; enforced boolean;
BEGIN
 IF EXISTS(SELECT 1 FROM public.users WHERE business_id=NEW.business_id AND is_active AND user_role='DEVELOPER') THEN RETURN NEW; END IF;
 enforced := EXISTS(SELECT 1 FROM public.app_account_pass_rollouts WHERE rollout_key='individual-v1' AND enforcement_enabled AND starts_at<=now());
 IF TG_TABLE_NAME='sites' THEN
  IF NOT enforced THEN RETURN NEW; END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended('business-app-resources:'||NEW.business_id::text,0));
  IF TG_OP='UPDATE' AND NEW.business_id=OLD.business_id THEN RETURN NEW; END IF;
  SELECT count(*) INTO used FROM public.sites WHERE business_id=NEW.business_id AND site_id<>NEW.site_id;
  IF used>=1 THEN RAISE EXCEPTION 'One site per business is included' USING ERRCODE='23514',CONSTRAINT='app_business_site_limit'; END IF;
  RETURN NEW;
 END IF;
 IF NEW.deployment_layer = 'MAIN_GATEWAY' THEN
  IF TG_OP='UPDATE' AND NEW.business_id=OLD.business_id AND OLD.deployment_layer='MAIN_GATEWAY' THEN RETURN NEW; END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended('business-app-resources:'||NEW.business_id::text,0));
  allowed := public.business_gateway_capacity(NEW.business_id);
  SELECT count(*) INTO used FROM public.devices WHERE business_id=NEW.business_id AND deployment_layer='MAIN_GATEWAY' AND device_id<>NEW.device_id;
  IF used>=allowed THEN
   RAISE EXCEPTION 'Gateway capacity reached' USING ERRCODE='23514', CONSTRAINT='app_business_gateway_capacity', DETAIL=format('used=%s capacity=%s', used, allowed);
  END IF;
  RETURN NEW;
 END IF;
 IF NEW.deployment_layer = 'DOWNSTREAM_SWITCH' AND enforced THEN
  IF TG_OP='UPDATE' AND NEW.business_id=OLD.business_id AND OLD.deployment_layer IN ('MAIN_GATEWAY','DOWNSTREAM_SWITCH') THEN RETURN NEW; END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended('business-app-resources:'||NEW.business_id::text,0));
  SELECT count(*) INTO used FROM public.devices WHERE business_id=NEW.business_id AND deployment_layer IN ('MAIN_GATEWAY','DOWNSTREAM_SWITCH') AND device_id<>NEW.device_id;
  IF used>=1 THEN RAISE EXCEPTION 'One router per business is included' USING ERRCODE='23514',CONSTRAINT='app_business_router_limit'; END IF;
 END IF;
 RETURN NEW;
END $$;
