ALTER TYPE public.app_role ADD VALUE IF NOT EXISTS 'agent';
ALTER TYPE public.app_role ADD VALUE IF NOT EXISTS 'pending';

CREATE TABLE IF NOT EXISTS public.account_referrals (
  user_id uuid PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
  agent_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS account_referrals_agent_idx ON public.account_referrals(agent_id);

GRANT SELECT ON public.account_referrals TO authenticated;
GRANT ALL ON public.account_referrals TO service_role;
ALTER TABLE public.account_referrals ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Agents read own referrals"
ON public.account_referrals FOR SELECT TO authenticated
USING (agent_id = auth.uid());

CREATE POLICY "Owners read all referrals"
ON public.account_referrals FOR SELECT TO authenticated
USING (EXISTS (
  SELECT 1 FROM public.user_roles ur
  WHERE ur.user_id = auth.uid() AND ur.role::text IN ('owner','admin')
));

CREATE TABLE IF NOT EXISTS public.agent_points (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  agent_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  referred_user_id uuid REFERENCES auth.users(id) ON DELETE SET NULL,
  kind text NOT NULL CHECK (kind IN ('signup','renewal')),
  points integer NOT NULL,
  note text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agent_points_agent_idx ON public.agent_points(agent_id, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS agent_points_one_signup_per_user
  ON public.agent_points(referred_user_id) WHERE kind = 'signup';

GRANT SELECT ON public.agent_points TO authenticated;
GRANT ALL ON public.agent_points TO service_role;
ALTER TABLE public.agent_points ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Agents read own points"
ON public.agent_points FOR SELECT TO authenticated
USING (agent_id = auth.uid());

CREATE POLICY "Owners read all points"
ON public.agent_points FOR SELECT TO authenticated
USING (EXISTS (
  SELECT 1 FROM public.user_roles ur
  WHERE ur.user_id = auth.uid() AND ur.role::text IN ('owner','admin')
));