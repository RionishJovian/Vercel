-- Voucher ledger reconciliation is deliberately append-only. It explains why a
-- code no longer exists on RouterOS without deleting the accounting record.
CREATE TABLE IF NOT EXISTS public.voucher_ledger_reconciliations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  voucher_id uuid NOT NULL REFERENCES public.voucher_codes(id) ON DELETE RESTRICT,
  router_id uuid REFERENCES public.router_connections(id) ON DELETE SET NULL,
  code text NOT NULL,
  action text NOT NULL CHECK (action IN ('keep', 'obsolete', 'investigate')),
  reason text NOT NULL CHECK (char_length(btrim(reason)) BETWEEN 3 AND 500),
  before_status text NOT NULL,
  after_status text NOT NULL,
  actor_user_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS voucher_ledger_reconciliations_voucher_idx
  ON public.voucher_ledger_reconciliations (voucher_id, created_at DESC);
CREATE INDEX IF NOT EXISTS voucher_ledger_reconciliations_owner_idx
  ON public.voucher_ledger_reconciliations (owner_id, created_at DESC);

GRANT SELECT ON public.voucher_ledger_reconciliations TO authenticated;
GRANT ALL ON public.voucher_ledger_reconciliations TO service_role;

ALTER TABLE public.voucher_ledger_reconciliations ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS voucher_ledger_reconciliations_read ON public.voucher_ledger_reconciliations;
CREATE POLICY voucher_ledger_reconciliations_read
  ON public.voucher_ledger_reconciliations FOR SELECT TO authenticated
  USING (
    owner_id = public.effective_owner(auth.uid())
    OR public.is_platform_admin(auth.uid())
  );

-- No authenticated INSERT, UPDATE, or DELETE policy: all event creation and
-- any status transition must pass through the constrained function below.
CREATE OR REPLACE FUNCTION public.reconcile_voucher_ledger(
  _voucher_ids uuid[],
  _action text,
  _reason text
)
RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_actor uuid := auth.uid();
  v_owner uuid;
  v_voucher public.voucher_codes%ROWTYPE;
  v_before text;
  v_after text;
  v_id uuid;
  v_count integer;
  v_changed integer := 0;
BEGIN
  IF v_actor IS NULL THEN
    RAISE EXCEPTION 'AUTH_REQUIRED';
  END IF;
  IF _action NOT IN ('keep', 'obsolete', 'investigate') THEN
    RAISE EXCEPTION 'INVALID_RECONCILIATION_ACTION';
  END IF;
  IF char_length(btrim(COALESCE(_reason, ''))) NOT BETWEEN 3 AND 500 THEN
    RAISE EXCEPTION 'RECONCILIATION_REASON_REQUIRED';
  END IF;
  IF COALESCE(cardinality(_voucher_ids), 0) = 0
     OR cardinality(_voucher_ids) > 500
     OR (SELECT count(DISTINCT x) FROM unnest(_voucher_ids) AS x) <> cardinality(_voucher_ids) THEN
    RAISE EXCEPTION 'INVALID_VOUCHER_SELECTION';
  END IF;
  IF NOT (
    public.has_role(v_actor, 'primary'::public.app_role)
    OR public.is_platform_admin(v_actor)
  ) THEN
    RAISE EXCEPTION 'VOUCHER_RECONCILIATION_FORBIDDEN';
  END IF;

  v_owner := public.effective_owner(v_actor);

  SELECT count(*) INTO v_count
  FROM public.voucher_codes
  WHERE id = ANY(_voucher_ids)
    AND (
      owner_id = v_owner
      OR public.is_platform_admin(v_actor)
    );
  IF v_count <> cardinality(_voucher_ids) THEN
    RAISE EXCEPTION 'VOUCHER_NOT_FOUND_OR_FORBIDDEN';
  END IF;

  -- A code which has been used or paid for must not be voided through a
  -- topology mismatch. It needs the existing payment/refund workflow instead.
  IF _action = 'obsolete' AND EXISTS (
    SELECT 1
    FROM public.voucher_codes v
    LEFT JOIN public.payment_orders o ON o.id = v.order_id
    WHERE v.id = ANY(_voucher_ids)
      AND (
        v.first_seen_at IS NOT NULL
        OR COALESCE(v.status, '') IN ('cancelled', 'deleted')
        OR o.status = 'settled'
      )
  ) THEN
    RAISE EXCEPTION 'ONLY_UNUSED_UNPAID_VOUCHERS_CAN_BE_MARKED_OBSOLETE';
  END IF;

  FOREACH v_id IN ARRAY _voucher_ids LOOP
    SELECT * INTO v_voucher
    FROM public.voucher_codes
    WHERE id = v_id
    FOR UPDATE;

    v_before := v_voucher.status;
    v_after := CASE WHEN _action = 'obsolete' THEN 'cancelled' ELSE v_before END;
    IF _action = 'obsolete' THEN
      UPDATE public.voucher_codes SET status = v_after WHERE id = v_voucher.id;
      v_changed := v_changed + 1;
    END IF;

    INSERT INTO public.voucher_ledger_reconciliations (
      owner_id, voucher_id, router_id, code, action, reason,
      before_status, after_status, actor_user_id
    ) VALUES (
      v_voucher.owner_id, v_voucher.id, v_voucher.router_id, v_voucher.code,
      _action, btrim(_reason), v_before, v_after, v_actor
    );
  END LOOP;

  RETURN jsonb_build_object(
    'ok', true,
    'processed', cardinality(_voucher_ids),
    'changed', v_changed,
    'action', _action
  );
END;
$$;

REVOKE ALL ON FUNCTION public.reconcile_voucher_ledger(uuid[], text, text) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.reconcile_voucher_ledger(uuid[], text, text)
  TO authenticated, service_role;
