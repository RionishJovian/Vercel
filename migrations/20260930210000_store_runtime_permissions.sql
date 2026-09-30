-- Repair existing Store installations without broad grants or RLS bypass.
GRANT SELECT, INSERT, UPDATE ON public.service_purchases TO authenticated;
GRANT SELECT, INSERT ON public.service_purchase_audit TO authenticated;
