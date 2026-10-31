import { useState } from "react";
import { useI18n } from "../i18n";
import { BackLink, Note } from "../components/ui";
import type { Me } from "../lib/types";

type T = (key: string) => string;
// Each entry reads its text through the translate function with a fixed key, so the key test sees every label.
const GUIDE: Array<{ href: string; title: (t: T) => string; desc: (t: T) => string; dev?: boolean }> = [
  { href: "#/revenue", title: (t) => t("title.revenue"), desc: (t) => t("gd.revenue") },
  { href: "#/vouchers", title: (t) => t("title.vouchers"), desc: (t) => t("gd.vouchers") },
  { href: "#/vouchers/guest", title: (t) => t("v.ws.guest"), desc: (t) => t("gd.guest") },
  { href: "#/router", title: (t) => t("title.router"), desc: (t) => t("gd.router") },
  { href: "#/sites", title: (t) => t("title.sites"), desc: (t) => t("gd.sites") },
  { href: "#/hub", title: (t) => t("hub.title"), desc: (t) => t("gd.hub") },
  { href: "#/network", title: (t) => t("title.network"), desc: (t) => t("gd.network") },
  { href: "#/devices", title: (t) => t("dv.title"), desc: (t) => t("gd.devices") },
  { href: "#/guests", title: (t) => t("gu.title"), desc: (t) => t("gd.guests") },
  { href: "#/topology", title: (t) => t("tp.title"), desc: (t) => t("gd.topology") },
  { href: "#/fleet", title: (t) => t("fl.title"), desc: (t) => t("gd.fleet") },
  { href: "#/incidents", title: (t) => t("inc.title"), desc: (t) => t("gd.incidents") },
  { href: "#/security", title: (t) => t("sec.title"), desc: (t) => t("gd.security") },
  { href: "#/backups", title: (t) => t("bk.title"), desc: (t) => t("gd.backups") },
  { href: "#/events", title: (t) => t("ev.title"), desc: (t) => t("gd.events") },
  { href: "#/jellyfin", title: (t) => t("title.jellyfin"), desc: (t) => t("gd.jellyfin") },
  { href: "#/store", title: (t) => t("title.store"), desc: (t) => t("gd.store") },
  { href: "#/profile", title: (t) => t("pf.title"), desc: (t) => t("gd.profile") },
  { href: "#/platform", title: (t) => t("title.platform"), desc: (t) => t("gd.platform"), dev: true },
];

export function Guide({ me }: { me: Me }) {
  const { t } = useI18n();
  const [q, setQ] = useState("");
  const isDev = me.user?.user_role === "DEVELOPER";
  const needle = q.trim().toLowerCase();
  const shown = GUIDE.filter((g) => (!g.dev || isDev) && (!needle || (g.title(t) + " " + g.desc(t)).toLowerCase().includes(needle)));
  return (
    <>
      <BackLink href="#/more">{t("tab.more")}</BackLink>
      <h1>{t("gd.title")}</h1><p className="soft">{t("gd.lead")}</p>
      <input className="input" type="search" placeholder={t("gd.search")} aria-label={t("gd.search")} value={q} onChange={(e) => setQ(e.target.value)} />
      {!shown.length && <Note>{t("gd.none")}</Note>}
      <div className="stack">{shown.map((g) => <a key={g.href} className="rowitem menu-link" href={g.href}><span className="stack" style={{ gap: 2 }}><b>{g.title(t)}</b><span className="soft small">{g.desc(t)}</span></span><span aria-hidden="true" className="soft">›</span></a>)}</div>
    </>
  );
}
