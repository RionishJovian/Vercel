-- Per-device SSH credentials and mandatory host-key pinning.
--
-- Rollout is intentionally two-stage. Existing installations may already have
-- encrypted SSH credentials but no host-key fingerprint. PostgreSQL CHECK ...
-- NOT VALID protects every new or changed row immediately while allowing this
-- migration to complete without deleting credentials or inventing trust data.
-- After operators enroll real fingerprints, run the validation query at the
-- bottom and then VALIDATE CONSTRAINT.

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30s';

ALTER TABLE public.devices ADD COLUMN IF NOT EXISTS ssh_port integer NOT NULL DEFAULT 22;
ALTER TABLE public.devices ADD COLUMN IF NOT EXISTS ssh_username text;
ALTER TABLE public.devices ADD COLUMN IF NOT EXISTS ssh_secret_ciphertext text;
ALTER TABLE public.devices ADD COLUMN IF NOT EXISTS ssh_secret_kind text;
ALTER TABLE public.devices ADD COLUMN IF NOT EXISTS ssh_host_key_fingerprint text;

-- Replace only an absent/older definition. A correct constraint may already be
-- VALIDATED; leaving it in place makes re-running this migration monotonic and
-- prevents a recovery run from silently downgrading it to NOT VALID.
DO $migration$
DECLARE
    constraint_definition text;
BEGIN
    SELECT pg_get_constraintdef(oid)
      INTO constraint_definition
      FROM pg_constraint
     WHERE conrelid = 'public.devices'::regclass
       AND conname = 'devices_ssh_creds_complete';

    IF constraint_definition IS NULL
       OR position('ssh_host_key_fingerprint' IN constraint_definition) = 0
       OR position('SHA256:' IN constraint_definition) = 0 THEN
        ALTER TABLE public.devices DROP CONSTRAINT IF EXISTS devices_ssh_creds_complete;
        ALTER TABLE public.devices ADD CONSTRAINT devices_ssh_creds_complete CHECK (
            (ssh_username IS NULL
             AND ssh_secret_ciphertext IS NULL
             AND ssh_secret_kind IS NULL
             AND ssh_host_key_fingerprint IS NULL)
         OR (ssh_username IS NOT NULL
             AND ssh_secret_ciphertext IS NOT NULL
             AND ssh_secret_kind IN ('password', 'key')
             AND ssh_host_key_fingerprint IS NOT NULL
             AND ssh_host_key_fingerprint ~ '^SHA256:[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]$')
        ) NOT VALID;
    END IF;

    SELECT pg_get_constraintdef(oid)
      INTO constraint_definition
      FROM pg_constraint
     WHERE conrelid = 'public.devices'::regclass
       AND conname = 'devices_ssh_port_valid';

    IF constraint_definition IS NULL
       OR position('ssh_port' IN constraint_definition) = 0
       OR position('65535' IN constraint_definition) = 0 THEN
        ALTER TABLE public.devices DROP CONSTRAINT IF EXISTS devices_ssh_port_valid;
        ALTER TABLE public.devices ADD CONSTRAINT devices_ssh_port_valid
            CHECK (ssh_port BETWEEN 1 AND 65535) NOT VALID;
    END IF;
END
$migration$;

CREATE INDEX IF NOT EXISTS devices_ssh_target_idx
    ON public.devices(device_id)
    WHERE management_ip IS NOT NULL AND ssh_username IS NOT NULL;

COMMENT ON COLUMN public.devices.ssh_host_key_fingerprint IS
    'Pinned OpenSSH SHA-256 host-key fingerprint; required before SSH control is usable';

COMMIT;

-- Pre-validation audit (must return zero rows):
-- SELECT device_id, business_id, hardware_name,
--        (ssh_username IS NOT NULL OR ssh_secret_ciphertext IS NOT NULL
--         OR ssh_secret_kind IS NOT NULL) AS has_legacy_ssh_credentials,
--        ssh_host_key_fingerprint
--   FROM public.devices
--  WHERE NOT (
--      (ssh_username IS NULL AND ssh_secret_ciphertext IS NULL
--       AND ssh_secret_kind IS NULL AND ssh_host_key_fingerprint IS NULL)
--   OR (ssh_username IS NOT NULL AND ssh_secret_ciphertext IS NOT NULL
--       AND ssh_secret_kind IN ('password', 'key')
--       AND ssh_host_key_fingerprint IS NOT NULL
--       AND ssh_host_key_fingerprint ~ '^SHA256:[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]$')
--  );
--
-- Only after the audit returns zero rows:
-- ALTER TABLE public.devices VALIDATE CONSTRAINT devices_ssh_creds_complete;
-- ALTER TABLE public.devices VALIDATE CONSTRAINT devices_ssh_port_valid;
