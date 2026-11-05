import { useI18n } from "../i18n";
import type { FleetPeer } from "../lib/useFleet";

const when = (value?: string) => { const d = value ? new Date(value) : null; return d && !Number.isNaN(d.getTime()) ? d.toLocaleString() : ""; };

/** Where a router's state came from and how fresh it is, as label and value pairs. */
export function statusFacts(t: (key: string) => string, peer?: FleetPeer): Array<[string, string]> {
  if (!peer) return [];
  const facts: Array<[string, string]> = [[t("fl.source"), t("fl.source." + (["hub_tunnel", "router_check", "router_read"].includes(peer.source ?? "") ? peer.source : "none"))]];
  const contact = when(peer.last_contact_at), checked = when(peer.checked_at);
  if (contact) facts.push([t("fl.lastContact"), contact]);
  if (checked) facts.push([t("fl.checked"), checked]);
  return facts;
}

export function StatusDetail({ peer }: { peer?: FleetPeer }) {
  const { t } = useI18n();
  return <>{statusFacts(t, peer).map(([label, value]) => <span className="soft small" key={label}>{label}: {value}</span>)}</>;
}
