import { useState } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../lib/api";
import { asArray, mmk } from "../lib/types";
import { useI18n } from "../i18n";
import { Note } from "../components/ui";
import { Skeleton } from "../components/Skeleton";

interface Summary { today: number; last7: number; last30: number; all_time: number; total: number; approved: number; pending: number; rejected: number; days: Array<{ key: string; amount: number }>; plans: Array<{ key: string; requests: number; approved: number; revenue: number }> }
interface Req { id: string; created_at?: string; decided_at?: string; plan_key?: string; amount?: number; bank_reference?: string; tx_last6?: string; status?: string }
const KEYS = ["today", "last7", "last30", "all_time", "total", "approved", "pending", "rejected"] as const;
const ok = (v: unknown) => typeof v === "number" && Number.isFinite(v) && v >= 0;
/** Totals are shown only when every figure is present; a partial answer is never presented as a total. */
export const validSummary = (c: unknown): Summary | null => { const s = c as Summary | null; return s && KEYS.every((k) => ok(s[k])) && Array.isArray(s.days) && Array.isArray(s.plans) ? s : null; };
/** A cell starting with = + - @ would run as a formula in a spreadsheet, so it is prefixed with an apostrophe. */
export const csvCell = (value: unknown) => { let text = String(value == null ? "" : value); if (/^(?:\s*[=+\-@]|\t|\r)/.test(text)) text = "'" + text; return `"${text.replaceAll('"', '""')}"`; };
const dayLabel = (k: string) => new Date(`${k}T12:00:00+06:30`).toLocaleDateString(undefined, { weekday: "short", timeZone: "Asia/Yangon" });
const errText = (e: unknown, f: string) => { const m = e instanceof Error ? e.message : ""; return m && !/^http-\d+$/.test(m) ? m : f; };

