import useSWR from "swr";
import { api, fetcher } from "../lib/api";
import type { Me } from "../lib/types";
import { Icon, type IconName } from "../components/icons";
import { useI18n } from "../i18n";
import { LangSwitch, ThemeToggle } from "../components/Shell";

/** Pages that live outside the five main tabs. Platform tools show only to the platform operator. */
type Gate = "dev" | "staff" | "nopass";
type Group = "services" | "business" | "operations" | "tools" | "account";
const LINKS: Array<{ href: string; key: string; icon: IconName; group: Group; dev?: boolean; gate?: Gate }> = [
  { href: "#/setup", key: "nav.setup", icon: "rocket", group: "services" }, { href: "#/hotspot", key: "nav.hotspot", icon: "wifi", group: "services" }, { href: "#/isp", key: "nav.isp", icon: "isp", group: "services" }, { href: "#/customers", key: "nav.customers", icon: "users", group: "services" },
  { href: "#/revenue", key: "title.revenue", icon: "cash", group: "business" }, { href: "#/jellyfin", key: "title.jellyfin", icon: "play", group: "business" }, { href: "#/store", key: "title.store", icon: "bag", group: "business" },
  { href: "#/approvals", key: "nav.approvals", icon: "approve", group: "operations" },
  { href: "#/sites", key: "title.sites", icon: "pin", group: "operations" }, { href: "#/hub", key: "hub.title", icon: "link", group: "operations" }, { href: "#/network", key: "title.network", icon: "nodes", group: "operations" }, { href: "#/topology", key: "tp.title", icon: "nodes", group: "operations" }, { href: "#/devices", key: "dv.title", icon: "device", group: "operations" }, { href: "#/managed-devices", key: "dm.title", icon: "device", group: "operations" }, { href: "#/guests", key: "gu.title", icon: "users", group: "operations" }, { href: "#/fleet", key: "fl.title", icon: "pulse", group: "operations" }, { href: "#/incidents", key: "inc.title", icon: "alert", group: "operations" }, { href: "#/security", key: "sec.title", icon: "shield", group: "operations" }, { href: "#/backups", key: "bk.title", icon: "save", group: "operations" }, { href: "#/events", key: "ev.title", icon: "list", group: "operations" },
  { href: "#/scans", key: "dev.sc.title", icon: "scan", group: "tools" }, { href: "#/templates", key: "dev.tpl.title", icon: "template", group: "tools", gate: "staff" }, { href: "#/termlog", key: "dev.tl.title", icon: "terminal", group: "tools", gate: "staff" }, { href: "#/aiusage", key: "dev.ai.title", icon: "spark", group: "tools", gate: "staff" }, { href: "#/coins", key: "dev.co.title", icon: "coins", group: "tools", gate: "nopass" }, { href: "#/unlockkeys", key: "dev.uk.title", icon: "key", group: "tools", gate: "nopass" }, { href: "#/team", key: "dev.tm.title", icon: "users", group: "tools", gate: "dev" }, { href: "#/i18n", key: "dev.tr.title", icon: "globe", group: "tools", gate: "dev" },
  { href: "#/profile", key: "pf.title", icon: "user", group: "account" }, { href: "#/guide", key: "gd.title", icon: "book", group: "account" }, { href: "#/platform", key: "title.platform", icon: "grid", group: "account", dev: true },
];
const GROUPS: Group[] = ["services", "business", "operations", "tools", "account"];

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
      <section className="glass pad stack" aria-label={t("more.display")}>
        <div className="row"><b>{t("more.display")}</b><div className="tools"><ThemeToggle /><LangSwitch /></div></div>
      </section>
      {GROUPS.map((g) => {
        const items = LINKS.filter((l) => l.group === g && allowed(l));
        if (!items.length) return null;
        return (
          <section className="stack" key={g} aria-label={t("more.g." + g)}>
            <h2 className="group-title">{t("more.g." + g)}</h2>
            <nav className="tiles">
              {items.map((l) => <a key={l.href} className="tile" href={l.href}><span className="tile-ico"><Icon name={l.icon} /></span><b>{t(l.key)}</b></a>)}
            </nav>
          </section>
        );
      })}
      <section className="glass pad stack">
        <p className="soft small">{t("more.signedIn")}</p>
        <p><b>{me.user?.full_name || me.user?.email}</b></p>
        {me.user?.full_name ? <p className="soft small">{me.user.email}</p> : null}
        <button className="btn quiet" type="button" onClick={async () => { try { await api.post("/auth/logout", {}); } catch { /* session may already be gone */ } onSignedOut(); }}>{t("c.signOut")}</button>
      </section>
    </>
  );
}
