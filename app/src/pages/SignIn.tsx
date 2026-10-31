import { useState, type FormEvent } from "react";
import { api, ApiError } from "../lib/api";
import { Wand } from "../components/icons";
import { Toggles } from "../components/Shell";
import { useI18n } from "../i18n";

export function SignIn({ notice, onDone }: { notice?: string; onDone: () => void }) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(notice ?? "");
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    setBusy(true); setError("");
    try { await api.post("/auth/login", { email: String(f.get("email")).trim(), password: String(f.get("password")) }); onDone(); }
    catch (err) { setError(err instanceof ApiError && err.message === "network-unreachable" ? t("s.network") : t("s.failed")); setBusy(false); }
  }
  return (
    <div className="signin">
      <div className="signin-tools"><Toggles /></div>
      <form className="glass signin-card" onSubmit={submit}>
        <div className="signin-mark"><Wand state="ok" /></div>
        <h1>Mega Magic</h1>
        <p className="soft">{t("home.sub")}</p>
        <label className="field"><span>{t("s.email")}</span><input className="input" name="email" type="email" autoComplete="username" required /></label>
        <label className="field"><span>{t("s.password")}</span><input className="input" name="password" type="password" autoComplete="current-password" required /></label>
        {error ? <p className="note bad" role="alert">{error}</p> : null}
        <button className="btn" type="submit" disabled={busy}>{busy ? t("s.working") : t("s.submit")}</button>
      </form>
      <p className="signin-create"><a className="link" href="/signin.html?register=1">{t("s.create")}</a></p>
    </div>
  );
}
