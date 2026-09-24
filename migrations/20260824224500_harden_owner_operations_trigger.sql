-- Trigger functions are invoked only by PostgreSQL. They must never be RPCs.
-- Keep SECURITY DEFINER for the cross-table owner check, but remove every
-- direct caller grant so neither anon nor authenticated users can execute it.
REVOKE EXECUTE ON FUNCTION public.validate_reseller_assignment_owner()
  FROM PUBLIC, anon, authenticated;
