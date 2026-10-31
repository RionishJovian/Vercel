import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";

interface Acc {
  user_id: string; email?: string; full_name?: string; user_role?: string; company_name?: string; is_active?: boolean; presence?: string;
  last_seen_at?: string; last_active_at?: string; last_login_at?: string; operator_workspace?: boolean; authorized_login?: boolean; primary_login_id?: string;
  pass?: { kind?: string; state?: string; activated_at?: string; expires_at?: string; transition_expires_at?: string; revision?: number; revoked_at?: string; authorized_login?: boolean };
  limits?: { router_count?: number; router_limit?: number; site_count?: number; site_limit?: number };
  subscription?: { enforced?: boolean; capacity_exempt?: boolean };
}
interface Res { accounts?: Acc[]; total?: number; as_of?: string }
const when = (v?: string) => (v ? new Date(v).toLocaleString() : "—");
const n = (v: unknown) => (typeof v === "number" ? v.toLocaleString() : "—");
const errText = (e: unknown, f: string) => { const m = e instanceof Error ? e.message : ""; return m && !/^http-\d+$/.test(m) ? m : f; };

function AccountCard({ a, onChanged }: { a: Acc; onChanged: () => void }) {
  const { t } = useI18n();
  const p = a.pass ?? {}, l = a.limits ?? {}, sub = a.subscription ?? {};
  const [msg, setMsg] = useState<{ k: string; text: string } | null>(null), [busy, setBusy] = useState(false);
  const cap = (v?: number) => (sub.capacity_exempt ? t("pa.unlimited") : n(v));
  async function record(form: HTMLFormElement, action: "activate" | "revoke") {
    const plan = String(new FormData(form).get("plan"));
    setBusy(true); setMsg(null);
    try { await api.put(`/platform/accounts/${encodeURIComponent(a.user_id)}/pass`, { plan, action, revision: Number(p.revision ?? 0) }); onChanged(); }
    catch (err) { if (isUnauthorized(err)) throw err; setMsg({ k: "pass", text: errText(err, t("pa.passFail")) }); }
    setBusy(false);
  }
  async function authorize(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const reason = String(new FormData(e.currentTarget).get("reason")).trim();
    setBusy(true); setMsg(null);
    try { await api.put(`/platform/accounts/${encodeURIComponent(a.user_id)}/authorized-login`, { expected_primary_user_id: a.primary_login_id ?? "", reason }); onChanged(); }
    catch (err) { if (isUnauthorized(err)) throw err; setMsg({ k: "auth", text: errText(err, t("pa.authFail")) }); }
    setBusy(false);
  }
  const tone = a.presence === "online" ? "ok" : a.presence === "idle" ? "warn" : "";
  const isDev = a.user_role === "DEVELOPER";
  return (
    <article className="glass pad stack">
      <div className="row"><b style={{ overflowWrap: "anywhere" }}>{a.email}</b><span className={"chip " + (a.is_active ? "ok" : "bad")}>{a.is_active ? t("pa.enabled") : t("pa.disabledAcc")}</span></div>
      <p className="soft small">{a.full_name} · {a.user_role} · {a.company_name}</p>
      <div className="row small"><span className={"chip " + tone}>{t("pa.presence." + (["online", "idle", "offline", "unobserved"].includes(a.presence ?? "") ? a.presence : "unobserved"))}</span><span className="soft">{t("pa.heartbeat")}: {when(a.last_seen_at)}</span></div>
      <dl className="facts">
        <dt>{t("pa.pass")}</dt><dd>{t("pa.k." + (["monthly", "annual", "trial", "spark", "none"].includes(p.kind ?? "") ? p.kind : "none"))} · {t("ap.s." + (p.state ?? "unknown"))}</dd>
        <dt>{t("ap.activation")}</dt><dd>{when(p.activated_at)}</dd><dt>{t("ap.expiry")}</dt><dd>{when(p.expires_at)}</dd>
        {p.transition_expires_at && <><dt>{t("ap.transitionEnds")}</dt><dd>{when(p.transition_expires_at)}</dd></>}
        <dt>{t("pa.routers")}</dt><dd>{n(l.router_count)} / {cap(l.router_limit)}</dd><dt>{t("pa.sites")}</dt><dd>{n(l.site_count)} / {cap(l.site_limit)}</dd>
        <dt>{t("pa.authLogin")}</dt><dd>{isDev ? t("pa.al.exempt") : a.operator_workspace ? t("pa.al.operator") : a.authorized_login === true ? t("pa.al.this") : a.primary_login_id ? t("pa.al.other") : t("pa.al.review")}</dd>
      </dl>
      {!p.activated_at && <p className="soft small">{t("pa.noPass")}</p>}
      <details><summary>{t("pa.manage")}</summary>
        <form className="stack" onSubmit={(e) => { e.preventDefault(); void record(e.currentTarget, "activate"); }}>
          <h3>{t("pa.record")}</h3><p className="soft small">{t("pa.recordHelp")}</p>
          <select className="input" name="plan" defaultValue={p.kind === "annual" ? "annual" : "monthly"} aria-label={t("ap.plan")}><option value="monthly">{t("ap.monthly")}</option><option value="annual">{t("ap.annual")}</option></select>
          <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
            <button className="btn" type="submit" disabled={busy || isDev || (sub.enforced && p.authorized_login === false)}>{t("pa.approve")}</button>
            <button className="btn quiet" type="button" disabled={busy || !p.revision || !!p.revoked_at} onClick={(e) => void record(e.currentTarget.form!, "revoke")}>{t("pa.revoke")}</button>
          </div>
          {msg?.k === "pass" && <Note kind="bad">{msg.text}</Note>}
        </form>
        {a.user_role === "OWNER" && !a.operator_workspace && a.authorized_login !== true && (
          <form className="stack" onSubmit={authorize}>
            <h3>{t("pa.chooseLogin")}</h3><p className="soft small">{t("pa.chooseLoginHelp")}</p>
            <input className="input" name="reason" required minLength={5} maxLength={500} placeholder={t("pa.reason")} aria-label={t("pa.reason")} />
            <label className="check"><span>{t("pa.reviewed")}</span><input type="checkbox" required /></label>
            <button className="btn quiet" type="submit" disabled={busy}>{t("pa.setLogin")}</button>
            {msg?.k === "auth" && <Note kind="bad">{msg.text}</Note>}
          </form>
        )}
      </details>
    </article>
  );
}

