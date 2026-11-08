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

interface SSIDReviewResponse {
  gateway_id: string;
  apply_available: boolean;
  observed_at?: string;
  preview?: {
    site_id: string;
    ssid: string;
    revision: string;
    review_hash: string;
    write_enabled: boolean;
    eligible_count: number;
    blocked_count: number;
    impacts: Array<{ device_id: string; status: string; reason: string; target_ssid: string }>;
  };
}

export function SSIDServerPreview({ id, siteId, canReview }: { id: string; siteId?: string | null; canReview: boolean }) {
  const { lang } = useI18n();
  const my = lang === "my";
  const [ssid, setSSID] = useState("");
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState("");
  const [result, setResult] = useState<SSIDReviewResponse | null>(null);
  // A UTF-8 SSID is limited to 32 bytes, not 32 characters.
  const valid = ssid.length > 0 && ssid.trim() === ssid &&
    new TextEncoder().encode(ssid).length <= 32 && !/[\r\n\0]/.test(ssid);
  async function preview() {
    if (!valid || !siteId || !canReview || busy) return;
    setBusy(true); setFailure(""); setResult(null);
    try {
      // Ephemeral request identifier; not a persisted site revision or apply token.
      const revision = "read-only-" + Date.now().toString(36);
      const response = await api.post<SSIDReviewResponse>(`/routers/${encodeURIComponent(id)}/ssid/preview`, {
        site_id: siteId, revision, ssid,
      });
      const p = response.preview;
      if (response.gateway_id !== id || response.apply_available !== false ||
          !p || p.site_id !== siteId || p.ssid !== ssid ||
          p.write_enabled !== false || !Array.isArray(p.impacts) ||
          p.impacts.some(item => item.status !== "blocked") ||
          p.eligible_count !== 0 || p.blocked_count !== p.impacts.length) {
        throw new Error("Server preview safety contract rejected");
      }
      setResult(response);
    } catch (e) {
      if (isUnauthorized(e)) throw e;
      setFailure(errorText(e, my ? "SSID အကြိုကြည့်ရှုမှု မရနိုင်ပါ" : "Server SSID preview unavailable"));
    } finally { setBusy(false); }
  }
  return <div className="stack" aria-label={my ? "SSID ပြောင်းလဲမှု အကြိုကြည့်ရန်" : "SSID change impact preview"}>
    <h3>{my ? "SSID ပြောင်းလဲမှု အကြိုကြည့်ရန်" : "Preview a site-wide SSID change"}</h3>
    <p className="soft small">{my
      ? "ပိုင်ရှင်အတွက် အကြိုကြည့်ရှုမှုသာ ဖြစ်သည်။ AP များကို ပြောင်းလဲမည်မဟုတ်ပါ။"
      : "Owner-only server review. CAPsMAN sightings do not prove AP manageability; no changes can be applied."}</p>
    {!siteId && <Note kind="warn">{my ? "ဤ Gateway ကို Site နှင့် ချိတ်ဆက်ရန် လိုအပ်သည်။" : "Assign this gateway to a site before reviewing an SSID change."}</Note>}
    {!canReview && <Note kind="warn">{my ? "SSID ပြောင်းလဲမှုကို ပိုင်ရှင်သာ ကြည့်ရှုနိုင်သည်။" : "SSID change review requires owner permission."}</Note>}
    <label className="stack small">
      <span>{my ? "Wi-Fi အမည်အသစ်" : "Proposed Wi-Fi name"}</span>
      <input className="input" aria-label="Proposed Wi-Fi name" maxLength={32} value={ssid}
        onChange={e => { setSSID(e.target.value); setResult(null); setFailure(""); }} placeholder="Shop Guest" />
    </label>
    <button className="pill" type="button" onClick={() => void preview()}
      disabled={!valid || !siteId || !canReview || busy}>
      {busy ? (my ? "စစ်ဆေးနေသည်…" : "Checking…") : (my ? "ဆာဗာတွင် ကြိုတင်စစ်ဆေးရန်" : "Review impact on server (no changes)")}
    </button>
    {failure && <Note kind="warn">{failure}</Note>}
    {result?.preview && <>
      <p className="soft small" role="status">{my ? "ပြောင်းလဲနိုင်ကြောင်း အတည်ပြုထားသော AP" : "APs eligible for change"}: 0 · {my ? "ပိတ်ထားသော AP" : "Blocked APs"}: {result.preview.blocked_count}</p>
      <p className="soft small">{my ? "လေ့လာချိန်" : "Observed at"}: {result.observed_at || "—"} · {my ? "အကြိုကြည့်ရှုမှု ID" : "Review ID"}: {result.preview.review_hash.slice(0, 12)}</p>
      {result.preview.impacts.map((impact, i) => <div className="note warn stack" key={impact.device_id + i}>
        <b>{impact.device_id}</b>
        <p className="small">{my ? "အဆိုပြု SSID" : "Proposed SSID"}: {impact.target_ssid}</p>
        <p className="soft small">{my ? "ပိတ်ထားသည် — စီမံခန့်ခွဲခွင့် မအတည်ပြုရသေးပါ" : "Blocked — unverified management capability"}</p>
      </div>)}
      {!result.preview.impacts.length && <Note kind="warn">{my ? "CAP မှတ်တမ်း မရှိပါ သို့မဟုတ် ဖတ်မရပါ" : "No assessable CAP records; this does not mean the site has no APs."}</Note>}
      <Note kind="warn">{my ? "ဖတ်ရန်သာ ဖြစ်ပြီး SSID ပြောင်းလဲမှုများကို ပိတ်ထားသည်။" : "Read-only review. SSID changes remain disabled."}</Note>
    </>}
  </div>;
}

export function CAPsMANInventoryPanel({ id, siteId, canReview }: { id: string; siteId?: string | null; canReview: boolean }) {
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
      <SSIDServerPreview key={id} id={id} siteId={siteId} canReview={canReview} />
      <Note kind="warn">{my
        ? "ယခု စာရင်းဖတ်ရန်သာ ဖြစ်ပြီး SSID ပြောင်းခြင်းကို မဖွင့်ထားပါ။"
        : "Read-only evidence. SSID synchronization and AP configuration are not enabled."}</Note>
    </>}
  </section>;
}
