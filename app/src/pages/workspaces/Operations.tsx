import { lazy, Suspense, useState } from "react";
import { PageBoundary } from "../../components/PageBoundary";
import useSWR from "swr";
import { fetcher } from "../../lib/api";
import { asArray, type Me } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Ago, IconTile, LinkRow, Metric, Recovery, StatePill } from "../../components/ui";
import { Bar, Loading, SkList, SkRows } from "../../components/Skeleton";
import { HomePorts } from "../routers/HomePorts";
import { stateOf, type Conn, type Site } from "../../lib/siteState";
import { incidentTone, useFleetSummary, useIncidents, useRouters } from "./data";
import type { FleetPeer } from "../../lib/useFleet";

const SiteMap = lazy(() => import("../../components/SiteMap"));
const KEY = "mm_selected_router";
const peerTone = (s?: string) => (s === "up" ? "ok" : s === "stale" ? "warn" : s === "offline" ? "bad" : "idle") as "ok" | "warn" | "bad" | "idle";
const peerKey = (s?: string) => (s === "up" ? "st.online" : s === "stale" ? "st.stale" : s === "offline" ? "st.offline" : "st.unknown");

/** Network: site health, routers with evidence, ports, and the way into every network screen. */
export function NetworkHub({ routerId }: { me: Me; routerId?: string | null }) {
  const { t } = useI18n();
  const health = useFleetSummary();
  const routers = useRouters();
  const sitesRes = useSWR<{ sites?: Site[] }>("/sites", fetcher, { shouldRetryOnError: false });
  const conns = useSWR<{ connections?: Conn[] }>("/router-connections", fetcher, { shouldRetryOnError: false });
  const sites = asArray<Site>(sitesRes.data?.sites), connList = asArray<Conn>(conns.data?.connections);
  const [site, setSite] = useState("");
  const [selectedId, setSelectedId] = useState(() => { if (routerId) return routerId; try { return sessionStorage.getItem(KEY) ?? ""; } catch { return ""; } });
  const list = routers.all;
  const selected = list?.find((r) => r.device_id === selectedId) ?? (list?.length === 1 ? list[0] : undefined);
  const peers = new Map((health.fleet.data?.peers ?? []).map((p: FleetPeer) => [p.device_id, p]));
  return (
    <>
      <header className="ws-head">
        <h1>{t("nav.network")}</h1>
        <div className="toolbar">{health.fleet.isLoading ? null : <StatePill tone={health.tone}>{t(health.key)}</StatePill>}{health.checkedAt ? <Ago at={health.checkedAt} prefix={t("ws.checked")} /> : null}</div>
      </header>
      <section className="glass pad stack" aria-labelledby="nh-routers">
        <div className="row"><h2 id="nh-routers">{t("nh.routers")}</h2><a className="link small" href="/app/hub">{t("hub.title")}</a></div>
        {routers.isLoading ? <Loading><SkRows n={1} pill /></Loading> : routers.error ? <Recovery title={t("nh.routersUnavailable")} onRetry={() => void routers.mutate()} /> : !list?.length ? (
          <Recovery title={t("nh.noRouters")} detail={t("nh.noRoutersHelp")} action={{ href: "/app/setup", label: t("su.continue") }} />
        ) : (
          <div className="linklist">
            {list.map((r) => { const p = peers.get(r.device_id); return (
              <a key={r.device_id} className="linkrow" href={"/app/router/" + encodeURIComponent(r.device_id)}>
                <IconTile name="router" tone={peerTone(p?.state) === "idle" ? "" : peerTone(p?.state)} />
                <span className="linkrow-text"><b>{r.hardware_name || r.device_id}</b><span className="soft small">{r.deployment_layer === "MAIN_GATEWAY" ? t("nh.gateway") : t("nh.device")} · <Ago at={p?.checked_at ?? p?.last_contact_at ?? r.last_seen_at} prefix={t("ws.checked")} /></span><span><StatePill tone={peerTone(p?.state)}>{t(peerKey(p?.state))}</StatePill></span></span>
              </a>
            ); })}
          </div>
        )}
        <p className="soft small">{t("nh.evidenceNote")}</p>
      </section>
      {routers.isLoading && <Loading><section className="glass pad stack"><Bar w="25%" size="1.15rem" lh={1.15} /><div className="skeleton" style={{ height: 180 }} /></section></Loading>}
      {list && list.length > 0 && (
        <section className="glass pad stack" aria-labelledby="nh-ports">
          <h2 id="nh-ports">{t("r.ports")}</h2>
          {list.length > 1 && <label className="field"><span>{t("net.working")}</span>
            <select className="input" value={selected?.device_id ?? ""} onChange={(e) => { setSelectedId(e.target.value); try { sessionStorage.setItem(KEY, e.target.value); } catch { /* ignore */ } }}>
              <option value="">{t("net.chooseRouter")}</option>{list.map((r) => <option key={r.device_id} value={r.device_id}>{r.hardware_name || r.device_id}</option>)}
            </select></label>}
          {selected ? <HomePorts key={selected.device_id} router={selected} /> : null}
        </section>
      )}
      <section className="stack" aria-label={t("nh.tools")}>
        <h2 className="group-title">{t("nh.tools")}</h2>
        <div className="linklist">
          <LinkRow href="/app/setup" icon="rocket" title={t("nav.setup")} detail={t("nh.setupHelp")} />
          <LinkRow href="/app/network" icon="globe" title={t("nh.internet")} detail={t("nh.internetHelp")} />
          <LinkRow href="/app/topology" icon="grid" title={t("nav.topology")} detail={t("nh.topologyHelp")} />
          <LinkRow href="/app/managed-devices" icon="device" title={t("nav.devices")} detail={t("nh.devicesHelp")} />
          <LinkRow href="/app/hub" icon="link" title={t("hub.title")} detail={t("nh.hubHelp")} />
          <LinkRow href="/app/sites" icon="pin" title={t("title.sites")} detail={t("nh.sitesHelp")} />
          <LinkRow href="/app/approvals" icon="approve" title={t("nav.approvals")} detail={t("nh.approvalsHelp")} />
          <LinkRow href="/app/backups" icon="save" title={t("bk.title")} detail={t("nh.backupsHelp")} />
        </div>
      </section>
      <section className="stack" aria-label={t("home.map")}>
        <h2 className="group-title">{t("home.map")}</h2>
        <PageBoundary inline><Suspense fallback={<Loading><div className="site-map skeleton" /></Loading>}>
          <SiteMap home sites={sites} states={Object.fromEntries(sites.map((s) => [s.site_id, stateOf(s, connList)]))} selected={site} onSelect={(id) => setSite(id === site ? "" : id)} />
        </Suspense></PageBoundary>
      </section>
    </>
  );
}

