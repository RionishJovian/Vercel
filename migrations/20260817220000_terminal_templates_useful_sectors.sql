-- Seed useful built-in Terminal REST templates per owner (RouterOS 7.1+ paths).
-- Idempotent: skips templates the owner already has by name.

CREATE OR REPLACE FUNCTION public.seed_terminal_templates(_owner uuid)
RETURNS void
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  INSERT INTO public.terminal_templates (
    owner_id, created_by, name, description, category, method, path, body, is_builtin
  )
  SELECT
    _owner,
    _owner,
    t.name,
    t.description,
    t.category,
    t.method,
    t.path,
    t.body,
    true
  FROM (
    VALUES
      ('System resource', 'CPU, memory, uptime and RouterOS version', 'system', 'GET', '/system/resource', NULL::text),
      ('Identity', 'Router identity/name', 'system', 'GET', '/system/identity', NULL),
      ('System health', 'Board temperature and health sensors', 'system', 'GET', '/system/health', NULL),
      ('RouterBoard', 'Model, serial number and board type', 'system', 'GET', '/system/routerboard', NULL),
      ('System clock', 'Current router date and time', 'system', 'GET', '/system/clock', NULL),
      ('Installed packages', 'RouterOS packages and versions', 'system', 'GET', '/system/package', NULL),
      ('Reboot router', 'Reboot the RouterBoard immediately', 'system', 'POST', '/system/reboot', NULL),
      ('IP services', 'List enabled admin services (www, ssh, api...)', 'security', 'GET', '/ip/service', NULL),
      ('Firewall filter', 'List firewall filter rules', 'security', 'GET', '/ip/firewall/filter', NULL),
      ('Firewall NAT', 'List firewall NAT rules', 'security', 'GET', '/ip/firewall/nat', NULL),
      ('Address list', 'List firewall address-lists', 'security', 'GET', '/ip/firewall/address-list', NULL),
      ('Active connections', 'Current firewall/NAT connection tracking entries', 'security', 'GET', '/ip/firewall/connection', NULL),
      ('Hotspot servers', 'Configured hotspot service instances', 'hotspot', 'GET', '/ip/hotspot', NULL),
      ('Hotspot active', 'Currently connected hotspot users', 'hotspot', 'GET', '/ip/hotspot/active', NULL),
      ('Hotspot users', 'Registered hotspot users / vouchers', 'hotspot', 'GET', '/ip/hotspot/user', NULL),
      ('Hotspot hosts', 'Hosts seen by hotspot service', 'hotspot', 'GET', '/ip/hotspot/host', NULL),
      ('IP bindings', 'Hotspot IP bindings (bypass/blocked)', 'hotspot', 'GET', '/ip/hotspot/ip-binding', NULL),
      ('Hotspot profiles', 'Server-side hotspot profiles', 'hotspot', 'GET', '/ip/hotspot/profile', NULL),
      ('User profiles', 'Bandwidth and time limits for hotspot users', 'hotspot', 'GET', '/ip/hotspot/user/profile', NULL),
      ('Walled garden', 'Destinations reachable before hotspot login', 'hotspot', 'GET', '/ip/hotspot/walled-garden', NULL),
      ('Hotspot cookies', 'Remember-me hotspot sessions', 'hotspot', 'GET', '/ip/hotspot/cookie', NULL),
      ('Create hotspot user', 'Add a voucher user — edit name/password before Send', 'hotspot', 'POST', '/ip/hotspot/user', '{"name":"voucher1","password":"changeme","profile":"default","comment":"Created from Terminal"}'),
      ('Block MAC address', 'Block a device by MAC via hotspot IP binding', 'hotspot', 'POST', '/ip/hotspot/ip-binding', '{"mac-address":"AA:BB:CC:DD:EE:FF","type":"blocked","comment":"Blocked from Terminal"}'),
      ('Interfaces', 'All interfaces with traffic counters', 'network', 'GET', '/interface', NULL),
      ('IP addresses', 'IPv4/IPv6 addresses assigned to interfaces', 'network', 'GET', '/ip/address', NULL),
      ('DNS settings', 'DNS servers and cache settings', 'network', 'GET', '/ip/dns', NULL),
      ('IP pools', 'Hotspot and DHCP address pools', 'network', 'GET', '/ip/pool', NULL),
      ('Pool usage', 'Addresses currently leased from pools', 'network', 'GET', '/ip/pool/used', NULL),
      ('Cloud DDNS', 'MikroTik cloud / DDNS status', 'network', 'GET', '/ip/cloud', NULL),
      ('WireGuard peers', 'WireGuard VPN peers (Magic Cloud hub, etc.)', 'network', 'GET', '/interface/wireguard/peers', NULL),
      ('DHCP leases', 'Active DHCP leases', 'network', 'GET', '/ip/dhcp-server/lease', NULL),
      ('ARP table', 'ARP entries', 'network', 'GET', '/ip/arp', NULL),
      ('Routes', 'Routing table', 'network', 'GET', '/ip/route', NULL),
      ('Queue tree', 'Simple queues and queue tree rules', 'qos', 'GET', '/queue/tree', NULL),
      ('Ping 1.1.1.1', 'Send 4 ICMP pings to Cloudflare DNS', 'diagnostics', 'POST', '/ping', '{"address":"1.1.1.1","count":"4"}'),
      ('Ping 8.8.8.8', 'Send 4 ICMP pings to Google DNS', 'diagnostics', 'POST', '/ping', '{"address":"8.8.8.8","count":"4"}'),
      ('DNS resolve', 'Resolve a hostname via the router DNS', 'diagnostics', 'POST', '/resolve', '{"address":"google.com","type":"A"}'),
      ('Traceroute 8.8.8.8', 'Trace route to Google DNS', 'diagnostics', 'POST', '/tool/traceroute', '{"address":"8.8.8.8","count":"1"}'),
      ('Torch WAN', 'Live traffic snapshot on ether1 (2s)', 'diagnostics', 'POST', '/tool/torch', '{"interface":"ether1","duration":"2s"}'),
      ('Bandwidth test TCP', '5s TCP bandwidth test to Cloudflare', 'diagnostics', 'POST', '/tool/bandwidth-test', '{"address":"1.1.1.1","protocol":"tcp","duration":"5s"}'),
      ('Logs (last 50)', 'Recent system logs', 'diagnostics', 'GET', '/log', NULL),
      ('List files', 'Files on the router (.backup, scripts, etc.)', 'backup', 'GET', '/file', NULL),
      ('Backup config', 'Create a full system backup', 'backup', 'POST', '/system/backup/save', '{"name":"magic-backup"}')
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

REVOKE EXECUTE ON FUNCTION public.seed_terminal_templates(uuid) FROM PUBLIC, anon, authenticated;

CREATE OR REPLACE FUNCTION public.trg_seed_terminal_templates_on_owner()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = public
AS $$
BEGIN
  IF NEW.role = 'owner'::public.app_role THEN
    PERFORM public.seed_terminal_templates(COALESCE(NEW.owner_id, NEW.user_id));
  END IF;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS seed_terminal_templates_on_owner ON public.user_roles;
CREATE TRIGGER seed_terminal_templates_on_owner
  AFTER INSERT ON public.user_roles
  FOR EACH ROW
  EXECUTE FUNCTION public.trg_seed_terminal_templates_on_owner();

-- Backfill for every existing owner account.
DO $$
DECLARE r RECORD;
BEGIN
  FOR r IN
    SELECT DISTINCT COALESCE(ur.owner_id, ur.user_id) AS owner_id
    FROM public.user_roles ur
    WHERE ur.role = 'owner'::public.app_role
  LOOP
    PERFORM public.seed_terminal_templates(r.owner_id);
  END LOOP;
END;
$$;
