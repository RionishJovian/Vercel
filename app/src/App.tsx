import { Suspense, lazy, useCallback, useEffect, useState } from "react";
import { useSWRConfig } from "swr";
import { api } from "./lib/api";
import { useRoute } from "./lib/route";
import type { Me } from "./lib/types";
import { useI18n } from "./i18n";
import { Shell } from "./components/Shell";
import { SignIn } from "./pages/SignIn";
import { Home } from "./pages/Home";
const Revenue = lazy(() => import("./pages/Revenue").then(m => ({ default: m.Revenue })));
import { More } from "./pages/More";
import { Soon } from "./pages/Soon";
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
import { Skeleton } from "./components/Skeleton";

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

  if (auth.state === "loading") return <div className="shell"><Skeleton h={240} /></div>;
  if (auth.state === "out") return <SignIn notice={auth.notice} onDone={() => { void mutate(() => true, undefined, { revalidate: false }); void boot(); }} />;

  const { me } = auth;
  const signedOut = () => { void mutate(() => true, undefined, { revalidate: false }); setAuth({ state: "out", notice: t("c.sessionEnded") }); };
  let page: React.ReactNode, tab = route.name;
  switch (route.name) {
    case "revenue": page = <Revenue me={me} />; tab = "home"; break;
    case "router": page = <Routers sub={route.sub} params={route.params} />; break;
    case "jellyfin": page = <Jellyfin sub={route.sub} me={me} />; break;
    case "fleet": page = <Fleet />; tab = "more"; break;
    case "guests": page = <Guests me={me} />; tab = "more"; break;
    case "sites": page = <Sites me={me} />; tab = "more"; break;
    case "network": page = <Network me={me} sub={route.sub} />; tab = "more"; break;
    case "incidents": page = <Incidents me={me} />; tab = "more"; break;
    case "events": page = <Events />; tab = "more"; break;
    case "security": page = <Security />; tab = "more"; break;
    case "backups": page = <Backups />; tab = "more"; break;
    case "topology": page = <Topology />; tab = "more"; break;
    case "devices": page = <Devices me={me} />; tab = "more"; break;
    case "profile": page = <ProfilePage />; tab = "more"; break;
    case "guide": page = <Guide me={me} />; tab = "more"; break;
    case "templates": page = <Templates me={me} />; tab = "more"; break;
    case "termlog": page = <TerminalLog />; tab = "more"; break;
    case "scans": page = <Scans />; tab = "more"; break;
    case "aiusage": page = <AIUsage />; tab = "more"; break;
    case "unlockkeys": page = <UnlockKeys />; tab = "more"; break;
    case "i18n": page = <Translations me={me} />; tab = "more"; break;
    case "coins": page = <Coins me={me} />; tab = "more"; break;
    case "team": page = <Team />; tab = "more"; break;
    case "managed-devices": page = <ManagedDevices me={me} />; tab = "more"; break;
    case "hub": page = <Hub me={me} />; tab = "more"; break;
    case "store": page = <Store me={me} />; tab = "more"; break;
    case "platform": page = <Platform me={me} sub={route.sub} />; tab = "more"; break;
    case "vouchers": page = <Vouchers sub={route.sub} />; break;
    case "more": page = <More me={me} onSignedOut={signedOut} />; break;
    case "home": page = <Home me={me} />; break;
    default: page = <Soon legacy={"#" + route.name} />; tab = "more";
  }
  return <Shell active={tab}><Suspense fallback={<Skeleton h={240} />}>{page}</Suspense></Shell>;
}
