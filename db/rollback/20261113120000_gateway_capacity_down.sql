-- Optional rollback of db/migrations/20261113120000_gateway_capacity.sql. Not run by mm-migrate.
-- Revert the application first (PR revert). The old code never reads the new objects.
-- This restores the 2026-10-24 one-router rule. It KEEPS gateway_capacity_grants (purchase evidence);
-- drop it only by a separate, explicit owner decision. Existing devices are never removed.
BEGIN;
CREATE OR REPLACE FUNCTION public.enforce_business_app_resource_cap() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
DECLARE used integer;
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.app_account_pass_rollouts WHERE rollout_key='individual-v1' AND enforcement_enabled AND starts_at<=now()) THEN RETURN NEW; END IF;
 IF EXISTS(SELECT 1 FROM public.users WHERE business_id=NEW.business_id AND is_active AND user_role='DEVELOPER') THEN RETURN NEW; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended('business-app-resources:'||NEW.business_id::text,0));
 IF TG_TABLE_NAME='sites' THEN
  IF TG_OP='UPDATE' AND NEW.business_id=OLD.business_id THEN RETURN NEW; END IF;
  SELECT count(*) INTO used FROM public.sites WHERE business_id=NEW.business_id AND site_id<>NEW.site_id;
  IF used>=1 THEN RAISE EXCEPTION 'One site per business is included' USING ERRCODE='23514',CONSTRAINT='app_business_site_limit'; END IF;
 ELSE
  IF NEW.deployment_layer NOT IN ('MAIN_GATEWAY','DOWNSTREAM_SWITCH') THEN RETURN NEW; END IF;
  IF TG_OP='UPDATE' AND NEW.business_id=OLD.business_id AND OLD.deployment_layer IN ('MAIN_GATEWAY','DOWNSTREAM_SWITCH') THEN RETURN NEW; END IF;
  SELECT count(*) INTO used FROM public.devices WHERE business_id=NEW.business_id AND deployment_layer IN ('MAIN_GATEWAY','DOWNSTREAM_SWITCH') AND device_id<>NEW.device_id;
  IF used>=1 THEN RAISE EXCEPTION 'One router per business is included' USING ERRCODE='23514',CONSTRAINT='app_business_router_limit'; END IF;
 END IF;
 RETURN NEW;
END $$;
DROP FUNCTION IF EXISTS business_gateway_capacity(uuid);
COMMIT;
