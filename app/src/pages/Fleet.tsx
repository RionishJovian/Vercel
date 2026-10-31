import useSWR from "swr";
import { fetcher } from "../lib/api";
import { asArray, num, type Router } from "../lib/types";
import { useI18n } from "../i18n";
import { BackLink, Note } from "../components/ui";
import { Skeleton } from "../components/Skeleton";

interface Peer { device_id?: string; hardware_name?: string; state?: string }
interface FleetData { peers?: Peer[]; total_peers?: number; up?: number; stale?: number; offline?: number; overall_state?: string }
const LEVEL: Record<string, number> = { up: 10, stale: 5, offline: 1, unknown: 0 };
const TONE: Record<string, string> = { up: "ok", stale: "warn", offline: "bad", unknown: "" };

export function Fleet() {
  const { t } = useI18n();
  const fleet = useSWR<FleetData>("/fleet/health", fetcher, { refreshInterval: 30000 });
  const routers = useSWR<{ routers?: Router[] }>("/routers", fetcher);
  const pulse = useSWR<{ open_incidents_head?: unknown[] }>("/fleet/pulse", fetcher, { shouldRetryOnError: false });
  const back = <BackLink href="#/more">{t("tab.more")}</BackLink>;
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
      {back}<h1>{t("fl.title")}</h1><p className="soft">{t("fl.lead")}</p>
      <section className="glass pad fleet-summary">
        <div><span className="soft small">{t("fl.availability")}</span><b className="num big">{availability}%</b><span className="soft small">{num(f.up) ?? 0} {t("c.online")}</span></div>
        <div><span className="soft small">{t("fl.attention")}</span><b className="num big">{attention}</b><span className="soft small">{num(f.stale) ?? 0} {t("fl.stale")} · {num(f.offline) ?? 0} {t("c.offline")}</span></div>
        <div><span className="soft small">{t("fl.incidents")}</span><b className="num big">{incidents}</b></div>
        <div><span className="soft small">{t("fl.overall")}</span><b className={"num big tone-" + TONE[overall]}>{t("fl.state." + (overall in LEVEL ? overall : "unknown"))}</b></div>
      </section>
      {!peers.length && <Note>{t("fl.none")}</Note>}
      <div className="stack">
        {peers.map((p, i) => {
          const state = String(p.state ?? "unknown").toLowerCase(), level = LEVEL[state] ?? 0, r = byId.get(p.device_id ?? "");
          return (
            <div className="rowitem fleet-row" key={p.device_id ?? i}>
              <div className="stack" style={{ gap: 2 }}><b>{p.hardware_name || t("fl.unnamed")}</b><span className="soft small">{r?.deployment_layer === "MAIN_GATEWAY" ? t("fl.gateway") : t("fl.peer")}</span></div>
              <span className="signal" role="img" aria-label={`${level} / 10`}>{Array.from({ length: 10 }, (_, k) => <i key={k} className={k < level ? "on " + state : ""} />)}</span>
              <span className={"chip " + TONE[state]}>{t("fl.state." + (state in LEVEL ? state : "unknown"))}</span>
            </div>
          );
        })}
      </div>
    </>
  );
}
