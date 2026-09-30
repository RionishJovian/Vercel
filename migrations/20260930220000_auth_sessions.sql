-- System-level authentication bootstrap table. Never grant tenant runtime access.
CREATE TABLE IF NOT EXISTS auth_sessions (
 session_id text PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
 business_id uuid NOT NULL REFERENCES businesses(business_id) ON DELETE CASCADE,
 password_digest text NOT NULL,
 user_role text NOT NULL,
 refresh_digest text NOT NULL,
 expires_at timestamptz NOT NULL,
 revoked boolean NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS auth_sessions_user_id_idx ON auth_sessions(user_id);
ALTER TABLE auth_sessions ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON auth_sessions FROM PUBLIC;
REVOKE ALL ON auth_sessions FROM authenticated;
-- Revoke permanently, including across later reactivation/restored roles.
CREATE OR REPLACE FUNCTION public.revoke_changed_user_sessions() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog AS $$
BEGIN
 IF OLD.password_hash IS DISTINCT FROM NEW.password_hash
 OR OLD.user_role IS DISTINCT FROM NEW.user_role
 OR OLD.business_id IS DISTINCT FROM NEW.business_id
 OR (OLD.is_active AND NOT NEW.is_active) THEN
 UPDATE public.auth_sessions SET revoked=true WHERE user_id=NEW.user_id;
 END IF;
 RETURN NEW;
END;
$$;
REVOKE ALL ON FUNCTION public.revoke_changed_user_sessions() FROM PUBLIC;
DROP TRIGGER IF EXISTS revoke_changed_user_sessions ON public.users;
CREATE TRIGGER revoke_changed_user_sessions AFTER UPDATE ON public.users
FOR EACH ROW EXECUTE FUNCTION public.revoke_changed_user_sessions();
CREATE OR REPLACE FUNCTION public.revoke_suspended_business_sessions() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog AS $$
BEGIN
 IF OLD.is_active AND NOT NEW.is_active THEN
 UPDATE public.auth_sessions SET revoked=true WHERE business_id=NEW.business_id;
 END IF;
 RETURN NEW;
END;
$$;
REVOKE ALL ON FUNCTION public.revoke_suspended_business_sessions() FROM PUBLIC;
DROP TRIGGER IF EXISTS revoke_suspended_business_sessions ON public.businesses;
CREATE TRIGGER revoke_suspended_business_sessions AFTER UPDATE ON public.businesses
FOR EACH ROW EXECUTE FUNCTION public.revoke_suspended_business_sessions();
