import { useState } from "react";
import useSWR from "swr";
import { fetcher } from "../../lib/api";
import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import type { RouterRow } from ".";

type Inv = { available?: Record<string, boolean>; objects?: Record<string, Array<{ values?: Record<string, string> }>> };
interface Dev { mac: string; name: string; ip: string; port: string; how: "" | "wifi" | "neighbor" | "learned" }

const rows = (inv: Inv | undefined, key: string) => (inv?.available?.[key] === true ? asArray<{ values?: Record<string, string> }>(inv.objects?.[key]).map((o) => o.values ?? {}) : []);
const macOf = (v: unknown) => (/^([0-9a-f]{2}:){5}[0-9a-f]{2}$/i.test(String(v)) ? String(v).toUpperCase() : "");
const yes = (v: unknown) => v === "true" || v === "yes";

/** Read-only projection. A forwarding-table entry is not proof of a cable, so every device keeps the way it was observed and nothing is invented. */
export function project(inv: Inv | undefined) {
  const own = new Set(rows(inv, "interfaces").map((v) => macOf(v["mac-address"])).filter(Boolean));
  const found = new Map<string, Dev>();
  for (const key of ["discovery_leases", "arp", "neighbors", "bridge_hosts", "wifi_clients", "hotspot_sessions"]) {
    for (const v of rows(inv, key)) {
      if (key === "bridge_hosts" && yes(v.local)) continue;
      const mac = macOf(v["mac-address"]);
      if (!mac || own.has(mac) || parseInt(mac.slice(0, 2), 16) & 1) continue;
      let d = found.get(mac);
      if (!d) { d = { mac, name: "", ip: "", port: "", how: "" }; found.set(mac, d); }
      if (v.address) d.ip = v.address;
      const label = v.identity || v["host-name"]; if (label) d.name = label;
      if (key === "bridge_hosts" && v["on-interface"]) { d.port ||= v["on-interface"]; d.how ||= "learned"; }
      if (key === "wifi_clients" && v.interface) { d.port = v.interface; d.how = "wifi"; }
      if (key === "neighbors" && v.interface) { d.port ||= v.interface; d.how ||= "neighbor"; }
    }
  }
  const ports = new Map<string, Dev[]>(), unplaced: Dev[] = [];
  for (const d of found.values()) { d.name ||= d.ip || d.mac; if (d.port) { ports.set(d.port, [...(ports.get(d.port) ?? []), d]); } else unplaced.push(d); }
  return { ports: [...ports.entries()].sort((a, b) => a[0].localeCompare(b[0], undefined, { numeric: true })), unplaced, total: found.size };
}

export function NetworkMap({ gateways, selected }: { gateways: RouterRow[]; selected: string | null }) {
  const { t, lang } = useI18n();
  const gw = gateways.find((g) => g.device_id === selected) ?? gateways[0];
  const { data, error, isLoading } = useSWR<{ inventory?: Inv }>(`/routers/${encodeURIComponent(gw.device_id)}/network/inventory`, fetcher);
  const [pick, setPick] = useState<Dev | null>(null);
  if (isLoading) return <Skeleton h={320} />;
  if (error) return <Note kind="bad">{t("map.failed")}</Note>;
  // The server answers { inventory: … }.
  const m = project(data?.inventory);
  const how = (d: Dev) => d.how === "wifi" ? t("map.how.wifi") : d.how === "neighbor" ? t("map.how.neighbor") : d.how === "learned" ? (lang === "my" ? `${d.port} ${t("map.how.learned")}` : `${t("map.how.learned")} ${d.port}`) : t("map.how.unknown");
  const Node = ({ d }: { d: Dev }) => <button type="button" className="mapnode dev" aria-pressed={pick?.mac === d.mac} onClick={() => setPick(d)}><b>{d.name}</b><span className="soft small">{how(d)}</span></button>;
  return (
    <div className="map-layout">
      <div className="stack">
        {gateways.length > 1 && <select className="input" aria-label={t("v.f.gateway")} value={gw.device_id} onChange={(e) => { location.hash = "#/router/map?id=" + encodeURIComponent(e.target.value); }}>{gateways.map((g) => <option key={g.device_id} value={g.device_id}>{g.hardware_name || g.device_id}</option>)}</select>}
        <section className="glass pad stack">
          <div className="mapnode router">
            <div><b>{gw.hardware_name || "RouterBOARD"}</b><div className="soft small">{m.total} {t("map.devices")}</div></div>
            <span className={"chip " + (gw.is_online ? "ok" : "bad")}>{gw.is_online ? t("c.online") : t("c.offline")}</span>
          </div>
          {m.total === 0 && <Note>{t("map.empty")}</Note>}
          {m.ports.map(([port, devs]) => (
            <div className="branch" key={port}>
              <div className="row"><span className="chip num">{port}</span><span className="soft small">{devs.length}</span></div>
              <div className="stack leaf">{devs.map((d) => <Node key={d.mac} d={d} />)}</div>
            </div>
          ))}
          {m.unplaced.length > 0 && <div className="branch"><span className="soft small">{t("map.unplaced")}</span><div className="stack leaf">{m.unplaced.map((d) => <Node key={d.mac} d={d} />)}</div></div>}
        </section>
      </div>
      <section className="glass pad stack inspector" aria-live="polite">
        {pick ? (<>
          <span className="soft small">{t("map.props")}</span><h2>{pick.name}</h2>
          <div className="row small"><span className="soft">IP</span><b className="num">{pick.ip || "—"}</b></div>
          <div className="row small"><span className="soft">MAC</span><b className="num">{pick.mac}</b></div>
          <div className="row small"><span className="soft">{t("map.seen")}</span><b>{how(pick)}</b></div>
          <p className="soft small">{t("map.note")}</p>
        </>) : <p className="soft small">{t("map.pick")}</p>}
      </section>
    </div>
  );
}
