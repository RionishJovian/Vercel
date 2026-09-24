-- RouterOS is not a financial ledger. This service-role-only function is called
-- after the server has freshly read the board and selected only router-only,
-- unused accounts. It records provenance for every imported voucher and never
-- writes to RouterOS.
CREATE TABLE IF NOT EXISTS public.voucher_legacy_imports (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  router_id uuid NOT NULL REFERENCES public.router_connections(id) ON DELETE RESTRICT,
  voucher_id uuid NOT NULL REFERENCES public.voucher_codes(id) ON DELETE RESTRICT,
  code text NOT NULL,
  router_profile text NOT NULL,
  plan_id uuid NOT NULL REFERENCES public.portal_plans(id) ON DELETE RESTRICT,
  import_reason text NOT NULL CHECK (char_length(btrim(import_reason)) BETWEEN 3 AND 500),
  actor_user_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS voucher_legacy_imports_owner_idx
  ON public.voucher_legacy_imports (owner_id, created_at DESC);

GRANT SELECT ON public.voucher_legacy_imports TO authenticated;
GRANT ALL ON public.voucher_legacy_imports TO service_role;
REVOKE INSERT, UPDATE, DELETE ON public.voucher_legacy_imports FROM authenticated;
ALTER TABLE public.voucher_legacy_imports ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS voucher_legacy_imports_read ON public.voucher_legacy_imports;
CREATE POLICY voucher_legacy_imports_read ON public.voucher_legacy_imports
  FOR SELECT TO authenticated
  USING (owner_id = public.effective_owner(auth.uid()) OR public.is_platform_admin(auth.uid()));

CREATE OR REPLACE FUNCTION public.import_legacy_router_vouchers(
  _owner_id uuid,
  _actor_user_id uuid,
  _router_id uuid,
  _imports jsonb,
  _reason text
)
RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_item jsonb;
  v_plan public.portal_plans%ROWTYPE;
  v_code text;
  v_profile text;
  v_voucher_id uuid;
  v_count integer := 0;
BEGIN
  IF NOT (public.has_role(_actor_user_id, 'primary'::public.app_role)
          OR public.is_platform_admin(_actor_user_id)) THEN
    RAISE EXCEPTION 'LEGACY_VOUCHER_IMPORT_FORBIDDEN';
  END IF;
  IF public.effective_owner(_actor_user_id) <> _owner_id
     AND NOT public.is_platform_admin(_actor_user_id) THEN
    RAISE EXCEPTION 'LEGACY_VOUCHER_IMPORT_OWNER_MISMATCH';
  END IF;
  IF char_length(btrim(COALESCE(_reason, ''))) NOT BETWEEN 3 AND 500 THEN
    RAISE EXCEPTION 'LEGACY_VOUCHER_IMPORT_REASON_REQUIRED';
  END IF;
  IF jsonb_typeof(_imports) <> 'array' OR jsonb_array_length(_imports) = 0
     OR jsonb_array_length(_imports) > 500 THEN
    RAISE EXCEPTION 'INVALID_LEGACY_VOUCHER_SELECTION';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM public.router_connections r WHERE r.id = _router_id AND r.owner_id = _owner_id) THEN
    RAISE EXCEPTION 'ROUTER_NOT_FOUND_OR_FORBIDDEN';
  END IF;

  FOR v_item IN SELECT value FROM jsonb_array_elements(_imports) LOOP
    v_code := upper(btrim(COALESCE(v_item->>'code', '')));
    v_profile := btrim(COALESCE(v_item->>'router_profile', ''));
    IF char_length(v_code) NOT BETWEEN 1 AND 120 OR v_profile = '' THEN
      RAISE EXCEPTION 'INVALID_LEGACY_VOUCHER';
    END IF;
    SELECT * INTO v_plan FROM public.portal_plans
      WHERE id = (v_item->>'plan_id')::uuid AND owner_id = _owner_id AND status <> 'inactive';
    IF NOT FOUND THEN RAISE EXCEPTION 'IMPORT_PLAN_NOT_FOUND_OR_INACTIVE'; END IF;

    INSERT INTO public.voucher_codes (
      owner_id, router_id, plan_id, plan_key, plan_label, code, price_mmk,
      duration_minutes, hotspot_profile, status
    ) VALUES (
      _owner_id, _router_id, v_plan.id, COALESCE(v_plan.plan_key, v_plan.label),
      v_plan.label, v_code, v_plan.price_mmk, v_plan.duration_minutes, v_profile, 'unused'
    ) RETURNING id INTO v_voucher_id;

    INSERT INTO public.voucher_legacy_imports (
      owner_id, router_id, voucher_id, code, router_profile, plan_id, import_reason, actor_user_id
    ) VALUES (
      _owner_id, _router_id, v_voucher_id, v_code, v_profile, v_plan.id, btrim(_reason), _actor_user_id
    );
    v_count := v_count + 1;
  END LOOP;
  RETURN jsonb_build_object('ok', true, 'imported', v_count);
END;
$$;

-- Direct browser calls must never be able to fabricate a device scan. The
-- server is the only caller and uses the service role after a fresh router read.
REVOKE ALL ON FUNCTION public.import_legacy_router_vouchers(uuid, uuid, uuid, jsonb, text)
  FROM PUBLIC, anon, authenticated;
GRANT EXECUTE ON FUNCTION public.import_legacy_router_vouchers(uuid, uuid, uuid, jsonb, text)
  TO service_role;
