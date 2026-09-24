-- Live i18n overrides (Layer B of the A+B i18n plan).
--
-- The bundled dictionaries (src/lib/i18n/locales/{my,zh}.ts) ship in the app
-- binary, so editing them requires a rebuild + redeploy. This table lets an
-- administrator correct a translation from the Language coverage page and have
-- it take effect immediately, for every user of that locale, with no rebuild.
--
-- Read  : any authenticated user (overrides are shared translations).
-- Write : platform administrators only.
--
-- Runtime merge precedence (client, see src/lib/i18n/useT.ts):
--   override  >  bundled dictionary  >  English source string

CREATE TABLE IF NOT EXISTS public.i18n_overrides (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  locale text NOT NULL CHECK (locale IN ('en', 'zh', 'my')),
  ns text NOT NULL CHECK (ns IN ('ui', 'copy')),
  key text NOT NULL,
  value text NOT NULL,
  updated_by uuid REFERENCES auth.users(id) ON DELETE SET NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (locale, ns, key)
);

CREATE INDEX IF NOT EXISTS i18n_overrides_locale_idx
  ON public.i18n_overrides (locale);

ALTER TABLE public.i18n_overrides ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS "i18n overrides read" ON public.i18n_overrides;
DROP POLICY IF EXISTS "i18n overrides platform write" ON public.i18n_overrides;

-- Every signed-in user reads every override (so a Burmese guest sees the
-- operator's corrected text), but only platform admins may change them.
CREATE POLICY "i18n overrides read" ON public.i18n_overrides
  FOR SELECT TO authenticated
  USING (true);

CREATE POLICY "i18n overrides platform write" ON public.i18n_overrides
  FOR ALL TO authenticated
  USING (public.is_platform_admin(auth.uid()))
  WITH CHECK (public.is_platform_admin(auth.uid()));

COMMENT ON TABLE public.i18n_overrides IS
  'Admin-authored translation overrides for the file-based i18n dictionaries. Merged over the bundled locales at runtime; no rebuild required.';

-- Single round-trip fetch of every override, grouped by locale/ns.
CREATE OR REPLACE FUNCTION public.get_i18n_overrides()
RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER
SET search_path = public
AS $$
  SELECT COALESCE(
    jsonb_object_agg(locale, ns_map),
    '{}'::jsonb
  )
  FROM (
    SELECT locale, jsonb_object_agg(ns, key_map) AS ns_map
    FROM (
      SELECT locale, ns, jsonb_object_agg(key, value) AS key_map
      FROM public.i18n_overrides
      GROUP BY locale, ns
    ) by_ns
    GROUP BY locale
  ) by_locale;
$$;

GRANT EXECUTE ON FUNCTION public.get_i18n_overrides() TO authenticated;

-- Upsert one override. Platform-admin gated server-side as well as by RLS.
CREATE OR REPLACE FUNCTION public.set_i18n_override(
  _locale text,
  _ns text,
  _key text,
  _value text
)
RETURNS void
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF NOT public.is_platform_admin(auth.uid()) THEN
    RAISE EXCEPTION 'NOT_AUTHORIZED';
  END IF;
  IF _locale NOT IN ('en', 'zh', 'my') THEN
    RAISE EXCEPTION 'BAD_LOCALE';
  END IF;
  IF _ns NOT IN ('ui', 'copy') THEN
    RAISE EXCEPTION 'BAD_NS';
  END IF;
  IF _value IS NULL OR length(btrim(_value)) = 0 THEN
    RAISE EXCEPTION 'EMPTY_VALUE';
  END IF;

  INSERT INTO public.i18n_overrides (locale, ns, key, value, updated_by, updated_at)
  VALUES (_locale, _ns, _key, _value, auth.uid(), now())
  ON CONFLICT (locale, ns, key)
  DO UPDATE SET value = EXCLUDED.value,
                updated_by = EXCLUDED.updated_by,
                updated_at = now();
END;
$$;

GRANT EXECUTE ON FUNCTION public.set_i18n_override(text, text, text, text) TO authenticated;

-- Remove an override so the bundled dictionary value applies again.
CREATE OR REPLACE FUNCTION public.delete_i18n_override(
  _locale text,
  _ns text,
  _key text
)
RETURNS void
LANGUAGE plpgsql SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF NOT public.is_platform_admin(auth.uid()) THEN
    RAISE EXCEPTION 'NOT_AUTHORIZED';
  END IF;
  DELETE FROM public.i18n_overrides
  WHERE locale = _locale AND ns = _ns AND key = _key;
END;
$$;

GRANT EXECUTE ON FUNCTION public.delete_i18n_override(text, text, text) TO authenticated;
