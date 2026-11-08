import { useState } from "react";
import { api, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { errorText } from "../../lib/errors";

export interface CAPRecord {
  identity?: string;
  address?: string;
  state?: string;
  state_known?: boolean;
  online_verified?: boolean;
  ssid_verified?: boolean;
}
export interface ControllerRecord {
  family: string;
  inventory_available: boolean;
  radio_inventory_available: boolean;
  caps?: CAPRecord[];
}
export interface SSIDProfile {
  family: string;
  name?: string;
  ssid?: string;
  broadcasting_verified?: boolean;
}
export interface CAPsMANView {
  controllers: ControllerRecord[];
  profiles: SSIDProfile[];
  write_enabled?: boolean;
}
interface ObservationResponse {
  inventory?: {
    observed_at?: string;
    capsman?: CAPsMANView;
    wifi_write_enabled?: boolean;
    write_enabled?: boolean;
  };
}

export function CAPsMANInventoryPanel({ id }: { id: string }) {
  const { lang } = useI18n();
  const my = lang === "my";
  const [state, setState] = useState<
    { kind: "idle" | "loading" } |
    { kind: "error"; message: string } |
    { kind: "ready"; gateway: string; observedAt?: string; view: CAPsMANView }
  >({ kind: "idle" });
  const ready = state.kind === "ready" && state.gateway === id ? state : null;
  async function inspect() {
    if (state.kind === "loading") return;
    setState({ kind: "loading" });
    try {
      const response = await api.get<ObservationResponse>(`/routers/${encodeURIComponent(id)}/observations`);
      const inv = response.inventory;
      if (!inv || inv.write_enabled !== false || inv.wifi_write_enabled !== false ||
          inv.capsman?.write_enabled !== false || !Array.isArray(inv.capsman.controllers) ||
          !Array.isArray(inv.capsman.profiles)) {
        throw new Error("Read-only CAPsMAN evidence is not available on this server");
      }
      setState({ kind: "ready", gateway: id, observedAt: inv.observed_at, view: inv.capsman });
    } catch (error) {
      if (isUnauthorized(error)) throw error;
      setState({ kind: "error", message: errorText(error, my ? "CAPsMAN အချက်အလက် မရနိုင်ပါ" : "CAPsMAN observations are unavailable") });
    }
  }
  return <section className="glass pad stack" aria-label="CAPsMAN Wi-Fi inventory">
    <h2>{my ? "Wi-Fi နှင့် CAPsMAN စာရင်း" : "Wi-Fi / CAPsMAN inventory"}</h2>
    <p className="soft small">{my
      ? "MikroTik Controller မှ CAP နှင့် SSID profile များကို ဖတ်ရန်သာ ကြည့်ပါသည်။ SSID ထုတ်လွှင့်နေသည် သို့မဟုတ် AP ကို ထိန်းချုပ်နိုင်သည်ဟု မအတည်ပြုပါ။"
      : "Read-only CAP and SSID profile evidence from this MikroTik controller. A saved profile does not prove an AP is broadcasting it or that Mega Magic can change it."}</p>
    <button type="button" className="pill" disabled={state.kind === "loading"} onClick={() => void inspect()}>
      {state.kind === "loading" ? (my ? "ဖတ်နေသည်…" : "Reading…") : (my ? "Wi-Fi စာရင်းဖတ်မည်" : "Read Wi-Fi inventory")}
    </button>
    {state.kind === "error" && <Note kind="warn">{state.message}</Note>}
    {ready && <>
      <p role="status" className="soft small">{my ? "ဖတ်ယူချိန်" : "Observed at"}: {ready.observedAt || (my ? "မသိ" : "Unknown")}</p>
      {ready.view.controllers.map((controller) => <div className="stack" key={controller.family}>
        <h3>{controller.family === "modern" ? "Modern WiFi CAPsMAN" : "Legacy CAPsMAN"}</h3>
        {!controller.inventory_available ? <Note kind="warn">{my ? "Controller စာရင်းဖတ်မရပါ" : "CAP inventory unavailable — AP count unknown."}</Note> :
          <p className="soft small">{controller.caps?.length ?? 0} {my ? "CAP စာရင်းမှတ်တမ်း" : "CAP records reported"} · {controller.radio_inventory_available
            ? (my ? "Radio စာရင်းရရှိ" : "Radio inventory readable")
            : (my ? "Radio စာရင်းဖတ်မရ" : "Radio inventory unavailable")}</p>}
        {controller.inventory_available && (controller.caps ?? []).map((cap, index) =>
          <div className="note stack" key={cap.identity || String(index)}>
            <b>{cap.identity || (my ? "CAP အမည်မသိ" : "Unnamed CAP")}</b>
            <p className="soft small">{cap.address || "—"} · {cap.state_known ? cap.state : (my ? "အခြေအနေမသိ" : "State unknown")}</p>
            <p className="soft small">{my ? "အွန်လိုင်းအတည်ပြုချက်မရှိ · SSID အတည်ပြုချက်မရှိ" : "Online unverified · Broadcast SSID unverified"}</p>
          </div>)}
      </div>)}
      <h3>{my ? "သိမ်းဆည်းထားသော SSID Profile များ" : "Configured SSID profiles"}</h3>
      {!ready.view.profiles.length && <p className="soft small">{my ? "ဖတ်နိုင်သော Profile မရှိပါ၊ သို့မဟုတ် မရရှိပါ" : "No readable profile records; this does not prove there are none."}</p>}
      {ready.view.profiles.map((p, i) => <p className="small" key={p.family + (p.name || i)}>
        <b>{p.name || "—"}</b> · {p.ssid || (my ? "SSID မသိ" : "SSID unknown")} · {p.family} · {my ? "ထုတ်လွှင့်မှု မအတည်ပြုနိုင်" : "Broadcast unverified"}
      </p>)}
      <Note kind="warn">{my
        ? "ယခု စာရင်းဖတ်ရန်သာ ဖြစ်ပြီး SSID ပြောင်းခြင်းကို မဖွင့်ထားပါ။"
        : "Read-only evidence. SSID synchronization and AP configuration are not enabled."}</Note>
    </>}
  </section>;
}
