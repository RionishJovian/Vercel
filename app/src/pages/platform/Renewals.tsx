import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { asArray, mmk } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";

interface Req { id: string; email?: string; plan?: string; created_at?: string; amount_mmk?: number; bank_reference?: string; bank_snapshot?: { bank_name?: string; account_number?: string; holder_name?: string } }
const errText = (e: unknown, f: string) => { const m = e instanceof Error ? e.message : ""; return m && !/^http-\d+$/.test(m) ? m : f; };

function Item({ r, onDone }: { r: Req; onDone: () => void }) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(false), [msg, setMsg] = useState("");
  const paid = !!r.amount_mmk;
  async function decide(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const approve = (e.nativeEvent as SubmitEvent).submitter?.getAttribute("data-decision") === "approve";
    setBusy(true); setMsg(t("pr.recording"));
    try { await api.post(`/platform/account-renewals/${encodeURIComponent(r.id)}/decision`, { approve, ...(paid && approve ? { payment_verified: true } : {}) }); onDone(); }
    catch (err) { if (isUnauthorized(err)) throw err; setMsg(errText(err, t("pr.notConfirmed"))); setBusy(false); }
  }
  return (
    <form className="glass pad stack" onSubmit={decide}>
      <b>{r.email}</b><p className="soft small">{r.plan} · {t("pr.requested")} {r.created_at ? new Date(r.created_at).toLocaleString() : ""}</p>
      {paid ? <p className="small">{t("ap.amount")}: {mmk(r.amount_mmk!)} MMK<br />{t("ap.bankRef")}: {r.bank_reference}<br />{t("pr.payee")}: {r.bank_snapshot?.bank_name} · {r.bank_snapshot?.account_number} · {r.bank_snapshot?.holder_name}</p> : <p className="small">{t("pr.noEvidence")}</p>}
      <label className="check"><span>{paid ? t("pr.checkedBank") : t("pr.reviewedReq")}</span><input type="checkbox" required /></label>
      <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}><button className="btn" type="submit" data-decision="approve" disabled={busy}>{t("pa.approve")}</button><button className="btn quiet" type="submit" data-decision="reject" disabled={busy}>{t("pr.reject")}</button></div>
      {msg && <p className="soft small" role="status">{msg}</p>}
    </form>
  );
}

export function Renewals() {
  const { t } = useI18n();
  const { data, error, isLoading, mutate } = useSWR<{ requests?: Req[] }>("/platform/account-renewals", fetcher, { shouldRetryOnError: false });
  if (isLoading) return <Skeleton h={200} />;
  if (error || !Array.isArray(data?.requests)) return <><Note kind="bad">{t("pr.unavailable")}</Note><button className="pill" type="button" onClick={() => void mutate()}>{t("c.retry")}</button></>;
  const list = asArray<Req>(data?.requests);
  return (
    <>
      <p className="soft small">{t("pr.lead")}</p>
      <button className="pill" type="button" onClick={() => void mutate()}>{t("pr.refresh")}</button>
      {!list.length ? <Note>{t("pr.none")}</Note> : <div className="stack">{list.map((r) => <Item key={r.id} r={r} onDone={() => void mutate()} />)}</div>}
    </>
  );
}
