-- Security follow-up for Magic Coin checkout.
--
-- A wallet-paid purchase can only be created by pay_service_with_magic_coins.
-- PostgREST clients may still create their own pending bank-transfer requests,
-- but can never label one as Magic Coins paid. Approval also verifies the
-- immutable, exact-value debit so an unexpected writer cannot activate a
-- service from a forged payment method.

CREATE OR REPLACE FUNCTION public.guard_magic_coin_service_purchase()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  -- This transaction-local flag is set only inside the security-definer wallet
  -- checkout RPC. A browser/PostgREST insert cannot set it.
  IF (
    TG_OP = 'INSERT'
    OR (TG_OP = 'UPDATE' AND NEW.payment_method IS DISTINCT FROM OLD.payment_method)
  )
  AND NEW.payment_method = 'magic_coins'
  AND current_setting('app.magic_coin_checkout', true) IS DISTINCT FROM 'true' THEN
    RAISE EXCEPTION 'MAGIC_COIN_CHECKOUT_RPC_REQUIRED';
  END IF;

  -- Entitlements are only granted after a matching, immutable wallet debit.
  -- This is deliberately enforced in the database as well as the app review
  -- code, so every approval channel has the same protection.
  IF TG_OP = 'UPDATE'
    AND OLD.status = 'pending'
    AND NEW.status = 'approved'
    AND NEW.payment_method = 'magic_coins'
    AND NOT EXISTS (
      SELECT 1
      FROM public.magic_coin_transactions t
      WHERE t.service_purchase_id = NEW.id
        AND t.user_id = NEW.user_id
        AND t.kind = 'service_payment'
        AND t.delta = -NEW.price_mmk
    ) THEN
    RAISE EXCEPTION 'MAGIC_COIN_DEBIT_REQUIRED';
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_service_purchase_magic_coin_guard ON public.service_purchases;
CREATE TRIGGER trg_service_purchase_magic_coin_guard
  BEFORE INSERT OR UPDATE ON public.service_purchases
  FOR EACH ROW EXECUTE FUNCTION public.guard_magic_coin_service_purchase();

-- The guard above requires this transaction-local capability immediately
-- before the wallet RPC writes its own pending purchase.
CREATE OR REPLACE FUNCTION public.pay_service_with_magic_coins(_service text)
RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_user uuid := auth.uid();
  v_owner uuid;
  v_balance numeric(12,2);
  v_price integer;
  v_label text;
  v_tier text;
  v_plus boolean;
  v_promo public.pricing_promo%ROWTYPE;
  v_purchase public.service_purchases%ROWTYPE;
BEGIN
  IF v_user IS NULL THEN RAISE EXCEPTION 'AUTH_REQUIRED'; END IF;
  IF _service NOT IN ('monthly', 'annual', 'plus') THEN RAISE EXCEPTION 'UNKNOWN_SERVICE'; END IF;

  v_owner := public.effective_owner(v_user);
  SELECT * INTO v_promo FROM public.pricing_promo WHERE id = true;
  SELECT tier, plus INTO v_tier, v_plus
  FROM public.account_entitlements WHERE user_id = v_user;
  v_tier := COALESCE(v_tier, 'trial');
  v_plus := COALESCE(v_plus, false);
  IF _service = 'plus' AND v_plus THEN RAISE EXCEPTION 'PLUS_ALREADY_ACTIVE'; END IF;

  IF _service = 'monthly' THEN
    v_label := 'Monthly';
    v_price := CASE WHEN COALESCE(v_promo.active, false)
      AND (now() AT TIME ZONE 'Asia/Yangon')::date BETWEEN v_promo.starts_on AND v_promo.ends_on
      THEN v_promo.monthly_promo_mmk ELSE v_promo.monthly_standard_mmk END;
  ELSIF _service = 'annual' THEN
    v_label := 'Annual';
    v_price := CASE WHEN COALESCE(v_promo.active, false)
      AND (now() AT TIME ZONE 'Asia/Yangon')::date BETWEEN v_promo.starts_on AND v_promo.ends_on
      THEN v_promo.annual_promo_mmk ELSE v_promo.annual_standard_mmk END;
  ELSE
    v_label := 'Plus';
    v_price := CASE WHEN v_tier = 'annual' THEN 300000 ELSE 30000 END;
  END IF;

  INSERT INTO public.magic_coin_wallets (user_id) VALUES (v_user)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT balance INTO v_balance FROM public.magic_coin_wallets WHERE user_id = v_user FOR UPDATE;
  IF v_balance < v_price THEN RAISE EXCEPTION 'INSUFFICIENT_MAGIC_COINS'; END IF;

  PERFORM set_config('app.magic_coin_checkout', 'true', true);
  INSERT INTO public.service_purchases (
    user_id, owner_id, service_key, service_label, price_mmk, status,
    reference, idempotency_key, payment_method
  ) VALUES (
    v_user, v_owner, _service, v_label, v_price, 'pending',
    'Magic Coins wallet', 'magic-coins:' || gen_random_uuid(), 'magic_coins'
  ) RETURNING * INTO v_purchase;

  v_balance := v_balance - v_price;
  UPDATE public.magic_coin_wallets SET balance = v_balance WHERE user_id = v_user;
  INSERT INTO public.magic_coin_transactions (
    user_id, owner_id, delta, balance_after, kind, service_purchase_id, note, actor_user_id
  ) VALUES (
    v_user, v_owner, -v_price, v_balance, 'service_payment', v_purchase.id,
    'Paid ' || v_label || ' with Magic Coins (1 Coin = 1 MMK)', v_user
  );
  INSERT INTO public.service_purchase_audit (purchase_id, actor_user_id, action, note)
  VALUES (v_purchase.id, v_user, 'submitted', 'Paid with Magic Coins; approval pending.');

  RETURN jsonb_build_object(
    'ok', true, 'purchase_id', v_purchase.id, 'price_coins', v_price, 'balance', v_balance
  );
END;
$$;

REVOKE ALL ON FUNCTION public.pay_service_with_magic_coins(text) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.pay_service_with_magic_coins(text) TO authenticated, service_role;
