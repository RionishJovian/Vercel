-- Finish Syslog AI ingest schema when `token` is already gone (re-runnable).
-- The first production migration hashed `token`; Cloud DBs that never had that
-- column (or already dropped it) fail with 42703. This block is safe either way.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE public.syslog_tokens
  ADD COLUMN IF NOT EXISTS token_hash text,
  ADD COLUMN IF NOT EXISTS token_prefix text,
  ADD COLUMN IF NOT EXISTS router_id uuid REFERENCES public.router_connections(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS site_id uuid REFERENCES public.sites(id) ON DELETE SET NULL;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'syslog_tokens'
      AND column_name = 'token'
  ) THEN
    EXECUTE $u$
      UPDATE public.syslog_tokens
      SET
        token_hash = encode(digest(convert_to(token, 'UTF8'), 'sha256'), 'hex'),
        token_prefix = left(token, 12)
      WHERE token_hash IS NULL AND token IS NOT NULL AND length(token) > 0
    $u$;
  END IF;
END $$;

DELETE FROM public.syslog_tokens WHERE token_hash IS NULL;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'syslog_tokens'
      AND column_name = 'token_hash'
      AND is_nullable = 'YES'
  ) THEN
    ALTER TABLE public.syslog_tokens ALTER COLUMN token_hash SET NOT NULL;
  END IF;
END $$;

ALTER TABLE public.syslog_tokens DROP COLUMN IF EXISTS token;

CREATE UNIQUE INDEX IF NOT EXISTS syslog_tokens_token_hash_uidx
  ON public.syslog_tokens (token_hash);

ALTER TABLE public.syslog_events
  ADD COLUMN IF NOT EXISTS token_id uuid REFERENCES public.syslog_tokens(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS syslog_events_token_time_idx
  ON public.syslog_events (token_id, received_at DESC);

CREATE INDEX IF NOT EXISTS syslog_events_received_at_idx
  ON public.syslog_events (received_at);
