-- Bank-transfer Magic Coin purchases.
--
-- The requested quantity is retained on the service-purchase record, while an
-- immutable wallet credit is created only when that exact record is approved.
-- This makes the reviewer flow auditable and prevents direct wallet credits.

DO $$
DECLARE
  v_constraint text;
BEGIN
  SELECT conname INTO v_constraint
  FROM pg_constraint
  WHERE conrelid = 'public.service_purchases'::regclass
    AND contype = 'c'
    AND pg_get_constraintdef(oid) ILIKE '%service_key%'
  LIMIT 1;
  IF v_constraint IS NOT NULL THEN
    EXECUTE format('ALTER TABLE public.service_purchases DROP CONSTRAINT %I', v_constraint);
  END IF;
END;
$$;

ALTER TABLE public.service_purchases
  ADD CONSTRAINT service_purchases_service_key_check
  CHECK (service_key IN ('monthly', 'annual', 'plus', 'magic_coins'));

ALTER TABLE public.service_purchases
  ADD COLUMN IF NOT EXISTS magic_coin_purchase_amount numeric(12,2);

ALTER TABLE public.service_purchases
  ADD CONSTRAINT service_purchases_magic_coin_purchase_amount_check
  CHECK (
    (service_key <> 'magic_coins' AND magic_coin_purchase_amount IS NULL)
    OR (
      service_key = 'magic_coins'
      AND payment_method = 'bank_transfer'
      AND magic_coin_purchase_amount IS NOT NULL
      AND magic_coin_purchase_amount > 0
      AND price_mmk = magic_coin_purchase_amount * 1000
    )
  ) NOT VALID;

ALTER TABLE public.service_purchases
  VALIDATE CONSTRAINT service_purchases_magic_coin_purchase_amount_check;

DO $$
DECLARE
  v_constraint text;
BEGIN
  SELECT conname INTO v_constraint
  FROM pg_constraint
  WHERE conrelid = 'public.magic_coin_transactions'::regclass
    AND contype = 'c'
    AND pg_get_constraintdef(oid) ILIKE '%kind%'
  LIMIT 1;
  IF v_constraint IS NOT NULL THEN
    EXECUTE format('ALTER TABLE public.magic_coin_transactions DROP CONSTRAINT %I', v_constraint);
  END IF;
END;
$$;

ALTER TABLE public.magic_coin_transactions
  ADD CONSTRAINT magic_coin_transactions_kind_check
  CHECK (kind IN (
    'agent_earn', 'agent_reversal', 'service_payment', 'service_refund',
    'feature_key_purchase', 'purchase_credit', 'manual_credit', 'manual_debit'
  ));

CREATE UNIQUE INDEX IF NOT EXISTS magic_coin_transactions_purchase_credit_once
  ON public.magic_coin_transactions(service_purchase_id)
  WHERE kind = 'purchase_credit';

CREATE OR REPLACE FUNCTION public.credit_magic_coin_purchase_on_approval()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
DECLARE
  v_balance numeric(12,2);
BEGIN
  IF OLD.status <> 'pending'
     OR NEW.status <> 'approved'
     OR NEW.service_key <> 'magic_coins' THEN
    RETURN NEW;
  END IF;

  IF NEW.magic_coin_purchase_amount IS NULL OR NEW.magic_coin_purchase_amount <= 0 THEN
    RAISE EXCEPTION 'MAGIC_COIN_PURCHASE_AMOUNT_REQUIRED';
  END IF;

  INSERT INTO public.magic_coin_wallets (user_id) VALUES (NEW.user_id)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT balance INTO v_balance
  FROM public.magic_coin_wallets
  WHERE user_id = NEW.user_id
  FOR UPDATE;

  IF EXISTS (
    SELECT 1 FROM public.magic_coin_transactions
    WHERE service_purchase_id = NEW.id AND kind = 'purchase_credit'
  ) THEN
    RAISE EXCEPTION 'MAGIC_COIN_PURCHASE_ALREADY_CREDITED';
  END IF;

  v_balance := v_balance + NEW.magic_coin_purchase_amount;
  UPDATE public.magic_coin_wallets SET balance = v_balance WHERE user_id = NEW.user_id;
  INSERT INTO public.magic_coin_transactions (
    user_id, owner_id, delta, balance_after, kind, service_purchase_id, note, actor_user_id
  ) VALUES (
    NEW.user_id, NEW.owner_id, NEW.magic_coin_purchase_amount, v_balance, 'purchase_credit', NEW.id,
    'Bank-transfer Magic Coin purchase approved', NEW.decided_by
  );
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_service_purchase_magic_coin_credit ON public.service_purchases;
CREATE TRIGGER trg_service_purchase_magic_coin_credit
  AFTER UPDATE ON public.service_purchases
  FOR EACH ROW EXECUTE FUNCTION public.credit_magic_coin_purchase_on_approval();
