-- Router Automation Connector: discovery/setup state, bootstrap audit, rate limiting.

CREATE TABLE public.connector_discovered_routers (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  connector_id uuid NOT NULL REFERENCES public.connectors(id) ON DELETE CASCADE,
  fingerprint text NOT NULL,
  identity text,
  model text,
  platform text,
  os_version text,
  ip text,
  mac text,
  serial text,
  state text NOT NULL DEFAULT 'discovered'
    CHECK (state IN ('discovered','authenticating','configuring','connected','offline','error')),
  last_error text,
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  backup_name text,
  rollback_script text,
  rollback_meta jsonb NOT NULL DEFAULT '{}'::jsonb,
  router_connection_id uuid REFERENCES public.router_connections(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT connector_discovered_routers_owner_fingerprint_key UNIQUE (owner_id, fingerprint)
);

CREATE INDEX connector_discovered_routers_connector_idx
  ON public.connector_discovered_routers (connector_id);

GRANT SELECT, DELETE ON public.connector_discovered_routers TO authenticated;
GRANT ALL ON public.connector_discovered_routers TO service_role;
ALTER TABLE public.connector_discovered_routers ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Tenant can read discovered routers"
  ON public.connector_discovered_routers FOR SELECT TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()));

CREATE POLICY "Tenant admins can delete discovered routers"
  ON public.connector_discovered_routers FOR DELETE TO authenticated
  USING (
    owner_id = public.effective_owner(auth.uid())
    AND (
      public.has_tenant_role(auth.uid(), 'owner'::public.app_role, owner_id)
      OR public.has_tenant_role(auth.uid(), 'admin'::public.app_role, owner_id)
    )
  );

CREATE TRIGGER update_connector_discovered_routers_updated_at
  BEFORE UPDATE ON public.connector_discovered_routers
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

CREATE TABLE public.connector_bootstrap_audit (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  connector_id uuid NOT NULL REFERENCES public.connectors(id) ON DELETE CASCADE,
  discovered_router_id uuid REFERENCES public.connector_discovered_routers(id) ON DELETE CASCADE,
  action text NOT NULL,
  status text NOT NULL DEFAULT 'ok',
  detail jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX connector_bootstrap_audit_owner_idx
  ON public.connector_bootstrap_audit (owner_id, created_at DESC);

GRANT SELECT ON public.connector_bootstrap_audit TO authenticated;
GRANT ALL ON public.connector_bootstrap_audit TO service_role;
ALTER TABLE public.connector_bootstrap_audit ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Tenant can read connector bootstrap audit"
  ON public.connector_bootstrap_audit FOR SELECT TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()));

CREATE TRIGGER update_connector_bootstrap_audit_updated_at
  BEFORE UPDATE ON public.connector_bootstrap_audit
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

-- Persistent, atomic rate limiting shared by all app instances.
CREATE TABLE public.connector_rate_limits (
  bucket_key text PRIMARY KEY,
  window_start timestamptz NOT NULL DEFAULT now(),
  hits integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

GRANT ALL ON public.connector_rate_limits TO service_role;
ALTER TABLE public.connector_rate_limits ENABLE ROW LEVEL SECURITY;
-- No policies: server-side (service role) access only.

CREATE OR REPLACE FUNCTION public.connector_rate_hit(
  _key text, _limit integer, _window_seconds integer
) RETURNS boolean
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  _hits integer;
BEGIN
  INSERT INTO public.connector_rate_limits AS r (bucket_key, window_start, hits)
  VALUES (_key, now(), 1)
  ON CONFLICT (bucket_key) DO UPDATE
    SET hits = CASE
          WHEN r.window_start < now() - make_interval(secs => _window_seconds) THEN 1
          ELSE r.hits + 1
        END,
        window_start = CASE
          WHEN r.window_start < now() - make_interval(secs => _window_seconds) THEN now()
          ELSE r.window_start
        END,
        updated_at = now()
  RETURNING r.hits INTO _hits;

  RETURN _hits <= _limit;
END;
$$;

REVOKE ALL ON FUNCTION public.connector_rate_hit(text, integer, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public.connector_rate_hit(text, integer, integer) TO service_role;