-- Presence contains only server observation times, never input content or tokens.
BEGIN;
ALTER TABLE public.auth_sessions ADD COLUMN IF NOT EXISTS last_seen_at timestamptz;
ALTER TABLE public.auth_sessions ADD COLUMN IF NOT EXISTS last_active_at timestamptz;
ALTER TABLE public.auth_sessions ADD COLUMN IF NOT EXISTS last_visible_at timestamptz;
COMMIT;
