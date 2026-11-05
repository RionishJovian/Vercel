import useSWR from "swr";
import { fetcher } from "../../lib/api";
import { asArray, type Router } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note, SectionHead, WorkspaceTabs, BackLink } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { deviceImage } from "../../lib/deviceImage";
import { Icon, type IconName } from "../../components/icons";
import { workspaceInfo, workspaces, type Workspace } from "../network/schema";
import { Detail } from "./Detail";
import { NetworkMap } from "./NetworkMap";
import { useHubStatus } from "../../lib/hubStatus";
import { HubChip } from "./HubChip";

export interface RouterRow extends Router { is_online?: boolean; transport?: string; last_seen_at?: string }

const NET_ICONS: Record<Workspace, IconName> = { wan: "globe", ip: "nodes", vlan: "grid", existing: "device", protection: "shield", qos: "pulse", wifi: "link", pppoe: "users" };

/** Internet setup is a first-class part of the Router tab, not buried under More. */
function InternetSetup() {
  const { t, tn } = useI18n();
  return (
    <section className="stack" aria-labelledby="net-setup">
      <h2 id="net-setup">{t("title.network")}</h2>
      <nav className="tiles">
        <a className="tile" href="#/network/new"><span className="tile-ico"><Icon name="pin" /></span><b>{t("nw.title")}</b></a>
        {workspaces.map((k) => <a key={k} className="tile" href={"#/network/" + k}><span className="tile-ico"><Icon name={NET_ICONS[k]} /></span><b>{tn(workspaceInfo[k][0])}</b></a>)}
      </nav>
    </section>
  );
}

const seenAt = (value: string | undefined, never: string) => (value && !Number.isNaN(new Date(value).getTime()) ? new Date(value).toLocaleString() : never);

export function Routers({ sub, params }: { sub: string; params: URLSearchParams }) {
  const { t } = useI18n();
  const { data, error, isLoading } = useSWR<{ routers?: RouterRow[] }>("/routers", fetcher);
  const hub = useHubStatus(asArray<RouterRow>(data?.routers).filter((d) => d.deployment_layer === "MAIN_GATEWAY").map((d) => d.device_id));
  if (isLoading) return <Skeleton h={260} />;
  if (error) return <Note kind="bad">{t("c.loadFailed")}</Note>;
  const list = asArray<RouterRow>(data?.routers);
  const onMap = sub === "map";
  if (sub && !onMap) return <><BackLink href="#/router">{t("r.ws.list")}</BackLink><Detail id={sub} router={list.find((d) => d.device_id === sub)} /></>;

  const tabs = <WorkspaceTabs items={[{ href: "#/router", label: t("r.ws.list"), current: !onMap }, { href: "#/router/map", label: t("r.ws.map"), current: onMap }, { href: "#/network", label: t("title.network"), current: false }]} />;
  if (onMap) {
    const gateways = list.filter((d) => d.deployment_layer === "MAIN_GATEWAY");
    return <><h1>{t("r.mapTitle")}</h1>{tabs}{gateways.length ? <NetworkMap gateways={gateways} selected={params.get("id")} hub={hub} /> : <Note kind="warn">{t("v.noGateway")}</Note>}</>;
  }
  return (
    <>
      <h1>{t("title.router")}</h1>
      {tabs}
      {list.length > 0 && <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
        {list.some((d) => d.deployment_layer === "MAIN_GATEWAY") && <a className="pill" href="#/hub">{t("hub.delete")} · {t("hub.title")}</a>}
        {list.some((d) => d.deployment_layer !== "MAIN_GATEWAY") && <a className="pill" href="#/managed-devices">{t("hub.delete")} · {t("dm.title")}</a>}
      </div>}
      {!list.length && <Note>{t("r.empty")}</Note>}
      <div className="router-grid">
        {list.map((d) => (
          <a className="glass pad router-card" key={d.device_id} href={"#/router/" + encodeURIComponent(d.device_id)}>
            <div className={"rc-pic" + (hub[d.device_id]?.state === "up" ? " on" : "")}><img src={deviceImage(d)} alt="" width={64} height={64} loading="lazy" /><i className="rc-led" aria-hidden="true" /></div>
            <div className="rc-body">
              <div className="row"><h2>{d.hardware_name || d.device_id}</h2><HubChip status={hub[d.device_id]} /></div>
              <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
                <span className="chip">{d.transport === "wireguard" ? "Magic Hub" : d.transport || "—"}</span>
                {d.deployment_layer === "MAIN_GATEWAY" && <span className="chip">Gateway</span>}
              </div>
              <p className="soft small">{hub[d.device_id]?.at ? t("r.hubSeen") : t("r.lastSeen")}: {seenAt(hub[d.device_id]?.at ?? d.last_seen_at, t("r.never"))}</p>
            </div>
          </a>
        ))}
      </div>
      <InternetSetup />
      <section className="glass pad stack"><SectionHead title={t("r.tools")} help={t("r.toolsHelp")} /><div className="scroll-x"><a className="pill" href="#/hub">{t("hub.title")}</a><a className="pill" href="#/network">{t("title.network")}</a><a className="pill" href="#/topology">{t("tp.title")}</a></div></section>
    </>
  );
}
