import { Fragment, useEffect, useRef, useState, type FormEvent } from "react";
import { api, ApiError, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { enrollError } from "./messages";
import { splitCidr } from "../../lib/age";
import { isWifiUplink, validWifi, withNewRouterNetwork, type Uplink } from "./newRouterNetwork";
import { findProfile, profileName, radioFor, routerProfiles, type WifiBand } from "./routerProfiles";

interface Enrolled { state?: string; router_configuration_required?: boolean; one_paste?: boolean; automatic_management?: boolean; router_script?: string; tunnel_address?: string; hub_public_key?: string; endpoint?: string; allowed_ips?: string[] }
interface Live { handshake_status?: string; connection?: { management?: string } }
const okScript = (s: unknown): s is string => typeof s === "string" && s.trim() !== "" && s.length <= 32768;
type StepNo = 1 | 2 | 3;
const STEPS: StepNo[] = [1, 2, 3];
const POLL_MS = 5000, SLOW_MS = 120000;

export function Script({ title, text }: { title: string; text: string }) {
  const { t } = useI18n();
  const area = useRef<HTMLTextAreaElement>(null);
  const [label, setLabel] = useState<"copy" | "copied" | "manual">("copy");
  // Some WebFig paste shortcuts send a stray character before the clipboard text.
  // Keep that character on its own line so the script still starts with its opening brace.
  const pasteText = text.startsWith("\n") ? text : "\n" + text;
  async function copy() {
    try { await navigator.clipboard.writeText(pasteText); setLabel("copied"); } catch { area.current?.focus(); area.current?.select(); setLabel("manual"); }
  }
  return (
    <div className="cmd">
      <div className="cmd-bar">
        <b>{title}</b>
        <button className="pill" type="button" data-done={label === "copied" ? "true" : undefined} onClick={() => void copy()}>{t(label === "copy" ? "hub.copy" : label === "copied" ? "hub.copied" : "hub.copyManual")}</button>
      </div>
      <textarea ref={area} className="input script" rows={9} readOnly spellCheck={false} aria-label={title} value={pasteText} />
    </div>
  );
}

function Rail({ step }: { step: StepNo }) {
  const { t } = useI18n();
  return (
    <ol className="wiz-rail" aria-label={t("hub.w.progress")}>
      {STEPS.map((n) => {
        const state = n === step ? "current" : n < step ? "done" : "todo";
        return (
          <li key={n} data-state={state}>
            <span className="wiz-dot" aria-current={n === step ? "step" : undefined}>
              <span className="wiz-num" aria-hidden="true">{state === "done" ? "✓" : n}</span>
              <span className="wiz-name">{t("hub.w.s" + n)}</span>
            </span>
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

/** Keep existing settings (default), or prepare a factory-default router of a listed model. The model list is routerProfiles.ts. */
function NetworkSetup() {
  const { t } = useI18n();
  const [uplink, setUplink] = useState<"existing" | Uplink>("existing");
  const [modelId, setModelId] = useState("");
  const profile = findProfile(modelId);
  const fresh = uplink !== "existing";
  const wifi = uplink === "5ghz" || uplink === "2ghz";
  // A model without a radio on a band cannot use that Wi-Fi option.
  const bandOk = (band: WifiBand) => !profile || radioFor(profile, band) !== undefined;
  function pickModel(id: string) {
    setModelId(id);
    const next = findProfile(id);
    if (next && (uplink === "5ghz" || uplink === "2ghz") && !radioFor(next, uplink)) setUplink("ethernet");
  }
  return (
    <div className="stack">
      <label className="stack">
        <span>{t("hub.net.setup")}</span>
        <select className="input" name="uplink" value={uplink} onChange={e => setUplink(e.target.value as typeof uplink)}>
          <option value="existing">{t("hub.net.existing")}</option>
          <option value="ethernet">{t("hub.net.ethernet")}</option>
          <option value="5ghz" disabled={!bandOk("5ghz")}>{t("hub.net.wifi5")}</option>
          <option value="2ghz" disabled={!bandOk("2ghz")}>{t("hub.net.wifi24")}</option>
        </select>
      </label>
      {fresh && <>
        <label className="stack">
          <span>{t("hub.net.model")}</span>
          <select className="input" name="model" value={modelId} onChange={e => pickModel(e.target.value)} required>
            <option value="">{t("hub.net.modelPick")}</option>
            {routerProfiles.map(p => <option key={p.id} value={p.id}>{profileName(p)}</option>)}
          </select>
        </label>
        {profile && (
          <dl className="spec">
            <dt>{t("hub.net.wanPort")}</dt><dd><code className="num">{profile.wan}</code></dd>
            {profile.radios.map(r => <Fragment key={r.name}><dt>{t(r.band === "5ghz" ? "hub.net.radio5" : "hub.net.radio24")}</dt><dd><code className="num">{r.name}</code></dd></Fragment>)}
            <dt>{t("hub.net.board")}</dt><dd><code className="num">{profile.boardModel}</code></dd>
          </dl>
        )}
        <p className="soft small">{t("hub.net.modelCheck")}</p>
        <p className="soft small">{t("hub.net.preview")}</p>
        <p className="soft small">{t(wifi ? "hub.net.wifiPreview" : "hub.net.ethernetPreview")}</p>
        <Note kind="warn">{t("hub.net.safe")}</Note>
      </>}
      {fresh && wifi && <>
        <label className="stack"><span>{t("hub.net.ssid")}</span><input className="input" name="ssid" maxLength={32} autoComplete="off" required /></label>
        <label className="stack"><span>{t("hub.net.password")}</span><input className="input" name="wifiPassphrase" type="password" minLength={8} maxLength={63} autoComplete="new-password" required /></label>
        <p className="soft small">{t("hub.net.private")}</p>
      </>}
    </div>
  );
}

/** One paste connects the router: the app finishes by itself, even if this page is closed. Nothing here asks for router credentials. */
export function Enrollment({ id, onVerified }: { id: string; onVerified: () => void }) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<{ ok: true; data: Enrolled } | { ok: false; key: string } | null>(null);
  const [networkSetup, setNetworkSetup] = useState(false);
  const [connected, setConnected] = useState(false), [reached, setReached] = useState(false), [slow, setSlow] = useState(false);
  const head = useRef<HTMLHeadingElement>(null);
  const first = useRef(true);
  const pending = useRef(false);
  const verified = useRef(onVerified);
  useEffect(() => { verified.current = onVerified; }, [onVerified]);
  const eid = encodeURIComponent(id);
  const waiting = result?.ok === true && !connected;
  const step: StepNo = connected ? 3 : result?.ok ? 2 : 1;

  // Move focus to the new step's heading so keyboard and screen-reader users land on it.
  useEffect(() => { if (first.current) { first.current = false; return; } head.current?.focus(); }, [step]);

  // The server connects the router as soon as it reaches the Hub. This only watches for that.
  useEffect(() => {
    if (!waiting) return;
    let live = true, timer: ReturnType<typeof setTimeout>;
    const started = Date.now();
    const poll = async () => {
      try {
        const r = await api.get<Live>(`/vps/peers/${eid}/live`);
        if (!live) return;
        if (r.connection?.management === "saved") { setConnected(true); verified.current(); return; }
        setReached(r.handshake_status === "recent_handshake");
      } catch (err) {
        if (isUnauthorized(err)) { location.reload(); return; }
      }
      if (!live) return;
      setSlow(Date.now() - started >= SLOW_MS);
      timer = setTimeout(() => void poll(), POLL_MS);
    };
    timer = setTimeout(() => void poll(), POLL_MS);
    return () => { live = false; clearTimeout(timer); };
  }, [waiting, eid]);

  async function enroll(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (pending.current) return;
    const form = new FormData(e.currentTarget);
    if (form.get("confirm") !== "on") { setResult({ ok: false, key: "hub.confirmNeeded" }); return; }
    const choice = form.get("uplink");
    const uplink: Uplink | null = choice === "ethernet" || choice === "5ghz" || choice === "2ghz" ? choice : null;
    const profile = findProfile(String(form.get("model") ?? ""));
    const ssid = String(form.get("ssid") ?? ""), passphrase = String(form.get("wifiPassphrase") ?? "");
    // Check everything before the enrollment request, so a refused choice creates nothing on the server.
    if (uplink && !profile) { setResult({ ok: false, key: "hub.net.modelNeeded" }); return; }
    if (uplink && profile && isWifiUplink(uplink)) {
      if (!radioFor(profile, uplink)) { setResult({ ok: false, key: "hub.net.noRadio" }); return; }
      if (!validWifi(ssid, passphrase)) { setResult({ ok: false, key: "hub.net.invalidWifi" }); return; }
    }
    pending.current = true; setBusy(true); setResult(null);
    try {
      const r = await api.post<Enrolled>(`/vps/peers/${eid}/quick-enroll`, { management_rest_port: 443, confirm_enrollment: true });
      if (r.state !== "hub_peer_enrolled" || r.router_configuration_required !== true || r.one_paste !== true || r.automatic_management !== true || !okScript(r.router_script)) throw new Error("unverified");
      const router_script = uplink && profile ? withNewRouterNetwork(r.router_script, profile, uplink, ssid, passphrase) : r.router_script;
      setNetworkSetup(uplink !== null);
      setResult({ ok: true, data: { ...r, router_script } });
    } catch (err) {
      if (isUnauthorized(err)) throw err;
      setResult({ ok: false, key: enrollError(err instanceof ApiError ? err.status : undefined, err instanceof ApiError ? (err.data as { code?: string } | null)?.code : undefined) });
    }
    finally { pending.current = false; setBusy(false); }
  }

  return (
    <div className="stack enroll wiz">
      <Rail step={step} />

      {step === 1 && (
        <section className="stack" aria-labelledby={"wiz-h-" + id}>
          <h3 id={"wiz-h-" + id} ref={head} tabIndex={-1}>{t("hub.w.t1")}</h3>
          <p className="soft small">{t("hub.w.b1")}</p>
          <form className="stack" onSubmit={enroll}>
            <fieldset className="nolock stack" disabled={busy}>
              <NetworkSetup />
              <label className="check"><span>{t("hub.authorize")}</span><input type="checkbox" name="confirm" required /></label>
              <button className="btn" type="submit">{busy ? t("hub.creating") : t("hub.generate")}</button>
            </fieldset>
          </form>
          {result && !result.ok && <Note kind="warn">{t(result.key)}</Note>}
        </section>
      )}

      {step === 2 && result?.ok && (
        <section className="stack" aria-labelledby={"wiz-h-" + id}>
          <h3 id={"wiz-h-" + id} ref={head} tabIndex={-1}>{t("hub.w.t2")}</h3>
          <Where />
          <p className="soft small">{t("hub.w.scriptBody")}</p>
          <Script title={t("hub.setupTitle")} text={result.data.router_script ?? ""} />
          <Note kind="warn">{t(networkSetup ? "hub.net.safe" : "hub.w.once")}</Note>
          <p className="soft small">{t("hub.keepPrivate")}</p>
          <div role="status" aria-live="polite" className="link-state" data-state={reached ? "up" : "wait"}><i className="led" data-state={reached ? "up" : "wait"} aria-hidden="true" /><span>{t(reached ? "hub.w.reached" : "hub.w.waiting")}</span></div>
          {slow && (
            <div className="stack" role="group" aria-label={t("hub.w.help")}>
              <b>{t("hub.w.help")}</b>
              <ul className="wiz-list">
                <li>{t("hub.w.h1")}</li><li>{t("hub.w.h2")}</li><li>{t("hub.w.h3")}</li><li>{t("hub.w.h4")}</li>
              </ul>
            </div>
          )}
          <details className="wiz-where"><summary>{t("hub.w.details")}</summary>
            <dl className="spec">
              <dt>{t("hub.addr")}</dt><dd>{(() => { const a = splitCidr(result.data.tunnel_address); return a ? <code className="num">{a.addr}<span className="cidr">{a.cidr}</span></code> : <span className="nil">{t("hub.notReported")}</span>; })()}</dd>
              <dt>{t("hub.pubkey")}</dt><dd>{result.data.hub_public_key ? <code className="mono wrap">{result.data.hub_public_key}</code> : <span className="nil">{t("hub.notReported")}</span>}</dd>
              <dt>{t("hub.endpoint")}</dt><dd>{result.data.endpoint ? <code className="num">{result.data.endpoint}</code> : <span className="nil">{t("hub.notReported")}</span>}</dd>
              <dt>{t("hub.allowed")}</dt><dd>{Array.isArray(result.data.allowed_ips) ? <code className="num">{result.data.allowed_ips.join(", ")}</code> : <span className="nil">{t("hub.notReported")}</span>}</dd>
            </dl>
          </details>
        </section>
      )}

      {step === 3 && (
        <section className="stack" aria-labelledby={"wiz-h-" + id}>
          <h3 id={"wiz-h-" + id} ref={head} tabIndex={-1}>{t("hub.w.t3")}</h3>
          <Note kind="ok">{t("hub.fin.ok")}</Note>
        </section>
      )}
    </div>
  );
}
