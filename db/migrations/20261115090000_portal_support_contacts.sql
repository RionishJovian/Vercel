-- Support contacts for the router login page's support bar (additive, forward-only).
--
-- A dialable phone number, a Telegram username (no @) and a Viber phone number. '' hides that button. The format is
-- validated by the application (routeros.NormalizeSupportPhone / NormalizeSupportTelegram) before it is stored and
-- again when the page is rendered, because each value becomes part of a link.
ALTER TABLE public.portal_configs ADD COLUMN IF NOT EXISTS support_phone text NOT NULL DEFAULT '';
ALTER TABLE public.portal_configs ADD COLUMN IF NOT EXISTS support_telegram text NOT NULL DEFAULT '';
ALTER TABLE public.portal_configs ADD COLUMN IF NOT EXISTS support_viber text NOT NULL DEFAULT '';
