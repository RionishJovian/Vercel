// Typed network settings. Ported one-to-one from the previous app so the server-side
// review/apply contract is unchanged. Labels are English router terms for now.
export type FieldType = "interface" | "state" | "select" | "record" | "password" | "number" | undefined;
export type Field = [key: string, label: string, type?: FieldType, arg?: string | Array<[string, string]>];
export interface Schema { title: string; menu: string | null; create?: boolean; readOnly?: boolean; fields: Field[] }

export const schema: Record<string, Schema> = {
  firewall: { title: "Invalid-traffic protection", menu: "filters", create: true, fields: [["interface", "Router port to protect", "interface"], ["disabled", "Protection rule", "state"]] },
  shield: { title: "Portal bypass shield", menu: "bindings", fields: [["disabled", "Selected bypass", "select", [["yes", "Disable this exemption; require portal login"]]]] },
  fairshare: { title: "Fairshare QoS", menu: "queues", create: true, fields: [["name", "Policy name"], ["target", "Customer subnet (e.g. 192.168.50.0/24)"], ["max-limit", "Total upload / download capacity (e.g. 10M/50M)"], ["disabled", "Fair sharing", "state"]] },
  addresses: { title: "Router IP addresses", menu: "addresses", readOnly: true, fields: [] },
  routes: { title: "Routing table", menu: "routes", readOnly: true, fields: [] },
  wan: { title: "Internet uplinks", menu: null, fields: [["mode", "Routing mode", "select", [["failover", "Primary + backup"], ["balance", "Share connections across both uplinks"]]], ["wan1", "Primary ISP port", "interface"], ["gateway1", "Primary ISP gateway"], ["wan2", "Second ISP port", "interface"], ["gateway2", "Second ISP gateway"]] },
  pppoe_client: { title: "PPPoE internet uplink", menu: "pppoe_clients", create: true, fields: [["name", "Uplink name"], ["interface", "ISP-facing port", "interface"], ["user", "ISP username"], ["password", "ISP password", "password"], ["distance", "Route priority (lower is preferred)", "number"], ["disabled", "Service state", "state"]] },
  pppoe_server: { title: "PPPoE subscriber service", menu: null, fields: [["name", "Service name"], ["interface", "Unused subscriber-facing port", "interface"], ["gateway_cidr", "Subscriber gateway / prefix"], ["pool_start", "First subscriber address"], ["pool_end", "Last subscriber address"], ["dns", "Subscriber DNS"], ["rate_limit", "Upload / download speed (e.g. 10M/20M)"]] },
  pppoe_service: { title: "Existing PPPoE services", menu: "pppoe_servers", fields: [["default-profile", "Subscriber plan", "record", "ppp_profiles"], ["disabled", "Service state", "state"]] },
  subscriber: { title: "PPPoE subscribers", menu: "subscribers", create: true, fields: [["name", "Subscriber login"], ["password", "Subscriber password", "password"], ["profile", "Subscriber plan", "record", "ppp_profiles"], ["disabled", "Account state", "state"]] },
  ppp_profile: { title: "Subscriber plans", menu: "ppp_profiles", fields: [["local-address", "Subscriber gateway address"], ["remote-address", "Address pool", "record", "pools"], ["dns-server", "Client DNS servers"], ["rate-limit", "Upload / download speed"], ["only-one", "One simultaneous session", "select", [["yes", "Yes"], ["no", "No"]]]] },
  wifi: { title: "Wi-Fi SSIDs", menu: "wifi", fields: [["ssid", "Wi-Fi network name"], ["country", "Actual installation country"], ["password", "New Wi-Fi password", "password"], ["disabled", "Radio state", "state"]] },
  wireless: { title: "Legacy Wi-Fi SSIDs", menu: "wireless", fields: [["ssid", "Wi-Fi network name"], ["country", "Actual installation country"], ["password", "New Wi-Fi password", "password"], ["disabled", "Radio state", "state"]] },
  dhcp: { title: "DHCP services", menu: "dhcp", fields: [["address-pool", "Client address pool", "record", "pools"], ["lease-time", "Lease duration (e.g. 1h)"], ["disabled", "Service state", "state"]] },
  pool: { title: "Address pools", menu: "pools", fields: [["ranges", "First-last client address"]] },
  dhcp_network: { title: "Client gateway and DNS", menu: "dhcp_networks", fields: [["gateway", "Client gateway"], ["dns-server", "Client DNS servers"]] },
  hotspot: { title: "Hotspot services", menu: "hotspots", fields: [["profile", "Hotspot profile", "record", "hotspot_profiles"], ["disabled", "Service state", "state"]] },
  hotspot_profile: { title: "Hotspot login profiles", menu: "hotspot_profiles", fields: [["dns-name", "Portal DNS name"], ["login-by", "Guest login", "select", [["http-chap", "Password (CHAP)"], ["http-chap,mac", "Password + MAC grace"]]], ["use-radius", "Use RADIUS", "select", [["yes", "Yes"], ["no", "No"]]]] },
  lease: { title: "Static DHCP reservations", menu: "leases", create: true, fields: [["address", "Reserved client address"], ["mac-address", "Client MAC address"], ["server", "DHCP service", "record", "dhcp"], ["disabled", "Reservation state", "state"]] },
  queue: { title: "Customer bandwidth limits", menu: "queues", create: true, fields: [["name", "Limit name"], ["target", "Client address / prefix"], ["max-limit", "Upload / download speed (e.g. 10M/20M)"], ["disabled", "Limit state", "state"]] },
};

