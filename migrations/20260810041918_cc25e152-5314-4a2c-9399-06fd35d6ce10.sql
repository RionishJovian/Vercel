ALTER TABLE public.router_connections
  ADD COLUMN IF NOT EXISTS cloud_peer_id text,
  ADD COLUMN IF NOT EXISTS cloud_wg_address text,
  ADD COLUMN IF NOT EXISTS cloud_wg_public_key text,
  ADD COLUMN IF NOT EXISTS cloud_wg_private_key_ciphertext text,
  ADD COLUMN IF NOT EXISTS cloud_status text NOT NULL DEFAULT 'pending',
  ADD COLUMN IF NOT EXISTS cloud_last_handshake_at timestamptz,
  ADD COLUMN IF NOT EXISTS cloud_last_seen_at timestamptz,
  ADD COLUMN IF NOT EXISTS cloud_last_error text;

CREATE INDEX IF NOT EXISTS router_connections_cloud_peer_idx
  ON public.router_connections (cloud_peer_id)
  WHERE cloud_peer_id IS NOT NULL;