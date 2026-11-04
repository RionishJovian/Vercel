import { useEffect, useRef, useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, ApiError, fetcher } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";

interface Account {
 id: string; username: string; profile: string; pending_operation?: string; observed_at?: string;
 observation: { first_use: number; expires_at: number; suspended: boolean; last_clock: number } | null;
 session?: { id: string; download_bytes: number; upload_bytes: number; uptime_seconds: number } | null; session_observed_at?: string;
}
const stamp = (epoch?: number) => epoch ? new Date(epoch * 1000).toLocaleString() : "—";

export function Broadband({ routerId, routerName, canEdit, onBusyChange }: { routerId: string; routerName: string; canEdit: boolean; onBusyChange?: (busy: boolean) => void }) {
 const { t } = useI18n(), base = `/routers/${encodeURIComponent(routerId)}/broadband`;
 const { data, error, mutate } = useSWR<{ accounts: Account[]; write_enabled: boolean }>(base + "/accounts", fetcher, { shouldRetryOnError: false });
 const { data: inventory } = useSWR<{ inventory?: { objects?: { ppp_profiles?: Array<{ values: { name?: string } }> } } }>(canEdit ? `/routers/${encodeURIComponent(routerId)}/network/inventory` : null, fetcher, { shouldRetryOnError: false });
 const plans = (inventory?.inventory?.objects?.ppp_profiles ?? []).map((p) => p.values.name).filter((p): p is string => !!p && !p.startsWith("mm-bb-"));
 const [busy, setBusy] = useState(false), [notice, setNotice] = useState(""), [failed, setFailed] = useState(false);
 const [days, setDays] = useState(30), [amount, setAmount] = useState(0);
 const alive = useRef(true), pending = useRef(false);
 useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
 useEffect(() => { onBusyChange?.(busy); return () => onBusyChange?.(false); }, [busy, onBusyChange]);
 async function act(path: string, body: Record<string, unknown>, sync = false) {
  const account = data?.accounts.find((a) => path.includes(a.id));
  const summary = [routerName, String(body.username ?? account?.username ?? ""), body.period_days ? `${t("bb.days")}: ${body.period_days}` : "", body.amount_mmk !== undefined ? `${t("bb.amount")}: ${body.amount_mmk}` : ""].filter(Boolean).join("\n");
  if (pending.current || (!sync && !window.confirm(`${summary}\n${t("bb.confirm")}`))) return false;
  pending.current = true; setBusy(true); setNotice(""); setFailed(false);
  try {
   const result = await api.post<{ operation?: { state?: string } }>(base + path, sync ? {} : { ...body, confirm: true });
   if (!sync && result.operation?.state !== "applied") throw new Error("unverified");
   if (alive.current) { setNotice(sync ? t("bb.sync") : t("bb.done")); void mutate(); }
   return true;
  } catch (e) { if (alive.current) { setFailed(true); setNotice(t(e instanceof ApiError && e.status === 422 ? "bb.rejected" : "bb.failed")); void mutate(); } return false; }
  finally { pending.current = false; if (alive.current) setBusy(false); }
 }
 function create(e: FormEvent<HTMLFormElement>) {
  e.preventDefault(); const form = e.currentTarget, f = new FormData(form);
  void act("/accounts", { operation_id: crypto.randomUUID(), username: String(f.get("username")), password: String(f.get("password")), profile: String(f.get("profile")), period_days: days, amount_mmk: amount }).then((applied) => { if (applied && form.isConnected) form.reset(); });
 }
 if (error) return <Note kind="warn">{t("bb.loadFailed")}</Note>;
 const locked = busy || !canEdit || data?.write_enabled !== true;
 return <section className="glass pad stack" aria-label={t("bb.title")}>
  <div className="row" style={{ flexWrap: "wrap" }}><h2>{t("bb.title")}</h2><b>{routerName}</b><button className="pill" type="button" disabled={busy || !data || !canEdit} onClick={() => void act("/sync", {}, true)}>{t("bb.sync")}</button></div>
  <details><summary>{t("c.help")}</summary><p className="soft small">{t("bb.help")}</p></details>
  {notice && <div role="status"><Note kind={failed ? "warn" : undefined}>{notice}</Note></div>}
  {data && !data.accounts.length && <p>{t("bb.empty")}</p>}
  {data?.accounts.map((a) => {
   const s = a.observation, now = Date.now() / 1000;
   const status = !s ? "unknown" : s.last_clock > now + 30 ? "clock" : s.suspended ? "suspended" : !s.first_use ? "unused" : s.expires_at <= now ? "expired" : "active";
   return <article className="rowitem stack" key={a.id}>
    <div className="row" style={{ flexWrap: "wrap", overflowWrap: "anywhere" }}><b>{a.username}</b><span>{a.profile}</span><span>{t("bb." + status)}</span></div>
    <p className="soft small">{t("bb.first")}: {stamp(s?.first_use)} · {t("bb.expiry")}: {stamp(s?.expires_at)}<br />{t("bb.observed")}: {a.observed_at ? new Date(a.observed_at).toLocaleString() : "—"}</p>
    {a.session && <p className="soft small">{t("bb.session")}: {a.session.id} · ↓ {a.session.download_bytes} B · ↑ {a.session.upload_bytes} B<br />{t("bb.observed")}: {a.session_observed_at ? new Date(a.session_observed_at).toLocaleString() : "—"}</p>}
    {canEdit && (a.pending_operation ? <button className="pill" disabled={locked} onClick={() => void act(`/operations/${a.pending_operation}/retry`, {})}>{t("bb.retry")}</button> : <div className="row">
     <button className="pill" disabled={locked || !s} onClick={() => void act(`/accounts/${a.id}/renew`, { operation_id: crypto.randomUUID(), period_days: days, amount_mmk: amount })}>{t("bb.renew")}</button>
     <button className="pill" disabled={locked || !s} onClick={() => void act(`/accounts/${a.id}/${s?.suspended ? "resume" : "suspend"}`, { operation_id: crypto.randomUUID() })}>{t(s?.suspended ? "bb.resume" : "bb.suspend")}</button>
    </div>)}
   </article>;
  })}
  {canEdit && <>
   {!data?.write_enabled && data && <Note>{t("bb.locked")}</Note>}
   <div className="row">
    <label className="field"><span>{t("bb.days")}</span><input className="input" type="number" min="1" max="366" value={days} disabled={busy} onChange={(e) => setDays(Number(e.target.value))} /></label>
    <label className="field"><span>{t("bb.amount")}</span><input className="input" type="number" min="0" max="1000000000" value={amount} disabled={busy} onChange={(e) => setAmount(Number(e.target.value))} /></label>
   </div>
   <details><summary>{t("bb.new")}</summary><form className="stack" onSubmit={create}>
    <fieldset disabled={locked} className="nolock stack">
     <label className="field"><span>{t("bb.user")}</span><input className="input" name="username" required maxLength={64} autoComplete="off" /></label>
     <label className="field"><span>{t("bb.password")}</span><input className="input" name="password" type="password" required minLength={10} maxLength={128} autoComplete="new-password" /></label>
     <label className="field"><span>{t("bb.profile")}</span><select className="input" name="profile" required defaultValue=""><option value="">—</option>{plans.map((p) => <option key={p} value={p}>{p}</option>)}</select><small className="soft">{t("bb.planHint")}</small></label>
     <p className="soft small">{t("bb.review")}</p><button className="btn">{t("bb.install")}</button>
    </fieldset>
   </form></details>
  </>}
 </section>;
}
