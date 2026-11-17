import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { asArray, type Router } from "../../lib/types";
import { useI18n } from "../../i18n";
import { errorText } from "../../lib/errors";
import { Field, Note } from "../../components/ui";

interface VIPRecord { id: string; device_id: string; max_devices: number; status: string; code?: string }
interface VIPResult { id?: string; state?: string; code?: string; message?: string }
const KEY = "mm_vip_pending";
type Pending = { request_id: string; device_id: string; max_devices: number; confirm_unlimited: true; id?: string };
function readPending(): Pending | null {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return null;
    const value = JSON.parse(raw) as Pending;
    return value && typeof value.request_id === "string" && typeof value.device_id === "string" ? value : null;
  } catch { return null; }
}
function persist(p: Pending | null) {
  try { if (p) localStorage.setItem(KEY, JSON.stringify(p)); else localStorage.removeItem(KEY); }
  catch { /* may be disabled */ }
}

export function VIP() {
  const { lang } = useI18n();
  const my = lang === "my";
  const routers = useSWR<{routers?: Router[]}>("/routers", fetcher);
  const vip = useSWR<{ codes?: VIPRecord[] }>("/vip-wifi", fetcher, { shouldRetryOnError: false });
  const gateways = asArray<Router>(routers.data?.routers).filter(d => d.deployment_layer === "MAIN_GATEWAY");
  const [selected, setSelected] = useState("");
  const router = gateways.find(g => g.device_id === selected) ?? gateways[0];
  const [max, setMax] = useState(2);
  const [confirmed, setConfirmed] = useState(false);
  const [pending, setPending] = useState(readPending);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  async function result(r: VIPResult, old: Pending) {
    if (r.state === "ready") {
      persist(null); setPending(null);
      setMessage(my ? "VIP Wi-Fi ကို Router တွင် အတည်ပြုပြီးပါပြီ။" : "VIP login installed and verified on the gateway.");
    } else {
      const next = { ...old, id: r.id || old.id };
      persist(next); setPending(next);
      setMessage(my ? "Router အတည်ပြုချက် မပြီးသေးပါ။ တူညီသော request ကို ပြန်စမ်းပါ။" : "Router result is not yet verified. Retry the saved identity; do not issue another code.");
    }
    await vip.mutate();
  }

  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!router || !confirmed || busy || pending) return;
    const old: Pending = { request_id: crypto.randomUUID(), device_id: router.device_id, max_devices: max, confirm_unlimited: true };
    persist(old); setPending(old); setError(""); setMessage(""); setBusy(true);
    try { await result(await api.post<VIPResult>("/vip-wifi", old), old); }
    catch (e) { if (isUnauthorized(e)) throw e; setError(errorText(e, my ? "VIP အသုံးပြုခွင့် မသေချာပါ" : "VIP provisioning is uncertain. Retry the saved request.")); }
    finally { setBusy(false); }
  }
  async function retry() {
    if (!pending || busy) return;
    setBusy(true); setError("");
    try {
      const endpoint = pending.id ? "/vip-wifi/" + encodeURIComponent(pending.id) + "/retry" : "/vip-wifi";
      await result(await api.post<VIPResult>(endpoint, pending.id ? {} : pending), pending);
    } catch (e) { if (isUnauthorized(e)) throw e; setError(errorText(e, "VIP verification is still unavailable.")); }
    finally { setBusy(false); }
  }
  async function revoke(id: string) {
    if (busy || !window.confirm(my ? "VIP ကို ပိတ်ပြီး လက်ရှိအသုံးပြုသူအားလုံးကို ဖြုတ်မည်လား?" : "Revoke this VIP code and disconnect every session using it?")) return;
    setBusy(true); setError(""); setMessage("");
    try {
      const r = await api.post<VIPResult>("/vip-wifi/" + encodeURIComponent(id) + "/revoke", {});
      setMessage(r.state === "revoked" ? (my ? "VIP ကို ပိတ်ပြီးပါပြီ" : "VIP revoked and sessions disconnected.") :
        (my ? "Router မှ အတည်ပြုချက် မရသေးပါ။ ထပ်မံစမ်းပါ။" : "Router revocation is pending; retry with the same code."));
      await vip.mutate();
    } catch (e) { if (isUnauthorized(e)) throw e; setError(errorText(e, "VIP revocation not verified.")); }
    finally { setBusy(false); }
  }

  return <div className="workspace-grid">
    <Note kind="warn">{my
      ? "သတိပြုရန် — VIP ကုဒ်တစ်ခုကို စက် ၂ မှ ၈ လုံး တစ်ပြိုင်နက် အသုံးပြုနိုင်ပြီး အချိန်၊ ဒေတာ ကန့်သတ်ချက်မရှိပါ။ ပိုင်ရှင်ခွင့်ပြုချက်ရှိသော ဝန်ထမ်းများအတွက်သာဖြစ်သည်။"
      : "Staff VIP only: one code can connect 2–8 devices simultaneously, with no time or data quota. Share it only with trusted staff. Standard customer vouchers remain single-device."}</Note>
    {vip.error && <Note kind="warn">{my
      ? "VIP backend ကို မဖွင့်ထားသေးနိုင်ပါသည်။ ထည့်သွင်းပြီးမှ အသုံးပြုနိုင်မည်။"
      : "VIP backend is unavailable or not enabled. No RouterOS changes can be issued until the server-side gate is enabled."}</Note>}
    {error && <Note kind="bad">{error}</Note>}
    {message && <Note>{message}</Note>}
    {pending && <Note kind="warn">{my
      ? "အတည်မပြုရသေးသော VIP request ရှိနေသည်။ နောက်ထပ်ကုဒ် အသစ်မထုတ်ပါနှင့်။"
      : "A saved VIP request remains pending. Retry its exact identity rather than creating another credential."}{" "}
      <button className="pill" type="button" disabled={busy || !!vip.error} onClick={() => void retry()}>{my ? "ထပ်မံစစ်ဆေး" : "Retry verification"}</button>
    </Note>}
    <form className="glass pad stack workspace-rail" onSubmit={create}>
      <h3>{my ? "ဝန်ထမ်း VIP ဖန်တီးရန်" : "Create staff VIP Wi-Fi code"}</h3>
      <Field label={my ? "Gateway" : "Gateway"} htmlFor="vip-gateway">
        <select id="vip-gateway" className="input" value={router?.device_id || ""} onChange={e => setSelected(e.target.value)} disabled={busy || !!pending} required>
          {gateways.map(g => <option key={g.device_id} value={g.device_id}>{g.hardware_name || g.device_id}</option>)}
        </select>
      </Field>
      <Field label={my ? "တစ်ပြိုင်နက်အသုံးပြုနိုင်သည့် စက်အရေအတွက်" : "Maximum simultaneous devices"} htmlFor="vip-max">
        <select id="vip-max" name="max" className="input" value={max} onChange={e => setMax(Number(e.target.value))} disabled={busy || !!pending}>
          {[2, 3, 4, 5, 6, 7, 8].map(n => <option key={n} value={n}>{n}</option>)}
        </select>
      </Field>
      <label className="row"><input type="checkbox" checked={confirmed} onChange={e => setConfirmed(e.target.checked)} disabled={busy || !!pending} />
        <span>{my ? "အချိန်နှင့် ဒေတာ မကန့်သတ်ထားကြောင်း သဘောတူပါသည်" : "I approve unlimited time and data for this staff VIP code."}</span>
      </label>
      <button type="submit" className="btn" disabled={!router || !confirmed || !!pending || busy || !!vip.error}>
        {busy ? (my ? "စစ်ဆေးနေသည်" : "Checking gateway…") : (my ? "VIP ကုဒ် ထုတ်မည်" : "Create and verify VIP")}
      </button>
    </form>
    <section className="glass pad stack workspace-main">
      <h3>{my ? "ဝန်ထမ်း VIP ကုဒ်များ" : "Staff VIP codes"}</h3>
      {vip.data && !asArray<VIPRecord>(vip.data.codes).length && <p className="soft small">{my ? "VIP မရှိသေးပါ" : "No VIP codes yet."}</p>}
      {asArray<VIPRecord>(vip.data?.codes).map(v => <div className="row" key={v.id}>
        <div className="stack" style={{ gap: 3 }}>
          <b className="num code">{v.code || (my ? "အတည်ပြုရန် လိုအပ်" : "Pending verification")}</b>
          <span className="soft small">{v.max_devices} {my ? "စက်" : "devices"} · {v.status}</span>
        </div>
        {v.status !== "revoked" && <button className="pill" type="button" disabled={busy} onClick={() => void revoke(v.id)}>{my ? "ပိတ်မည်" : "Revoke"}</button>}
      </div>)}
    </section>
  </div>;
}
