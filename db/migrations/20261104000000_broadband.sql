-- Additive router-local broadband ledger; existing products and accounts are untouched.
CREATE UNIQUE INDEX IF NOT EXISTS devices_id_business_key ON devices(device_id,business_id);
CREATE TABLE broadband_accounts (
 id uuid NOT NULL,
 business_id uuid NOT NULL REFERENCES businesses(business_id),
 device_id uuid NOT NULL,
 username text NOT NULL CHECK(length(username) BETWEEN 1 AND 64),
 retired boolean NOT NULL DEFAULT false,
 profile text NOT NULL CHECK(length(profile) BETWEEN 1 AND 64),
 observation jsonb,
 router_operation uuid,
 observed_at timestamptz,
 session jsonb,
 session_observed_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(business_id,id),
 UNIQUE(business_id,device_id,id),
 FOREIGN KEY(device_id,business_id) REFERENCES devices(device_id,business_id)
);
CREATE UNIQUE INDEX broadband_account_name ON broadband_accounts(business_id,device_id,username) WHERE NOT retired;
CREATE TABLE broadband_operations (
 id uuid NOT NULL,
 operation_order bigint GENERATED ALWAYS AS IDENTITY,
 business_id uuid NOT NULL,
 device_id uuid NOT NULL,
 account_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('create','renew','suspend','resume')),
 amount_mmk bigint NOT NULL DEFAULT 0 CHECK(amount_mmk BETWEEN 0 AND 1000000000),
 period_days integer NOT NULL DEFAULT 0,
 request_hash text NOT NULL,
 payload_ciphertext text NOT NULL,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','applied','rejected')),
 created_at timestamptz NOT NULL DEFAULT now(),
 applied_at timestamptz,
 PRIMARY KEY(business_id,id),
 FOREIGN KEY(business_id,device_id,account_id) REFERENCES broadband_accounts(business_id,device_id,id),
 CHECK((kind IN ('create','renew') AND period_days BETWEEN 1 AND 366) OR
       (kind IN ('suspend','resume') AND period_days=0 AND amount_mmk=0)),
 CHECK((state IN ('pending','rejected') AND applied_at IS NULL) OR (state='applied' AND applied_at IS NOT NULL))
);
CREATE UNIQUE INDEX broadband_one_pending_operation ON broadband_operations(business_id,account_id) WHERE state='pending';
CREATE INDEX broadband_router_accounts ON broadband_accounts(business_id,device_id);
CREATE INDEX broadband_payment_report ON broadband_operations(business_id,applied_at) WHERE state='applied';
CREATE TABLE broadband_session_usage (
 business_id uuid NOT NULL,
 device_id uuid NOT NULL,
 account_id uuid NOT NULL,
 session_id text NOT NULL CHECK(length(session_id) BETWEEN 1 AND 64),
 download_bytes bigint NOT NULL CHECK(download_bytes>=0),
 upload_bytes bigint NOT NULL CHECK(upload_bytes>=0),
 uptime_seconds bigint NOT NULL CHECK(uptime_seconds>=0),
 counter_reset boolean NOT NULL DEFAULT false,
 observed_at timestamptz NOT NULL,
 PRIMARY KEY(business_id,account_id,session_id),
 FOREIGN KEY(business_id,device_id,account_id) REFERENCES broadband_accounts(business_id,device_id,id)
);
ALTER TABLE broadband_session_usage ENABLE ROW LEVEL SECURITY;
ALTER TABLE broadband_session_usage FORCE ROW LEVEL SECURITY;
CREATE POLICY broadband_session_usage_tenant ON broadband_session_usage TO authenticated
 USING(business_id=nullif(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK(business_id=nullif(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON broadband_session_usage TO authenticated;
ALTER TABLE broadband_accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE broadband_accounts FORCE ROW LEVEL SECURITY;
ALTER TABLE broadband_operations ENABLE ROW LEVEL SECURITY;
ALTER TABLE broadband_operations FORCE ROW LEVEL SECURITY;
CREATE POLICY broadband_accounts_tenant ON broadband_accounts TO authenticated
 USING(business_id=nullif(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK(business_id=nullif(current_setting('app.current_business_id',true),'')::uuid);
CREATE POLICY broadband_operations_tenant ON broadband_operations TO authenticated
 USING(business_id=nullif(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK(business_id=nullif(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON broadband_accounts,broadband_operations TO authenticated;
GRANT USAGE,SELECT ON SEQUENCE broadband_operations_operation_order_seq TO authenticated;
CREATE FUNCTION guard_broadband_operation() RETURNS trigger LANGUAGE plpgsql SET search_path=public AS $$
BEGIN
 IF NEW.operation_order IS DISTINCT FROM OLD.operation_order THEN RAISE EXCEPTION 'operation order is immutable'; END IF;
 IF (NEW.id,NEW.business_id,NEW.device_id,NEW.account_id,NEW.kind,NEW.amount_mmk,NEW.period_days,NEW.request_hash,NEW.created_at)
 IS DISTINCT FROM
 (OLD.id,OLD.business_id,OLD.device_id,OLD.account_id,OLD.kind,OLD.amount_mmk,OLD.period_days,OLD.request_hash,OLD.created_at)
 OR (current_user='authenticated' AND NEW.payload_ciphertext IS DISTINCT FROM OLD.payload_ciphertext)
 OR (OLD.state IN ('applied','rejected') AND (NEW.state,NEW.applied_at) IS DISTINCT FROM (OLD.state,OLD.applied_at)) THEN
  RAISE EXCEPTION 'broadband operation is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER broadband_operation_guard BEFORE UPDATE ON broadband_operations FOR EACH ROW EXECUTE FUNCTION guard_broadband_operation();