interface Ev { received_at?: string; severity?: string; program?: string; message?: string }

/** Activity: alerts first, then what changed and what the monitor saw, each with its time and source. */
export function Activity() {
  const { t } = useI18n();
  const health = useFleetSummary();
  const incidents = useIncidents();
  const events = useSWR<{ events?: Ev[] }>("/syslog-events?limit=8&offset=0", fetcher, { shouldRetryOnError: false });
  const open = asArray<{ id: string; severity?: string; subject_label?: string; detail?: string; kind?: string; opened_at?: string }>(incidents.data?.incidents);
  const evs = asArray<Ev>(events.data?.events);
  return (
    <>
      <header className="ws-head"><h1>{t("ac.title")}</h1><p className="ws-lead">{t("ac.lead")}</p></header>
      <section className="glass pad stack" aria-labelledby="ac-monitor">
        <div className="row"><h2 id="ac-monitor">{t("ac.monitor")}</h2>{health.fleet.isLoading ? null : <StatePill tone={health.tone}>{t(health.key)}</StatePill>}</div>
        <div className="metrics four">
          <Metric label={t("nw.online")} value={health.total === null ? null : `${health.up}/${health.total}`} />
          <Metric label={t("nw.stale")} value={health.total === null ? null : health.stale} tone={health.stale ? "warn" : undefined} />
          <Metric label={t("nw.offline")} value={health.total === null ? null : health.offline} tone={health.offline ? "bad" : undefined} />
          <Metric label={t("ac.unknown")} value={health.total === null ? null : health.unknown} />
        </div>
        <p className="soft small">{t("ac.interval")} {health.checkedAt ? <Ago at={health.checkedAt} prefix={t("ws.checked")} /> : null}</p>
      </section>
      <section className="glass pad stack" aria-labelledby="ac-alerts">
        <div className="row"><h2 id="ac-alerts">{t("nav.alerts")}</h2><a className="link small" href="/app/incidents">{t("hm.open")}</a></div>
        {incidents.isLoading ? <Loading><SkRows n={1} /></Loading> : incidents.error ? <Recovery title={t("ac.alertsUnavailable")} onRetry={() => void incidents.mutate()} /> : open.length === 0 ? (
          <p className="note ok small" role="status">{t("ac.noAlerts")}</p>
        ) : (
          <div className="linklist">
            {open.slice(0, 5).map((i) => (
              <a key={i.id} className="linkrow" href="/app/incidents">
                <IconTile name="alert" tone={incidentTone(i.severity) === "bad" ? "bad" : "warn"} />
                <span className="linkrow-text"><b>{i.subject_label || i.kind || t("al.issue")}</b><span className="soft small">{i.detail || t("al.openForDetail")}</span></span>
                <Ago at={i.opened_at} />
              </a>
            ))}
          </div>
        )}
        <p className="soft small">{t("ac.recoveryNote")}</p>
      </section>
      <section className="glass pad stack" aria-labelledby="ac-events">
        <div className="row"><h2 id="ac-events">{t("ac.events")}</h2><a className="link small" href="/app/events">{t("hm.open")}</a></div>
        {events.isLoading ? <Loading><SkList n={3} /></Loading> : events.error ? <p className="note warn small">{t("ac.eventsUnavailable")}</p> : evs.length === 0 ? <p className="soft small">{t("ac.noEvents")}</p> : (
          <ul className="list-table">
            {evs.map((e, i) => <li key={i} className="sub-row"><span className="small" style={{ overflowWrap: "anywhere" }}>{e.message}</span><span className="chip">{e.severity || "info"}</span><span className="meta"><span>{e.program || t("ac.router")}</span><Ago at={e.received_at} /></span></li>)}
          </ul>
        )}
      </section>
      <section className="stack" aria-label={t("ac.more")}>
        <h2 className="group-title">{t("ac.more")}</h2>
        <div className="linklist">
          <LinkRow href="/app/guests" icon="users" title={t("ac.traffic")} detail={t("ac.trafficHelp")} />
          <LinkRow href="/app/fleet" icon="pulse" title={t("fl.title")} detail={t("ac.fleetHelp")} />
          <LinkRow href="/app/security" icon="shield" title={t("sec.title")} detail={t("ac.securityHelp")} />
        </div>
      </section>
    </>
  );
}

