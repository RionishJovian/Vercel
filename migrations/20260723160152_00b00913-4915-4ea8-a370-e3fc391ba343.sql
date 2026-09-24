
-- Add username column to profiles for username-based sign-in
ALTER TABLE public.profiles ADD COLUMN IF NOT EXISTS username text UNIQUE;
CREATE INDEX IF NOT EXISTS profiles_username_lower_idx ON public.profiles (lower(username));

-- Table to record app owner credential mapping (username -> auth user)
CREATE TABLE IF NOT EXISTS public.owner_accounts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  username text NOT NULL UNIQUE,
  auth_email text NOT NULL,
  user_id uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT ON public.owner_accounts TO authenticated;
GRANT ALL ON public.owner_accounts TO service_role;

ALTER TABLE public.owner_accounts ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Owners can view owner_accounts"
  ON public.owner_accounts FOR SELECT
  TO authenticated
  USING (public.has_role(auth.uid(), 'owner'));
