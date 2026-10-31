import { useCallback, useEffect, useState } from "react";
import { useSWRConfig } from "swr";
import { api } from "./lib/api";
import { useRoute } from "./lib/route";
import type { Me } from "./lib/types";
import { useI18n } from "./i18n";
import { Shell } from "./components/Shell";
import { SignIn } from "./pages/SignIn";
import { Home } from "./pages/Home";
import { Revenue } from "./pages/Revenue";
import { More } from "./pages/More";
import { Soon } from "./pages/Soon";
import { Vouchers } from "./pages/vouchers";
import { Routers } from "./pages/routers";
import { Jellyfin } from "./pages/Jellyfin";
import { Store } from "./pages/Store";
import { Platform } from "./pages/Platform";
import { Fleet } from "./pages/Fleet";
import { Guests } from "./pages/Guests";
import { Sites } from "./pages/Sites";
import { Hub } from "./pages/hub";
import { Network } from "./pages/network";
import { Incidents } from "./pages/Incidents";
import { Events } from "./pages/Events";
import { Security } from "./pages/Security";
import { Backups } from "./pages/Backups";
import { Topology } from "./pages/Topology";
import { Devices } from "./pages/Devices";
import { ProfilePage } from "./pages/Profile";
import { Guide } from "./pages/Guide";
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
    case "revenue": page = <Revenue />; tab = "home"; break;
    case "router": page = <Routers sub={route.sub} params={route.params} />; break;
    case "jellyfin": page = <Jellyfin sub={route.sub} />; break;
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
    case "hub": page = <Hub me={me} />; tab = "more"; break;
    case "store": page = <Store me={me} />; tab = "more"; break;
    case "platform": page = <Platform me={me} />; tab = "more"; break;
    case "vouchers": page = <Vouchers sub={route.sub} />; break;
    case "more": page = <More me={me} onSignedOut={signedOut} />; break;
    case "home": page = <Home me={me} />; break;
    default: page = <Soon legacy={"#" + route.name} />; tab = "more";
  }
  return <Shell active={tab}>{page}</Shell>;
}
