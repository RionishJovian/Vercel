import { useEffect, useMemo, useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, ApiError, fetcher, isUnauthorized } from "../../lib/api";
import { asArray, mmk, type Me } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";

export interface Pass { state?: string; kind?: string; authorized_login?: boolean; activated_at?: string; expires_at?: string; transition_expires_at?: string }
interface Renewal { id?: string; plan: string; status: string; created_at?: string; amount_mmk?: number; bank_reference?: string; idempotency_key?: string }
interface Bank { id: string; bank_name?: string; holder_name?: string; account_number?: string }
interface Options { available?: boolean; plans?: Record<string, { amount_mmk?: number }>; bank_accounts?: Bank[] }
interface Draft { plan: "monthly" | "annual"; bank_account_id: string; bank_reference: string; expected_amount_mmk?: number; idempotency_key: string }
const when = (v?: string) => (v ? new Date(v).toLocaleString() : "");
const price = (v: unknown, none: string) => (Number.isInteger(v) && (v as number) > 0 ? `${mmk(v as number)} MMK` : none);
const errText = (e: unknown, f: string) => { const m = e instanceof Error ? e.message : ""; return m && !/^http-\d+$/.test(m) && m !== "network-unreachable" ? m : f; };

/** Monthly/annual pass checkout. The request ID is stored BEFORE sending, so a lost response is retried with the same
 *  details and never asks the client to transfer money twice. */
