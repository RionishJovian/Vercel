ALTER TABLE public.connector_discovered_routers
  ADD COLUMN IF NOT EXISTS api_username text,
  ADD COLUMN IF NOT EXISTS api_password_encrypted text,
  ADD COLUMN IF NOT EXISTS tls_fingerprint text;