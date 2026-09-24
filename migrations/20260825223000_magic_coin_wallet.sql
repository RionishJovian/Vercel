-- Magic Coins are a per-account, append-only wallet. One Magic Coin equals
-- one MMK for service checkout. Balances are a cached total protected by a
-- row lock; the transaction ledger remains the audit source of truth.

CREATE TABLE IF NOT EXISTS public.magic_coin_wallets (
  user_id uuid PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
  balance numeric(12,2) NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS public.magic_coin_transactions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE RESTRICT,
  delta numeric(12,2) NOT NULL CHECK (delta <> 0),
  balance_after numeric(12,2) NOT NULL,
  kind text NOT NULL CHECK (kind IN (
    'agent_earn', 'agent_reversal', 'service_payment', 'service_refund',
    'manual_credit', 'manual_debit'
  )),
  source_agent_point_id uuid REFERENCES public.agent_points(id) ON DELETE RESTRICT,
  service_purchase_id uuid REFERENCES public.service_purchases(id) ON DELETE RESTRICT,
  note text NOT NULL,
  actor_user_id uuid REFERENCES auth.users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS magic_coin_transactions_agent_point_once
  ON public.magic_coin_transactions(source_agent_point_id)
  WHERE source_agent_point_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS magic_coin_transactions_service_payment_once
  ON public.magic_coin_transactions(service_purchase_id)
  WHERE kind = 'service_payment';
CREATE UNIQUE INDEX IF NOT EXISTS magic_coin_transactions_service_refund_once
  ON public.magic_coin_transactions(service_purchase_id)
  WHERE kind = 'service_refund';
CREATE INDEX IF NOT EXISTS magic_coin_transactions_user_created_idx
  ON public.magic_coin_transactions(user_id, created_at DESC);

GRANT SELECT ON public.magic_coin_wallets, public.magic_coin_transactions TO authenticated;
GRANT ALL ON public.magic_coin_wallets, public.magic_coin_transactions TO service_role;

ALTER TABLE public.magic_coin_wallets ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.magic_coin_transactions ENABLE ROW LEVEL SECURITY;

CREATE POLICY magic_coin_wallets_read ON public.magic_coin_wallets
  FOR SELECT TO authenticated
  USING (
    user_id = auth.uid()
    OR public.is_platform_admin(auth.uid())
    OR (
      public.has_role(auth.uid(), 'primary'::public.app_role)
      AND public.effective_owner(user_id) = public.effective_owner(auth.uid())
    )
  );

CREATE POLICY magic_coin_transactions_read ON public.magic_coin_transactions
  FOR SELECT TO authenticated
  USING (
    user_id = auth.uid()
    OR public.is_platform_admin(auth.uid())
    OR (
      public.has_role(auth.uid(), 'primary'::public.app_role)
      AND owner_id = public.effective_owner(auth.uid())
    )
  );

CREATE OR REPLACE FUNCTION public.magic_coin_wallet_touch()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = public
AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_magic_coin_wallet_touch ON public.magic_coin_wallets;
CREATE TRIGGER trg_magic_coin_wallet_touch
  BEFORE UPDATE ON public.magic_coin_wallets
  FOR EACH ROW EXECUTE FUNCTION public.magic_coin_wallet_touch();

CREATE OR REPLACE FUNCTION public.magic_coin_transaction_immutable()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = public
AS $$
BEGIN
  RAISE EXCEPTION 'MAGIC_COIN_TRANSACTION_IMMUTABLE';
END;
$$;

DROP TRIGGER IF EXISTS trg_magic_coin_transaction_immutable ON public.magic_coin_transactions;
CREATE TRIGGER trg_magic_coin_transaction_immutable
  BEFORE UPDATE OR DELETE ON public.magic_coin_transactions
  FOR EACH ROW EXECUTE FUNCTION public.magic_coin_transaction_immutable();

-- Every existing account gets an empty wallet; historical agent earnings are
-- then replayed below. New profiles receive a wallet immediately as well.
INSERT INTO public.magic_coin_wallets (user_id)
SELECT p.id FROM public.profiles p
ON CONFLICT (user_id) DO NOTHING;

CREATE OR REPLACE FUNCTION public.create_magic_coin_wallet_for_profile()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  INSERT INTO public.magic_coin_wallets (user_id) VALUES (NEW.id)
  ON CONFLICT (user_id) DO NOTHING;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_profile_magic_coin_wallet ON public.profiles;
CREATE TRIGGER trg_profile_magic_coin_wallet
  AFTER INSERT ON public.profiles
  FOR EACH ROW EXECUTE FUNCTION public.create_magic_coin_wallet_for_profile();

-- Replaying the existing commission ledger preserves every earned and
-- reversed Magic Coin. `balance_after` is deterministic by event time/id.
WITH replay AS (
  SELECT
    ap.id,
    ap.agent_id,
    COALESCE(ap.owner_id, public.effective_owner(ap.agent_id)) AS owner_id,
    ap.points,
    ap.kind,
    ap.note,
    ap.created_at,
    SUM(ap.points) OVER (
      PARTITION BY ap.agent_id ORDER BY ap.created_at, ap.id ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
    ) AS balance_after
  FROM public.agent_points ap
)
INSERT INTO public.magic_coin_transactions (
  user_id, owner_id, delta, balance_after, kind, source_agent_point_id, note, created_at
)
SELECT
  agent_id,
  owner_id,
  points,
  balance_after,
  CASE WHEN points < 0 THEN 'agent_reversal' ELSE 'agent_earn' END,
  id,
  COALESCE(note, 'Migrated Magic Coin commission'),
  created_at
FROM replay
ON CONFLICT DO NOTHING;

UPDATE public.magic_coin_wallets w
SET balance = COALESCE((
  SELECT SUM(t.delta) FROM public.magic_coin_transactions t WHERE t.user_id = w.user_id
), 0);

-- Future agent commission rows automatically feed the wallet. A refund can
-- make a wallet negative if coins were already spent; future spending is then
-- blocked until it is settled, rather than silently losing the reversal.
CREATE OR REPLACE FUNCTION public.credit_magic_coins_from_agent_point()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_balance numeric(12,2);
  v_owner uuid := COALESCE(NEW.owner_id, public.effective_owner(NEW.agent_id));
BEGIN
  INSERT INTO public.magic_coin_wallets (user_id) VALUES (NEW.agent_id)
  ON CONFLICT (user_id) DO NOTHING;

  SELECT balance INTO v_balance
  FROM public.magic_coin_wallets
  WHERE user_id = NEW.agent_id
  FOR UPDATE;
  v_balance := v_balance + NEW.points;

  UPDATE public.magic_coin_wallets SET balance = v_balance WHERE user_id = NEW.agent_id;
  INSERT INTO public.magic_coin_transactions (
    user_id, owner_id, delta, balance_after, kind, source_agent_point_id, note
  ) VALUES (
    NEW.agent_id,
    v_owner,
    NEW.points,
    v_balance,
    CASE WHEN NEW.points < 0 THEN 'agent_reversal' ELSE 'agent_earn' END,
    NEW.id,
    COALESCE(NEW.note, 'Magic Coin commission')
  );
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_agent_points_magic_coin_wallet ON public.agent_points;
CREATE TRIGGER trg_agent_points_magic_coin_wallet
  AFTER INSERT ON public.agent_points
  FOR EACH ROW EXECUTE FUNCTION public.credit_magic_coins_from_agent_point();

ALTER TABLE public.service_purchases
  ADD COLUMN IF NOT EXISTS payment_method text NOT NULL DEFAULT 'bank_transfer'
  CHECK (payment_method IN ('bank_transfer', 'magic_coins'));

-- Coin checkout only creates a paid, pending purchase. The existing reviewer
-- workflow activates the service. The wallet debit and pending row are one
-- database transaction, so a double click cannot spend twice.
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

-- Every non-approval terminal outcome returns the original wallet debit once.
CREATE OR REPLACE FUNCTION public.refund_magic_coin_service_purchase()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_balance numeric(12,2);
BEGIN
  IF NEW.payment_method <> 'magic_coins'
     OR NEW.status NOT IN ('rejected', 'cancelled', 'refunded')
     OR OLD.status = NEW.status THEN
    RETURN NEW;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM public.magic_coin_transactions
    WHERE service_purchase_id = NEW.id AND kind = 'service_payment'
  ) OR EXISTS (
    SELECT 1 FROM public.magic_coin_transactions
    WHERE service_purchase_id = NEW.id AND kind = 'service_refund'
  ) THEN
    RETURN NEW;
  END IF;

  INSERT INTO public.magic_coin_wallets (user_id) VALUES (NEW.user_id)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT balance INTO v_balance FROM public.magic_coin_wallets WHERE user_id = NEW.user_id FOR UPDATE;
  v_balance := v_balance + NEW.price_mmk;
  UPDATE public.magic_coin_wallets SET balance = v_balance WHERE user_id = NEW.user_id;
  INSERT INTO public.magic_coin_transactions (
    user_id, owner_id, delta, balance_after, kind, service_purchase_id, note
  ) VALUES (
    NEW.user_id, NEW.owner_id, NEW.price_mmk, v_balance, 'service_refund', NEW.id,
    'Magic Coins returned after service purchase ' || NEW.status
  );
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_service_purchase_magic_coin_refund ON public.service_purchases;
CREATE TRIGGER trg_service_purchase_magic_coin_refund
  AFTER UPDATE OF status ON public.service_purchases
  FOR EACH ROW EXECUTE FUNCTION public.refund_magic_coin_service_purchase();

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
  RETURN jsonb_build_object('balance', v_balance, 'mmk_value', v_balance, 'transactions', v_transactions);
END;
$$;

REVOKE ALL ON FUNCTION public.pay_service_with_magic_coins(text), public.get_magic_coin_wallet() FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.pay_service_with_magic_coins(text), public.get_magic_coin_wallet()
  TO authenticated, service_role;