export function AccountPass({ me, pass, onRefresh }: { me: Me; pass: Pass; onRefresh: () => void }) {
  const { t } = useI18n();
  const hist = useSWR<{ requests?: Renewal[] }>("/me/app-pass/renewals", fetcher, { shouldRetryOnError: false });
  const opts = useSWR<Options>("/me/app-pass/checkout-options", fetcher, { shouldRetryOnError: false });
  const requests = hist.error ? null : hist.data ? asArray<Renewal>(hist.data.requests) : undefined;
  const options = opts.error ? null : opts.data;
  const banks = asArray<Bank>(options?.bank_accounts);
  const pending = requests?.find((r) => r.status === "pending");
  const exempt = pass.state === "platform_exempt";
  const key = me.user?.user_id && me.business?.business_id ? `mm-account-pass-checkout:${me.business.business_id}:${me.user.user_id}` : null;
  const [draft, setDraft] = useState<Draft | null>(null), [storageOK, setStorageOK] = useState(!!key);
  const [form, setForm] = useState<{ open: boolean; plan: "monthly" | "annual"; bank: string; ref: string; confirmed: boolean }>({ open: false, plan: "monthly", bank: "", ref: "", confirmed: false });
  const [status, setStatus] = useState(""), [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!key) return;
    try {
      const raw = sessionStorage.getItem(key);
      if (raw) { const s = JSON.parse(raw) as Draft; if (["monthly", "annual"].includes(s.plan) && typeof s.idempotency_key === "string") setDraft(s); else setStorageOK(false); }
    } catch { setStorageOK(false); }
  }, [key]);
  useEffect(() => { // a saved draft that the server already recorded is finished
    if (draft && requests?.some((r) => r.idempotency_key === draft.idempotency_key) && key) { try { sessionStorage.removeItem(key); } catch { /* ignore */ } setDraft(null); }
  }, [draft, requests, key]);
  useEffect(() => { if (banks.length && !form.bank) setForm((f) => ({ ...f, bank: banks[0].id })); }, [banks, form.bank]);

  const planAmount = (p: string) => options?.plans?.[p]?.amount_mmk;
  const unavailable = useMemo(() => exempt ? t("ap.u.exempt") : pass.authorized_login === false ? t("ap.u.login") : requests === null ? t("ap.u.history") : pending ? t("ap.u.pending") : !options ? t("ap.u.options") : !options.available || !banks.length ? t("ap.u.noBank") : !storageOK ? t("ap.u.storage") : "", [exempt, pass.authorized_login, requests, pending, options, banks.length, storageOK, t]);
  const disabled = !!unavailable;
  const bank = banks.find((b) => b.id === (draft?.bank_account_id ?? form.bank));
  const plan = draft?.plan ?? form.plan;
  if (hist.isLoading || opts.isLoading) return <Skeleton h={300} />;

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (busy || disabled || !key) return;
    let d = draft;
    if (!d) {
      const ref = form.ref.trim();
      if (!form.confirmed || !ref || ref.length > 120 || !banks.some((b) => b.id === form.bank)) { setStatus(t("ap.selectBank")); return; }
      d = { plan: form.plan, bank_account_id: form.bank, bank_reference: ref, expected_amount_mmk: planAmount(form.plan), idempotency_key: crypto.randomUUID() };
      try { sessionStorage.setItem(key, JSON.stringify(d)); } catch { setStatus(t("ap.noRetryId")); return; }
      setDraft(d);
    }
    setBusy(true); setStatus(t("ap.submitting"));
    try {
      await api.post("/me/app-pass/renewals", d);
      try { sessionStorage.removeItem(key); } catch { /* ignore */ }
      setDraft(null); setForm((f) => ({ ...f, open: false, ref: "", confirmed: false })); setStatus(""); void hist.mutate(); onRefresh();
    } catch (err) {
      if (isUnauthorized(err)) throw err;
      const detail = errText(err, t("ap.notConfirmed"));
      if (err instanceof ApiError && [400, 403, 409].includes(err.status)) { try { sessionStorage.removeItem(key); } catch { /* ignore */ } setDraft(null); void hist.mutate(); onRefresh(); }
      setStatus(detail);
    }
    setBusy(false);
  }

  const card = (k: "monthly" | "annual") => (
    <article className={"glass pad stack plan-card" + (k === "annual" ? " featured" : "")} key={k}>
      <span className="soft small">{t("ap." + k)}</span><b className="num price">{price(planAmount(k), t("ap.notPublished"))}</b><span className="soft small">{t("ap." + k + "Len")}</span>
      <button className={k === "annual" ? "btn" : "btn quiet"} type="button" disabled={disabled || !!draft || !Number.isInteger(planAmount(k))} onClick={() => { setForm((f) => ({ ...f, open: true, plan: k })); }}>{exempt ? t("ap.exempt") : t("ap.choose." + k)}</button>
    </article>
  );
  return (
    <>
      <section className="glass pad stack">
        <h2>{t("ap.title")}</h2>
        <p><b>{t("ap.s." + (pass.state ?? "unknown"))}</b></p>
        {exempt ? <p className="soft small">{t("ap.noPassNeeded")}</p> : (
          <p className="soft small">{t("ap.activation")}: {pass.activated_at ? when(pass.activated_at) : t("ap.notActivated")}<br />{t("ap.expiry")}: {pass.expires_at ? when(pass.expires_at) : t("ap.notRecorded")}{pass.transition_expires_at ? <><br />{t("ap.transitionEnds")}: {when(pass.transition_expires_at)}</> : null}</p>
        )}
        <p className="soft small">{t("ap.features")}</p><p className="soft small">{t("ap.expiryNote")}</p>
        <button className="pill" type="button" onClick={onRefresh}>{t("net.refresh")}</button>
        {unavailable && <p className="note" role="status">{unavailable}</p>}
      </section>
      <div className="store-grid">{card("monthly")}{card("annual")}</div>
      {!exempt && pass.authorized_login !== false && (form.open || draft) && (
        <form className="glass pad stack" onSubmit={submit}>
          <h3>{t("ap.submitRef")}</h3><p className="soft small">{t("ap.transferHelp")}</p>
          <label className="field"><span>{t("ap.plan")}</span><select className="input" value={plan} disabled={!!draft || busy} onChange={(e) => setForm((f) => ({ ...f, plan: e.target.value as "monthly" | "annual" }))}><option value="monthly">{t("ap.monthly")}</option><option value="annual">{t("ap.annual")}</option></select></label>
          <p className="num">{t("ap.amount")}: {price(draft ? draft.expected_amount_mmk : planAmount(plan), t("ap.notPublished"))}</p>
          <label className="field"><span>{t("ap.account")}</span><select className="input" value={draft?.bank_account_id ?? form.bank} disabled={!!draft || busy} onChange={(e) => setForm((f) => ({ ...f, bank: e.target.value }))}>{banks.map((b) => <option key={b.id} value={b.id}>{b.bank_name} · {b.holder_name}</option>)}</select></label>
          <p className="small num">{bank ? `${bank.bank_name} · ${bank.account_number} · ${bank.holder_name}` : t("ap.noAccount")}</p>
          <label className="field"><span>{t("ap.reference")}</span><input className="input" required maxLength={120} autoComplete="off" placeholder={t("ap.referenceHint")} value={draft?.bank_reference ?? form.ref} disabled={!!draft || busy} onChange={(e) => setForm((f) => ({ ...f, ref: e.target.value }))} /></label>
          <label className="check"><span>{t("ap.iTransferred")}</span><input type="checkbox" required checked={!!draft || form.confirmed} disabled={!!draft || busy} onChange={(e) => setForm((f) => ({ ...f, confirmed: e.target.checked }))} /></label>
          {draft && <Note kind="warn">{t("ap.retryNote")}</Note>}
          {status && <p className="note" role="status">{status}</p>}
          <button className="btn" type="submit" disabled={busy}>{draft ? t("ap.retry") : t("ap.submit")}</button>
        </form>
      )}
      <section className="glass pad stack">
        <h2>{t("ap.requests")}</h2>
        {requests === null ? <Note kind="bad">{t("ap.reqFail")}</Note> : !requests?.length ? <p className="soft small">{t("ap.noRequests")}</p> : requests.map((r, i) => (
          <article className="stack rq" key={r.id ?? i}>
            <b>{t("ap." + (r.plan === "annual" ? "annual" : "monthly"))} · {t("st.status." + (["pending", "approved", "rejected"].includes(r.status) ? r.status : "pending"))}</b>
            <p className="soft small">{when(r.created_at)}{Number.isInteger(r.amount_mmk) ? ` · ${price(r.amount_mmk, "")} · ${t("ap.bankRef")}: ${r.bank_reference ?? ""}` : ` · ${t("ap.activationReq")}`}</p>
            {r.status === "pending" && <p className="small">{t("ap.dontPayAgain")}</p>}
            {r.status === "rejected" && r.amount_mmk ? <p className="small">{t("ap.rejectedNote")}</p> : null}
          </article>
        ))}
      </section>
    </>
  );
}
