import { api } from "../lib/api";
import type { Me } from "../lib/types";
import { useI18n } from "../i18n";

/** Pages that live outside the five main tabs. Platform tools show only to the platform operator. */
const LINKS: Array<{ href: string; key: string; dev?: boolean }> = [
  { href: "#/revenue", key: "title.revenue" }, { href: "#/sites", key: "title.sites" }, { href: "#/hub", key: "hub.title" }, { href: "#/network", key: "title.network" }, { href: "#/guests", key: "gu.title" }, { href: "#/fleet", key: "fl.title" },
  { href: "#/store", key: "title.store" }, { href: "#/platform", key: "title.platform", dev: true },
];

export function More({ me, onSignedOut }: { me: Me; onSignedOut: () => void }) {
  const { t } = useI18n();
  const isDev = me.user?.user_role === "DEVELOPER";
  return (
    <>
      <h1>{t("more.title")}</h1>
      <nav className="stack" aria-label={t("more.title")}>
        {LINKS.filter((l) => !l.dev || isDev).map((l) => <a key={l.href} className="rowitem menu-link" href={l.href}><b>{t(l.key)}</b><span aria-hidden="true" className="soft">›</span></a>)}
      </nav>
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
