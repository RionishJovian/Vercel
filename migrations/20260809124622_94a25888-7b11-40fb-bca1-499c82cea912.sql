ALTER TABLE public.profiles ADD COLUMN IF NOT EXISTS language text NOT NULL DEFAULT 'en';
ALTER TABLE public.sites ADD COLUMN IF NOT EXISTS latitude numeric;
ALTER TABLE public.sites ADD COLUMN IF NOT EXISTS longitude numeric;