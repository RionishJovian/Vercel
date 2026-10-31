import { useRef, useState, type FormEvent } from "react";
import { api, ApiError, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { enrollError, finishError } from "./messages";

interface Enrolled { state?: string; router_configuration_required?: boolean; one_paste?: boolean; automatic_management?: boolean; existing_tunnel_required?: boolean; router_script?: string; tunnel_address?: string; hub_public_key?: string; endpoint?: string; allowed_ips?: string[] }
const okScript = (s: unknown): s is string => typeof s === "string" && s.trim() !== "" && s.length <= 32768;

function Script({ title, text }: { title: string; text: string }) {
  const { t } = useI18n();
  const area = useRef<HTMLTextAreaElement>(null);
  const [label, setLabel] = useState<"copy" | "copied" | "manual">("copy");
  async function copy() {
    try { await navigator.clipboard.writeText(text); setLabel("copied"); } catch { area.current?.focus(); area.current?.select(); setLabel("manual"); }
  }
  return (
    <div className="stack">
      <h3>{title}</h3>
      <textarea ref={area} className="input script" rows={9} readOnly spellCheck={false} aria-label={title} value={text} />
      <button className="pill" type="button" onClick={() => void copy()}>{t(label === "copy" ? "hub.copy" : label === "copied" ? "hub.copied" : "hub.copyManual")}</button>
    </div>
  );
}

/** One-paste connection for a gateway. The router keeps its own private key; nothing here asks for router credentials. */
export function Enrollment({ id, onVerified }: { id: string; onVerified: () => void }) {
  const { t } = useI18n();
  const [prep, setPrep] = useState<{ kind: "idle" | "busy" | "ready" | "fail"; script?: string }>({ kind: "idle" });
  const [recovery, setRecovery] = useState(false), [showRecovery, setShowRecovery] = useState(false);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<{ ok: true; data: Enrolled; repair: boolean } | { ok: false; key: string } | null>(null);
  const [fin, setFin] = useState<{ busy: boolean; key?: string; ok?: boolean }>({ busy: false });
  const eid = encodeURIComponent(id);

  async function prepare() {
    setPrep({ kind: "busy" });
    try {
      const r = await api.get<{ state?: string; bootstrap_script?: string }>(`/vps/peers/${eid}/bootstrap`);
      if (r.state !== "router_key_required" || !okScript(r.bootstrap_script)) throw new Error("unverified");
      setPrep({ kind: "ready", script: r.bootstrap_script });
    } catch (e) { if (isUnauthorized(e)) throw e; setPrep({ kind: "fail" }); }
  }
  async function enroll(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (busy) return;
    const f = new FormData(e.currentTarget);
    if (f.get("confirm") !== "on") { setResult({ ok: false, key: "hub.confirmNeeded" }); return; }
    const resume = f.get("resume") === "on";
    setBusy(true); setResult(null);
    try {
      const r = await api.post<Enrolled>(`/vps/peers/${eid}/quick-enroll`, { management_rest_port: Number(f.get("port")), confirm_enrollment: true, ...(resume ? { resume_existing: true } : {}) });
      const repair = r.existing_tunnel_required === true && r.one_paste === false && resume;
      if (r.state !== "hub_peer_enrolled" || r.router_configuration_required !== true || (!repair && r.one_paste !== true) || r.automatic_management !== true || !okScript(r.router_script)) throw new Error("unverified");
      setResult({ ok: true, data: r, repair });
    } catch (err) {
      if (isUnauthorized(err)) throw err;
      const code = err instanceof ApiError ? (err.data as { code?: string } | null)?.code : undefined;
      if (code === "hub_router_key_unavailable") setShowRecovery(true);
      setResult({ ok: false, key: enrollError(err instanceof ApiError ? err.status : undefined, code) });
    }
    setBusy(false);
  }
  async function finish() {
    setFin({ busy: true });
    try {
      const r = await api.post<{ state?: string }>(`/vps/peers/${eid}/finish`, {});
      if (r.state !== "management_authenticated") throw new Error("unverified");
      setFin({ busy: false, ok: true, key: "hub.fin.ok" });
      onVerified();
    } catch (err) {
      if (isUnauthorized(err)) throw err;
      setFin({ busy: false, key: finishError(err instanceof ApiError ? err.status : undefined, err instanceof ApiError ? (err.data as { code?: string } | null)?.code : undefined) });
    }
  }

  return (
    <div className="stack enroll">
      <section className="stack">
        <h3>{t("hub.step1")}</h3>
        <p className="soft small">{t("hub.step1Body")}</p>
        <button className="pill" type="button" disabled={prep.kind === "busy"} onClick={() => void prepare()}>{t("hub.prepare")}</button>
        {prep.kind === "busy" && <p className="soft small" role="status">{t("hub.preparing")}</p>}
        {prep.kind === "fail" && <Note kind="warn">{t("hub.prepFail")}</Note>}
        {prep.kind === "ready" && prep.script && <Script title={t("hub.prepTitle")} text={prep.script} />}
        <form className="stack" onSubmit={enroll}>
          <label className="field"><span>{t("hub.port")}</span><input className="input" name="port" type="number" min={1} max={65535} defaultValue={443} required /></label>
          <label className="check"><span>{t("hub.authorize")}</span><input type="checkbox" name="confirm" required /></label>
          {showRecovery && <label className="check"><span>{t("hub.repair")}</span><input type="checkbox" name="resume" checked={recovery} onChange={(e) => setRecovery(e.target.checked)} /></label>}
          <p className="soft small">{t("hub.thirty")}</p>
          <button className="btn" type="submit" disabled={busy}>{busy ? t("hub.creating") : t("hub.generate")}</button>
        </form>
        {result && !result.ok && <Note kind="warn">{t(result.key)}</Note>}
        {result?.ok && (
          <div className="stack" role="status" aria-live="polite">
            <Note kind="ok">{result.repair ? t("hub.repairReady") : t("hub.scriptReady")}</Note>
            <p className="soft small">{result.repair ? t("hub.repairBody") : t("hub.scriptBody")}</p>
            <Script title={t("hub.setupTitle")} text={result.data.router_script ?? ""} />
            <dl className="facts">
              <dt>{t("hub.addr")}</dt><dd><code>{result.data.tunnel_address}</code></dd>
              <dt>{t("hub.pubkey")}</dt><dd><code>{result.data.hub_public_key || t("hub.notReported")}</code></dd>
              <dt>{t("hub.endpoint")}</dt><dd><code>{result.data.endpoint || t("hub.notReported")}</code></dd>
              <dt>{t("hub.allowed")}</dt><dd><code>{Array.isArray(result.data.allowed_ips) ? result.data.allowed_ips.join(", ") : t("hub.notReported")}</code></dd>
            </dl>
            <p className="soft small">{t("hub.keepPrivate")}</p>
          </div>
        )}
      </section>
      <section className="stack">
        <h3>{t("hub.step2")}</h3>
        <p className="soft small">{t("hub.step2Body")}</p>
        <button className="btn quiet" type="button" disabled={fin.busy} onClick={() => void finish()}>{fin.busy ? t("hub.checking") : t("hub.finish")}</button>
        {fin.key && <Note kind={fin.ok ? "ok" : "warn"}>{t(fin.key)}</Note>}
      </section>
    </div>
  );
}
