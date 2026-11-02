import { useEffect, useState } from "react";
import useSWR from "swr";
import { fetcher } from "../../lib/api";
import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import type { RouterRow } from ".";
import { Capabilities } from "./Capabilities";
import { FirmwareUpgrade } from "./FirmwareUpgrade";
import { PortPanelView } from "./PortPanel";
import { cableHints } from "./cableHealth";
import { panelFor } from "../../lib/portPanels";
import { rateText, stillMeasuring } from "./speed";
import { firmwareText, healthLabel, healthValue, linkText, softwareText, type Firmware, type Health } from "./portInfo";

interface Iface { name: string; disabled?: boolean; running?: boolean; rx_bps?: number; tx_bps?: number; link_rate?: string; full_duplex?: boolean; negotiation?: string; max_rate_mbps?: number; partner_max_mbps?: number; phy_errors?: string; phy_errors_recent?: number }
interface Telemetry { state?: string; snapshot?: { read_at?: string; resources?: Record<string, unknown> & { board_name?: string; routeros_version?: string; cpu_percent?: number; memory_used_bytes?: number | string; memory_total_bytes?: number | string; uptime_seconds?: number }; hotspot_sessions?: number | null; dhcp_bound_leases?: number | null; interfaces?: Iface[]; issues?: string[]; health?: Health[] | null; firmware?: Firmware | null } }

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
  const [reads, setReads] = useState(0), readAt = snap?.read_at;
  useEffect(() => { if (readAt) setReads((n) => n + 1); }, [readAt]);
  const words = { full: t("r.full"), half: t("r.half"), updateAvail: t("r.updateAvail"), noNewer: t("r.noNewer"), fwUpgrade: t("r.fwUpgrade") };
  const measuring = stillMeasuring(asArray<Iface>(snap?.interfaces), reads) ? t("r.measuring") : null;
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
          {asArray<Iface>(snap.interfaces).length > 0 && (
            <section className="glass pad stack">
              <h2>{t("r.ports")}</h2>
              {(() => { const panel = panelFor(r.board_name, router?.hardware_name); return panel ? <PortPanelView panel={panel} ports={asArray<Iface>(snap.interfaces)} words={words} measuring={measuring} /> : null; })()}
              {/* A strip of port lamps, like the front of the board: lit when the link is up. */}
              <div className="portstrip" aria-hidden="true">{asArray<Iface>(snap.interfaces).map((i) => <i key={i.name} className={i.disabled ? "off" : i.running ? "up" : "down"} title={i.name} />)}</div>
              {asArray<Iface>(snap.interfaces).map((i) => (
                <div key={i.name}>
                  <div className="row small">
                    <span>{i.name} <span className={"chip " + (i.disabled ? "bad" : i.running ? "ok" : "warn")}>{i.disabled ? t("r.off") : i.running ? t("c.online") : t("r.down")}</span></span>
                    <span className="soft num">{linkText(i, words) && <>{linkText(i, words)} · </>}↓ {rateText(i.rx_bps, measuring)} ↑ {rateText(i.tx_bps, measuring)}</span>
                  </div>
                  {cableHints(i).map((h) => <p key={h.id} className="small cable-hint" data-tone={h.tone}>{t("ch." + h.id)}</p>)}
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
