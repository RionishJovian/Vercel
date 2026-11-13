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
 const { data, error, isValidating: recordsChecking, mutate } = useSWR<{ accounts: Account[]; write_enabled: boolean }>(base + "/accounts", fetcher, { shouldRetryOnError: false });
 const { data: inventory, error: inventoryError, isValidating: inventoryChecking, mutate: recheckPlans } = useSWR<{ inventory?: { available?: { ppp_profiles?: boolean }; objects?: { ppp_profiles?: Array<{ values: { name?: string } }> } } }>(canEdit ? `/routers/${encodeURIComponent(routerId)}/network/inventory` : null, fetcher, { shouldRetryOnError: false });
 const plans = (inventory?.inventory?.objects?.ppp_profiles ?? []).map((p) => p.values.name).filter((p): p is string => !!p && !p.startsWith("mm-bb-"));
 const plansLoaded = Array.isArray(inventory?.inventory?.objects?.ppp_profiles) && inventory?.inventory?.available?.ppp_profiles !== false;
 const managementRequired = inventoryError instanceof ApiError && (inventoryError.data as { code?: string } | null)?.code === "router_management_required";
 const plansUnavailable = !!inventoryError || !!inventory && !plansLoaded;
 const [busy, setBusy] = useState(false), [notice, setNotice] = useState(""), [failed, setFailed] = useState(false);
 const [days, setDays] = useState(30), [amount, setAmount] = useState(0);
 const alive = useRef(true), pending = useRef(false);
 // Retain an uncertain request's identity in memory, never browser storage.
 // The same submitted values must not become a second payment after a lost reply.
 const unconfirmed = useRef(new Map<string, string>());
 useEffect(() => { alive.current = true; const requests = unconfirmed.current; return () => { alive.current = false; requests.clear(); }; }, []);
 useEffect(() => { onBusyChange?.(busy); return () => onBusyChange?.(false); }, [busy, onBusyChange]);
 function finishUnconfirmed(id: string | undefined) {
  for (const [key, value] of unconfirmed.current) if (value === id) unconfirmed.current.delete(key);
 }
 async function act(path: string, body: Record<string, unknown>, sync = false) {
  if (pending.current || error) return false;
  const recoveryID = path.match(/^\/operations\/([0-9a-f-]{36})\/retry$/)?.[1];
  const account = data?.accounts.find((a) => path.includes(a.id) || !!recoveryID && a.pending_operation === recoveryID);
  const { operation_id, ...intent } = body;
  const paidChange = path === "/accounts" || /^\/accounts\/[0-9a-f-]{36}\/renew$/.test(path);
  const identityKey = paidChange && typeof operation_id === "string" ? JSON.stringify([base, path, intent]) : "";
  const cached = identityKey ? unconfirmed.current.get(identityKey) : undefined;
  const summary = [cached ? t("bb.retryPrevious") : "", routerName, String(body.username ?? account?.username ?? ""), body.period_days ? `${t("bb.days")}: ${body.period_days}` : "", body.amount_mmk !== undefined ? `${t("bb.amount")}: ${body.amount_mmk}` : ""].filter(Boolean).join("\n");
  if (!sync && !window.confirm(`${summary}\n${t("bb.confirm")}`)) return false;
  const submitted = cached ? { ...body, operation_id: cached } : body;
  const submittedID = typeof submitted.operation_id === "string" ? submitted.operation_id : recoveryID;
  if (identityKey && submittedID) unconfirmed.current.set(identityKey, submittedID);
  pending.current = true; setBusy(true); setNotice(""); setFailed(false);
  try {
   const result = await api.post<{ operation?: { state?: string } }>(base + path, sync ? {} : { ...submitted, confirm: true });
   if (!sync && result.operation?.state !== "applied") throw new Error("unverified");
   finishUnconfirmed(submittedID);
   if (alive.current) { setNotice(sync ? t("bb.sync") : t("bb.done")); void mutate(); }
   return true;
  } catch (e) {
   if (e instanceof ApiError && e.status === 422) finishUnconfirmed(submittedID);
   if (alive.current) { setFailed(true); setNotice(t(sync ? "bb.syncFailed" : e instanceof ApiError && e.status === 422 ? "bb.rejected" : identityKey ? "bb.unconfirmedRequest" : "bb.failed")); void mutate(); }
   return false;
  }
  finally { pending.current = false; if (alive.current) { setBusy(false); if (sync) void recheckPlans(); } }
 }
 function create(e: FormEvent<HTMLFormElement>) {
  e.preventDefault(); const form = e.currentTarget, f = new FormData(form);
  void act("/accounts", { operation_id: crypto.randomUUID(), username: String(f.get("username")), password: String(f.get("password")), profile: String(f.get("profile")), period_days: days, amount_mmk: amount }).then((applied) => { if (applied && form.isConnected) form.reset(); });
 }
 const locked = busy || !!error || !canEdit || data?.write_enabled !== true;
 return <section className="glass pad workspace-grid" aria-label={t("bb.title")}>
  <div className="row" style={{ flexWrap: "wrap" }}><h2>{t("bb.title")}</h2><b>{routerName}</b><button className="pill" type="button" disabled={busy || !!error || !data || !canEdit} onClick={() => void act("/sync", {}, true)}>{t("bb.sync")}</button></div>
  {error && <Note kind="warn">
   <p>{t("bb.loadFailed")}</p>
   {data && <p>{t("bb.savedRecords")}</p>}
   <button className="pill" type="button" disabled={busy || recordsChecking} onClick={() => void mutate()}>{t("bb.reload")}</button>
  </Note>}
  <details><summary>{t("c.help")}</summary><p className="soft small">{t("bb.help")}</p></details>
  {canEdit && (plansUnavailable || plansLoaded && !plans.length) && <Note kind="warn">
   <p>{t(managementRequired ? "bb.connectFirst" : plansUnavailable ? "bb.plansFailed" : "bb.noPlans")}</p>
   <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
    {managementRequired && <a className="pill" href="/app/hub">{t("hub.connect")}</a>}
    <button className="pill" type="button" disabled={busy || inventoryChecking} onClick={() => void recheckPlans()}>{t("bb.recheck")}</button>
   </div>
  </Note>}
  {notice && <div role="status"><Note kind={failed ? "warn" : undefined}>{notice}</Note></div>}
  {data && !error && <dl className="capsules wrap" aria-label={t("bb.overview")}>
   <div className="capsule"><dt>{t("bb.title")}</dt><dd className="num">{data.accounts.length}</dd></div>
   <div className="capsule"><dt>{t("bb.sessionsSeen")}</dt><dd className="num">{data.accounts.filter(a => a.session).length}</dd></div>
   <div className="capsule"><dt>{t("bb.suspendedCount")}</dt><dd className="num">{data.accounts.filter(a => a.observation?.suspended === true).length}</dd></div>
  </dl>}
  <div className="workspace-main workspace-stack">
  {data && !data.accounts.length && <p>{t("bb.empty")}</p>}
  {data?.accounts.map((a) => {
   const s = a.observation, now = Date.now() / 1000;
   const status = !s ? "unknown" : s.last_clock > now + 30 ? "clock" : s.suspended ? "suspended" : !s.first_use ? "unused" : s.expires_at <= now ? "expired" : "active";
   return <article className="rowitem stack" key={a.id}>
    <div className="row" style={{ flexWrap: "wrap", overflowWrap: "anywhere" }}><b>{a.username}</b><span>{a.profile}</span><span>{t("bb." + status)}</span></div>
    <p className="soft small tnum">{t("bb.first")}: {stamp(s?.first_use)} · {t("bb.expiry")}: {stamp(s?.expires_at)}<br />{t("bb.observed")}: {a.observed_at ? new Date(a.observed_at).toLocaleString() : "—"}</p>
    {a.session && <p className="soft small tnum">{t("bb.session")}: {a.session.id} · ↓ {a.session.download_bytes} B · ↑ {a.session.upload_bytes} B<br />{t("bb.observed")}: {a.session_observed_at ? new Date(a.session_observed_at).toLocaleString() : "—"}</p>}
    {canEdit && (a.pending_operation ? <button className="pill" disabled={locked} onClick={() => void act(`/operations/${a.pending_operation}/retry`, {})}>{t("bb.retry")}</button> : <div className="row">
     <button className="pill" disabled={locked || !s} onClick={() => void act(`/accounts/${a.id}/renew`, { operation_id: crypto.randomUUID(), period_days: days, amount_mmk: amount })}>{t("bb.renew")}</button>
     <button className="pill" disabled={locked || !s} onClick={() => void act(`/accounts/${a.id}/${s?.suspended ? "resume" : "suspend"}`, { operation_id: crypto.randomUUID() })}>{t(s?.suspended ? "bb.resume" : "bb.suspend")}</button>
    </div>)}
   </article>;
  })}
  </div>
  {canEdit && <div className="workspace-rail workspace-stack">
   {!data?.write_enabled && data && <Note>{t("bb.locked")}</Note>}
   <div className="row">
    <label className="field"><span>{t("bb.days")}</span><input className="input" type="number" inputMode="numeric" min="1" max="366" value={days} disabled={busy || !!error} onChange={(e) => setDays(Number(e.target.value))} /></label>
    <label className="field"><span>{t("bb.amount")}</span><input className="input" type="number" inputMode="numeric" min="0" max="1000000000" value={amount} disabled={busy || !!error} onChange={(e) => setAmount(Number(e.target.value))} /></label>
   </div>
   <details><summary>{t("bb.new")}</summary><form className="stack" onSubmit={create}>
    <fieldset disabled={locked || inventoryChecking || !!inventoryError || !plansLoaded || !plans.length} className="nolock stack">
     <label className="field"><span>{t("bb.user")}</span><input className="input" name="username" required maxLength={64} autoComplete="off" /></label>
     <label className="field"><span>{t("bb.password")}</span><input className="input" name="password" type="password" required minLength={10} maxLength={128} autoComplete="new-password" /></label>
     <label className="field"><span>{t("bb.profile")}</span><select className="input" name="profile" required defaultValue=""><option value="">—</option>{plans.map((p) => <option key={p} value={p}>{p}</option>)}</select><small className="soft">{t("bb.planHint")}</small></label>
     <p className="soft small">{t("bb.review")}</p><button className="btn">{t("bb.install")}</button>
    </fieldset>
   </form></details>
  </div>}
 </section>;
}
