-- Let an explicit "end this router's Hub connection" clear a Hub router's management
-- address, and nothing else.
--
-- guard_hub_management_address_move() blocks every change of management_ip on a verified
-- tunnel router unless a verified management address migration is in its router_retained
-- phase. DeviceRepository.ClearManagementTXN (router delete, when guest voucher history
-- forces the record to stay) clears the address to NULL, so the guard rejected it and the
-- delete returned 500.
--
-- The exception is deliberately narrow. The caller sets the transaction-local setting
-- app.hub_disconnect_device to the device's own id, and only then may an update:
--   * set management_ip to NULL (never to another address),
--   * leave transport unchanged (the transport rule above still applies),
--   * while the device has no management address migration in progress.
-- The setting is local to one transaction and names one device, so it cannot widen an
-- ordinary address change, and a move to a different address still needs the migration.
CREATE OR REPLACE FUNCTION guard_hub_management_address_move() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.transport='tunnel' AND OLD.management_ip IS NOT NULL AND OLD.ssh_secret_ciphertext IS NOT NULL AND NEW.transport IS DISTINCT FROM OLD.transport THEN
  RAISE EXCEPTION 'disconnect the verified Hub connection before changing transport';
 END IF;
 IF OLD.transport='tunnel' AND OLD.management_ip IS NOT NULL AND OLD.ssh_secret_ciphertext IS NOT NULL AND NEW.management_ip IS DISTINCT FROM OLD.management_ip THEN
  IF NEW.management_ip IS NULL
     AND NEW.ssh_secret_ciphertext IS NULL
     AND current_setting('app.hub_disconnect_device',true) = OLD.device_id::text
     AND NOT EXISTS (SELECT 1 FROM management_address_migrations m WHERE m.device_id=OLD.device_id AND m.business_id=OLD.business_id AND m.phase NOT IN ('completed','rolled_back')) THEN
   RETURN NEW;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM management_address_migrations m WHERE m.id::text=current_setting('app.management_migration_id',true) AND m.device_id=OLD.device_id AND m.business_id=OLD.business_id AND m.phase='router_retained' AND m.old_address=OLD.management_ip AND m.new_address=NEW.management_ip AND m.fingerprint=OLD.ssh_host_key_fingerprint) THEN
   RAISE EXCEPTION 'use verified management address migration';
  END IF;
 END IF;
 RETURN NEW;
END $$;
