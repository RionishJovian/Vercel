-- Staff-only multi-device Wi-Fi codes. Deliberately separate from
-- guest_voucher_grants and ordinary one-device vouchers; no existing row
-- is migrated or modified.
CREATE TABLE IF NOT EXISTS vip_wifi_codes (
 id uuid NOT NULL DEFAULT uuid_generate_v4(),
 business_id uuid NOT NULL REFERENCES businesses(business_id),
 request_id uuid NOT NULL,
 device_id uuid NOT NULL,
 code text NOT NULL CHECK(code ~ '^VIP-[ABCDEFGHJKMNPQRSTVWXYZ]{8}$'),
 max_devices integer NOT NULL CHECK(max_devices BETWEEN 2 AND 8),
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','ready','revoking','revoked')),
 lease_token text,
 lease_until timestamptz,
 created_by uuid,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(business_id,id),
 UNIQUE(business_id,request_id),
 UNIQUE(business_id,code),
 FOREIGN KEY(device_id,business_id) REFERENCES devices(device_id,business_id),
 CHECK((lease_token IS NULL)=(lease_until IS NULL))
);
CREATE INDEX IF NOT EXISTS vip_wifi_codes_device ON vip_wifi_codes(business_id,device_id);

CREATE OR REPLACE FUNCTION guard_vip_wifi_code_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.business_id,NEW.id,NEW.request_id,NEW.device_id,NEW.code,NEW.max_devices,NEW.created_by,NEW.created_at)
 IS DISTINCT FROM (OLD.business_id,OLD.id,OLD.request_id,OLD.device_id,OLD.code,OLD.max_devices,OLD.created_by,OLD.created_at) THEN
   RAISE EXCEPTION 'VIP identity and device policy are immutable';
 END IF;
 IF OLD.status='revoked' AND NEW.status<>'revoked' THEN
   RAISE EXCEPTION 'revoked VIP cannot be reissued';
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS vip_wifi_code_identity_guard ON vip_wifi_codes;
CREATE TRIGGER vip_wifi_code_identity_guard BEFORE UPDATE ON vip_wifi_codes
 FOR EACH ROW EXECUTE FUNCTION guard_vip_wifi_code_identity();

ALTER TABLE vip_wifi_codes ENABLE ROW LEVEL SECURITY;
ALTER TABLE vip_wifi_codes FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS vip_wifi_codes_tenant ON vip_wifi_codes;
CREATE POLICY vip_wifi_codes_tenant ON vip_wifi_codes FOR ALL TO authenticated
 USING(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid)
 WITH CHECK(business_id=NULLIF(current_setting('app.current_business_id',true),'')::uuid);
GRANT SELECT,INSERT,UPDATE ON vip_wifi_codes TO authenticated;
