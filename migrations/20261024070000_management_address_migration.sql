BEGIN;
CREATE UNIQUE INDEX IF NOT EXISTS devices_migration_tenant_identity ON devices(device_id,business_id);
CREATE TABLE IF NOT EXISTS management_address_migrations (
 id uuid PRIMARY KEY,
 business_id uuid NOT NULL REFERENCES businesses(business_id),
 device_id uuid NOT NULL,
 requested_by uuid NOT NULL,
 old_address inet NOT NULL, new_address inet NOT NULL,
 interface_name text NOT NULL, old_record text NOT NULL,
 fingerprint text NOT NULL, router_key text NOT NULL,
 guard_job text NOT NULL DEFAULT '', backup_name text NOT NULL DEFAULT '',
 phase text NOT NULL CHECK (phase IN ('planned','starting','hub_staged','router_staged','new_verified','hub_committed','router_retained','connection_saved','router_cleaned','completed','rolled_back')),
 revision integer NOT NULL DEFAULT 0,
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(device_id,business_id) REFERENCES devices(device_id,business_id) ON DELETE CASCADE,
 FOREIGN KEY(requested_by,business_id) REFERENCES users(user_id,business_id),
 CHECK (family(old_address)=4 AND family(new_address)=4 AND masklen(old_address)=32 AND masklen(new_address)=32 AND old_address<>new_address)
);
CREATE UNIQUE INDEX IF NOT EXISTS management_migration_one_active ON management_address_migrations(device_id) WHERE phase NOT IN ('completed','rolled_back');
ALTER TABLE management_address_migrations ENABLE ROW LEVEL SECURITY;
ALTER TABLE management_address_migrations FORCE ROW LEVEL SECURITY;
CREATE POLICY management_migration_tenant ON management_address_migrations USING (business_id = NULLIF(current_setting('app.current_business_id',true),'')::uuid) WITH CHECK (business_id = NULLIF(current_setting('app.current_business_id',true),'')::uuid);
CREATE OR REPLACE FUNCTION guard_hub_management_address_move() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.transport='tunnel' AND OLD.management_ip IS NOT NULL AND OLD.ssh_secret_ciphertext IS NOT NULL AND NEW.transport IS DISTINCT FROM OLD.transport THEN
  RAISE EXCEPTION 'disconnect the verified Hub connection before changing transport';
 END IF;
 IF OLD.transport='tunnel' AND OLD.management_ip IS NOT NULL AND OLD.ssh_secret_ciphertext IS NOT NULL AND NEW.management_ip IS DISTINCT FROM OLD.management_ip THEN
  IF NOT EXISTS (SELECT 1 FROM management_address_migrations m WHERE m.id::text=current_setting('app.management_migration_id',true) AND m.device_id=OLD.device_id AND m.business_id=OLD.business_id AND m.phase='router_retained' AND m.old_address=OLD.management_ip AND m.new_address=NEW.management_ip AND m.fingerprint=OLD.ssh_host_key_fingerprint) THEN
   RAISE EXCEPTION 'use verified management address migration';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER guard_hub_management_address_move BEFORE UPDATE OF management_ip,transport ON devices FOR EACH ROW EXECUTE FUNCTION guard_hub_management_address_move();
CREATE OR REPLACE FUNCTION guard_pending_management_device_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM management_address_migrations WHERE device_id=OLD.device_id AND business_id=OLD.business_id AND phase NOT IN ('completed','rolled_back')) THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='management_migration_pending';
 END IF;
 RETURN OLD;
END $$;
CREATE TRIGGER guard_pending_management_device_delete BEFORE DELETE ON devices FOR EACH ROW EXECUTE FUNCTION guard_pending_management_device_delete();
GRANT SELECT,INSERT,UPDATE ON management_address_migrations TO authenticated;
COMMIT;
