-- Add read-only RouterOS configuration templates to the Terminal library.
-- Existing linked application scripts are untouched; this is a separate,
-- idempotent seed for the REST Terminal only.

CREATE OR REPLACE FUNCTION public.seed_terminal_common_configuration_templates(_owner uuid)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $$
BEGIN
  INSERT INTO public.terminal_templates (
    owner_id, created_by, name, description, category, method, path, body, is_builtin
  )
  SELECT _owner, _owner, t.name, t.description, t.category, t.method, t.path, t.body, true
  FROM (
    VALUES
      ('Bridge configuration', 'Bridge settings, protocol mode and VLAN filtering state', 'network', 'GET', '/interface/bridge', NULL::text),
      ('Bridge ports', 'Ports assigned to each bridge and their PVID settings', 'network', 'GET', '/interface/bridge/port', NULL::text),
      ('Bridge VLANs', 'Bridge VLAN table and tagged or untagged membership', 'network', 'GET', '/interface/bridge/vlan', NULL::text),
      ('DHCP servers', 'Configured DHCP servers and address pools', 'network', 'GET', '/ip/dhcp-server', NULL::text),
      ('DHCP networks', 'DHCP network gateway and DNS options', 'network', 'GET', '/ip/dhcp-server/network', NULL::text),
      ('WiFi interfaces', 'RouterOS 7 WiFi package interface configuration', 'wireless', 'GET', '/interface/wifi', NULL::text),
      ('Wireless legacy interfaces', 'Legacy wireless interface configuration for non-WiFi-package boards', 'wireless', 'GET', '/interface/wireless', NULL::text),
      ('Hotspot server profiles', 'Captive-portal login and HTML-directory settings', 'hotspot', 'GET', '/ip/hotspot/profile', NULL::text)
  ) AS t(name, description, category, method, path, body)
  WHERE NOT EXISTS (
    SELECT 1
    FROM public.terminal_templates existing
    WHERE existing.owner_id = _owner
      AND existing.name = t.name
      AND existing.is_builtin = true
  );
END;
$$;

REVOKE EXECUTE ON FUNCTION public.seed_terminal_common_configuration_templates(uuid)
  FROM PUBLIC, anon, authenticated;

CREATE OR REPLACE FUNCTION public.trg_seed_terminal_common_configuration_templates()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = ''
AS $$
BEGIN
  IF NEW.role = 'primary'::public.app_role THEN
    PERFORM public.seed_terminal_common_configuration_templates(COALESCE(NEW.owner_id, NEW.user_id));
  END IF;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS seed_terminal_common_configuration_templates_on_owner ON public.user_roles;
CREATE TRIGGER seed_terminal_common_configuration_templates_on_owner
  AFTER INSERT ON public.user_roles
  FOR EACH ROW
  EXECUTE FUNCTION public.trg_seed_terminal_common_configuration_templates();

DO $$
DECLARE r RECORD;
BEGIN
  FOR r IN
    SELECT DISTINCT COALESCE(ur.owner_id, ur.user_id) AS owner_id
    FROM public.user_roles ur
    WHERE ur.role = 'primary'::public.app_role
    UNION
    SELECT pa.user_id AS owner_id FROM public.platform_admins pa
  LOOP
    PERFORM public.seed_terminal_common_configuration_templates(r.owner_id);
  END LOOP;
END;
$$;
