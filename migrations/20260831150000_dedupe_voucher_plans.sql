-- Voucher plans are tenant-scoped products. Historical duplicates must not be
-- deleted because voucher_codes, payment_orders, and legacy imports may still
-- reference their plan ids. Archive duplicate rows, then prevent new ones.
BEGIN;

-- Give legacy rows without a usable key a stable identity before normalizing.
UPDATE public.portal_plans
SET plan_key = 'custom-' || replace(id::text, '-', '')
WHERE plan_key IS NULL OR btrim(plan_key) = '';

UPDATE public.portal_plans
SET plan_key = lower(btrim(plan_key))
WHERE plan_key IS DISTINCT FROM lower(btrim(plan_key));

-- Keep the row with the strongest historical relationship. If none is used,
-- keep the active row, then the oldest row for deterministic cleanup.
WITH ranked AS (
  SELECT
    p.id,
    row_number() OVER (
      PARTITION BY p.owner_id, lower(btrim(p.plan_key))
      ORDER BY
        EXISTS (SELECT 1 FROM public.voucher_codes v WHERE v.plan_id = p.id) DESC,
        EXISTS (SELECT 1 FROM public.payment_orders o WHERE o.plan_id = p.id) DESC,
        EXISTS (SELECT 1 FROM public.voucher_legacy_imports i WHERE i.plan_id = p.id) DESC,
        (p.status = 'active') DESC,
        p.created_at ASC,
        p.id ASC
    ) AS duplicate_rank
  FROM public.portal_plans p
)
UPDATE public.portal_plans p
SET
  status = 'inactive',
  label = CASE
    WHEN p.label ILIKE 'Archived — %' THEN p.label
    ELSE 'Archived — ' || p.label
  END,
  plan_key = 'archived-' || replace(p.id::text, '-', '')
FROM ranked r
WHERE p.id = r.id
  AND r.duplicate_rank > 1;

CREATE UNIQUE INDEX IF NOT EXISTS portal_plans_owner_plan_key_unique
  ON public.portal_plans (owner_id, lower(btrim(plan_key)))
  WHERE plan_key IS NOT NULL AND btrim(plan_key) <> '';

COMMIT;
