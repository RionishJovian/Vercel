-- Additive record of which vouchers the owner has handed out. App data only: nothing on the router changes.
CREATE TABLE IF NOT EXISTS voucher_handouts (
 business_id uuid NOT NULL,
 voucher_id uuid NOT NULL,
 handed_out_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(business_id,voucher_id),
 FOREIGN KEY(voucher_id,business_id) REFERENCES vouchers(id,business_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS voucher_handouts_voucher ON voucher_handouts(voucher_id,business_id);
ALTER TABLE voucher_handouts ENABLE ROW LEVEL SECURITY;
ALTER TABLE voucher_handouts FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS voucher_handouts_tenant ON voucher_handouts;
CREATE POLICY voucher_handouts_tenant ON voucher_handouts TO authenticated
 USING(business_id=nullif(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK(business_id=nullif(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT,INSERT,DELETE ON voucher_handouts TO authenticated;
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='mega_magic_runtime') THEN
  GRANT SELECT,INSERT,DELETE ON voucher_handouts TO mega_magic_runtime;
  DROP POLICY IF EXISTS mm_runtime_service ON voucher_handouts;
  CREATE POLICY mm_runtime_service ON voucher_handouts FOR ALL TO mega_magic_runtime USING (true) WITH CHECK (true);
 END IF;
END $$;
