import { useState } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { errorText } from "../../lib/errors";

type APProposal = {
  device_id: string;
  mode: "manual" | "automatic";
  band: string;
  before_channel: number;
  after_channel: number;
  before_power_dbm: number;
  after_power_dbm: number;
  warnings?: string[];
};
type Review = {
  id: string;
  device_id: string;
  status: "pending" | "approved" | "rejected" | "expired" | "consumed";
  proposal: APProposal;
  proposal_hash: string;
  fingerprint: string;
  expires_at: string;
};
type Capability = {
  proposal_review: boolean;
  automatic_proposals: boolean;
  manual_proposals: boolean;
  live_apply: boolean;
  reason?: string;
};

export function APOptimizationReviews() {
  const { lang } = useI18n();
  const my = lang === "my";
  const caps = useSWR<Capability>("/ap-optimization/capabilities", fetcher, { shouldRetryOnError: false });
  const enabled = caps.data?.proposal_review === true;
  const reviews = useSWR<{ proposals: Review[] }>(enabled ? "/ap-optimization/proposals" : null, fetcher, { shouldRetryOnError: false });
  const [checked, setChecked] = useState<Record<string, boolean>>({});
  const [busy, setBusy] = useState<string | null>(null);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  async function decide(row: Review, approve: boolean) {
    if (busy || (approve && !checked[row.id])) return;
    const question = approve
      ? (my ? "ဤ AP radio အပြောင်းအလဲကို အတည်ပြုမည်လား? ယခု AP settings ကို မပြောင်းသေးပါ။" : "Approve this exact AP radio proposal? This does not change the access point yet.")
      : (my ? "ဤ အဆိုပြုချက်ကို ပယ်ချမည်လား?" : "Reject this AP radio proposal?");
    if (!window.confirm(question)) return;
    setBusy(row.id); setError(""); setMessage("");
    try {
      await api.post("/ap-optimization/proposals/" + encodeURIComponent(row.id) + "/decision", {
        approve,
        proposal_hash: row.proposal_hash,
        fingerprint: row.fingerprint,
      });
      setChecked(prev => ({ ...prev, [row.id]: false }));
      setMessage(approve
        ? (my ? "ပိုင်ရှင်အတည်ပြုချက်ကို သိမ်းပြီးပါပြီ။ AP ကို ယခု မပြောင်းသေးပါ။" : "Owner approval recorded. No AP settings were applied.")
        : (my ? "အဆိုပြုချက်ကို ပယ်ချပြီးပါပြီ။" : "Proposal rejected."));
      await reviews.mutate();
    } catch (e) {
      if (isUnauthorized(e)) throw e;
      setError(errorText(e, my ? "အတည်ပြုချက်ကို မသိမ်းဆည်းနိုင်ပါ" : "Approval was not recorded. Refresh before retrying."));
    } finally { setBusy(null); }
  }

  const items = Array.isArray(reviews.data?.proposals) ? reviews.data.proposals : [];
  return <section className="glass pad stack" aria-label={my ? "AP စနစ်ကောင်းမွန်ရေး အတည်ပြုချက်များ" : "AP optimization reviews"}>
    <h2>{my ? "AP စနစ်ကောင်းမွန်ရေး" : "AP optimization"}</h2>
    <p className="soft small">{my
      ? "အလိုအလျောက်နှင့် လက်ဖြင့် ချန်နယ်ပြင်ဆင်ရန် အဆိုပြုချက်များကို ပိုင်ရှင်က စစ်ဆေးပြီးမှသာ အတည်ပြုနိုင်ပါသည်။"
      : "Automatic and manual channel proposals require the owner's explicit review. Approval alone never changes an AP."}</p>
    {!enabled && !caps.isLoading && <Note kind="warn">{my
      ? "AP optimizer ကို ဆာဗာတွင် မဖွင့်ရသေးပါ။ အတည်ပြုနိုင်သည့် vendor API မရှိသေးသဖြင့် AP ကို ပြင်မည်မဟုတ်ပါ။"
      : "AP optimization is not enabled. No verified controller radio API or write adapter is connected; no changes can be sent to an AP."}</Note>}
    {enabled && !caps.data?.live_apply && <Note kind="warn">{my
      ? "အဆိုပြုချက်များကို စစ်ဆေးရန်သာ ဖွင့်ထားသည်။ AP radio ပြင်ဆင်မှုကို ပိတ်ထားဆဲဖြစ်သည်။"
      : "Review-only mode: live AP radio changes are still disabled."}</Note>}
    {error && <Note kind="bad">{error}</Note>}
    {message && <Note>{message}</Note>}
    {reviews.error && <Note kind="bad">{my ? "အဆိုပြုချက်များကို မဖတ်နိုင်ပါ" : "Could not load AP proposals."}</Note>}
    {enabled && !reviews.isLoading && items.length === 0 && <p className="soft small">{my
      ? "စစ်ဆေးနိုင်သည့် အဆိုပြုချက် မရှိသေးပါ။"
      : "No verified proposals yet. Unsupported devices will not generate guessed recommendations."}</p>}
    {items.map(row => {
      const p = row.proposal;
      const expired = Date.parse(row.expires_at) <= Date.now();
      const pending = row.status === "pending" && !expired;
      return <article key={row.id} className="stack" style={{ borderTop: "1px solid var(--edge-soft)", paddingTop: 12 }}>
        <div className="row"><b>{p.mode === "automatic"
          ? (my ? "အလိုအလျောက် အဆိုပြုချက်" : "Automatic recommendation")
          : (my ? "လက်ဖြင့် အဆိုပြုချက်" : "Manual recommendation")}</b>
          <span className="chip">{expired && row.status === "pending" ? "expired" : row.status}</span></div>
        <p className="small">{p.band} · {my ? "ချန်နယ်" : "Channel"} <b>{p.before_channel} → {p.after_channel}</b>
          {" · "}{my ? "ထုတ်လွှင့်အား" : "Transmit power"} {p.before_power_dbm} → {p.after_power_dbm} dBm</p>
        <p className="soft small">{my ? "သက်တမ်းကုန်ဆုံး" : "Expires"}: {new Date(row.expires_at).toLocaleString()}</p>
        {(p.warnings || []).map((warning, idx) => <p className="soft small" key={idx}>{warning}</p>)}
        {pending && <div className="stack">
          <label className="row small"><input type="checkbox" checked={!!checked[row.id]}
            disabled={!!busy} onChange={e => setChecked(old => ({ ...old, [row.id]: e.target.checked }))} />
            <span>{my ? "Wi-Fi ခဏပြတ်နိုင်ကြောင်း သိရှိပြီး ပြောင်းလဲမှုကို ပြန်လည်သုံးသပ်ပြီးပါပြီ။"
              : "I reviewed the exact change and understand it may disconnect Wi-Fi clients."}</span></label>
          <div className="row">
            <button type="button" className="pill on" disabled={!checked[row.id] || !!busy}
              onClick={() => void decide(row, true)}>{my ? "အတည်ပြု" : "Approve review"}</button>
            <button type="button" className="pill" disabled={!!busy}
              onClick={() => void decide(row, false)}>{my ? "ပယ်ချ" : "Reject"}</button>
          </div>
        </div>}
      </article>;
    })}
  </section>;
}
