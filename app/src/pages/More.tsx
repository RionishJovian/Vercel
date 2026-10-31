import useSWR from "swr";
import { api, fetcher } from "../lib/api";
import type { Me } from "../lib/types";
import { useI18n } from "../i18n";

/** Pages that live outside the five main tabs. Platform tools show only to the platform operator. */
type Gate = "dev" | "staff" | "nopass";
const LINKS: Array<{ href: string; key: string; dev?: boolean; gate?: Gate }> = [
  { href: "#/revenue", key: "title.revenue" }, { href: "#/sites", key: "title.sites" }, { href: "#/hub", key: "hub.title" }, { href: "#/network", key: "title.network" }, { href: "#/topology", key: "tp.title" }, { href: "#/devices", key: "dv.title" }, { href: "#/managed-devices", key: "dm.title" }, { href: "#/guests", key: "gu.title" }, { href: "#/incidents", key: "inc.title" }, { href: "#/security", key: "sec.title" }, { href: "#/backups", key: "bk.title" }, { href: "#/events", key: "ev.title" }, { href: "#/fleet", key: "fl.title" },
  { href: "#/store", key: "title.store" }, { href: "#/scans", key: "dev.sc.title" }, { href: "#/templates", key: "dev.tpl.title", gate: "staff" }, { href: "#/termlog", key: "dev.tl.title", gate: "staff" }, { href: "#/aiusage", key: "dev.ai.title", gate: "staff" }, { href: "#/coins", key: "dev.co.title", gate: "nopass" }, { href: "#/unlockkeys", key: "dev.uk.title", gate: "nopass" }, { href: "#/team", key: "dev.tm.title", gate: "dev" }, { href: "#/i18n", key: "dev.tr.title", gate: "dev" }, { href: "#/profile", key: "pf.title" }, { href: "#/guide", key: "gd.title" }, { href: "#/platform", key: "title.platform", dev: true },
];

export function More({ me, onSignedOut }: { me: Me; onSignedOut: () => void }) {
  const { t } = useI18n();
  const isDev = me.user?.user_role === "DEVELOPER";
  const role = me.user?.user_role ?? "";
  const sub = useSWR<{ mode?: string }>("/subscription", fetcher, { shouldRetryOnError: false });
  // Presentation only: the API still decides who may do what. Pass accounts do not see coins or keys.
  const passMode = ["subscription_only", "individual_pass"].includes(sub.data?.mode ?? "");
  const allowed = (l: (typeof LINKS)[number]) => (!l.dev || isDev) && (l.gate !== "dev" || isDev) && (l.gate !== "staff" || ["OWNER", "ADMIN", "DEVELOPER"].includes(role)) && (l.gate !== "nopass" || !passMode);
  return (
    <>
      <h1>{t("more.title")}</h1>
      <nav className="stack" aria-label={t("more.title")}>
        {LINKS.filter(allowed).map((l) => <a key={l.href} className="rowitem menu-link" href={l.href}><b>{t(l.key)}</b><span aria-hidden="true" className="soft">›</span></a>)}
      </nav>
      <section className="glass pad stack">
        <p className="soft small">{t("more.signedIn")}</p>
        <p><b>{me.user?.full_name || me.user?.email}</b></p>
        {me.user?.full_name ? <p className="soft small">{me.user.email}</p> : null}
        <button className="btn quiet" type="button" onClick={async () => { try { await api.post("/auth/logout", {}); } catch { /* session may already be gone */ } onSignedOut(); }}>{t("c.signOut")}</button>
      </section>
    </>
  );
}
