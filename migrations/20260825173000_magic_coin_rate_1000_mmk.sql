-- Magic Coin exchange rate: one Coin is worth 1,000 MMK.
--
-- Persist the Coin amount on every wallet-paid service request. This keeps
-- existing requests auditable and ensures any later refund returns the exact
-- number of Coins originally debited, even after a future rate change.

ALTER TABLE public.service_purchases
  ADD COLUMN IF NOT EXISTS coin_amount numeric(12,2);

UPDATE public.service_purchases
SET coin_amount = price_mmk
WHERE payment_method = 'magic_coins'
  AND coin_amount IS NULL;

ALTER TABLE public.service_purchases
  ADD CONSTRAINT service_purchases_magic_coin_amount_check
  CHECK (
    (payment_method <> 'magic_coins' AND coin_amount IS NULL)
    OR (payment_method = 'magic_coins' AND coin_amount IS NOT NULL AND coin_amount > 0)
  ) NOT VALID;

ALTER TABLE public.service_purchases
  VALIDATE CONSTRAINT service_purchases_magic_coin_amount_check;

CREATE OR REPLACE FUNCTION public.guard_magic_coin_service_purchase()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF (
    TG_OP = 'INSERT'
    OR (TG_OP = 'UPDATE' AND NEW.payment_method IS DISTINCT FROM OLD.payment_method)
  )
  AND NEW.payment_method = 'magic_coins'
  AND current_setting('app.magic_coin_checkout', true) IS DISTINCT FROM 'true' THEN
    RAISE EXCEPTION 'MAGIC_COIN_CHECKOUT_RPC_REQUIRED';
  END IF;

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
        AND t.delta = -NEW.coin_amount
    ) THEN
    RAISE EXCEPTION 'MAGIC_COIN_DEBIT_REQUIRED';
  END IF;

  RETURN NEW;
END;
$$;

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
  v_price_mmk integer;
  v_price_coins numeric(12,2);
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
    v_price_mmk := CASE WHEN COALESCE(v_promo.active, false)
      AND (now() AT TIME ZONE 'Asia/Yangon')::date BETWEEN v_promo.starts_on AND v_promo.ends_on
      THEN v_promo.monthly_promo_mmk ELSE v_promo.monthly_standard_mmk END;
  ELSIF _service = 'annual' THEN
    v_label := 'Annual';
    v_price_mmk := CASE WHEN COALESCE(v_promo.active, false)
      AND (now() AT TIME ZONE 'Asia/Yangon')::date BETWEEN v_promo.starts_on AND v_promo.ends_on
      THEN v_promo.annual_promo_mmk ELSE v_promo.annual_standard_mmk END;
  ELSE
    v_label := 'Plus';
    v_price_mmk := CASE WHEN v_tier = 'annual' THEN 300000 ELSE 30000 END;
  END IF;
  v_price_coins := v_price_mmk::numeric / 1000;

  INSERT INTO public.magic_coin_wallets (user_id) VALUES (v_user)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT balance INTO v_balance FROM public.magic_coin_wallets WHERE user_id = v_user FOR UPDATE;
  IF v_balance < v_price_coins THEN RAISE EXCEPTION 'INSUFFICIENT_MAGIC_COINS'; END IF;

  PERFORM set_config('app.magic_coin_checkout', 'true', true);
  INSERT INTO public.service_purchases (
    user_id, owner_id, service_key, service_label, price_mmk, coin_amount, status,
    reference, idempotency_key, payment_method
  ) VALUES (
    v_user, v_owner, _service, v_label, v_price_mmk, v_price_coins, 'pending',
    'Magic Coins wallet', 'magic-coins:' || gen_random_uuid(), 'magic_coins'
  ) RETURNING * INTO v_purchase;

  v_balance := v_balance - v_price_coins;
  UPDATE public.magic_coin_wallets SET balance = v_balance WHERE user_id = v_user;
  INSERT INTO public.magic_coin_transactions (
    user_id, owner_id, delta, balance_after, kind, service_purchase_id, note, actor_user_id
  ) VALUES (
    v_user, v_owner, -v_price_coins, v_balance, 'service_payment', v_purchase.id,
    'Paid ' || v_label || ' with Magic Coins (1 Coin = 1,000 MMK)', v_user
  );
  INSERT INTO public.service_purchase_audit (purchase_id, actor_user_id, action, note)
  VALUES (v_purchase.id, v_user, 'submitted', 'Paid with Magic Coins; approval pending.');

  RETURN jsonb_build_object(
    'ok', true, 'purchase_id', v_purchase.id, 'price_coins', v_price_coins, 'balance', v_balance
  );
END;
$$;

CREATE OR REPLACE FUNCTION public.refund_magic_coin_service_purchase()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_balance numeric(12,2);
  v_refund_amount numeric(12,2);
BEGIN
  IF NEW.payment_method <> 'magic_coins'
     OR NEW.status NOT IN ('rejected', 'cancelled', 'refunded')
     OR OLD.status = NEW.status THEN
    RETURN NEW;
  END IF;
  IF EXISTS (
    SELECT 1 FROM public.magic_coin_transactions
    WHERE service_purchase_id = NEW.id AND kind = 'service_refund'
  ) THEN
    RETURN NEW;
  END IF;

  SELECT -delta INTO v_refund_amount
  FROM public.magic_coin_transactions
  WHERE service_purchase_id = NEW.id AND kind = 'service_payment';
  IF v_refund_amount IS NULL THEN RETURN NEW; END IF;

  INSERT INTO public.magic_coin_wallets (user_id) VALUES (NEW.user_id)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT balance INTO v_balance FROM public.magic_coin_wallets WHERE user_id = NEW.user_id FOR UPDATE;
  v_balance := v_balance + v_refund_amount;
  UPDATE public.magic_coin_wallets SET balance = v_balance WHERE user_id = NEW.user_id;
  INSERT INTO public.magic_coin_transactions (
    user_id, owner_id, delta, balance_after, kind, service_purchase_id, note
  ) VALUES (
    NEW.user_id, NEW.owner_id, v_refund_amount, v_balance, 'service_refund', NEW.id,
    'Magic Coins returned after service purchase ' || NEW.status
  );
  RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.get_magic_coin_wallet()
RETURNS jsonb
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_user uuid := auth.uid();
  v_balance numeric(12,2);
  v_transactions jsonb;
BEGIN
  IF v_user IS NULL THEN RAISE EXCEPTION 'AUTH_REQUIRED'; END IF;
  INSERT INTO public.magic_coin_wallets (user_id) VALUES (v_user)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT balance INTO v_balance FROM public.magic_coin_wallets WHERE user_id = v_user;
  SELECT COALESCE(jsonb_agg(to_jsonb(x)), '[]'::jsonb) INTO v_transactions
  FROM (
    SELECT delta, balance_after, kind, note, created_at
    FROM public.magic_coin_transactions
    WHERE user_id = v_user
    ORDER BY created_at DESC, id DESC
    LIMIT 30
  ) x;
  RETURN jsonb_build_object(
    'balance', v_balance,
    'mmk_value', v_balance * 1000,
    'transactions', v_transactions
  );
END;
$$;
