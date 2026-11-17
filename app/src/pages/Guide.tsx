import { useState } from "react";
import { useI18n } from "../i18n";
import { BackLink, Note, PageHead } from "../components/ui";
import type { Me } from "../lib/types";

type T = (key: string) => string;
// Each entry reads its text through the translate function with a fixed key, so the key test sees every label.
const GUIDE: Array<{ href: string; title: (t: T) => string; desc: (t: T) => string; dev?: boolean }> = [
  { href: "/app/revenue", title: (t) => t("title.revenue"), desc: (t) => t("gd.revenue") },
  { href: "/app/vouchers", title: (t) => t("title.vouchers"), desc: (t) => t("gd.vouchers") },
  { href: "/app/vouchers/guest", title: (t) => t("v.ws.guest"), desc: (t) => t("gd.guest") },
  { href: "/app/router", title: (t) => t("title.router"), desc: (t) => t("gd.router") },
  { href: "/app/sites", title: (t) => t("title.sites"), desc: (t) => t("gd.sites") },
  { href: "/app/hub", title: (t) => t("hub.title"), desc: (t) => t("gd.hub") },
  { href: "/app/network", title: (t) => t("title.network"), desc: (t) => t("gd.network") },
  { href: "/app/devices", title: (t) => t("dv.title"), desc: (t) => t("gd.devices") },
  { href: "/app/guests", title: (t) => t("gu.title"), desc: (t) => t("gd.guests") },
  { href: "/app/topology", title: (t) => t("tp.title"), desc: (t) => t("gd.topology") },
  { href: "/app/fleet", title: (t) => t("fl.title"), desc: (t) => t("gd.fleet") },
  { href: "/app/incidents", title: (t) => t("inc.title"), desc: (t) => t("gd.incidents") },
  { href: "/app/security", title: (t) => t("sec.title"), desc: (t) => t("gd.security") },
  { href: "/app/backups", title: (t) => t("bk.title"), desc: (t) => t("gd.backups") },
  { href: "/app/events", title: (t) => t("ev.title"), desc: (t) => t("gd.events") },
  { href: "/app/jellyfin", title: (t) => t("title.jellyfin"), desc: (t) => t("gd.jellyfin") },
  { href: "/app/store", title: (t) => t("title.store"), desc: (t) => t("gd.store") },
  { href: "/app/profile", title: (t) => t("pf.title"), desc: (t) => t("gd.profile") },
  { href: "/app/platform", title: (t) => t("title.platform"), desc: (t) => t("gd.platform"), dev: true },
];

export function Guide({ me }: { me: Me }) {
  const { t } = useI18n();
  const [q, setQ] = useState("");
  const isDev = me.user?.user_role === "DEVELOPER";
  const needle = q.trim().toLowerCase();
  const shown = GUIDE.filter((g) => (!g.dev || isDev) && (!needle || (g.title(t) + " " + g.desc(t)).toLowerCase().includes(needle)));
  return (
    <>
      <BackLink href="/app/more">{t("tab.more")}</BackLink>
      <PageHead title={t("gd.title")} help={t("gd.lead")} />
      <input className="input workspace-rail" type="search" placeholder={t("gd.search")} aria-label={t("gd.search")} value={q} onChange={(e) => setQ(e.target.value)} />
      {!shown.length && <Note>{t("gd.none")}</Note>}
      <div className="workspace-cards workspace-main">{shown.map((g) => <a key={g.href} className="rowitem menu-link" href={g.href}><span className="stack" style={{ gap: 2 }}><b>{g.title(t)}</b><span className="soft small">{g.desc(t)}</span></span><span aria-hidden="true" className="soft">›</span></a>)}</div>
    </>
  );
}
