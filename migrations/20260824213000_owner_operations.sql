-- Owner Operations: inventory accountability without reseller logins.
-- These records are deliberately separate from Agent points and Tier Pass orders.

CREATE TABLE IF NOT EXISTS public.voucher_resellers (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  shop_name text NOT NULL CHECK (char_length(shop_name) BETWEEN 1 AND 120),
  contact_name text NOT NULL CHECK (char_length(contact_name) BETWEEN 1 AND 120),
  location text,
  phone text,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS public.voucher_reseller_assignments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  reseller_id uuid NOT NULL REFERENCES public.voucher_resellers(id) ON DELETE RESTRICT,
  voucher_id uuid NOT NULL REFERENCES public.voucher_codes(id) ON DELETE RESTRICT,
  status text NOT NULL DEFAULT 'issued' CHECK (status IN ('issued','sold','returned','cancelled')),
  issued_at timestamptz NOT NULL DEFAULT now(),
  settled_at timestamptz,
  cash_due_mmk integer NOT NULL DEFAULT 0 CHECK (cash_due_mmk >= 0),
  note text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (owner_id, voucher_id)
);

CREATE TABLE IF NOT EXISTS public.owner_daily_closes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  site_id uuid REFERENCES public.sites(id) ON DELETE SET NULL,
  router_id uuid REFERENCES public.router_connections(id) ON DELETE SET NULL,
  business_day date NOT NULL,
  counted_cash_mmk integer NOT NULL DEFAULT 0,
  note text,
  closed_by uuid NOT NULL REFERENCES auth.users(id),
  closed_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE NULLS NOT DISTINCT (owner_id, site_id, router_id, business_day)
);

CREATE INDEX IF NOT EXISTS voucher_resellers_owner_idx ON public.voucher_resellers(owner_id, active);
CREATE INDEX IF NOT EXISTS voucher_reseller_assignments_owner_idx ON public.voucher_reseller_assignments(owner_id, status, issued_at DESC);
CREATE INDEX IF NOT EXISTS owner_daily_closes_owner_day_idx ON public.owner_daily_closes(owner_id, business_day DESC);

ALTER TABLE public.voucher_resellers ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.voucher_reseller_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.owner_daily_closes ENABLE ROW LEVEL SECURITY;

CREATE OR REPLACE FUNCTION public.owner_operations_can_read(_owner uuid)
RETURNS boolean LANGUAGE sql STABLE SECURITY INVOKER SET search_path = public AS $$
  SELECT _owner = public.effective_owner(auth.uid()) OR public.is_platform_admin(auth.uid());
$$;
CREATE OR REPLACE FUNCTION public.owner_operations_can_write(_owner uuid)
RETURNS boolean LANGUAGE sql STABLE SECURITY INVOKER SET search_path = public AS $$
  SELECT public.owner_operations_can_read(_owner)
    AND (public.has_role(auth.uid(), 'primary'::public.app_role) OR public.is_platform_admin(auth.uid()));
$$;
REVOKE ALL ON FUNCTION public.owner_operations_can_read(uuid) FROM PUBLIC, anon;
REVOKE ALL ON FUNCTION public.owner_operations_can_write(uuid) FROM PUBLIC, anon;
GRANT EXECUTE ON FUNCTION public.owner_operations_can_read(uuid), public.owner_operations_can_write(uuid) TO authenticated, service_role;

DO $$
DECLARE tbl text;
BEGIN
  FOREACH tbl IN ARRAY ARRAY['voucher_resellers','voucher_reseller_assignments','owner_daily_closes'] LOOP
    EXECUTE format('DROP POLICY IF EXISTS owner_operations_read ON public.%I', tbl);
    EXECUTE format('DROP POLICY IF EXISTS owner_operations_write ON public.%I', tbl);
    EXECUTE format('CREATE POLICY owner_operations_read ON public.%I FOR SELECT TO authenticated USING (public.owner_operations_can_read(owner_id))', tbl);
    EXECUTE format('CREATE POLICY owner_operations_write ON public.%I FOR ALL TO authenticated USING (public.owner_operations_can_write(owner_id)) WITH CHECK (public.owner_operations_can_write(owner_id))', tbl);
  END LOOP;
END $$;

DROP TRIGGER IF EXISTS touch_voucher_resellers ON public.voucher_resellers;
CREATE TRIGGER touch_voucher_resellers BEFORE UPDATE ON public.voucher_resellers FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();
DROP TRIGGER IF EXISTS touch_voucher_reseller_assignments ON public.voucher_reseller_assignments;
CREATE TRIGGER touch_voucher_reseller_assignments BEFORE UPDATE ON public.voucher_reseller_assignments FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

-- A reseller can only receive a voucher belonging to the same tenant.
CREATE OR REPLACE FUNCTION public.validate_reseller_assignment_owner()
RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path = public AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM public.voucher_resellers r WHERE r.id = NEW.reseller_id AND r.owner_id = NEW.owner_id) THEN
    RAISE EXCEPTION 'RESELLER_OWNER_MISMATCH';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM public.voucher_codes v WHERE v.id = NEW.voucher_id AND v.owner_id = NEW.owner_id) THEN
    RAISE EXCEPTION 'VOUCHER_OWNER_MISMATCH';
  END IF;
  RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS validate_reseller_assignment_owner ON public.voucher_reseller_assignments;
CREATE TRIGGER validate_reseller_assignment_owner BEFORE INSERT OR UPDATE ON public.voucher_reseller_assignments
FOR EACH ROW EXECUTE FUNCTION public.validate_reseller_assignment_owner();
