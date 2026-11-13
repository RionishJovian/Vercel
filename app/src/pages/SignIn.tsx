import { useState, type FormEvent, type KeyboardEvent } from "react";
import { api, ApiError } from "../lib/api";
import { BrandMark, Wordmark } from "../components/Brand";
import { Toggles } from "../components/Shell";
import { useI18n } from "../i18n";
import { hashToPath } from "../lib/route";

/**
 * The console's own sign-in, shown in place when a signed-out person opens any /app/ page.
 * Because it renders where they were, the page they asked for is kept and opens after sign-in.
 * Matches the public /signin page: same fields, autofill hints, show-password and Caps Lock help.
 */
export function SignIn({ notice, onDone }: { notice?: string; onDone: () => void }) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(notice ?? "");
  const [show, setShow] = useState(false);
  const [caps, setCaps] = useState(false);
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (busy) return;
    const f = new FormData(e.currentTarget);
    setBusy(true); setError("");
    try { await api.post("/auth/login", { email: String(f.get("email")).trim(), password: String(f.get("password")) }); onDone(); }
    catch (err) {
      const status = err instanceof ApiError ? err.status : 0;
      setError(status === 429 ? t("s.locked") : status === 403 ? t("s.suspended") : err instanceof ApiError && err.message === "network-unreachable" ? t("s.network") : t("s.failed"));
      setBusy(false);
    }
  }
  const capsCheck = (e: KeyboardEvent<HTMLInputElement>) => setCaps(e.getModifierState?.("CapsLock") ?? false);
  // Keep the requested console page so creating an account comes back to it.
  const next = encodeURIComponent(/^#\//.test(location.hash) ? hashToPath(location.hash) : location.pathname + location.search);
  return (
    <div className="signin">
      <div className="signin-tools"><Toggles /></div>
      <form className="glass signin-card" onSubmit={submit} noValidate={false}>
        <div className="signin-brand"><BrandMark size={72} /><h1><Wordmark /></h1><p className="soft">{t("s.tagline")}</p></div>
        {/* type="text" with an email keyboard: older logins have no domain suffix (e.g. kyaw@mikromagic). */}
        <label className="field"><span>{t("s.email")}</span><input className="input" name="email" type="text" inputMode="email" autoComplete="username" autoCapitalize="none" spellCheck={false} required autoFocus /></label>
        <div className="field">
          <label htmlFor="signin-password"><span>{t("s.password")}</span></label>
          <div className="pw">
            <input className="input" id="signin-password" name="password" type={show ? "text" : "password"} autoComplete="current-password" required onKeyDown={capsCheck} onKeyUp={capsCheck} onBlur={() => setCaps(false)} />
            <button type="button" className="pw-toggle" aria-pressed={show} aria-controls="signin-password" aria-label={show ? t("s.hidePw") : t("s.showPw")} onClick={() => setShow((s) => !s)}>
              {show
                ? <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 3l18 18M10.6 6.1A9.6 9.6 0 0 1 12 6c6 0 9.5 6 9.5 6a16 16 0 0 1-3 3.6M6.6 7.6A15.6 15.6 0 0 0 2.5 12S6 18 12 18a9 9 0 0 0 4.3-1.1M9.9 9.9a3 3 0 0 0 4.2 4.2" /></svg>
                : <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12z" /><circle cx="12" cy="12" r="3" /></svg>}
            </button>
          </div>
          {caps ? <p className="caps-note" role="status">{t("s.caps")}</p> : null}
        </div>
        {error ? <p className="note bad" role="alert">{error}</p> : null}
        <button className="btn" type="submit" disabled={busy} aria-busy={busy || undefined}>{busy ? <><span className="spin" aria-hidden="true" />{t("s.working")}</> : t("s.submit")}</button>
        <p className="signin-create"><a className="link" href={`/signin?register=1&next=${next}`}>{t("s.create")}</a></p>
        <p className="small soft signin-help">{t("s.forgot")} <a className="link" href="https://t.me/nish2769" target="_blank" rel="noopener noreferrer">Telegram</a></p>
      </form>
    </div>
  );
}
