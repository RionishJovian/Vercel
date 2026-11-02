import { useEffect, useRef, useState } from "react";
import { api, ApiError, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import type { Firmware } from "./portInfo";

interface Review { confirmation: string; current: string; target: string; steps: string[]; warnings: string[]; model?: string }
type Mode = "held" | "preview" | "true";
type Phase =
  | { k: "idle" }
  | { k: "loading" }
  | { k: "review"; review: Review }
  | { k: "applying"; review: Review }
  | { k: "restarting"; target: string; since: number }
  | { k: "done"; target: string }
  | { k: "timeout"; target: string }
  | { k: "failed"; text: string };

const POLL_MS = 5000, GIVE_UP_MS = 10 * 60 * 1000;
const serverText = (e: unknown, fallback: string) => (e instanceof ApiError ? (e.data as { error?: string } | null)?.error : undefined) || fallback;

/**
 * Board firmware upgrade. The router restarts and the change cannot be undone from the app, so
 * this only ever offers a reviewed, confirmed upgrade, and it reports success only when the router
 * itself reports the new firmware again. The server decides who may see it (a non-admin gets 403
 * from the status call and sees nothing) and whether it is switched on.
 */
export function FirmwareUpgrade({ id, fw, onDone }: { id: string; fw?: Firmware; onDone: () => void }) {
  const { t } = useI18n();
  const eid = encodeURIComponent(id);
  const [mode, setMode] = useState<Mode | null>(null);
  const [phase, setPhase] = useState<Phase>({ k: "idle" });
  const [agreed, setAgreed] = useState(false);
  const done = useRef(onDone);
  done.current = onDone;

  useEffect(() => {
    let live = true;
    api.get<{ mode?: string }>(`/routers/${eid}/firmware/status`)
      .then((r) => { if (live) setMode(r.mode === "true" || r.mode === "preview" ? r.mode : "held"); })
      .catch((e) => { if (isUnauthorized(e)) return; if (live) setMode(null); });
    return () => { live = false; };
  }, [eid]);

  // While restarting, the only proof of success is the router reporting the new firmware again.
  useEffect(() => {
    if (phase.k !== "restarting") return;
    const { target, since } = phase;
    let live = true;
    const timer = window.setInterval(async () => {
      if (Date.now() - since > GIVE_UP_MS) { if (live) setPhase({ k: "timeout", target }); return; }
      try {
        const r = await api.get<{ telemetry?: { snapshot?: { firmware?: Firmware | null } } }>(`/routers/${eid}/telemetry`);
        if (live && r.telemetry?.snapshot?.firmware?.current_firmware === target) { setPhase({ k: "done", target }); done.current(); }
      } catch (e) { if (isUnauthorized(e)) throw e; /* the router is still restarting */ }
    }, POLL_MS);
    return () => { live = false; window.clearInterval(timer); };
  }, [phase, eid]);

  const available = fw?.firmware_upgrade_available === true && !!fw.upgrade_firmware;
  if (mode === null || (phase.k === "idle" && !available)) return null;

  async function preview() {
    setPhase({ k: "loading" }); setAgreed(false);
    try { setPhase({ k: "review", review: (await api.post<{ review: Review }>(`/routers/${eid}/firmware/preview`, {})).review }); }
    catch (e) { if (isUnauthorized(e)) throw e; setPhase({ k: "failed", text: serverText(e, t("fw.previewFail")) }); }
  }
  async function apply(review: Review) {
    setPhase({ k: "applying", review });
    try {
      await api.post(`/routers/${eid}/firmware/apply`, { confirmation: review.confirmation });
      setPhase({ k: "restarting", target: review.target, since: Date.now() });
    } catch (e) { if (isUnauthorized(e)) throw e; setPhase({ k: "failed", text: serverText(e, t("fw.applyFail")) }); }
  }

  const p = phase;
  return (
    <section className="glass pad stack" aria-live="polite">
      <h2>{t("fw.title")}</h2>
      {p.k === "idle" && (mode === "held"
        ? <Note>{t("fw.held")}</Note>
        : <><p className="soft small">{fw?.current_firmware} → {fw?.upgrade_firmware}</p><button className="btn" type="button" onClick={() => void preview()}>{t("fw.upgrade")}</button></>)}
      {p.k === "loading" && <p className="soft small" role="status">{t("fw.checking")}</p>}
      {(p.k === "review" || p.k === "applying") && (
        <>
          <p><b>{p.review.current} → {p.review.target}</b>{p.review.model ? <span className="soft small"> · {p.review.model}</span> : null}</p>
          <b>{t("fw.steps")}</b>
          <ol className="wiz-list">{p.review.steps.map((s) => <li key={s}>{s}</li>)}</ol>
          <Note kind="warn"><b>{t("fw.before")}</b><ul className="wiz-list">{p.review.warnings.map((w) => <li key={w}>{w}</li>)}</ul></Note>
          {mode === "preview" ? <Note>{t("fw.previewOnly")}</Note> : (
            <label className="check"><span>{t("fw.understand")}</span><input type="checkbox" checked={agreed} onChange={(e) => setAgreed(e.target.checked)} /></label>
          )}
          <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
            <button className="pill" type="button" disabled={p.k === "applying"} onClick={() => setPhase({ k: "idle" })}>{t("c.cancel")}</button>
            {mode === "true" && <button className="btn" type="button" disabled={!agreed || p.k === "applying"} onClick={() => void apply(p.review)}>{p.k === "applying" ? t("fw.applying") : t("fw.go")}</button>}
          </div>
        </>
      )}
      {p.k === "restarting" && <Note>{t("fw.restarting")}</Note>}
      {p.k === "done" && <Note kind="ok">{t("fw.done")} {p.target}</Note>}
      {p.k === "timeout" && <Note kind="warn">{t("fw.timeout")}</Note>}
      {p.k === "failed" && <><Note kind="bad">{p.text}</Note><button className="pill" type="button" onClick={() => setPhase({ k: "idle" })}>{t("c.retry")}</button></>}
    </section>
  );
}
