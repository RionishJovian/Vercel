CREATE TABLE public.ai_scan_limits (
  user_id uuid PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
  owner_id uuid,
  monthly_limit integer NOT NULL DEFAULT 30 CHECK (monthly_limit >= 0 AND monthly_limit <= 500),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.ai_scan_limits TO authenticated;
GRANT ALL ON public.ai_scan_limits TO service_role;

ALTER TABLE public.ai_scan_limits ENABLE ROW LEVEL SECURITY;

CREATE POLICY "read own scan limit"
  ON public.ai_scan_limits FOR SELECT TO authenticated
  USING (user_id = auth.uid());

CREATE POLICY "owners manage tenant scan limits"
  ON public.ai_scan_limits FOR ALL TO authenticated
  USING (
    (public.has_role(auth.uid(), 'owner') OR public.has_role(auth.uid(), 'admin'))
    AND owner_id = public.effective_owner(auth.uid())
  )
  WITH CHECK (
    (public.has_role(auth.uid(), 'owner') OR public.has_role(auth.uid(), 'admin'))
    AND owner_id = public.effective_owner(auth.uid())
  );

CREATE TRIGGER touch_ai_scan_limits
  BEFORE UPDATE ON public.ai_scan_limits
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();