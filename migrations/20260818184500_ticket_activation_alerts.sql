-- One tenant-wide switch: notify when a hotspot voucher (ticket) is first used.
ALTER TABLE public.portal_settings
  ADD COLUMN IF NOT EXISTS notify_ticket_activation boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN public.portal_settings.notify_ticket_activation IS
  'When true, notify tenant staff the moment a voucher ticket is first activated on a router.';
