import { useState } from "react";
import { useI18n } from "../../i18n";
import type { PortPanel as Panel } from "../../lib/portPanels";
import { linkText, type Words } from "./portInfo";
import { rateText } from "./speed";
import { cableHints, needsAttention, type CableFacts } from "./cableHealth";

export interface PortState extends CableFacts { name: string; rx_bps?: number; tx_bps?: number }
type Tone = "up" | "down" | "off" | "unknown";
const toneOf = (i?: PortState): Tone => (!i ? "unknown" : i.disabled ? "off" : i.running ? "up" : "down");

/** The router's real back panel with each Ethernet port lit by what the router reports. Never guesses: a port the router did not report is shown as unknown. */
export function PortPanelView({ panel, ports, words, measuring, settings }: { panel: Panel; ports: readonly PortState[]; words: Words; measuring: string | null; settings?: (port: string) => string }) {
  const { t } = useI18n();
  const byName = new Map(ports.map((p) => [p.name, p]));
  const [picked, setPicked] = useState<string>("");
  const label = (name: string) => {
    const i = byName.get(name), tone = toneOf(i);
    const status = tone === "up" ? t("c.online") : tone === "off" ? t("r.off") : tone === "down" ? t("r.down") : t("pp.unknown");
    const link = i ? linkText(i, words) : "";
    const speed = tone === "up" && i ? `↓ ${rateText(i.rx_bps, measuring)} ↑ ${rateText(i.tx_bps, measuring)}` : "";
    const hints = i ? cableHints(i) : [];
    return { status, line: [link, speed].filter(Boolean).join(" · "), tone, hints, attention: needsAttention(hints) };
  };
  const active = picked ? label(picked) : null;
  return (
    <div className="port-panel-wrap stack">
      <div className={"port-panel" + (panel.height > panel.width ? " portrait" : "")} style={{ aspectRatio: `${panel.width} / ${panel.height}` }}>
        <img src={import.meta.env.BASE_URL + panel.image} alt={panel.alt} width={panel.width} height={panel.height} loading="lazy" />
        {panel.ports.map((spot) => {
          const l = label(spot.name);
          return (
            <button key={spot.name} type="button" className={"port-dot " + l.tone + (l.attention ? " flag" : "") + (picked === spot.name ? " on" : "")}
              style={{ left: spot.x + "%", top: spot.y + "%", width: panel.size + "%" }}
              aria-pressed={picked === spot.name} aria-label={`${spot.name}: ${l.status}${l.line ? ", " + l.line : ""}${l.attention ? ", " + t("ch.attention") : ""}`}
              onClick={() => setPicked(picked === spot.name ? "" : spot.name)} />
          );
        })}
      </div>
      <p className="soft small" role="status">
        {active ? <><b>{picked}</b> <span className={"chip " + (active.tone === "up" ? "ok" : active.tone === "off" ? "bad" : "warn")}>{active.status}</span>{active.line ? <span className="num"> {active.line}</span> : null}</> : t("pp.hint")}
      </p>
      {active && byName.has(picked) && settings && <a className="pill port-settings" href={settings(picked)}>{t("home.managePorts")} · {picked}</a>}
      {active && active.hints.map((h) => <p key={h.id} className="small cable-hint" data-tone={h.tone}>{t("ch." + h.id)}</p>)}
    </div>
  );
}
