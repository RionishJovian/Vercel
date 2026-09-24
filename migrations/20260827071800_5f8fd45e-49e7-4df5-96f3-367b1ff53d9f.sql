CREATE TABLE public.router_support_grants (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL,
  router_id uuid NOT NULL REFERENCES public.router_connections(id) ON DELETE CASCADE,
  grantee_user_id uuid NOT NULL,
  actions text[] NOT NULL DEFAULT ARRAY['sync_plans']::text[],
  reason text,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  created_by uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX router_support_grants_router_grantee_idx
  ON public.router_support_grants (router_id, grantee_user_id);
CREATE INDEX router_support_grants_owner_idx
  ON public.router_support_grants (owner_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.router_support_grants TO authenticated;
GRANT ALL ON public.router_support_grants TO service_role;

ALTER TABLE public.router_support_grants ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Tenant and grantee can read router support grants"
  ON public.router_support_grants
  FOR SELECT
  TO authenticated
  USING (
    grantee_user_id = auth.uid()
    OR public.owner_operations_can_read(owner_id)
  );

CREATE POLICY "Tenant can create router support grants"
  ON public.router_support_grants
  FOR INSERT
  TO authenticated
  WITH CHECK (
    public.owner_operations_can_write(owner_id)
    AND created_by = auth.uid()
    AND grantee_user_id <> auth.uid()
  );

CREATE POLICY "Tenant can update router support grants"
  ON public.router_support_grants
  FOR UPDATE
  TO authenticated
  USING (public.owner_operations_can_write(owner_id))
  WITH CHECK (public.owner_operations_can_write(owner_id));

CREATE POLICY "Tenant can delete router support grants"
  ON public.router_support_grants
  FOR DELETE
  TO authenticated
  USING (public.owner_operations_can_write(owner_id));

CREATE TRIGGER router_support_grants_touch
  BEFORE UPDATE ON public.router_support_grants
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

CREATE OR REPLACE FUNCTION public.validate_router_support_grant()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
  router_owner uuid;
BEGIN
  SELECT owner_id INTO router_owner FROM public.router_connections WHERE id = NEW.router_id;
  IF router_owner IS NULL THEN
    RAISE EXCEPTION 'Router not found for support grant.';
  END IF;
  IF router_owner <> NEW.owner_id THEN
    RAISE EXCEPTION 'Support grant owner must match the router owner.';
  END IF;
  IF NEW.revoked_at IS NULL AND NEW.expires_at <= now() THEN
    RAISE EXCEPTION 'Support grant expiry must be in the future.';
  END IF;
  IF array_length(NEW.actions, 1) IS NULL THEN
    RAISE EXCEPTION 'Support grant must allow at least one action.';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER router_support_grants_validate
  BEFORE INSERT OR UPDATE ON public.router_support_grants
  FOR EACH ROW EXECUTE FUNCTION public.validate_router_support_grant();