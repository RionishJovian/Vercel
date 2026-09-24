-- Remove the DIY agentless outbound tunnel path forever.
-- Magic Hub keeps router_connections.tunnel_address (WireGuard peer IP).

-- Convert any leftover DIY tunnel rows to direct so they stay readable.
UPDATE public.router_connections
SET
  connection_mode = 'direct',
  tunnel_hub_id = NULL,
  tunnel_public_key = NULL,
  tunnel_private_key_ciphertext = NULL,
  tunnel_listen_port = NULL,
  pending_tunnel_public_key = NULL,
  pending_tunnel_private_key_ciphertext = NULL,
  connection_preference = 'direct',
  last_active_path = NULL,
  tunnel_last_ok = NULL,
  tunnel_last_check_at = NULL,
  rotation_started_at = NULL,
  last_rotated_at = NULL
WHERE connection_mode = 'tunnel';

ALTER TABLE public.router_connections
  DROP CONSTRAINT IF EXISTS router_connections_tunnel_hub_id_fkey;

ALTER TABLE public.router_connections
  DROP COLUMN IF EXISTS tunnel_hub_id,
  DROP COLUMN IF EXISTS tunnel_public_key,
  DROP COLUMN IF EXISTS tunnel_private_key_ciphertext,
  DROP COLUMN IF EXISTS tunnel_listen_port,
  DROP COLUMN IF EXISTS pending_tunnel_public_key,
  DROP COLUMN IF EXISTS pending_tunnel_private_key_ciphertext,
  DROP COLUMN IF EXISTS connection_preference,
  DROP COLUMN IF EXISTS last_active_path,
  DROP COLUMN IF EXISTS tunnel_last_ok,
  DROP COLUMN IF EXISTS tunnel_last_check_at,
  DROP COLUMN IF EXISTS rotation_started_at,
  DROP COLUMN IF EXISTS last_rotated_at;

DROP TABLE IF EXISTS public.tunnel_hubs CASCADE;