export const workspaces = ["wan", "ip", "existing", "protection", "qos", "wifi", "pppoe"] as const;
export type Workspace = (typeof workspaces)[number];
export const choices: Record<Workspace, string[]> = {
  protection: ["firewall", "shield"], qos: ["fairshare", "queue"], ip: ["addresses", "routes", "dhcp", "pool", "dhcp_network", "lease"],
  wan: ["wan", "pppoe_client"], pppoe: ["pppoe_server", "pppoe_service", "subscriber", "ppp_profile"], wifi: ["wifi", "wireless"],
  existing: ["dhcp", "pool", "dhcp_network", "hotspot", "hotspot_profile", "lease", "queue"],
};
export const workspaceInfo: Record<Workspace, [string, string]> = {
  protection: ["Protection", "Manage router protection and review portal exemptions."],
  qos: ["Fairshare QoS", "Control customer bandwidth and fair sharing."],
  ip: ["IP and DHCP", "Inspect IP addresses and routes; manage client address allocation."],
  wan: ["Internet connections", "Choose how your business connects to the internet."],
  pppoe: ["Customer internet", "Manage subscriber services, customer logins and speed plans."],
  wifi: ["Wi-Fi networks", "Manage the radios on the selected device. Remote access points need their own connected device."],
  existing: ["LAN and Hotspot", "Manage the services already running on your customer network."],
};
export const descriptions: Record<string, string> = {
  firewall: "Drop invalid IPv4 connections to the router on one selected port. This switch changes only the Mega Magic protection rule; all other firewall rules stay in place.",
  shield: "Review HotSpot IP-binding exemptions. Disable one unwanted bypass after review; managed grace access is protected. This does not claim to prevent every bypass method.",
  fairshare: "Share total upload/download bandwidth between client IP addresses using PCQ. Active FastTrack and overlapping queues must be reviewed first.",
  addresses: "Inspect assigned router addresses. Use Create customer network for a new LAN; existing management addresses are protected from casual edits.",
  routes: "Inspect routes and priorities. Use Internet connections for reviewed dual-uplink changes.",
  wan: "Keep a backup ISP ready or distribute connections across two ISPs.", pppoe_client: "Connect an ISP that gave you a PPPoE username and password.",
  pppoe_server: "Set up internet service for your wired subscribers.", pppoe_service: "Change the plan or state of an existing subscriber service.",
  subscriber: "Create or manage a customer login.", ppp_profile: "Set address allocation and speeds for a subscriber plan.",
  wifi: "Change the name and password of an onboard Wi-Fi network.", wireless: "Manage radios using the older RouterOS wireless driver.",
  dhcp: "Manage automatic client address assignment.", pool: "Adjust the addresses available to clients.", dhcp_network: "Set the gateway and DNS supplied to LAN clients.",
  hotspot: "Manage an existing guest internet service.", hotspot_profile: "Configure how guests sign in to a Hotspot.", lease: "Reserve an address for a specific client.", queue: "Set a client upload and download limit.",
};
export const currentNames: Record<string, string> = {
  interface: "Port", gateway: "Gateway", "add-default-route": "Automatic default route", disabled: "State", "dst-address": "Destination", distance: "Route priority", "default-route-distance": "Route priority", "use-peer-dns": "Use ISP DNS",
  "configuration.ssid": "Wi-Fi network name", "configuration.country": "Country", "configuration.mode": "Radio mode", ssid: "Wi-Fi network name", country: "Country", mode: "Mode", user: "ISP username", name: "Name", address: "Address", dynamic: "Dynamic", "routing-table": "Routing table", comment: "Comment",
};
export const isSecretField = (key: string) => /password|passphrase|pre-shared-key|private-key|secret/i.test(key);

