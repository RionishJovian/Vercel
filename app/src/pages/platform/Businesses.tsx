import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { asArray, type Me } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Field, Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";

interface Biz { business_id: string; company_name?: string; owner_email?: string; access_tier?: string; device_quota_limit?: number; is_active?: boolean; created_at?: string }
interface State { company_name?: string; access_tier?: string; is_active?: boolean; owner_email?: string; device_count?: number; device_quota_limit?: number; open_incidents?: number; purchased_extra_slots?: number; ai_token_balance?: number }
const errText = (e: unknown, f: string) => (e as { data?: { error?: string } } | null)?.data?.error || (e instanceof Error && e.message ? e.message : f);

function Row({ b, self, onChanged }: { b: Biz; self: boolean; onChanged: () => void }) {
  const { t } = useI18n();
  const [pw, setPw] = useState<string | null>(null), [st, setSt] = useState<State | null>(null), [msg, setMsg] = useState(""), [busy, setBusy] = useState(false);
  const run = async (fn: () => Promise<unknown>, ok?: string) => { setBusy(true); setMsg(""); try { await fn(); if (ok) setMsg(ok); onChanged(); } catch (e) { if (isUnauthorized(e)) throw e; setMsg(errText(e, t("pb.failed"))); } setBusy(false); };
  return (
    <article className="glass pad stack">
      <div className="row"><b>{b.company_name}</b><span className={"chip " + (b.is_active ? "ok" : "")}>{b.is_active ? t("dev.tm.active") : t("pb.suspended")}</span></div>
      <p className="soft small">{b.owner_email} · {b.access_tier} · {b.device_quota_limit} · {b.created_at ? new Date(b.created_at).toLocaleDateString() : ""}</p>
      <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
        {self ? <span className="soft small">{t("pb.yours")}</span> : <button className="pill" type="button" disabled={busy} onClick={() => void run(() => api.post(`/platform/businesses/${encodeURIComponent(b.business_id)}/${b.is_active ? "suspend" : "unsuspend"}`, {}))}>{b.is_active ? t("pb.suspend") : t("pb.unsuspend")}</button>}
        <button className="pill" type="button" onClick={() => setPw(pw === null ? "" : null)}>{t("pb.resetPw")}</button>
        <button className="pill" type="button" disabled={busy} onClick={() => void (async () => { try { setSt(await api.get<State>(`/platform/businesses/${encodeURIComponent(b.business_id)}/state`)); } catch (e) { if (isUnauthorized(e)) throw e; setMsg(errText(e, t("pb.failed"))); } })()}>{t("pb.state")}</button>
      </div>
      {pw !== null && (
        <div className="note stack"><input className="input" type="password" autoComplete="new-password" minLength={8} value={pw} onChange={(e) => setPw(e.target.value)} aria-label={t("pb.newPw")} placeholder={t("pb.newPw")} />
          <button className="pill danger" type="button" disabled={busy || pw.length < 8} onClick={() => void run(async () => { await api.post(`/platform/businesses/${encodeURIComponent(b.business_id)}/reset-owner-password`, { new_password: pw }); setPw(null); }, t("pb.pwReset"))}>{t("pb.resetPw")}</button></div>
      )}
      {st && <dl className="facts"><dt>{t("pb.owner")}</dt><dd>{st.owner_email}</dd><dt>{t("pb.devices")}</dt><dd>{st.device_count} / {st.device_quota_limit}</dd><dt>{t("pb.incidents")}</dt><dd>{st.open_incidents}</dd><dt>{t("pb.extra")}</dt><dd>{st.purchased_extra_slots}</dd><dt>AI</dt><dd>{st.ai_token_balance}</dd></dl>}
      {msg && <p className="soft small" role="status">{msg}</p>}
    </article>
  );
}

export function Businesses({ me }: { me: Me }) {
  const { t } = useI18n();
  const { data, error, isLoading, mutate } = useSWR<{ businesses?: Biz[] } | Biz[]>("/platform/businesses", fetcher, { shouldRetryOnError: false });
  const [msg, setMsg] = useState(""), [busy, setBusy] = useState(false);
  if (isLoading) return <Skeleton h={240} />;
  if (error) return <Note kind="bad">{t("c.loadFailed")}</Note>;
  const list = asArray<Biz>(Array.isArray(data) ? data : data?.businesses);
  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget, f = new FormData(form);
    const body = { company_name: String(f.get("company")).trim(), owner_email: String(f.get("email")).trim(), owner_password: String(f.get("pass")), access_tier: "SMALL_WISP", device_quota_limit: 1 };
    if (!body.company_name || !body.owner_email || body.owner_password.length < 8) { setMsg(t("pb.required")); return; }
    setBusy(true); setMsg("");
    try { await api.post("/platform/businesses", body); form.reset(); void mutate(); } catch (err) { if (isUnauthorized(err)) throw err; setMsg(errText(err, t("pb.failed"))); }
    setBusy(false);
  }
  return (
    <>
      <form className="glass pad stack" onSubmit={create}>
        <h2>{t("pb.create")}</h2><p className="soft small">{t("pb.createHelp")}</p>
        <Field label={t("pb.company")}><input className="input" name="company" required /></Field>
        <Field label={t("pb.ownerEmail")}><input className="input" name="email" type="email" required /></Field>
        <Field label={t("pb.ownerPw")}><input className="input" name="pass" type="password" autoComplete="new-password" minLength={8} required /></Field>
        {msg && <Note kind="bad">{msg}</Note>}
        <button className="btn" type="submit" disabled={busy}>{t("pb.createBtn")}</button>
      </form>
      <h2>{t("pb.records")}</h2><p className="soft small">{t("pb.legacy")}</p>
      {!list.length ? <Note>{t("pb.none")}</Note> : <div className="stack">{list.map((b) => <Row key={b.business_id} b={b} self={!!me.business?.business_id && b.business_id === me.business.business_id} onChanged={() => void mutate()} />)}</div>}
    </>
  );
}
