-- Sandphase 1: one virtual MikroTik per tenant that does not consume quota
-- and never dials hardware. Tables may already exist on Lovable Cloud.

ALTER TABLE public.router_connections
  ADD COLUMN IF NOT EXISTS is_virtual boolean NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS public.sandbox_routers (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  router_connection_id uuid NOT NULL UNIQUE
    REFERENCES public.router_connections(id) ON DELETE CASCADE,
  model text NOT NULL DEFAULT 'RB5009UG+S+',
  attached_aps jsonb NOT NULL DEFAULT '[]'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS public.sandbox_state (
  router_connection_id uuid PRIMARY KEY
    REFERENCES public.router_connections(id) ON DELETE CASCADE,
  owner_id uuid NOT NULL,
  world jsonb NOT NULL DEFAULT '{}'::jsonb,
  updated_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.sandbox_routers TO authenticated;
GRANT ALL ON public.sandbox_routers TO service_role;
GRANT SELECT, INSERT, UPDATE, DELETE ON public.sandbox_state TO authenticated;
GRANT ALL ON public.sandbox_state TO service_role;

DO $$
BEGIN
  ALTER TABLE public.sandbox_routers
    DROP CONSTRAINT IF EXISTS sandbox_routers_router_connection_id_fkey;
  ALTER TABLE public.sandbox_routers
    ADD CONSTRAINT sandbox_routers_router_connection_id_fkey
    FOREIGN KEY (router_connection_id) REFERENCES public.router_connections(id) ON DELETE CASCADE;

  ALTER TABLE public.sandbox_state
    DROP CONSTRAINT IF EXISTS sandbox_state_router_connection_id_fkey;
  ALTER TABLE public.sandbox_state
    ADD CONSTRAINT sandbox_state_router_connection_id_fkey
    FOREIGN KEY (router_connection_id) REFERENCES public.router_connections(id) ON DELETE CASCADE;
EXCEPTION
  WHEN undefined_table THEN
    NULL;
END
$$;

ALTER TABLE public.sandbox_routers ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.sandbox_state ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "sandbox_routers tenant read" ON public.sandbox_routers;
DROP POLICY IF EXISTS "sandbox_routers tenant write" ON public.sandbox_routers;
CREATE POLICY "sandbox_routers tenant read" ON public.sandbox_routers
  FOR SELECT TO authenticated
  USING (public.can_manage_router_tenant(auth.uid(), owner_id));
CREATE POLICY "sandbox_routers tenant write" ON public.sandbox_routers
  FOR ALL TO authenticated
  USING (public.can_manage_router_tenant(auth.uid(), owner_id))
  WITH CHECK (public.can_manage_router_tenant(auth.uid(), owner_id));

DROP POLICY IF EXISTS "sandbox_state tenant read" ON public.sandbox_state;
DROP POLICY IF EXISTS "sandbox_state tenant write" ON public.sandbox_state;
CREATE POLICY "sandbox_state tenant read" ON public.sandbox_state
  FOR SELECT TO authenticated
  USING (public.can_manage_router_tenant(auth.uid(), owner_id));
CREATE POLICY "sandbox_state tenant write" ON public.sandbox_state
  FOR ALL TO authenticated
  USING (public.can_manage_router_tenant(auth.uid(), owner_id))
  WITH CHECK (public.can_manage_router_tenant(auth.uid(), owner_id));

CREATE UNIQUE INDEX IF NOT EXISTS router_connections_one_sandbox_per_owner
  ON public.router_connections (owner_id)
  WHERE connection_mode = 'sandbox';

CREATE OR REPLACE FUNCTION public.enforce_device_quota_trigger()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  _kind text := TG_ARGV[0];
  _max integer;
  _used integer;
  _uid uuid := auth.uid();
BEGIN
  IF NEW.owner_id IS NULL THEN
    RETURN NEW;
  END IF;

  -- Virtual lab routers never consume the paid device quota.
  -- Read via to_jsonb so this shared trigger also works on sites/controllers
  -- (those tables have no connection_mode column).
  IF _kind = 'routers' THEN
    IF (to_jsonb(NEW)->>'connection_mode') = 'sandbox' THEN
      RETURN NEW;
    END IF;
  END IF;

  -- Owner / admin accounts have no device limits.
  IF _uid IS NOT NULL AND (
       public.has_role(_uid, 'owner'::public.app_role)
    OR public.has_role(_uid, 'admin'::public.app_role)
  ) THEN
    RETURN NEW;
  END IF;

  PERFORM pg_advisory_xact_lock(hashtext(NEW.owner_id::text || ':' || _kind));

  SELECT CASE _kind
           WHEN 'routers' THEN da.routers
           WHEN 'controllers' THEN da.controllers
           ELSE da.sites
         END
    INTO _max
    FROM public.device_allowances da
   WHERE da.owner_id = NEW.owner_id;

  _max := COALESCE(_max, 1);

  IF _kind = 'routers' THEN
    SELECT count(*) INTO _used FROM public.router_connections
     WHERE owner_id = NEW.owner_id
       AND connection_mode IS DISTINCT FROM 'sandbox';
  ELSIF _kind = 'controllers' THEN
    SELECT count(*) INTO _used FROM public.unifi_controllers WHERE owner_id = NEW.owner_id;
  ELSE
    SELECT count(*) INTO _used FROM public.sites WHERE owner_id = NEW.owner_id;
  END IF;

  IF _used >= _max THEN
    RAISE EXCEPTION 'DEVICE_QUOTA_EXCEEDED: this account already uses % of % allowed %.', _used, _max, _kind
      USING ERRCODE = 'check_violation';
  END IF;

  RETURN NEW;
END;
$$;
