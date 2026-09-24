-- Customizable payment_methods for portal commerce cards.
-- Hardened for Lovable SQL Editor (dollar-quoted JSON).

ALTER TABLE public.portal_settings ADD COLUMN IF NOT EXISTS payment_methods jsonb NOT NULL DEFAULT '[]'::jsonb;

UPDATE public.portal_settings
SET payment_methods = $pm$[
  {"id":"transfer","enabled":true,"label":"Bank / wallet transfer","description":"Get temporary access, then transfer and buy a voucher code.","accentHex":"#3b82f6","action":"pay_info","infoTitle":"Transfer details","infoBody":"Replace this text with your bank or mobile-wallet account details.","copyValue":"","sort":0},
  {"id":"seller","enabled":true,"label":"Talk to seller","description":"Temporary internet to message our seller and buy a code.","accentHex":"#22c55e","action":"seller","infoTitle":"","infoBody":"","copyValue":"","sort":1},
  {"id":"pos","enabled":true,"label":"Point of sale","description":"Find nearby shops and buy a voucher in person.","accentHex":"#a855f7","action":"pos","infoTitle":"","infoBody":"","copyValue":"","sort":2}
]$pm$::jsonb
WHERE payment_methods = '[]'::jsonb OR payment_methods IS NULL;
