import { useState } from "react";
import { api, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { errorText } from "../../lib/errors";

interface Capability { name: string; status: string; evidence?: string }
interface DiscoveredDevice {
  mac: string;
  name: string;
  kind: string;
  kind_evidence: string;
  reported_vendor?: string;
  reported_model?: string;
  vendor_evidence?: string;
  addresses: string[];
  sources: string[];
  attachments: Array<{ source: string; gateway_interface: string; relation: string; direct_physical_link_verified: boolean }>;
  capabilities: Capability[];
  possibly_randomized_mac: boolean;
  online_verified: boolean;
}
interface DiscoverySnapshot {
  observed_at?: string;
  devices: DiscoveredDevice[];
  sources: Array<{ name: string; available: boolean; count: number }>;
  partial: boolean;
  truncated: boolean;
  reads_only: boolean;
  physical_links_verified: boolean;
  notes: string[];
}

export function UniversalDiscoveryPanel({ gatewayId }: { gatewayId: string }) {
  const { lang } = useI18n();
  const my = lang === "my";
  const [data, setData] = useState<DiscoverySnapshot | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [loadedGateway, setLoadedGateway] = useState("");
  const active = loadedGateway === gatewayId ? data : null;
  async function inspect() {
    if (busy) return;
    setBusy(true); setError(""); setData(null); setLoadedGateway(gatewayId);
    try {
      const response = await api.get<{ discovery: DiscoverySnapshot }>(`/routers/${encodeURIComponent(gatewayId)}/discovery`);
      if (response.discovery?.reads_only !== true || response.discovery.physical_links_verified !== false) {
        throw new Error("Unexpected discovery verification contract");
      }
      setData(response.discovery);
    } catch (e) {
      if (isUnauthorized(e)) throw e;
      setError(errorText(e, my ? "စက်ရှာဖွေမှု မရနိုင်ပါ" : "Read-only device discovery is unavailable."));
    } finally { setBusy(false); }
  }
  return <section className="glass pad stack" aria-label={my ? "စက်များကို အလိုအလျောက်ရှာဖွေခြင်း" : "Universal device discovery"}>
    <h2>{my ? "စက်များကို အလိုအလျောက်ရှာဖွေခြင်း" : "Universal device discovery"}</h2>
    <p className="soft small">{my
      ? "MikroTik Gateway တွင် ရရှိပြီးသား DHCP, ARP, Neighbor, Bridge နှင့် Wi-Fi မှတ်တမ်းများမှ စက်များကို စုစည်းပါသည်။ Scan မလုပ်၊ စက်များကို မပြင်ပါ။"
      : "Combine the gateway's existing DHCP, ARP, neighbor, bridge and Wi-Fi observations. No LAN scan, device login or configuration change."}</p>
    <button className="pill" type="button" disabled={busy} onClick={() => void inspect()}>
      {busy ? (my ? "ဖတ်နေသည်…" : "Reading…") : (my ? "စက်များကို စုစည်းကြည့်မည်" : "Discover from gateway")}
    </button>
    {error && <Note kind="warn">{error}</Note>}
    {active && <>
      <p className="soft small" role="status">{active.devices.length} {my ? "စက်များတွေ့ရှိ" : "MAC identities observed"} · {active.observed_at || (my ? "အချိန် မသိ" : "Observation time unknown")}</p>
      {(active.partial || active.truncated) && <Note kind="warn">{my
        ? "အချက်အလက်အချို့ ဖတ်မရသဖြင့် စာရင်း မပြည့်စုံနိုင်ပါ။"
        : "Partial discovery: one or more sources were unavailable or the bounded result was truncated."}</Note>}
      <Note kind="warn">{my
        ? "ပိုင်ရှင်မှ နာမည်ပေးထားခြင်း၊ Vendor အမည်တွေ့ခြင်း သို့မဟုတ် Port မှတစ်ဆင့် တွေ့ခြင်းသည် စက်ကို အဝေးမှပြင်နိုင်ကြောင်း၊ အွန်လိုင်းရှိကြောင်း၊ ကြိုးတိုက်ရိုက်ချိတ်ထားကြောင်း အထောက်အထား မဟုတ်ပါ။"
        : "Vendor names and MAC/port sightings do not verify manageability, live reachability or a direct physical cable. Configuration capabilities remain unknown."}</Note>
      <p className="soft small">{active.sources.map(s => `${s.name}: ${s.available ? s.count : "unavailable"}`).join(" · ")}</p>
      {!active.devices.length && <p className="soft small">{my ? "မြင်နိုင်သော စက် မရှိသေးပါ" : "No valid device MACs in the available tables."}</p>}
      {active.devices.map(d => <article className="stack" key={d.mac} style={{ borderTop: "1px solid var(--edge-soft)", paddingTop: 12 }}>
        <b>{d.name}</b>
        <p className="small num">{d.mac}{d.addresses.length ? " · " + d.addresses.join(", ") : ""}</p>
        <p className="soft small">{d.reported_vendor || (my ? "Brand မသိသေး" : "Vendor unknown")}
          {d.reported_model ? " · " + d.reported_model : ""}
          {" · "}{d.kind !== "unknown" ? d.kind + " (" + d.kind_evidence + ")" : (my ? "အမျိုးအစားမသိ" : "Type unknown")}</p>
        <p className="soft small">{my ? "အချက်အလက်ရင်းမြစ်" : "Sources"}: {d.sources.join(", ")}</p>
        {d.attachments.map(a => <p key={a.source + a.gateway_interface} className="soft small">
          {a.source}: {a.gateway_interface} · {my ? "တိုက်ရိုက်ချိတ်ဆက်မှု မအတည်ပြုနိုင်" : "physical hop unverified"}
        </p>)}
        <p className="soft small">{my ? "စီမံခန့်ခွဲနိုင်မှု" : "Management"}: {d.capabilities.find(c => c.name === "third_party_management")?.status || "unknown"}
          {" · "}{my ? "Wi-Fi ပြင်ဆင်နိုင်မှု" : "Radio optimization"}: {d.capabilities.find(c => c.name === "radio_optimization")?.status || "unknown"}
          {d.possibly_randomized_mac ? " · " + (my ? "MAC ပြောင်းနိုင်" : "Possibly randomized MAC") : ""}</p>
      </article>)}
    </>}
  </section>;
}
