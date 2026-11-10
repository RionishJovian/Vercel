import useSWR from "swr";
import { fetcher } from "../../lib/api";
import { asArray, type Router, type Me } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note, SectionHead, WorkspaceTabs, BackLink } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { deviceImage } from "../../lib/deviceImage";
import { Detail } from "./Detail";
import { NetworkMap } from "./NetworkMap";
import { hubStatusFrom, type HubStatus } from "../../lib/hubStatus";
import { useFleet, type FleetData } from "../../lib/useFleet";
import { HubChip } from "./HubChip";

export interface RouterRow extends Router { site_id?: string | null; is_online?: boolean; transport?: string; last_seen_at?: string }

// Once the status has loaded, a router it does not list is "unavailable", not "checking".
const hubStatusFromFleet = (fleet: FleetData | undefined, routers: RouterRow[]): Record<string, HubStatus> => ({ ...Object.fromEntries(routers.map((d): [string, HubStatus] => [d.device_id, { state: "unknown" }])), ...hubStatusFrom(fleet) });

const seenAt = (value: string | undefined, never: string) => (value && !Number.isNaN(new Date(value).getTime()) ? new Date(value).toLocaleString() : never);

export function Routers({ sub, params, me }: { sub: string; params: URLSearchParams; me: Me }) {
  const { t } = useI18n();
  const { data, error, isLoading } = useSWR<{ routers?: RouterRow[] }>("/routers", fetcher);
  const fleet = useFleet({ shouldRetryOnError: false });
  const hub = fleet.data || fleet.error ? hubStatusFromFleet(fleet.data, asArray<RouterRow>(data?.routers)) : {};
  if (isLoading) return <Skeleton h={260} />;
  if (error) return <Note kind="bad">{t("c.loadFailed")}</Note>;
  const list = asArray<RouterRow>(data?.routers);
  const onMap = sub === "map";
  if (sub && !onMap) return <><BackLink href="#/router">{t("r.ws.list")}</BackLink><Detail id={sub} router={list.find((d) => d.device_id === sub)} canReview={["OWNER", "DEVELOPER"].includes(me.user?.user_role ?? "")} /></>;

  const tabs = <WorkspaceTabs items={[{ href: "#/router", label: t("r.ws.list"), current: !onMap }, { href: "#/router/map", label: t("r.ws.map"), current: onMap }, { href: "#/network", label: t("title.network"), current: false }]} />;
  if (onMap) {
    const gateways = list.filter((d) => d.deployment_layer === "MAIN_GATEWAY");
    return <><h1>{t("r.mapTitle")}</h1>{tabs}{gateways.length ? <NetworkMap gateways={gateways} selected={params.get("id")} hub={hub} /> : <Note kind="warn">{t("v.noGateway")}</Note>}</>;
  }
  return (
    <>
      <h1>{t("title.router")}</h1>
      {tabs}
      {!list.length && <Note>{t("r.empty")} <a className="link" href="#/setup">{t("qs.title")}</a></Note>}
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
      <section className="glass pad stack"><SectionHead title={t("r.tools")} help={t("r.toolsHelp")} /><div className="scroll-x">
        <a className="pill" href="#/hub">{t("hub.title")}</a>
        {list.some((d) => d.deployment_layer !== "MAIN_GATEWAY") && <a className="pill" href="#/managed-devices">{t("dm.title")}</a>}
        <a className="pill" href="#/topology">{t("tp.title")}</a>
      </div></section>
    </>
  );
}
