import { Suspense, lazy, useCallback, useEffect, useState } from "react";
import { useSWRConfig } from "swr";
import { api } from "./lib/api";
import { useRoute } from "./lib/route";
import type { Me } from "./lib/types";
import { useI18n } from "./i18n";
import { Shell } from "./components/Shell";
import { PageBoundary } from "./components/PageBoundary";
import { SignIn } from "./pages/SignIn";
import { Home } from "./pages/Home";
const Revenue = lazy(() => import("./pages/Revenue").then(m => ({ default: m.Revenue })));
import { More } from "./pages/More";
const Vouchers = lazy(() => import("./pages/vouchers").then(m => ({ default: m.Vouchers })));
const Routers = lazy(() => import("./pages/routers").then(m => ({ default: m.Routers })));
const Jellyfin = lazy(() => import("./pages/Jellyfin").then(m => ({ default: m.Jellyfin })));
const Store = lazy(() => import("./pages/Store").then(m => ({ default: m.Store })));
const Platform = lazy(() => import("./pages/platform").then(m => ({ default: m.Platform })));
const Fleet = lazy(() => import("./pages/Fleet").then(m => ({ default: m.Fleet })));
const Guests = lazy(() => import("./pages/Guests").then(m => ({ default: m.Guests })));
const Sites = lazy(() => import("./pages/Sites").then(m => ({ default: m.Sites })));
const Hub = lazy(() => import("./pages/hub").then(m => ({ default: m.Hub })));
const Network = lazy(() => import("./pages/network").then(m => ({ default: m.Network })));
const Incidents = lazy(() => import("./pages/Incidents").then(m => ({ default: m.Incidents })));
const Events = lazy(() => import("./pages/Events").then(m => ({ default: m.Events })));
const Security = lazy(() => import("./pages/Security").then(m => ({ default: m.Security })));
const Backups = lazy(() => import("./pages/Backups").then(m => ({ default: m.Backups })));
const Topology = lazy(() => import("./pages/Topology").then(m => ({ default: m.Topology })));
const Devices = lazy(() => import("./pages/Devices").then(m => ({ default: m.Devices })));
const ManagedDevices = lazy(() => import("./pages/devices/ManagedDevices").then(m => ({ default: m.ManagedDevices })));
const ProfilePage = lazy(() => import("./pages/Profile").then(m => ({ default: m.ProfilePage })));
const Guide = lazy(() => import("./pages/Guide").then(m => ({ default: m.Guide })));
const Templates = lazy(() => import("./pages/dev/Templates").then(m => ({ default: m.Templates })));
const TerminalLog = lazy(() => import("./pages/dev/TerminalLog").then(m => ({ default: m.TerminalLog })));
const Scans = lazy(() => import("./pages/dev/Scans").then(m => ({ default: m.Scans })));
const AIUsage = lazy(() => import("./pages/dev/AIUsage").then(m => ({ default: m.AIUsage })));
const UnlockKeys = lazy(() => import("./pages/dev/UnlockKeys").then(m => ({ default: m.UnlockKeys })));
const Translations = lazy(() => import("./pages/dev/Translations").then(m => ({ default: m.Translations })));
const Coins = lazy(() => import("./pages/dev/Coins").then(m => ({ default: m.Coins })));
const Team = lazy(() => import("./pages/dev/Team").then(m => ({ default: m.Team })));
const Setup = lazy(() => import("./pages/workspaces/Setup").then(m => ({ default: m.Setup })));
const Hotspot = lazy(() => import("./pages/workspaces/Services").then(m => ({ default: m.Hotspot })));
const Isp = lazy(() => import("./pages/workspaces/Services").then(m => ({ default: m.Isp })));
const Customers = lazy(() => import("./pages/workspaces/Services").then(m => ({ default: m.Customers })));
const NetworkHub = lazy(() => import("./pages/workspaces/Operations").then(m => ({ default: m.NetworkHub })));
const Activity = lazy(() => import("./pages/workspaces/Operations").then(m => ({ default: m.Activity })));
const Approvals = lazy(() => import("./pages/workspaces/Operations").then(m => ({ default: m.Approvals })));
import { PageSkeleton } from "./components/Skeleton";

const PAGE_TITLES: Record<string, string> = {
  home: "title.home", router: "title.router", vouchers: "title.vouchers", jellyfin: "title.jellyfin", more: "title.more", revenue: "title.revenue", store: "title.store",
  platform: "title.platform", network: "title.network", pppoe: "title.pppoe", sites: "title.sites", fleet: "fl.title", guests: "gu.title", incidents: "inc.title", events: "ev.title", security: "sec.title",
  backups: "bk.title", topology: "tp.title", devices: "dv.title", "managed-devices": "dm.title", profile: "pf.title", guide: "gd.title", hub: "hub.title", templates: "dev.tpl.title",
  termlog: "dev.tl.title", scans: "dev.sc.title", aiusage: "dev.ai.title", unlockkeys: "dev.uk.title", i18n: "dev.tr.title", coins: "dev.co.title", team: "dev.tm.title",
  setup: "su.title", hotspot: "hs.title", isp: "isp.title", customers: "cu.title", net: "nav.network", activity: "ac.title", approvals: "cfg.title",
};
const titleKey = (name: string, sub: string) => (name === "router" && sub === "map" ? "r.mapTitle" : PAGE_TITLES[name]);

