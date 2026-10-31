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
    case "store": page = <Store me={me} />; tab = "more"; break;
    case "platform": page = <Platform me={me} />; tab = "more"; break;
    case "vouchers": page = <Vouchers sub={route.sub} />; break;
    case "more": page = <More me={me} onSignedOut={signedOut} />; break;
    case "home": page = <Home me={me} />; break;
    default: page = <Soon legacy={"#" + route.name} />; tab = "more";
  }
  return <Shell active={tab}>{page}</Shell>;
}
