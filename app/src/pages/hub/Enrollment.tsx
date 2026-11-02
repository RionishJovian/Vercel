import { useEffect, useRef, useState, type FormEvent } from "react";
import { api, ApiError, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { enrollError, finishError } from "./messages";

interface Enrolled { state?: string; router_configuration_required?: boolean; one_paste?: boolean; automatic_management?: boolean; existing_tunnel_required?: boolean; router_script?: string; tunnel_address?: string; hub_public_key?: string; endpoint?: string; allowed_ips?: string[] }
const okScript = (s: unknown): s is string => typeof s === "string" && s.trim() !== "" && s.length <= 32768;
type StepNo = 1 | 2 | 3;
const STEPS: StepNo[] = [1, 2, 3];

function Script({ title, text }: { title: string; text: string }) {
  const { t } = useI18n();
  const area = useRef<HTMLTextAreaElement>(null);
  const [label, setLabel] = useState<"copy" | "copied" | "manual">("copy");
  async function copy() {
    try { await navigator.clipboard.writeText(text); setLabel("copied"); } catch { area.current?.focus(); area.current?.select(); setLabel("manual"); }
  }
  return (
    <div className="stack">
      <h4>{title}</h4>
      <textarea ref={area} className="input script" rows={9} readOnly spellCheck={false} aria-label={title} value={text} />
      <button className="pill" type="button" onClick={() => void copy()}>{t(label === "copy" ? "hub.copy" : label === "copied" ? "hub.copied" : "hub.copyManual")}</button>
    </div>
  );
}

/** Numbered progress rail. A finished step can be reopened; a step ahead of the current one cannot. */
function Rail({ step, reached, onGo }: { step: StepNo; reached: StepNo; onGo: (s: StepNo) => void }) {
  const { t } = useI18n();
  return (
    <ol className="wiz-rail" aria-label={t("hub.w.progress")}>
      {STEPS.map((n) => {
        const state = n < step ? "done" : n === step ? "current" : "todo";
        const label = t("hub.w.s" + n);
        return (
          <li key={n} data-state={state}>
            <button type="button" className="wiz-dot" aria-current={n === step ? "step" : undefined} disabled={n > reached || n === step} onClick={() => onGo(n)}>
              <span className="wiz-num" aria-hidden="true">{state === "done" ? "✓" : n}</span>
              <span className="wiz-name">{label}</span>
            </button>
          </li>
        );
      })}
    </ol>
  );
}

function Where() {
  const { t } = useI18n();
  return <details className="wiz-where"><summary>{t("hub.w.where")}</summary><p className="soft small">{t("hub.w.whereBody")}</p></details>;
}

/** Step-by-step connection for a gateway. The router keeps its own private key; nothing here asks for router credentials. */
export function Enrollment({ id, onVerified }: { id: string; onVerified: () => void }) {
  const { t } = useI18n();
  const [step, setStep] = useState<StepNo>(1);
  const [reached, setReached] = useState<StepNo>(1);
  const [prep, setPrep] = useState<{ kind: "idle" | "busy" | "ready" | "fail"; script?: string }>({ kind: "idle" });
  const [recovery, setRecovery] = useState(false), [showRecovery, setShowRecovery] = useState(false);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<{ ok: true; data: Enrolled; repair: boolean } | { ok: false; key: string } | null>(null);
  const [fin, setFin] = useState<{ busy: boolean; key?: string; ok?: boolean }>({ busy: false });
  const head = useRef<HTMLHeadingElement>(null);
  const first = useRef(true);
  const eid = encodeURIComponent(id);

  // Move focus to the new step's heading so keyboard and screen-reader users land on it.
  useEffect(() => { if (first.current) { first.current = false; return; } head.current?.focus(); }, [step]);
  const go = (s: StepNo) => { setStep(s); setReached((r) => (s > r ? s : r)); };

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
    <div className="stack enroll wiz">
      <Rail step={step} reached={reached} onGo={setStep} />

      {step === 1 && (
        <section className="stack" aria-labelledby={"wiz-h-" + id}>
          <h3 id={"wiz-h-" + id} ref={head} tabIndex={-1}>{t("hub.w.t1")}</h3>
          <p className="soft small">{t("hub.w.b1")}</p>
          <Where />
          <button className={prep.kind === "ready" ? "pill" : "btn"} type="button" disabled={prep.kind === "busy"} onClick={() => void prepare()}>{prep.kind === "ready" ? t("hub.w.again1") : t("hub.w.make1")}</button>
          {prep.kind === "busy" && <p className="soft small" role="status">{t("hub.preparing")}</p>}
          {prep.kind === "fail" && <Note kind="warn">{t("hub.prepFail")}</Note>}
          {prep.kind === "ready" && prep.script && (
            <>
              <Script title={t("hub.prepTitle")} text={prep.script} />
              <Note>{t("hub.w.expect")}</Note>
              <button className="btn" type="button" onClick={() => go(2)}>{t("hub.w.pasted")}</button>
            </>
          )}
        </section>
      )}

      {step === 2 && (
        <section className="stack" aria-labelledby={"wiz-h-" + id}>
          <h3 id={"wiz-h-" + id} ref={head} tabIndex={-1}>{t("hub.w.t2")}</h3>
          <p className="soft small">{t("hub.w.b2")}</p>
          <form className="stack" onSubmit={enroll}>
            <label className="field"><span>{t("hub.port")}</span><input className="input" name="port" type="number" min={1} max={65535} defaultValue={443} required /></label>
            <label className="check"><span>{t("hub.authorize")}</span><input type="checkbox" name="confirm" required /></label>
            {showRecovery && <label className="check"><span>{t("hub.repair")}</span><input type="checkbox" name="resume" checked={recovery} onChange={(e) => setRecovery(e.target.checked)} /></label>}
            <button className="btn" type="submit" disabled={busy}>{busy ? t("hub.creating") : result?.ok ? t("hub.w.again2") : t("hub.w.make2")}</button>
          </form>
          {result && !result.ok && <Note kind="warn">{t(result.key)}</Note>}
          {result?.ok && (
            <div className="stack" role="status" aria-live="polite">
              <Note kind="ok">{result.repair ? t("hub.repairReady") : t("hub.scriptReady")}</Note>
              <Where />
              <p className="soft small">{result.repair ? t("hub.repairBody") : t("hub.w.scriptBody")}</p>
              <Script title={t("hub.setupTitle")} text={result.data.router_script ?? ""} />
              <Note kind="warn">{t("hub.w.once")}</Note>
              <details className="wiz-where"><summary>{t("hub.w.details")}</summary>
                <dl className="facts">
                  <dt>{t("hub.addr")}</dt><dd><code>{result.data.tunnel_address}</code></dd>
                  <dt>{t("hub.pubkey")}</dt><dd><code>{result.data.hub_public_key || t("hub.notReported")}</code></dd>
                  <dt>{t("hub.endpoint")}</dt><dd><code>{result.data.endpoint || t("hub.notReported")}</code></dd>
                  <dt>{t("hub.allowed")}</dt><dd><code>{Array.isArray(result.data.allowed_ips) ? result.data.allowed_ips.join(", ") : t("hub.notReported")}</code></dd>
                </dl>
              </details>
              <p className="soft small">{t("hub.keepPrivate")}</p>
              <button className="btn" type="button" onClick={() => go(3)}>{t("hub.w.pasted")}</button>
            </div>
          )}
        </section>
      )}

      {step === 3 && (
        <section className="stack" aria-labelledby={"wiz-h-" + id}>
          <h3 id={"wiz-h-" + id} ref={head} tabIndex={-1}>{t("hub.w.t3")}</h3>
          <p className="soft small">{t("hub.w.b3")}</p>
          <button className="btn" type="button" disabled={fin.busy || fin.ok} onClick={() => void finish()}>{fin.busy ? t("hub.checking") : fin.key && !fin.ok ? t("hub.w.checkAgain") : t("hub.w.check")}</button>
          {fin.key && <Note kind={fin.ok ? "ok" : "warn"}>{t(fin.key)}</Note>}
          {fin.key && !fin.ok && (
            <div className="stack" role="group" aria-label={t("hub.w.help")}>
              <b>{t("hub.w.help")}</b>
              <ul className="wiz-list">
                <li>{t("hub.w.h1")}</li><li>{t("hub.w.h2")}</li><li>{t("hub.w.h3")}</li><li>{t("hub.w.h4")}</li>
              </ul>
              <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
                <button className="pill" type="button" onClick={() => go(2)}>{t("hub.w.back2")}</button>
                <button className="pill" type="button" onClick={() => { setPrep({ kind: "idle" }); setResult(null); setFin({ busy: false }); setReached(1); setStep(1); }}>{t("hub.w.restart")}</button>
              </div>
            </div>
          )}
        </section>
      )}
    </div>
  );
}