/** The loading layout that matches each screen, so content replaces it without jumping. */
const skeletonFor = (name: string) => (name === "home" ? "home" : name === "setup" ? "steps" : ["hotspot", "isp", "customers", "net", "activity", "approvals"].includes(name) ? "hub" : "page");

type Auth = { state: "loading" } | { state: "out"; notice?: string } | { state: "in"; me: Me };

export function App() {
  const { t } = useI18n();
  const { mutate } = useSWRConfig();
  const route = useRoute();
  const [auth, setAuth] = useState<Auth>({ state: "loading" });

  const boot = useCallback(async () => {
    try { setAuth({ state: "in", me: await api.get<Me>("/me") }); } catch { setAuth({ state: "out" }); }
  }, []);
  useEffect(() => { void boot(); }, [boot]);
  const key = auth.state === "out" ? "title.signin" : auth.state === "in" ? titleKey(route.name, route.sub) : undefined;
  const pageTitle = key ? t(key) : "";
  useEffect(() => { document.title = pageTitle ? pageTitle + " · MegaMagic" : "MegaMagic"; }, [pageTitle]);

  if (auth.state === "loading") return <Shell route={route.name}><PageSkeleton variant={skeletonFor(route.name)} /></Shell>;
  if (auth.state === "out") return <SignIn notice={auth.notice} onDone={() => { void mutate(() => true, undefined, { revalidate: false }); void boot(); }} />;

  const { me } = auth;
  const signedOut = () => { void mutate(() => true, undefined, { revalidate: false }); setAuth({ state: "out", notice: t("c.sessionEnded") }); };
  let page: React.ReactNode;
  switch (route.name) {
    case "revenue": page = <Revenue me={me} />; break;
    case "router": page = <Routers sub={route.sub} params={route.params} me={me} />; break;
    case "jellyfin": page = <Jellyfin sub={route.sub} me={me} />; break;
    case "fleet": page = <Fleet />; break;
    case "guests": page = <Guests me={me} />; break;
    case "sites": page = <Sites me={me} />; break;
    case "pppoe": page = <Network me={me} sub="pppoe" params={route.params} />; break;
    case "network": page = <Network me={me} sub={route.sub} params={route.params} />; break;
    case "incidents": page = <Incidents me={me} />; break;
    case "events": page = <Events />; break;
    case "security": page = <Security />; break;
    case "backups": page = <Backups />; break;
    case "topology": page = <Topology me={me} />; break;
    case "devices": page = <Devices me={me} />; break;
    case "profile": page = <ProfilePage />; break;
    case "guide": page = <Guide me={me} />; break;
    case "templates": page = <Templates me={me} />; break;
    case "termlog": page = <TerminalLog />; break;
    case "scans": page = <Scans />; break;
    case "aiusage": page = <AIUsage />; break;
    case "unlockkeys": page = <UnlockKeys />; break;
    case "i18n": page = <Translations me={me} />; break;
    case "coins": page = <Coins me={me} />; break;
    case "team": page = <Team />; break;
    case "managed-devices": page = <ManagedDevices me={me} />; break;
    case "hub": page = <Hub me={me} />; break;
    case "store": page = <Store me={me} />; break;
    case "platform": page = <Platform me={me} sub={route.sub} />; break;
    case "vouchers": page = <Vouchers sub={route.sub} />; break;
    case "setup": page = <Setup />; break;
    case "hotspot": page = <Hotspot />; break;
    case "isp": page = <Isp me={me} />; break;
    case "customers": page = <Customers params={route.params} />; break;
    case "net": page = <NetworkHub me={me} routerId={route.params.get("router")} />; break;
    case "activity": page = <Activity />; break;
    case "approvals": page = <Approvals />; break;
    case "portal": location.replace("#/vouchers/guest"); page = null; break;
    case "more": page = <More me={me} onSignedOut={signedOut} />; break;
    case "home": page = <Home key={route.params.get("router")} me={me} routerId={route.params.get("router")} />; break;
    default: location.replace("#/home"); page = null;
  }
  return <Shell route={route.name} business={me.business?.company_name}><PageBoundary key={route.name + "/" + route.sub}><Suspense fallback={<PageSkeleton variant={skeletonFor(route.name)} />}>{page}</Suspense></PageBoundary></Shell>;
}