export interface NetObject { id: string; values: Record<string, string> }
export interface Inventory {
  interfaces?: Array<{ name: string; type: string; running?: boolean }>;
  available?: Record<string, boolean>;
  objects?: Record<string, NetObject[]>;
  write_enabled?: boolean; wifi_write_enabled?: boolean;
}
export const objectsOf = (inv: Inventory, key: string): NetObject[] => inv.objects?.[key] ?? [];
export const labelOf = (o: NetObject) => o.values.name || o.values["service-name"] || o.values.address || o.id;
const yes = (v?: string) => v === "yes" || v === "true";

/** Which records each task may edit. Only the app's own rules are offered for the managed kinds. */
export function listFor(inv: Inventory, kind: string): NetObject[] {
  const s = schema[kind];
  let list = s.menu ? objectsOf(inv, s.menu) : [];
  if (kind === "firewall") list = list.filter((o) => o.values.comment?.startsWith("mm-protection-invalid-"));
  if (kind === "shield") list = list.filter((o) => o.values.type === "bypassed" && !yes(o.values.disabled) && !/^mm-/i.test(o.values.comment || ""));
  if (kind === "fairshare") list = list.filter((o) => o.values.comment?.startsWith("mm-fairshare-v1:"));
  if (kind === "queue") list = list.filter((o) => !o.values.comment?.startsWith("mm-fairshare-v1:"));
  if (kind === "lease") list = list.filter((o) => !yes(o.values.dynamic));
  return list;
}

/** Starting values for the form: the chosen record, or sensible suggestions that are never applied without review. */
export function initialValues(kind: string, selected: NetObject | undefined, defaults: Record<string, string>): Record<string, string> {
  const v: Record<string, string> = { ...(selected?.values ?? {}) };
  if (!selected) Object.assign(v, { name: kind === "pppoe_server" ? "subscribers" : "customer", distance: "2", disabled: "no", dns: "1.1.1.1,8.8.8.8", rate_limit: "10M/20M", mode: "failover", gateway_cidr: defaults.gateway_cidr || "", pool_start: defaults.pool_start || "", pool_end: defaults.pool_end || "" });
  if (kind === "firewall") v.interface = v["in-interface"] || "";
  if (kind === "shield") v.disabled = "yes";
  if (kind === "fairshare") v.name = selected ? (v.name || "").replace(/^mm-fair-/, "") : "guests";
  if (kind === "wifi") { v.ssid = v["configuration.ssid"]; v.country = v["configuration.country"]; }
  return v;
}
