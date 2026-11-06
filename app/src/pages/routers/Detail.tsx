import useSWR from "swr";
import { fetcher } from "../../lib/api";
import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import type { RouterRow } from ".";
import { Capabilities } from "./Capabilities";
import { FirmwareUpgrade } from "./FirmwareUpgrade";
import { firmwareText, healthLabel, healthValue, softwareText, type Firmware, type Health } from "./portInfo";

interface Telemetry { state?: string; snapshot?: { read_at?: string; resources?: Record<string, unknown> & { board_name?: string; routeros_version?: string; cpu_percent?: number; memory_used_bytes?: number | string; memory_total_bytes?: number | string; uptime_seconds?: number }; hotspot_sessions?: number | null; dhcp_bound_leases?: number | null; issues?: string[]; health?: Health[] | null; firmware?: Firmware | null } }

const pct = (v: unknown) => (typeof v === "number" ? Math.round(v) + "%" : "—");
const mb = (v: unknown) => (Number.isFinite(Number(v)) ? (Number(v) / 1048576).toFixed(0) + " MB" : "—");
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
  const words = { updateAvail: t("r.updateAvail"), noNewer: t("r.noNewer"), fwUpgrade: t("r.fwUpgrade"), full: t("r.full"), half: t("r.half") };
  return (
    <>
      <h1>{router?.hardware_name || id}</h1>
      {/* Outside the loading/error branches so a restart (when telemetry briefly fails) does not close it. */}
      <FirmwareUpgrade id={id} fw={snap?.firmware ?? undefined} onDone={() => void mutate()} />
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
            {asArray<Health>(snap.health).map((h) => <Kv key={h.name} k={healthLabel(h.name)} v={healthValue(h)} />)}
            {snap.firmware && <Kv k={t("r.software")} v={softwareText(r.routeros_version, snap.firmware, words)} />}
            {firmwareText(snap.firmware ?? undefined, words) && <Kv k={t("r.firmware")} v={firmwareText(snap.firmware ?? undefined, words)} />}
            <Kv k={t("r.guests")} v={snap.hotspot_sessions == null ? "—" : String(snap.hotspot_sessions)} />
            <Kv k={t("r.leases")} v={snap.dhcp_bound_leases == null ? "—" : String(snap.dhcp_bound_leases)} />
          </section>
          <a className="pill" href={"#/home?router=" + encodeURIComponent(id)}>{t("home.managePorts")}</a>
          {asArray<string>(snap.issues).length > 0 && <Note kind="warn">{asArray<string>(snap.issues).join(" · ")}</Note>}
          <p className="soft small">{t("r.readOnly")}</p>
          <Capabilities id={id} />
        </>
      )}
    </>
  );
}
