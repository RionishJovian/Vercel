import { useState } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../lib/api";
import { asArray, type Router } from "../lib/types";
import { useI18n } from "../i18n";
import { BackLink, Note } from "../components/ui";
import { Skeleton } from "../components/Skeleton";

interface Backup { id: string; label?: string; kind?: string; size_bytes?: number; created_at?: string }
interface BackupRes { backups?: Backup[]; restore_available?: boolean; capture_available?: boolean }
interface Change { timestamp?: string; user_role?: string; action?: string; reason?: string }
const errText = (e: unknown) => (e as { data?: { error?: string } } | null)?.data?.error || (e instanceof Error ? e.message : "") || "failed";

export function Backups() {
  const { t } = useI18n();
  const routers = useSWR<{ routers?: Router[] }>("/routers", fetcher);
  const [dev, setDev] = useState("");
  const list = asArray<Router>(routers.data?.routers);
  const id = dev || list[0]?.device_id || "";
  const bk = useSWR<BackupRes>(id ? `/devices/${encodeURIComponent(id)}/backups` : null, fetcher, { shouldRetryOnError: false });
  const ch = useSWR<{ changes?: Change[] }>(id ? `/devices/${encodeURIComponent(id)}/changes` : null, fetcher, { shouldRetryOnError: false });
  const [busy, setBusy] = useState(false), [msg, setMsg] = useState<{ bad: boolean; text: string } | null>(null);
  const [restoring, setRestoring] = useState<{ id: string; otp: string } | null>(null);
  const back = <BackLink href="#/more">{t("tab.more")}</BackLink>;
  if (routers.isLoading) return <>{back}<Skeleton h={260} /></>;
  if (!list.length) return <>{back}<h1>{t("bk.title")}</h1><Note>{t("bk.noDevices")}</Note></>;
  const backups = asArray<Backup>(bk.data?.backups), changes = asArray<Change>(ch.data?.changes);

  async function capture() {
    setBusy(true); setMsg(null);
    try { await api.post(`/devices/${encodeURIComponent(id)}/backups`, {}); await bk.mutate(); await ch.mutate(); } catch (e) { if (isUnauthorized(e)) throw e; setMsg({ bad: true, text: errText(e) }); }
    setBusy(false);
  }
  async function restore() {
    if (!restoring) return;
    setBusy(true); setMsg(null);
    try { await api.post(`/devices/${encodeURIComponent(id)}/restore/${encodeURIComponent(restoring.id)}`, { otp: restoring.otp }); setMsg({ bad: false, text: t("bk.restored") }); setRestoring(null); await ch.mutate(); }
    catch (e) { if (isUnauthorized(e)) throw e; setMsg({ bad: true, text: errText(e) }); }
    setBusy(false);
  }
  return (
    <>
      {back}<h1>{t("bk.title")}</h1><p className="soft">{t("bk.lead")}</p>
      <section className="glass pad stack">
        <label className="field"><span>{t("bk.device")}</span><select className="input" value={id} onChange={(e) => { setDev(e.target.value); setRestoring(null); setMsg(null); }}>{list.map((d) => <option key={d.device_id} value={d.device_id}>{d.hardware_name || d.device_id}</option>)}</select></label>
        <button className="btn" type="button" disabled={busy || bk.data?.capture_available !== true} onClick={() => void capture()}>{t("bk.capture")}</button>
        <p className="soft small" role="status">{bk.isLoading ? t("bk.checking") : bk.error ? t("bk.unconfirmed") : bk.data?.capture_available === true ? t("bk.available") : t("bk.unavailable")}</p>
      </section>
      {msg && <Note kind={msg.bad ? "bad" : "ok"}>{msg.text}</Note>}
      <section className="stack"><h2>{t("bk.backups")}</h2>
        {bk.error ? <Note kind="bad">{t("bk.loadFail")}</Note> : bk.isLoading ? <Skeleton h={100} /> : !backups.length ? <Note>{t("bk.none")}</Note> : backups.map((b) => (
          <div className="glass pad stack" key={b.id}>
            <div className="row"><b>{b.label || "—"}</b><span className="chip">{b.kind}</span></div>
            <p className="soft small num">{b.size_bytes} B · {b.created_at ? new Date(b.created_at).toLocaleString() : ""}</p>
            {bk.data?.restore_available ? (
              restoring?.id === b.id ? (
                <div className="note warn stack">
                  <p className="small">{t("bk.otpAsk")}</p>
                  <input className="input" inputMode="numeric" autoComplete="one-time-code" value={restoring.otp} onChange={(e) => setRestoring({ id: b.id, otp: e.target.value })} aria-label={t("bk.otp")} />
                  <div className="row" style={{ justifyContent: "flex-start" }}><button className="pill" type="button" onClick={() => setRestoring(null)}>{t("c.cancel")}</button><button className="pill danger" type="button" disabled={busy || !restoring.otp.trim()} onClick={() => void restore()}>{t("bk.restore")}</button></div>
                </div>
              ) : <button className="pill warn" type="button" disabled={busy} onClick={() => setRestoring({ id: b.id, otp: "" })}>{t("bk.restore")}</button>
            ) : <span className="soft small">{t("bk.restoreOff")}</span>}
          </div>
        ))}
      </section>
      <section className="stack"><h2>{t("bk.history")}</h2>
        {ch.error ? <Note kind="bad">{t("bk.histFail")}</Note> : ch.isLoading ? <Skeleton h={80} /> : !changes.length ? <Note>{t("bk.noChanges")}</Note> : changes.map((c, i) => (
          <div className="rowitem" key={i}><div className="stack" style={{ gap: 2 }}><b className="small">{c.action}</b><span className="soft small">{c.reason}</span></div><span className="soft small num">{c.user_role} · {c.timestamp ? new Date(c.timestamp).toLocaleString() : ""}</span></div>
        ))}
      </section>
    </>
  );
}
