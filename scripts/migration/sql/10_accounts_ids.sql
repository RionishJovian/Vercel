-- Phase 1 · Step 1.1 (revised) — accounts, with OLD UUIDs PRESERVED.
--
-- Why preserve: in the old single-tenant schema the tenant key is the owner's
-- auth-user id (`sites.owner_id`, `managed_devices.owner_id`, `voucher_codes.owner_id`,
-- `incidents.owner_id`, …). Carrying that exact uuid into `businesses.business_id`
-- AND `users.user_id` means every later step is a direct column copy — no lookups,
-- and old FKs like `incidents.acknowledged_by` still resolve to a real user.
--
-- Runs against the COPY database `mega_merge` only.
BEGIN;

ALTER TABLE _mig_accounts ADD COLUMN business_id uuid DEFAULT uuid_generate_v4();

UPDATE _mig_accounts SET business_id = old_user_id::uuid;   -- preserve the tenant key

INSERT INTO businesses
    (business_id, company_name, owner_email, access_tier, is_active, device_quota_limit, created_at, updated_at)
SELECT business_id,
       COALESCE(NULLIF(site_name, ''), NULLIF(full_name, ''), username),
       lower(email),
       'SMALL_WISP', true, 10,
       COALESCE(created_at, now()), now()
  FROM _mig_accounts;

INSERT INTO users
    (user_id, business_id, full_name, email, password_hash, user_role, is_active, created_at, updated_at)
SELECT old_user_id::uuid,
       business_id,
       COALESCE(NULLIF(full_name, ''), username),
       lower(email),
       password_hash,
       new_role,
       true,
       COALESCE(created_at, now()), now()
  FROM _mig_accounts;

COMMIT;
