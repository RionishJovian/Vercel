import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { errorText } from "../../lib/errors";

type Controller = { id: string; base_url: string; omada_id: string; site_id: string; created_at: string };
type Inventory = { source: string; devices: Array<{
  mac: string; name?: string; model?: string; firmware?: string; device_type?: string; status: number;
}>; radio_optimization_supported: boolean; reason?: string };

export function OmadaEnrollment() {
  const { lang } = useI18n();
  const my = lang === "my";
  const origins = useSWR<{ allowed_origins: string[] }>("/ap-optimization/omada/origins", fetcher, { shouldRetryOnError: false });
  const enabled = !!origins.data?.allowed_origins?.length;
  const registrations = useSWR<{ controllers: Controller[] }>(enabled ? "/ap-optimization/omada" : null, fetcher, { shouldRetryOnError: false });
  const [origin, setOrigin] = useState("");
  const [omadaID, setOmadaID] = useState("");
  const [siteID, setSiteID] = useState("");
  const [clientID, setClientID] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [inventory, setInventory] = useState<{ controller: string; data: Inventory } | null>(null);
  const allowedOrigins = origins.data?.allowed_origins || [];

  async function enroll(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (busy || !enabled || !clientSecret || !clientID || !siteID || !omadaID) return;
    setBusy("enroll"); setError(""); setMessage("");
    try {
      await api.post("/ap-optimization/omada", {
        base_url: origin || allowedOrigins[0],
        omada_id: omadaID,
        site_id: siteID,
        client_id: clientID,
        client_secret: clientSecret,
      });
      setClientSecret(""); setClientID("");
      setMessage(my
        ? "Controller ကို ဖတ်ရန်သာ မှတ်ပုံတင်ပြီးပါပြီ။ ချိတ်ဆက်မှုစမ်းသပ်မှု မလုပ်ရသေးပါ။"
        : "Controller credentials encrypted and saved. Connection is not verified until inventory succeeds.");
      await registrations.mutate();
    } catch (e) {
      if (isUnauthorized(e)) throw e;
      setError(errorText(e, my ? "Controller ကို မသိမ်းနိုင်ပါ" : "Could not register controller."));
    } finally { setBusy(""); }
  }
  async function readInventory(row: Controller) {
    if (busy) return;
    setBusy(row.id); setError(""); setMessage(""); setInventory(null);
    try {
      const data = await api.get<Inventory>("/ap-optimization/omada/" + encodeURIComponent(row.id) + "/devices");
      setInventory({ controller: row.id, data });
    } catch (e) {
      if (isUnauthorized(e)) throw e;
      setError(errorText(e, my ? "Controller မှ စက်စာရင်း မဖတ်နိုင်ပါ" : "Controller inventory unavailable; verify site privileges and secure network access."));
    } finally { setBusy(""); }
  }
  async function revoke(row: Controller) {
    if (busy || !window.confirm(my ? "Controller ချိတ်ဆက်မှုနှင့် သိမ်းထားသော secret ကို ဖျက်မည်လား?" : "Remove this controller and its stored API secret?")) return;
    setBusy(row.id); setError(""); setMessage("");
    try {
      await api.del("/ap-optimization/omada/" + encodeURIComponent(row.id));
      setInventory(null);
      await registrations.mutate();
      setMessage(my ? "Controller ချိတ်ဆက်မှုကို ဖျက်ပြီးပါပြီ။" : "Controller enrollment revoked.");
    } catch (e) {
      if (isUnauthorized(e)) throw e;
      setError(errorText(e, "Controller could not be removed."));
    } finally { setBusy(""); }
  }
  return <section className="stack">
    <h3>{my ? "TP-Link Omada ချိတ်ဆက်မှု" : "TP-Link Omada controller (read-only)"}</h3>
    <p className="soft small">{my
      ? "စက်စာရင်းကိုသာ ဖတ်နိုင်သည်။ AP radio ချန်နယ်ပြင်ခြင်း သို့မဟုတ် အလိုအလျောက်ထိန်းချုပ်ခြင်းကို မဖွင့်ထားပါ။"
      : "Inventory only. Register an existing Omada Open API client; this does not authorize changes to radio channels, SSIDs or controller settings."}</p>
    {!enabled && !origins.isLoading && <Note kind="warn">{my
      ? "ဆာဗာတွင် controller ကန့်သတ်ချက်များ မဖွင့်ရသေးပါ။"
      : "Controller enrollment is disabled until the server operator configures approved HTTPS origins, management CIDRs and an encryption key."}</Note>}
    {error && <Note kind="bad">{error}</Note>}
    {message && <Note>{message}</Note>}
    {enabled && <form className="stack" onSubmit={enroll}>
      <label className="field"><span>{my ? "Controller လိပ်စာ" : "Approved controller address"}</span>
        <select className="input" value={origin || allowedOrigins[0]} onChange={e => setOrigin(e.target.value)} disabled={!!busy}>
          {allowedOrigins.map(a => <option value={a} key={a}>{a}</option>)}
        </select></label>
      <label className="field"><span>Omada ID</span>
        <input className="input" autoComplete="off" required maxLength={128} value={omadaID} onChange={e => setOmadaID(e.target.value)} disabled={!!busy}/></label>
      <label className="field"><span>Site ID</span>
        <input className="input" autoComplete="off" required maxLength={128} value={siteID} onChange={e => setSiteID(e.target.value)} disabled={!!busy}/></label>
      <label className="field"><span>Client ID</span>
        <input className="input" autoComplete="off" required maxLength={256} value={clientID} onChange={e => setClientID(e.target.value)} disabled={!!busy}/></label>
      <label className="field"><span>Client Secret</span>
        <input className="input" type="password" autoComplete="new-password" required minLength={16} value={clientSecret}
          onChange={e => setClientSecret(e.target.value)} disabled={!!busy}/></label>
      <button className="btn" type="submit" disabled={!!busy}>{my ? "လျှို့ဝှက်ကုဒ်ကို လုံခြုံစွာ သိမ်းဆည်း" : "Save encrypted read-only connection"}</button>
    </form>}
    {Array.isArray(registrations.data?.controllers) && registrations.data.controllers.map(row =>
      <div className="glass pad stack" key={row.id}>
        <b>{row.base_url}</b>
        <p className="soft small">Omada ID: {row.omada_id} · Site: {row.site_id}</p>
        <div className="row">
          <button type="button" className="pill" onClick={() => void readInventory(row)} disabled={!!busy}>{my ? "စက်စာရင်း ဖတ်မည်" : "Read inventory"}</button>
          <button type="button" className="pill danger" onClick={() => void revoke(row)} disabled={!!busy}>{my ? "ဖယ်ရှားမည်" : "Remove"}</button>
        </div>
        {inventory?.controller === row.id && <div className="stack">
          <p className="small">{my ? "တွေ့ရှိသော စက်" : "Controller-reported devices"}: <b>{inventory.data.devices.length}</b></p>
          <Note kind="warn">{my ? "ဤစက်စာရင်းသည် RF အကောင်းဆုံးချန်နယ် သို့မဟုတ် ချိတ်ဆက်ကြိုးပုံစံကို မအတည်ပြုနိုင်သေးပါ။"
            : "Inventory does not establish RF measurements, safe channel settings, physical switch links or write access."}</Note>
          {inventory.data.devices.map((d, idx) => <p className="small" key={d.mac + idx}>
            {d.name || d.mac} · {[d.device_type, d.model, d.firmware].filter(Boolean).join(" · ")}
          </p>)}
        </div>}
      </div>
    )}
  </section>;
}