export function Accounts() {
  const { t } = useI18n();
  const [offset, setOffset] = useState(0);
  const { data, error, isLoading, mutate } = useSWR<Res>(`/platform/accounts?limit=50&offset=${offset}`, fetcher, { refreshInterval: 30000, revalidateOnFocus: false, shouldRetryOnError: false, keepPreviousData: true });
  if (isLoading && !data) return <Skeleton h={300} />;
  if (error && !data) return <><Note kind="warn">{t("pa.unavailable")}</Note><button className="pill" type="button" onClick={() => void mutate()}>{t("c.retry")}</button></>;
  const list = asArray<Acc>(data?.accounts), total = data?.total ?? 0;
  return (
    <>
      <p className="soft small">{t("pa.lead")} {data?.as_of ? `${t("pa.observed")} ${new Date(data.as_of).toLocaleString()}.` : ""}</p>
      <div className="stack">{list.map((a) => <AccountCard key={a.user_id} a={a} onChanged={() => void mutate()} />)}</div>
      <div className="row"><button className="pill" type="button" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 50))}>{t("ev.newer")}</button><span className="soft small">{total ? offset + 1 : 0}–{Math.min(offset + list.length, total)} / {total}</span><button className="pill" type="button" disabled={offset + list.length >= total} onClick={() => setOffset(offset + 50)}>{t("ev.older")}</button></div>
    </>
  );
}
