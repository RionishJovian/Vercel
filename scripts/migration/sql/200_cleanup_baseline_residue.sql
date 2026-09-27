-- Phase 3 · cleanup — remove baseline demo/seed residue.
--
-- WHY THIS EXISTS
-- The working copy (`mega_merge`) is rebuilt from the pristine BASELINE each run.
-- The baseline carries the new project's own dev seed, and deleting the retired
-- test TENANTS does not remove their dependent rows, so seed/demo data survives
-- into the merged dataset. Independent verification (id-set fingerprints against
-- the source) found exactly two such rows:
--   1. devices         cc00a111-…-aaaa  hardware_name = 'portal-test'  (a fixture)
--   2. portal_configs  4a437be9-…       the platform business's demo guest portal
-- Both belong to Magic Development (the platform business) and neither exists in
-- the old system.
--
-- The merged dataset should be exactly: the old system's real rows + platform
-- identity essentials. This step enforces that by deleting any row in these
-- tables whose key is absent from the source, so it is self-maintaining rather
-- than hard-coded to those two ids.

-- 1) Devices with no counterpart in the old fleet
DELETE FROM devices
 WHERE device_id::text NOT IN (SELECT id FROM _stg_src_device_ids);

-- 2) Portal configs for a business that had no config in the old system
DELETE FROM portal_configs
 WHERE business_id::text NOT IN (SELECT owner_id FROM _stg_src_portal_owners);
