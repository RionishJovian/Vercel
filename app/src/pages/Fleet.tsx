import useSWR from "swr";
import { fetcher } from "../lib/api";
import { useFleet, type FleetPeer } from "../lib/useFleet";
import { asArray, num, type Router } from "../lib/types";
import { useI18n } from "../i18n";
import { BackLink, Note } from "../components/ui";
import { Skeleton } from "../components/Skeleton";
import { Ring } from "../components/Ring";
import { deviceImage } from "../lib/deviceImage";
import { useCountUp } from "../lib/useCountUp";
import { StatusDetail } from "../components/StatusDetail";

type Peer = FleetPeer;
const LEVEL: Record<string, number> = { up: 10, stale: 5, offline: 1, unknown: 0 };
const TONE: Record<string, string> = { up: "ok", stale: "warn", offline: "bad", unknown: "" };

export function Fleet() {
  const { t } = useI18n();
  const fleet = useFleet();
  const routers = useSWR<{ routers?: Router[] }>("/routers", fetcher);
  const pulse = useSWR<{ open_incidents_head?: unknown[] }>("/fleet/pulse", fetcher, { shouldRetryOnError: false });
  const availNow = (() => { const tp = num(fleet.data?.total_peers) ?? asArray(fleet.data?.peers).length; return tp ? Math.round(((num(fleet.data?.up) ?? 0) / tp) * 100) : 0; })();
  const shownAvail = useCountUp(availNow);
  const back = <BackLink href="/app/more">{t("tab.more")}</BackLink>;
  if (fleet.isLoading) return <>{back}<Skeleton h={320} /></>;
  if (fleet.error || !fleet.data) return <>{back}<Note kind="bad">{t("h.fleetFailed")}</Note></>;
  const f = fleet.data, peers = asArray<Peer>(f.peers);
  const byId = new Map(asArray<Router>(routers.data?.routers).map((r) => [r.device_id, r]));
  const total = num(f.total_peers) ?? peers.length;
  const availability = total ? Math.round(((num(f.up) ?? 0) / total) * 100) : 0;
  const attention = (num(f.stale) ?? 0) + (num(f.offline) ?? 0);
  const incidents = asArray(pulse.data?.open_incidents_head).length;
  const overall = String(f.overall_state ?? "unknown").toLowerCase();
  return (
    <>
      {back}<h1>{t("fl.title")}</h1>
      <section className="glass instrument fleet-hero">
        <Ring fraction={availability / 100} label={`${availability}% ${t("fl.availability")}`}>
          <span className="mute small">{t("fl.availability")}</span>
          <span className="num hero-figure">{shownAvail ?? 0}%</span>
          <span className="mute small">{num(f.up) ?? 0} / {total} {t("c.online")}</span>
        </Ring>
        <div className="capsules">
          <div className="capsule"><span className="mute small">{t("fl.attention")}</span><b className="num">{attention}</b></div>
          <div className="capsule"><span className="mute small">{t("fl.incidents")}</span><b className="num">{incidents}</b></div>
          <div className="capsule"><span className="mute small">{t("fl.overall")}</span><b className={"tone-" + TONE[overall]}>{t("fl.state." + (overall in LEVEL ? overall : "unknown"))}</b></div>
        </div>
      </section>
      {!peers.length && <Note>{t("fl.none")}</Note>}
      <div className="workspace-cards">
        {peers.map((p, i) => {
          const state = String(p.state ?? "unknown").toLowerCase(), level = LEVEL[state] ?? 0, r = byId.get(p.device_id ?? "");
          return (
            <div className="rowitem fleet-row" key={p.device_id ?? i}>
              <span className={"rc-pic sm" + (state === "up" ? " on" : "")}><img src={deviceImage({ hardware_name: p.hardware_name, deployment_layer: r?.deployment_layer })} alt="" width={36} height={36} loading="lazy" /><i className="rc-led" aria-hidden="true" /></span>
              <div className="stack" style={{ gap: 2, flex: 1, minWidth: 0 }}><b>{p.hardware_name || t("fl.unnamed")}</b><span className="soft small">{r?.deployment_layer === "MAIN_GATEWAY" ? t("fl.gateway") : t("fl.peer")}</span><StatusDetail peer={p} /></div>
              <span className="signal" role="img" aria-label={`${level} / 10`}>{Array.from({ length: 10 }, (_, k) => <i key={k} className={k < level ? "on " + state : ""} />)}</span>
              <span className={"chip " + TONE[state]}>{t("fl.state." + (state in LEVEL ? state : "unknown"))}</span>
            </div>
          );
        })}
      </div>
    </>
  );
}