export function RevenuePayments({ canDecide }: { canDecide: boolean }) {
  const { t } = useI18n();
  const sum = useSWR<Summary>("/guest-wifi/revenue", fetcher, { shouldRetryOnError: false });
  const inbox = useSWR<{ requests?: Req[] } | Req[]>("/guest-wifi/requests", fetcher, { shouldRetryOnError: false });
  const [busy, setBusy] = useState(""), [msg, setMsg] = useState("");
  const s = validSummary(sum.data);
  const reqs = asArray<Req>(Array.isArray(inbox.data) ? inbox.data : inbox.data?.requests);
  async function decide(id: string, action: "approve" | "reject") {
    setBusy(id + action); setMsg(`${t(action === "approve" ? "rp.approving" : "rp.rejecting")}…`);
    try { await api.post(`/guest-wifi/${action}/${encodeURIComponent(id)}`, {}); setMsg(""); await Promise.all([inbox.mutate(), sum.mutate()]); }
    catch (e) { if (isUnauthorized(e)) throw e; setMsg(errText(e, t(action === "approve" ? "rp.approveFail" : "rp.rejectFail"))); }
    setBusy("");
  }
  function exportCsv() {
    const head = ["source", "submitted_at", "decided_at", "plan", "amount_mmk", "bank_reference", "status"];
    const rows = reqs.map((r) => ["guest-wifi", r.created_at, r.decided_at || "", r.plan_key || "", Number(r.amount || 0), r.bank_reference || "", r.status || ""]);
    const url = URL.createObjectURL(new Blob([[head, ...rows].map((row) => row.map(csvCell).join(",")).join("\n")], { type: "text/csv;charset=utf-8" }));
    const a = document.createElement("a"); a.href = url; a.download = `mega-magic-payment-inbox-${new Date().toISOString().slice(0, 10)}.csv`;
    document.body.appendChild(a); a.click(); a.remove(); URL.revokeObjectURL(url);
  }
  if (sum.isLoading && inbox.isLoading) return <Skeleton h={260} />;
  const max = Math.max(1, ...(s?.days ?? []).map((d) => Number(d.amount)));
  return (
    <>
      <h2>{t("rp.title")}</h2>
      {!s ? <Note kind="warn">{t("rp.noTotals")}</Note> : (
        <>
          <div className="capsules wrap">
            {([["today", s.today, "rp.today"], ["last7", s.last7, "h.last7"], ["last30", s.last30, "h.last30"], ["all", s.all_time, "rv.all"]] as const).map(([k, v, label]) => <div className="capsule" key={k}><span className="soft small">{t(label)}</span><b className="num">{mmk(v)}</b></div>)}
          </div>
          <p className="soft small">{t("rp.help")} {s.approved} {t("rp.approvedSales")}</p>
          <section className="glass pad stack"><h3>{t("rp.sevenDay")}</h3>
            <div className="bars" role="img" aria-label={t("rp.sevenDay")}>{s.days.map((d) => <div className="bar-col" key={d.key} title={`${d.key} · ${mmk(Number(d.amount))}`}><span className="small num">{Number(d.amount) ? mmk(Number(d.amount)) : "—"}</span><i style={{ height: Math.max(Number(d.amount) ? 8 : 2, Math.round((Number(d.amount) / max) * 120)) }} /><span className="soft small">{dayLabel(d.key)}</span></div>)}</div>
          </section>
          <section className="glass pad stack"><h3>{t("rp.funnel")}</h3>
            {([["rp.submitted", s.total], ["rp.approved", s.approved], ["rp.pending", s.pending], ["rp.rejected", s.rejected]] as const).map(([l, v]) => <div className="row small" key={l}><span>{t(l)}</span><b className="num">{v}</b></div>)}
            <p className="soft small">{t("rp.rate")}: {s.total ? Math.round((s.approved / s.total) * 100) : 0}% · {t("rp.manual")}</p>
          </section>
          <section className="glass pad stack"><h3>{t("rp.plans")}</h3>
            {!s.plans.length ? <p className="soft small">{t("rp.noPlans")}</p> : s.plans.map((p) => <div className="row small" key={p.key}><b>{p.key}</b><span className="soft num">{p.requests} / {p.approved} ({p.requests ? Math.round((p.approved / p.requests) * 100) : 0}%)</span><b className="num">{mmk(p.revenue)}</b></div>)}
          </section>
        </>
      )}
      <section className="stack"><div className="row"><h2>{t("rp.inbox")}</h2><button className="pill" type="button" disabled={!reqs.length} onClick={exportCsv}>{t("rp.export")}</button></div>
        <p className="soft small">{t("rp.inboxHelp")}</p>
        {inbox.error && <Note kind="warn">{t("rp.inboxDown")}</Note>}
        {msg && <p className="note" role="status">{msg}</p>}
        {!reqs.length && !inbox.error ? <Note>{t("rp.none")}</Note> : reqs.map((r) => {
          const state = String(r.status || "unknown").toLowerCase(), ref = r.bank_reference || (r.tx_last6 ? `MMQR ••••••${r.tx_last6}` : "—");
          return (
            <article className="glass pad stack" key={r.id}>
              <div className="row"><b className="num">{mmk(Number(r.amount || 0))} MMK</b><span className={"chip " + (state === "approved" ? "ok" : state === "rejected" ? "bad" : "warn")}>{t("st.status." + (["pending", "approved", "rejected"].includes(state) ? state : "pending"))}</span></div>
              <p className="soft small">{r.plan_key || "—"} · {r.created_at ? new Date(r.created_at).toLocaleString() : ""}</p><p className="small num">{ref}</p>
              {state === "pending" && (canDecide ? <div className="row" style={{ justifyContent: "flex-start" }}><button className="pill on" type="button" disabled={!!busy} onClick={() => void decide(r.id, "approve")}>{t("pf.approve")}</button><button className="pill danger" type="button" disabled={!!busy} onClick={() => void decide(r.id, "reject")}>{t("pr.reject")}</button></div> : <span className="soft small">{t("rp.readOnly")}</span>)}
            </article>
          );
        })}
      </section>
    </>
  );
}