interface Change { timestamp?: string; user_role?: string; action?: string; reason?: string }

/** Configuration & approvals: every reviewed change path, and the recent change record per router. */
export function Approvals() {
  const { t } = useI18n();
  const routers = useRouters();
  const first = routers.all?.[0];
  const changes = useSWR<{ changes?: Change[] }>(first ? `/devices/${encodeURIComponent(first.device_id)}/changes` : null, fetcher, { shouldRetryOnError: false });
  const ap = useSWR<{ proposals?: Array<{ id: string; status?: string }> }>("/ap-optimization/proposals", fetcher, { shouldRetryOnError: false });
  const pendingAp = asArray<{ status?: string }>(ap.data?.proposals).filter((p) => !p.status || /pending|proposed|review/i.test(p.status)).length;
  const list = asArray<Change>(changes.data?.changes).slice(0, 8);
  return (
    <>
      <header className="ws-head"><h1>{t("cfg.title")}</h1><p className="ws-lead">{t("cfg.lead")}</p></header>
      <ol className="steps" aria-label={t("cfg.flow")}>
        {["edit", "review", "approve", "apply", "verify"].map((k, i) => (
          <li key={k} className="step"><span className="step-n" aria-hidden="true">{i + 1}</span><div className="step-body"><b>{t("cfg.f." + k)}</b><p className="soft small">{t("cfg.f." + k + "Help")}</p></div></li>
        ))}
      </ol>
      <section className="stack" aria-label={t("cfg.where")}>
        <h2 className="group-title">{t("cfg.where")}</h2>
        <div className="linklist">
          <LinkRow href="/app/network" icon="globe" title={t("nh.internet")} detail={t("cfg.internetHelp")} />
          <LinkRow href="/app/managed-devices" icon="device" title={t("cfg.apOpt")} detail={t("cfg.apOptHelp")} badge={pendingAp ? <span className="chip warn">{pendingAp}</span> : undefined} />
          <LinkRow href="/app/topology" icon="grid" title={t("cfg.deviceControls")} detail={t("cfg.deviceControlsHelp")} />
          <LinkRow href="/app/backups" icon="save" title={t("cfg.rollback")} detail={t("cfg.rollbackHelp")} />
        </div>
      </section>
      <section className="glass pad stack" aria-labelledby="ap-recent">
        <div className="row"><h2 id="ap-recent">{t("cfg.recent")}</h2>{first ? <span className="soft small">{first.hardware_name || first.device_id}</span> : null}</div>
        {!first ? <p className="soft small">{t("nh.noRoutersHelp")}</p> : changes.isLoading ? <Loading><SkList n={3} /></Loading> : changes.error ? <p className="note warn small">{t("cfg.recentUnavailable")}</p> : list.length === 0 ? <p className="soft small">{t("cfg.noChanges")}</p> : (
          <ul className="list-table">
            {list.map((c, i) => <li key={i} className="sub-row"><b>{c.action || t("cfg.change")}</b><span className="chip">{c.user_role || "—"}</span><span className="meta">{c.reason ? <span>{c.reason}</span> : null}<Ago at={c.timestamp} /></span></li>)}
          </ul>
        )}
      </section>
    </>
  );
}
