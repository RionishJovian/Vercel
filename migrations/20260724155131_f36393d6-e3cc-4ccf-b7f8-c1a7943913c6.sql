CREATE TABLE public.terminal_templates (
  id UUID NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
  owner_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  created_by UUID REFERENCES auth.users(id) ON DELETE SET NULL,
  name TEXT NOT NULL,
  description TEXT,
  category TEXT NOT NULL DEFAULT 'general',
  method TEXT NOT NULL DEFAULT 'GET' CHECK (method IN ('GET','POST','PATCH','PUT','DELETE')),
  path TEXT NOT NULL CHECK (path LIKE '/%'),
  body TEXT,
  is_builtin BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX terminal_templates_owner_idx ON public.terminal_templates(owner_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON public.terminal_templates TO authenticated;
GRANT ALL ON public.terminal_templates TO service_role;

ALTER TABLE public.terminal_templates ENABLE ROW LEVEL SECURITY;

CREATE POLICY "owners view own templates" ON public.terminal_templates
  FOR SELECT TO authenticated
  USING (public.has_role(auth.uid(), 'owner') AND owner_id = COALESCE(public.effective_owner(auth.uid()), auth.uid()));

CREATE POLICY "owners insert own templates" ON public.terminal_templates
  FOR INSERT TO authenticated
  WITH CHECK (public.has_role(auth.uid(), 'owner') AND owner_id = COALESCE(public.effective_owner(auth.uid()), auth.uid()));

CREATE POLICY "owners update own templates" ON public.terminal_templates
  FOR UPDATE TO authenticated
  USING (public.has_role(auth.uid(), 'owner') AND owner_id = COALESCE(public.effective_owner(auth.uid()), auth.uid()))
  WITH CHECK (public.has_role(auth.uid(), 'owner') AND owner_id = COALESCE(public.effective_owner(auth.uid()), auth.uid()));

CREATE POLICY "owners delete own templates" ON public.terminal_templates
  FOR DELETE TO authenticated
  USING (public.has_role(auth.uid(), 'owner') AND owner_id = COALESCE(public.effective_owner(auth.uid()), auth.uid()));

CREATE TRIGGER touch_terminal_templates
  BEFORE UPDATE ON public.terminal_templates
  FOR EACH ROW EXECUTE FUNCTION public.touch_updated_at();

-- Seed built-in templates for every existing owner
INSERT INTO public.terminal_templates (owner_id, created_by, name, description, category, method, path, body, is_builtin)
SELECT ur.user_id, ur.user_id, t.name, t.description, t.category, t.method, t.path, t.body, true
FROM public.user_roles ur
CROSS JOIN (VALUES
  ('System resource', 'CPU, memory, uptime and RouterOS version', 'system', 'GET', '/system/resource', NULL),
  ('Identity', 'Router identity/name', 'system', 'GET', '/system/identity', NULL),
  ('Reboot router', 'Reboot the RouterBoard immediately', 'system', 'POST', '/system/reboot', NULL),
  ('IP services', 'List enabled admin services (www, ssh, api...)', 'security', 'GET', '/ip/service', NULL),
  ('Firewall filter', 'List firewall filter rules', 'security', 'GET', '/ip/firewall/filter', NULL),
  ('Firewall NAT', 'List firewall NAT rules', 'security', 'GET', '/ip/firewall/nat', NULL),
  ('Address list', 'List firewall address-lists', 'security', 'GET', '/ip/firewall/address-list', NULL),
  ('Hotspot active', 'Currently connected hotspot users', 'hotspot', 'GET', '/ip/hotspot/active', NULL),
  ('Hotspot users', 'Registered hotspot users / vouchers', 'hotspot', 'GET', '/ip/hotspot/user', NULL),
  ('Hotspot hosts', 'Hosts seen by hotspot service', 'hotspot', 'GET', '/ip/hotspot/host', NULL),
  ('IP bindings', 'Hotspot IP bindings (bypass/blocked)', 'hotspot', 'GET', '/ip/hotspot/ip-binding', NULL),
  ('Interfaces', 'All interfaces with traffic counters', 'network', 'GET', '/interface', NULL),
  ('DHCP leases', 'Active DHCP leases', 'network', 'GET', '/ip/dhcp-server/lease', NULL),
  ('ARP table', 'ARP entries', 'network', 'GET', '/ip/arp', NULL),
  ('Routes', 'Routing table', 'network', 'GET', '/ip/route', NULL),
  ('Ping 1.1.1.1', 'Send 4 ICMP pings to Cloudflare DNS', 'diagnostics', 'POST', '/ping', '{"address":"1.1.1.1","count":"4"}'),
  ('Ping 8.8.8.8', 'Send 4 ICMP pings to Google DNS', 'diagnostics', 'POST', '/ping', '{"address":"8.8.8.8","count":"4"}'),
  ('Torch WAN', 'Live traffic snapshot on ether1 (2s)', 'diagnostics', 'POST', '/tool/torch', '{"interface":"ether1","duration":"2s"}'),
  ('Logs (last 50)', 'Recent system logs', 'diagnostics', 'GET', '/log', NULL),
  ('Backup config', 'Create a full system backup', 'backup', 'POST', '/system/backup/save', '{"name":"magic-backup"}')
) AS t(name, description, category, method, path, body)
WHERE ur.role = 'owner';