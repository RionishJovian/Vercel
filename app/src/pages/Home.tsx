import { useEffect, useState } from "react";
import useSWR from "swr";
import { fetcher } from "../lib/api";
import { navigate } from "../lib/route";
import { asArray, mmk, num, type Me } from "../lib/types";
import { useI18n } from "../i18n";
import { Ago, IconTile, Metric, Recovery, Segmented, StatePill } from "../components/ui";
import { Icon } from "../components/icons";
import { Bar, Loading, SkMetrics } from "../components/Skeleton";
import type { Site } from "../lib/siteState";
import { incidentTone, useBroadbandReport, useFleetSummary, useIncidents, usePppoe, useRouters, useVoucherReport } from "./workspaces/data";
import { useSetupProgress } from "./workspaces/Setup";

type Service = "hotspot" | "pppoe";
const KEY = "mm_home_service";
const readService = (): Service => { try { return sessionStorage.getItem(KEY) === "pppoe" ? "pppoe" : "hotspot"; } catch { return "hotspot"; } };

/** Home: what needs attention now, and the two services side by side. Each card fails on its own. */
export function Home({ me, routerId }: { me: Me; routerId?: string | null }) {
  const { t } = useI18n();
  // Older links opened router ports on Home; ports now live on Network.
  useEffect(() => { if (routerId) navigate("/app/net?router=" + encodeURIComponent(routerId), true); }, [routerId]);
  const name = me.user?.full_name || (me.user?.email ?? "").split("@")[0];
  const [service, setServiceState] = useState<Service>(readService);
  const setService = (s: Service) => { setServiceState(s); try { sessionStorage.setItem(KEY, s); } catch { /* ignore */ } };
  const health = useFleetSummary();
  const routers = useRouters();
  const setup = useSetupProgress();
  const incidents = useIncidents();
  const open = asArray<{ id: string; severity?: string; subject_label?: string; detail?: string; kind?: string; opened_at?: string }>(incidents.data?.incidents);

  return (
    <>
      <header className="ws-head greet">
        <p className="soft small">{me.business?.company_name || t("hm.yourBusiness")}</p>
        <h1>{t("h.greet")} <span className="accent">{name}</span></h1>
        <div className="toolbar">
          {health.fleet.isLoading ? <span className="chip idle">{t("st.checking")}</span> : <StatePill tone={health.tone}>{t(health.key)}</StatePill>}
          {health.total ? <span className="soft small num">{t("hm.routersUp").replace("{up}", String(health.up)).replace("{total}", String(health.total))}</span> : null}
          {health.checkedAt ? <Ago at={health.checkedAt} prefix={t("ws.checked")} /> : null}
        </div>
      </header>

      <section className="glass pad stack workspace-summary" aria-labelledby="hm-network">
        <div className="row"><h2 id="hm-network">{t("nav.network")}</h2><a className="link small" href="/app/net">{t("hm.open")}</a></div>
        <NetworkStrip />
      </section>

      <div className="workspace-rail workspace-stack">
      {setup.loading ? (
        <Loading><div className="glass pad stack setup-card"><div className="row"><Bar w="55%" /><Bar w="12%" size=".875rem" /></div><span className="progress" /><Bar w="50%" size=".875rem" /></div></Loading>
      ) : !setup.complete && (
        <a className="glass pad stack setup-card" href="/app/setup">
          <div className="row"><b>{t("su.homeTitle")}</b><span className="soft small num">{setup.done}/{setup.total}</span></div>
          <div className="progress" role="progressbar" aria-valuemin={0} aria-valuemax={setup.total} aria-valuenow={setup.done} aria-label={t("su.title")}><i style={{ width: `${(setup.done / Math.max(1, setup.total)) * 100}%` }} /></div>
          <span className="row" style={{ justifyContent: "flex-start" }}><span className="soft small">{t("su.next")}: {setup.next ? t(setup.next.title) : ""}</span><Icon name="chevron" className="linkrow-chev" /></span>
        </a>
      )}

      {open.length > 0 && (
        <section className="glass pad stack" aria-labelledby="hm-alerts">
          <div className="row"><h2 id="hm-alerts">{t("hm.attention")}</h2><a className="link small" href="/app/incidents">{t("hm.allAlerts")} ({open.length})</a></div>
          <div className="linklist">
            {open.slice(0, 2).map((i) => (
              <a key={i.id} className="linkrow" href="/app/incidents">
                <IconTile name="alert" tone={incidentTone(i.severity) === "bad" ? "bad" : "warn"} />
                <span className="linkrow-text"><b>{i.subject_label || i.kind || t("al.issue")}</b><span className="soft small">{i.detail || t("al.openForDetail")}</span></span>
                <Ago at={i.opened_at} />
              </a>
            ))}
          </div>
        </section>
      )}

      <section className="stack" aria-label={t("hm.quick")}>
        <h2 className="group-title">{t("hm.quick")}</h2>
        <nav className="tiles">
          <a className="tile" href="/app/vouchers/create"><span className="tile-ico"><Icon name="ticket" /></span><b>{t("hm.qCreate")}</b></a>
          <a className="tile" href="/app/guests"><span className="tile-ico"><Icon name="users" /></span><b>{t("hm.qGuests")}</b></a>
          <a className="tile" href="/app/pppoe"><span className="tile-ico"><Icon name="isp" /></span><b>{t("hm.qSubscriber")}</b></a>
          <a className="tile" href="/app/hub"><span className="tile-ico"><Icon name="router" /></span><b>{t("hm.qRouter")}</b></a>
        </nav>
      </section>
      </div>
      <div className="workspace-main workspace-stack">
      <div className="only-narrow">
        <Segmented<Service> label={t("hm.service")} value={service} onChange={setService} options={[["hotspot", t("nav.hotspot")], ["pppoe", t("nav.isp")]]} />
      </div>
      <div className="ws-grid cols2 dual home-services">
        <div className="svc" data-active={service === "hotspot" || undefined}><HotspotCard /></div>
        <div className="svc" data-active={service === "pppoe" || undefined}><PppoeCard gateways={routers.gateways} /></div>
      </div>

      </div>
    </>
  );
}

