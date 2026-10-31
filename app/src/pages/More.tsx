import { api } from "../lib/api";
import type { Me } from "../lib/types";
import { useI18n } from "../i18n";

export function More({ me, onSignedOut }: { me: Me; onSignedOut: () => void }) {
  const { t } = useI18n();
  return (
    <>
      <h1>{t("more.title")}</h1>
      <section className="glass pad stack">
        <p className="soft small">{t("more.signedIn")}</p>
        <p><b>{me.user?.full_name || me.user?.email}</b></p>
        {me.user?.full_name ? <p className="soft small">{me.user.email}</p> : null}
        <a className="btn quiet" href="/app.html">{t("more.legacy")}</a>
        <button className="btn quiet" type="button" onClick={async () => { try { await api.post("/auth/logout", {}); } catch { /* session may already be gone */ } onSignedOut(); }}>{t("c.signOut")}</button>
      </section>
    </>
  );
}
