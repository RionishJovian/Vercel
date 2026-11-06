import { useEffect, useState } from "react";
import useSWR from "swr";
import { Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { useI18n } from "../../i18n";
import { fetcher } from "../../lib/api";
import { panelFor } from "../../lib/portPanels";
import { asArray, type Router } from "../../lib/types";
import { PortPanelView, type PortState } from "./PortPanel";
import { cableHints } from "./cableHealth";
import { linkText } from "./portInfo";
import { rateText, stillMeasuring } from "./speed";

interface PortSnapshot { read_at?: string; resources?: { board_name?: string }; interfaces?: PortState[] }

export function HomePorts({ router }: { router: Router }) {
  const { t } = useI18n();
  const { data, error, isLoading, mutate } = useSWR<{ telemetry?: { snapshot?: PortSnapshot } }>(
    `/routers/${encodeURIComponent(router.device_id)}/telemetry`, fetcher, { refreshInterval: 15000 },
  );
  const snap = data?.telemetry?.snapshot;
  const [reads, setReads] = useState(0), readAt = snap?.read_at;
  useEffect(() => { if (readAt) setReads(n => n + 1); }, [readAt]);
  if (isLoading) return <Skeleton h={180} />;
  if (error || !snap) return <><Note kind="warn">{t("r.telFail")}</Note><button className="pill" type="button" onClick={() => void mutate()}>{t("c.retry")}</button></>;
  const ports = asArray<PortState>(snap.interfaces);
  if (!ports.length) return <Note>{t("home.noPorts")}</Note>;
  const panel = panelFor(snap.resources?.board_name, router.hardware_name);
  const words = { full: t("r.full"), half: t("r.half"), updateAvail: t("r.updateAvail"), noNewer: t("r.noNewer"), fwUpgrade: t("r.fwUpgrade") };
  const measuring = stillMeasuring(ports, reads) ? t("r.measuring") : null;
  const settings = router.deployment_layer === "MAIN_GATEWAY"
    ? (port: string) => "#/network/ip?" + new URLSearchParams({ router: router.device_id, task: "ip_address", interface: port })
    : undefined;
  return (
    <div className="home-port-grid">
      {panel && <PortPanelView panel={panel} ports={ports} words={words} measuring={measuring} settings={settings} />}
      <ul className="stack home-port-list">
        {ports.map(i => (
          <li key={i.name}>
            <div className="row small">
              <span>{i.name} <span className={"chip " + (i.disabled ? "bad" : i.running ? "ok" : "warn")}>{i.disabled ? t("r.off") : i.running ? t("c.online") : t("r.down")}</span></span>
              <span className="soft num">{linkText(i, words) && <>{linkText(i, words)} · </>}↓ {rateText(i.rx_bps, measuring)} ↑ {rateText(i.tx_bps, measuring)}</span>
            </div>
            {cableHints(i).map(h => <p key={h.id} className="small cable-hint" data-tone={h.tone}>{t("ch." + h.id)}</p>)}
            {!panel?.ports.some(p => p.name === i.name) && settings && <a className="pill port-settings" href={settings(i.name)}>{t("home.managePorts")} · {i.name}</a>}
          </li>
        ))}
      </ul>
    </div>
  );
}
