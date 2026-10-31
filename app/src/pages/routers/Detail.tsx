import useSWR from "swr";
import { fetcher } from "../../lib/api";
import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import type { RouterRow } from ".";
import { Capabilities } from "./Capabilities";

interface Iface { name: string; disabled?: boolean; running?: boolean; rx_bps?: number; tx_bps?: number }
interface Telemetry { state?: string; snapshot?: { resources?: Record<string, unknown> & { board_name?: string; routeros_version?: string; cpu_percent?: number; memory_used_bytes?: number | string; memory_total_bytes?: number | string; uptime_seconds?: number }; hotspot_sessions?: number | null; dhcp_bound_leases?: number | null; interfaces?: Iface[]; issues?: string[] } }

const pct = (v: unknown) => (typeof v === "number" ? Math.round(v) + "%" : "—");
const mb = (v: unknown) => (Number.isFinite(Number(v)) ? (Number(v) / 1048576).toFixed(0) + " MB" : "—");
const rate = (v?: number) => (typeof v === "number" ? (v >= 1e6 ? (v / 1e6).toFixed(1) + " Mbps" : Math.round(v / 1e3) + " kbps") : "—");
function uptime(sec: unknown) {
  if (typeof sec !== "number") return "—";
  const d = Math.floor(sec / 86400), h = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
  return (d ? d + "d " : "") + h + "h " + m + "m";
}
const Kv = ({ k, v }: { k: string; v: string }) => <div className="row small"><span className="soft">{k}</span><b className="num">{v}</b></div>;

/** Read-only live health. Network writes stay held server-side until a physical router acceptance test has passed. */
export function Detail({ id, router }: { id: string; router?: RouterRow }) {
  const { t } = useI18n();
  const { data, error, isLoading, mutate } = useSWR<{ telemetry?: Telemetry }>(`/routers/${encodeURIComponent(id)}/telemetry`, fetcher, { refreshInterval: 15000 });
  const tele = data?.telemetry, snap = tele?.snapshot, r = snap?.resources ?? {};
  return (
    <>
      <h1>{router?.hardware_name || id}</h1>
      {isLoading ? <Skeleton h={300} /> : error || !snap ? (
        <><Note kind="warn">{t("r.telFail")}</Note><button className="pill" type="button" onClick={() => void mutate()}>{t("c.retry")}</button></>
      ) : (
        <>
          <section className="glass pad stack">
            <div className="row"><h2>{t("r.health")}</h2><span className={"chip " + (tele?.state === "live" || tele?.state === "syncing" ? "ok" : "warn")}>{tele?.state || "—"}</span></div>
            <Kv k={t("r.board")} v={(r.board_name || "—") + (r.routeros_version ? " " + r.routeros_version : "")} />
            <Kv k="CPU" v={pct(r.cpu_percent)} />
            <Kv k={t("r.memory")} v={`${mb(r.memory_used_bytes)} / ${mb(r.memory_total_bytes)}`} />
            <Kv k={t("r.uptime")} v={uptime(r.uptime_seconds)} />
            <Kv k={t("r.guests")} v={snap.hotspot_sessions == null ? "—" : String(snap.hotspot_sessions)} />
            <Kv k={t("r.leases")} v={snap.dhcp_bound_leases == null ? "—" : String(snap.dhcp_bound_leases)} />
          </section>
          {asArray<Iface>(snap.interfaces).length > 0 && (
            <section className="glass pad stack">
              <h2>{t("r.ports")}</h2>
              {/* A strip of port lamps, like the front of the board: lit when the link is up. */}
              <div className="portstrip" aria-hidden="true">{asArray<Iface>(snap.interfaces).map((i) => <i key={i.name} className={i.disabled ? "off" : i.running ? "up" : "down"} title={i.name} />)}</div>
              {asArray<Iface>(snap.interfaces).map((i) => (
                <div className="row small" key={i.name}>
                  <span>{i.name} <span className={"chip " + (i.disabled ? "bad" : i.running ? "ok" : "warn")}>{i.disabled ? t("r.off") : i.running ? t("c.online") : t("r.down")}</span></span>
                  <span className="soft num">↓ {rate(i.rx_bps)} ↑ {rate(i.tx_bps)}</span>
                </div>
              ))}
            </section>
          )}
          {asArray<string>(snap.issues).length > 0 && <Note kind="warn">{asArray<string>(snap.issues).join(" · ")}</Note>}
          <p className="soft small">{t("r.readOnly")}</p>
          <Capabilities id={id} />
        </>
      )}
    </>
  );
}
