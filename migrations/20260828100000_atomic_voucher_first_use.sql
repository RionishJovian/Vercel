-- Atomic first-use claim for the existing RouterOS observation workflow.
-- This does not change voucher pricing or plan semantics. It makes the single
-- authoritative database transition winner-takes-all when observations race.

CREATE OR REPLACE FUNCTION public.claim_voucher_first_use(
  _router_id uuid,
  _voucher_id uuid,
  _device_mac text DEFAULT NULL
)
RETURNS TABLE (
  id uuid,
  first_seen_at timestamptz,
  expires_at timestamptz,
  device_mac text
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v public.voucher_codes%ROWTYPE;
  v_now timestamptz := now();
BEGIN
  SELECT * INTO v
  FROM public.voucher_codes
  WHERE id = _voucher_id
    AND router_id = _router_id
  FOR UPDATE;

  IF NOT FOUND
     OR v.first_seen_at IS NOT NULL
     OR COALESCE(v.status, '') IN ('cancelled', 'deleted', 'expired')
     OR (v.expires_at IS NOT NULL AND v.expires_at <= v_now) THEN
    RETURN;
  END IF;

  RETURN QUERY
  UPDATE public.voucher_codes
  SET first_seen_at = v_now,
      status = 'active',
      device_mac = COALESCE(NULLIF(btrim(v.device_mac), ''), NULLIF(btrim(_device_mac), '')),
      expires_at = CASE
        WHEN v.duration_minutes IS NOT NULL
          THEN v_now + make_interval(mins => v.duration_minutes)
        ELSE v.expires_at
      END
  WHERE id = v.id
    AND first_seen_at IS NULL
  RETURNING voucher_codes.id, voucher_codes.first_seen_at,
            voucher_codes.expires_at, voucher_codes.device_mac;
END;
$$;

REVOKE ALL ON FUNCTION public.claim_voucher_first_use(uuid, uuid, text) FROM PUBLIC, anon, authenticated;
GRANT EXECUTE ON FUNCTION public.claim_voucher_first_use(uuid, uuid, text) TO service_role;

COMMENT ON FUNCTION public.claim_voucher_first_use(uuid, uuid, text) IS
  'Atomically claims the first observed use of an existing tenant voucher; service-role observation path only.';
