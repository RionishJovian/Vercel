import { useCallback, useEffect, useRef, useState } from "react";
import { api, ApiError, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";

interface Job { id: string; revision: number; phase: string; old_address?: string; new_address?: string; expires_at?: string; backup?: string }
const FINAL = ["completed", "rolled_back"];
const PHASES = ["planned", "starting", "hub_staged", "router_staged", "new_verified", "hub_committed", "router_retained", "connection_saved", "router_cleaned", "completed", "rolled_back"];
const expiry = (v?: string) => (Number.isFinite(Date.parse(v ?? "")) ? new Date(v!).toLocaleString() : "—");

/** Moves a gateway to a server-allocated management address. The old address is kept until the new one is verified,
 *  and the durable checkpoint, not this screen, decides what recovery does. */
export function Migration({ id, name, canEdit }: { id: string; name: string; canEdit: boolean }) {
  const { t } = useI18n();
  const [job, setJob] = useState<Job | null>(null), [available, setAvailable] = useState(false);
  const [status, setStatus] = useState(""), [busy, setBusy] = useState(false), [consent, setConsent] = useState(false), [loading, setLoading] = useState(true);
  const rev = useRef(0);
  const base = `/routers/${encodeURIComponent(id)}/management-migration`;

  const load = useCallback(async () => {
    const mine = ++rev.current; setLoading(true); setStatus(t("mg.reading"));
    try {
      const d = await api.get<{ enabled?: boolean; reason?: string; job?: Job | null }>(base);
      if (mine !== rev.current) return;
      setAvailable(d.enabled === true); setJob(d.job ?? null); setConsent(false); setStatus(d.enabled === true ? t("mg.loaded") : d.reason || t("mg.unavailable"));
    } catch (e) { if (isUnauthorized(e)) throw e; if (mine === rev.current) { setAvailable(false); setJob(null); setStatus(t("mg.statusFail")); } }
    if (mine === rev.current) setLoading(false);
  }, [base, t]);
  useEffect(() => { void load(); }, [load]);

  async function run(action: "preview" | "apply" | "recover") {
    if (busy || !available || !canEdit || (action !== "preview" && !job)) return;
    const saved = job; setBusy(true); setStatus(action === "preview" ? t("mg.checking") : t("mg.working"));
    let failed = "";
    try { await api.post(`${base}/${action}`, action === "preview" ? {} : { id: saved!.id, revision: saved!.revision, confirmed: true }); }
    catch (e) { if (isUnauthorized(e)) throw e; failed = (e instanceof ApiError && typeof (e.data as { error?: string } | null)?.error === "string" ? (e.data as { error: string }).error : t("mg.notVerified")); }
    await load();
    if (failed) setStatus((s) => `${s} ${failed} ${t("mg.dontRepeat")}`);
    setBusy(false);
  }
  const expired = job?.phase === "planned" && Number.isFinite(Date.parse(job.expires_at ?? "")) && Date.parse(job!.expires_at!) <= Date.now();
  return (
    <div className="stack">
      <h3>{t("mg.title")}</h3><p className="soft small">{t("mg.lead")}</p>
      <p className="soft small" role="status">{status}</p>
      {!loading && !available && <><Note>{t("mg.notEnabled")}</Note><button className="pill" type="button" disabled={busy} onClick={() => void load()}>{t("mg.refresh")}</button></>}
      {available && job && (
        <dl className="facts"><dt>{t("hub.title")}</dt><dd>{name}</dd><dt>{t("mg.current")}</dt><dd>{job.old_address}</dd><dt>{t("mg.new")}</dt><dd>{job.new_address}</dd><dt>{t("mg.phase")}</dt><dd>{t("mg.p." + (PHASES.includes(job.phase) ? job.phase : "unknown"))}</dd><dt>{t("mg.expires")}</dt><dd>{expiry(job.expires_at)}</dd></dl>
      )}
      {available && job?.backup && <p className="soft small">{t("nw.backup")}: {job.backup}</p>}
      {available && !canEdit && <Note>{t("mg.needAdmin")}</Note>}
      {available && canEdit && (!job || FINAL.includes(job.phase)) && (
        <>
          {job?.phase === "completed" && <p className="soft small">{t("mg.done")}</p>}
          {job?.phase === "rolled_back" && <p className="soft small">{t("mg.rolled")}</p>}
          <button className="btn quiet" type="button" disabled={busy} onClick={() => void run("preview")}>{t("mg.prepare")}</button>
        </>
      )}
      {available && canEdit && job && !FINAL.includes(job.phase) && (job.phase === "planned" ? (
        <>
          {expired ? <Note kind="warn">{t("mg.expired")}</Note> : <p className="soft small">{t("mg.reviewBoth")}</p>}
          <label className="check"><span>{t("mg.consent")}</span><input type="checkbox" checked={consent} disabled={busy || expired} onChange={(e) => setConsent(e.target.checked)} /></label>
          <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
            <button className="btn" type="button" disabled={busy || !consent || expired} onClick={() => void run("apply")}>{t("mg.apply")}</button>
            <button className="pill" type="button" disabled={busy} onClick={() => void run("recover")}>{t("mg.cancel")}</button>
          </div>
        </>
      ) : (
        <>
          <p className="soft small">{t("mg.needsRecovery")}</p>
          <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}><button className="btn quiet" type="button" disabled={busy} onClick={() => void run("recover")}>{t("mg.recover")}</button><button className="pill" type="button" disabled={busy} onClick={() => void load()}>{t("mg.refreshDurable")}</button></div>
        </>
      ))}
    </div>
  );
}
