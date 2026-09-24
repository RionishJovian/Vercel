-- Remove retired guest cloud-checkout scaffolding.
-- Portal checkout tokens powered /portal/checkout + /api/public/checkout/*.
-- Desk cash sales, Tier Pass receipts, and RouterOS Hotspot vouchers are untouched.
-- Use CASCADE so policies/triggers go with the table; IF EXISTS so missing table is OK.

DROP TABLE IF EXISTS public.portal_checkout_tokens CASCADE;