function HotspotCard() {
  const { t } = useI18n();
  const report = useVoucherReport();
  const rep = report.data;
  return (
    <section className="glass pad svc-card" aria-labelledby="hm-hotspot">
      <header><h2 id="hm-hotspot"><IconTile name="wifi" />{t("nav.hotspot")}</h2><a className="link small" href="/app/hotspot">{t("hm.open")}</a></header>
      {report.isLoading ? <Loading><SkMetrics n={4} /></Loading> : report.error ? (
        <Recovery title={t("hm.vouchersUnavailable")} detail={t("hm.vouchersUnavailableHelp")} onRetry={() => void report.mutate()} />
      ) : (
        <div className="metrics">
          <Metric label={t("hm.available")} value={rep && num(rep.available_count) !== null ? mmk(rep.available_count) : null} sub={t("hm.readyToSell")} />
          <Metric label={t("hm.startedToday")} value={rep && num(rep.activation_today) !== null ? mmk(rep.activation_today) : null} sub={t("hm.firstUse")} />
          <Metric label={t("hm.reserved")} value={rep && num(rep.reserved_count) !== null ? mmk(rep.reserved_count) : null} sub={t("hm.onRouterOrShop")} />
          <Metric label={t("h.revenue")} value={rep && num(rep.activation_value) !== null ? `${mmk(rep.activation_value)} ${t("c.mmk")}` : null} sub={t("hm.notCash")} />
        </div>
      )}
      <div className="actions-row">
        <a className="btn small" href="/app/vouchers/create">{t("hm.qCreate")}</a>
        <a className="btn small quiet" href="/app/guests">{t("hm.liveSessions")}</a>
      </div>
    </section>
  );
}

function PppoeCard({ gateways }: { gateways: ReturnType<typeof useRouters>["gateways"] }) {
  const { t } = useI18n();
  const pppoe = usePppoe(gateways);
  const report = useBroadbandReport();
  const s = pppoe.data;
  const noRouter = gateways !== null && gateways.length === 0;
  return (
    <section className="glass pad svc-card" aria-labelledby="hm-pppoe">
      <header><h2 id="hm-pppoe"><IconTile name="isp" />{t("nav.isp")}</h2><a className="link small" href="/app/isp">{t("hm.open")}</a></header>
      {noRouter ? (
        <Recovery title={t("isp.noGateway")} detail={t("isp.noGatewayHelp")} action={{ href: "/app/setup", label: t("su.continue") }} />
      ) : gateways === null || pppoe.isLoading ? <Loading><SkMetrics n={4} /></Loading> : pppoe.error ? (
        <Recovery title={t("isp.unavailable")} onRetry={() => void pppoe.mutate()} />
      ) : (
        <>
          <div className="metrics">
            <Metric label={t("isp.subscribers")} value={s ? mmk(s.total) : null} />
            <Metric label={t("isp.connected")} value={s ? mmk(s.connected) : null} sub={t("isp.connectedHint")} tone={s && s.connected ? "ok" : undefined} />
            <Metric label={t("isp.suspended")} value={s ? mmk(s.suspended) : null} tone={s && s.suspended ? "warn" : undefined} />
            <Metric label={t("isp.paidToday")} value={report.data && num(report.data.today_mmk) !== null ? `${mmk(report.data.today_mmk)} ${t("c.mmk")}` : null} sub={t("isp.recorded")} />
          </div>
          {s?.failed.length ? <p className="note warn small tnum">{t("isp.partial").replace("{n}", String(s.failed.length))}</p> : null}
          {s?.newestObservation ? <Ago at={s.newestObservation} prefix={t("ws.reported")} /> : null}
        </>
      )}
      <div className="actions-row">
        <a className="btn small" href="/app/pppoe">{t("isp.manage")}</a>
        <a className="btn small quiet" href="/app/customers?type=pppoe">{t("isp.viewList")}</a>
      </div>
    </section>
  );
}

function NetworkStrip() {
  const { t } = useI18n();
  const h = useFleetSummary();
  const sites = useSWR<{ sites?: Site[] }>("/sites", fetcher, { shouldRetryOnError: false });
  const siteCount = sites.data ? asArray<Site>(sites.data.sites).length : null;
  if (h.fleet.isLoading) return <Loading><SkMetrics n={4} cols="four" sub={false} /></Loading>;
  return (
    <div className="metrics four">
      <Metric label={t("nw.sites")} value={siteCount} />
      <Metric label={t("nw.online")} value={h.total === null ? null : `${h.up}/${h.total}`} tone={h.tone === "ok" ? "ok" : undefined} />
      <Metric label={t("nw.stale")} value={h.total === null ? null : h.stale} tone={h.stale ? "warn" : undefined} />
      <Metric label={t("nw.offline")} value={h.total === null ? null : h.offline} tone={h.offline ? "bad" : undefined} />
    </div>
  );
}
