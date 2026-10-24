-- The rollout clock is armed explicitly at enforcement deployment, not migration.
BEGIN;
CREATE TABLE app_account_pass_rollouts (
 rollout_key text PRIMARY KEY CHECK(rollout_key='individual-v1'),
 starts_at timestamptz NOT NULL,
 transition_days integer NOT NULL CHECK(transition_days=7),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE app_account_pass_transitions (
 user_id uuid PRIMARY KEY,
 business_id uuid NOT NULL,
 rollout_key text NOT NULL REFERENCES app_account_pass_rollouts(rollout_key),
 expires_at timestamptz NOT NULL,
 FOREIGN KEY(user_id,business_id) REFERENCES users(user_id,business_id)
);
ALTER TABLE app_account_pass_transitions ENABLE ROW LEVEL SECURITY;
ALTER TABLE app_account_pass_transitions FORCE ROW LEVEL SECURITY;
CREATE POLICY app_account_pass_transitions_tenant_read ON app_account_pass_transitions FOR SELECT TO authenticated
 USING(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT ON app_account_pass_transitions TO authenticated;
CREATE TABLE app_account_pass_renewals (
 id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
 user_id uuid NOT NULL,
 business_id uuid NOT NULL,
 plan text NOT NULL CHECK(plan IN ('monthly','annual')),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected')),
 created_at timestamptz NOT NULL DEFAULT now(),
 decided_at timestamptz,
 decided_by uuid REFERENCES users(user_id),
 pass_revision bigint,
 FOREIGN KEY(user_id,business_id) REFERENCES users(user_id,business_id)
);
CREATE UNIQUE INDEX app_account_one_pending_renewal ON app_account_pass_renewals(user_id) WHERE status='pending';
ALTER TABLE app_account_pass_renewals ENABLE ROW LEVEL SECURITY;
ALTER TABLE app_account_pass_renewals FORCE ROW LEVEL SECURITY;
CREATE POLICY app_account_pass_renewals_tenant ON app_account_pass_renewals TO authenticated
 USING(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT,INSERT ON app_account_pass_renewals TO authenticated;

-- A business has one authorized customer login; other user/history rows remain.
CREATE TABLE business_app_primary_users (
 business_id uuid PRIMARY KEY REFERENCES businesses(business_id),
 user_id uuid NOT NULL,
 updated_by uuid REFERENCES users(user_id),
 updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(user_id,business_id) REFERENCES users(user_id,business_id)
);
ALTER TABLE business_app_primary_users ENABLE ROW LEVEL SECURITY;
ALTER TABLE business_app_primary_users FORCE ROW LEVEL SECURITY;
CREATE POLICY business_app_primary_read ON business_app_primary_users FOR SELECT TO authenticated
 USING(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT ON business_app_primary_users TO authenticated;
CREATE TABLE business_app_primary_events (
 id bigserial PRIMARY KEY,
 business_id uuid NOT NULL REFERENCES businesses(business_id),
 user_id uuid NOT NULL,
 previous_user_id uuid,
 changed_by uuid NOT NULL REFERENCES users(user_id),
 reason text NOT NULL CHECK(length(reason) BETWEEN 5 AND 500),
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(user_id,business_id) REFERENCES users(user_id,business_id)
);
ALTER TABLE business_app_primary_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE business_app_primary_events FORCE ROW LEVEL SECURITY;
-- System audit only; no authenticated mutation privileges.
-- Existing canonical owner identity is evidence; ambiguous owners are not guessed.
INSERT INTO business_app_primary_users(business_id,user_id)
 SELECT b.business_id,u.user_id FROM businesses b JOIN users u ON u.business_id=b.business_id AND u.email=b.owner_email AND u.user_role='OWNER';
CREATE FUNCTION assign_new_business_primary_login() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
 IF NEW.user_role='OWNER' AND EXISTS(SELECT 1 FROM public.businesses b WHERE b.business_id=NEW.business_id AND b.owner_email=NEW.email) THEN
  INSERT INTO public.business_app_primary_users(business_id,user_id) VALUES(NEW.business_id,NEW.user_id) ON CONFLICT(business_id) DO NOTHING;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER new_business_primary_login AFTER INSERT ON users FOR EACH ROW EXECUTE FUNCTION assign_new_business_primary_login();
ALTER TABLE app_account_pass_rollouts ADD COLUMN enforcement_enabled boolean NOT NULL DEFAULT false;
-- Serialize every creation path, including Hub helpers that bypass UI handlers.
CREATE FUNCTION enforce_business_app_resource_cap() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
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
CREATE TRIGGER app_site_capacity BEFORE INSERT OR UPDATE OF business_id ON sites FOR EACH ROW EXECUTE FUNCTION enforce_business_app_resource_cap();
CREATE TRIGGER app_router_capacity BEFORE INSERT OR UPDATE OF business_id,deployment_layer ON devices FOR EACH ROW EXECUTE FUNCTION enforce_business_app_resource_cap();
COMMIT;
