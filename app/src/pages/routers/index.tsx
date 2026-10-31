import useSWR from "swr";
import { fetcher } from "../../lib/api";
import { asArray, type Router } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note, WorkspaceTabs, BackLink } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { Detail } from "./Detail";
import { NetworkMap } from "./NetworkMap";

export interface RouterRow extends Router { is_online?: boolean; transport?: string; last_seen_at?: string }

export function Routers({ sub, params }: { sub: string; params: URLSearchParams }) {
  const { t } = useI18n();
  const { data, error, isLoading } = useSWR<{ routers?: RouterRow[] }>("/routers", fetcher);
  if (isLoading) return <Skeleton h={260} />;
  if (error) return <Note kind="bad">{t("c.loadFailed")}</Note>;
  const list = asArray<RouterRow>(data?.routers);
  const onMap = sub === "map";
  if (sub && !onMap) return <><BackLink href="#/router">{t("r.ws.list")}</BackLink><Detail id={sub} router={list.find((d) => d.device_id === sub)} /></>;

  const tabs = <WorkspaceTabs items={[{ href: "#/router", label: t("r.ws.list"), current: !onMap }, { href: "#/router/map", label: t("r.ws.map"), current: onMap }]} />;
  if (onMap) {
    const gateways = list.filter((d) => d.deployment_layer === "MAIN_GATEWAY");
    return <>{tabs}{gateways.length ? <NetworkMap gateways={gateways} selected={params.get("id")} /> : <Note kind="warn">{t("v.noGateway")}</Note>}</>;
  }
  return (
    <>
      <h1>{t("title.router")}</h1>
      {tabs}
      {!list.length && <Note>{t("r.empty")}</Note>}
      <div className="router-grid">
        {list.map((d) => (
          <a className="glass pad router-card" key={d.device_id} href={"#/router/" + encodeURIComponent(d.device_id)}>
            <div className="row"><h2>{d.hardware_name || d.device_id}</h2><span className={"chip " + (d.is_online ? "ok" : "bad")}>{d.is_online ? t("c.online") : t("c.offline")}</span></div>
            <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
              <span className="chip">{d.transport === "wireguard" ? "Magic Hub" : d.transport || "—"}</span>
              {d.deployment_layer === "MAIN_GATEWAY" && <span className="chip">Gateway</span>}
            </div>
            <p className="soft small">{t("r.lastSeen")}: {d.last_seen_at && !Number.isNaN(new Date(d.last_seen_at).getTime()) ? new Date(d.last_seen_at).toLocaleString() : t("r.never")}</p>
          </a>
        ))}
      </div>
      <section className="glass pad stack"><h2>{t("r.tools")}</h2><p className="soft small">{t("r.toolsHelp")}</p><div className="scroll-x"><a className="pill" href="#/hub">{t("hub.title")}</a><a className="pill" href="#/network">{t("title.network")}</a><a className="pill" href="#/topology">{t("tp.title")}</a></div></section>
    </>
  );
}
