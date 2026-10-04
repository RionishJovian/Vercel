-- Per-business media server for LAN-only playback.
--
-- The owner's Jellyfin (or any LAN media server) is stored once per business so
-- a portal redeploy does not require retyping it. This value is used to write a
-- scoped HotSpot walled-garden rule on that business's router; it never enables
-- cloud media transport. Video bytes stay on the business network.
--
-- Hardened in the same style as the other migrations: one statement per line,
-- ADD COLUMN IF NOT EXISTS, and a CHECK that refuses a non-private address.

ALTER TABLE public.portal_configs ADD COLUMN IF NOT EXISTS media_host text;
ALTER TABLE public.portal_configs ADD COLUMN IF NOT EXISTS media_port text NOT NULL DEFAULT '8096';

-- Guard the port range. A blank or non-numeric value is rejected outright rather
-- than silently becoming 0, which would produce an unusable walled-garden rule.
ALTER TABLE public.portal_configs DROP CONSTRAINT IF EXISTS portal_configs_media_port_check;
ALTER TABLE public.portal_configs ADD CONSTRAINT portal_configs_media_port_check
  CHECK (media_port ~ '^[0-9]{1,5}$' AND media_port::integer >= 1 AND media_port::integer <= 65535);

-- Refuse anything that is not a plain private IPv4 literal. Loopback, multicast,
-- unspecified and public addresses would expose a LAN service through the walled
-- garden, so they are rejected at the storage layer as well as in the app.
ALTER TABLE public.portal_configs DROP CONSTRAINT IF EXISTS portal_configs_media_host_check;
ALTER TABLE public.portal_configs ADD CONSTRAINT portal_configs_media_host_check
  CHECK (
    media_host IS NULL
    OR (
      media_host ~ '^([0-9]{1,3}\.){3}[0-9]{1,3}$'
      -- Every octet must be a real value, not just the first and last:
      -- without this, "10.999.1.5" would pass the private branch below.
      AND split_part(media_host, '.', 1)::integer BETWEEN 1 AND 255
      AND split_part(media_host, '.', 2)::integer BETWEEN 0 AND 255
      AND split_part(media_host, '.', 3)::integer BETWEEN 0 AND 255
      AND split_part(media_host, '.', 4)::integer BETWEEN 1 AND 254
      -- Reject multicast (224-239) and reserved (240+) outright.
      AND split_part(media_host, '.', 1)::integer < 224
      AND (
        (split_part(media_host, '.', 1)::integer = 10)
        OR (split_part(media_host, '.', 1)::integer = 192 AND split_part(media_host, '.', 2)::integer = 168)
        OR (split_part(media_host, '.', 1)::integer = 172 AND split_part(media_host, '.', 2)::integer BETWEEN 16 AND 31)
      )
    )
  );