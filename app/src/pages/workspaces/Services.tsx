import { useMemo, useState } from "react";
import { asArray, mmk, num, type Me } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Ago, IconTile, LinkRow, Metric, Recovery, Segmented, StatePill } from "../../components/ui";
import { Loading, SkList, SkMetrics } from "../../components/Skeleton";
import { bytes, duration, subscriberState, useBroadbandReport, useFleetSummary, usePppoe, useRouters, useVoucherReport } from "./data";

const STAFF = ["OWNER", "ADMIN", "DEVELOPER"];

/** Hotspot service: the MikroTik captive portal, its vouchers and the people using it. */
export function Hotspot() {
  const { t } = useI18n();
  const health = useFleetSummary();
  const routers = useRouters();
  const report = useVoucherReport();
  const rep = report.data;
  const gw = routers.gateways;
  return (
    <>
      <header className="ws-head">
        <h1>{t("hs.title")}</h1>
        <p className="ws-lead">{t("hs.lead")}</p>
      </header>
      <section className="glass pad svc-card workspace-rail" aria-labelledby="hs-status">
        <header><h2 id="hs-status">{t("hs.status")}</h2>{health.fleet.isLoading ? null : <StatePill tone={health.tone}>{t(health.key)}</StatePill>}</header>
        {gw && gw.length === 0 ? (
          <Recovery title={t("isp.noGateway")} detail={t("hs.noGatewayHelp")} action={{ href: "/app/setup", label: t("su.continue") }} />
        ) : (
          <dl className="kv">
            <dt>{t("hs.gateway")}</dt><dd>{gw === null ? (routers.error ? t("c.unavailable") : "…") : gw.map((g) => g.hardware_name || g.device_id).join(", ")}</dd>
            <dt>{t("ws.checked")}</dt><dd><Ago at={health.checkedAt} /></dd>
            <dt>{t("hs.interval")}</dt><dd>{t("hs.intervalValue")}</dd>
          </dl>
        )}
        <p className="soft small">{t("hs.statusNote")}</p>
      </section>
      <section className="glass pad stack workspace-summary" aria-labelledby="hs-usage">
        <h2 id="hs-usage">{t("hs.usage")}</h2>
        {report.isLoading ? <Loading><SkMetrics n={4} cols="four" sub={false} /></Loading> : report.error ? <Recovery title={t("hm.vouchersUnavailable")} onRetry={() => void report.mutate()} /> : (
          <div className="metrics four">
            <Metric label={t("hm.available")} value={rep ? mmk(rep.available_count) : null} />
            <Metric label={t("hs.started7")} value={rep && num(rep.activation_last7) !== null ? mmk(rep.activation_last7) : null} />
            <Metric label={t("hs.expired")} value={rep && num(rep.expired_count) !== null ? mmk(rep.expired_count) : null} />
            <Metric label={t("hs.revoked")} value={rep && num(rep.revoked_count) !== null ? mmk(rep.revoked_count) : null} />
          </div>
        )}
      </section>
      <section className="stack workspace-main" aria-label={t("hs.manage")}>
        <h2 className="group-title">{t("hs.manage")}</h2>
        <div className="linklist">
          <LinkRow href="/app/guests" icon="users" title={t("hs.sessions")} detail={t("hs.sessionsHelp")} />
          <LinkRow href="/app/vouchers" icon="ticket" title={t("nav.vouchers")} detail={t("hs.vouchersHelp")} />
          <LinkRow href="/app/vouchers/guest" icon="globe" title={t("hs.portal")} detail={t("hs.portalHelp")} />
          <LinkRow href="/app/vouchers/vip" icon="shield" title={t("hs.policies")} detail={t("hs.policiesHelp")} />
          <LinkRow href="/app/network" icon="gear" title={t("hs.serverSettings")} detail={t("hs.serverSettingsHelp")} />
        </div>
      </section>
    </>
  );
}

/** PPPoE ISP: subscribers who connect through the router's PPPoE server. Not the router's own PPPoE internet link. */
export function Isp({ me }: { me: Me }) {
  const { t } = useI18n();
  const routers = useRouters();
  const pppoe = usePppoe(routers.gateways);
  const report = useBroadbandReport();
  const s = pppoe.data;
  const staff = STAFF.includes(me.user?.user_role ?? "");
  return (
    <>
      <header className="ws-head">
        <h1>{t("isp.title")}</h1>
        <p className="ws-lead">{t("isp.lead")}</p>
      </header>
      <div className="note small">{t("isp.notWan")} <a className="link" href="/app/network/wan?task=pppoe_client">{t("isp.wanLink")}</a></div>
      <section className="glass pad stack workspace-summary" aria-labelledby="isp-overview">
        <div className="row"><h2 id="isp-overview">{t("isp.overview")}</h2>{s?.newestObservation ? <Ago at={s.newestObservation} prefix={t("ws.reported")} /> : null}</div>
        {routers.gateways && routers.gateways.length === 0 ? (
          <Recovery title={t("isp.noGateway")} detail={t("isp.noGatewayHelp")} action={{ href: "/app/setup", label: t("su.continue") }} />
        ) : routers.error && !routers.data ? <Recovery title={t("isp.unavailable")} onRetry={() => void routers.mutate()} />
        : pppoe.isLoading || routers.isLoading ? <Loading><SkMetrics n={4} cols="four" /></Loading> : pppoe.error ? <Recovery title={t("isp.unavailable")} onRetry={() => void pppoe.mutate()} /> : (
          <>
            <div className="metrics four">
              <Metric label={t("isp.subscribers")} value={s ? mmk(s.total) : null} />
              <Metric label={t("isp.connected")} value={s ? mmk(s.connected) : null} sub={t("isp.connectedHint")} />
              <Metric label={t("isp.suspended")} value={s ? mmk(s.suspended) : null} tone={s?.suspended ? "warn" : undefined} />
              <Metric label={t("isp.expired")} value={s ? mmk(s.expired) : null} tone={s?.expired ? "bad" : undefined} />
            </div>
            {s?.failed.map((f) => <Recovery key={f.routerName} title={t("isp.routerFailed").replace("{router}", f.routerName)} detail={t("isp.routerFailedHelp")} action={{ href: "/app/hub", label: t("su.a.hub") }} onRetry={() => void pppoe.mutate()} advanced={f.reason} />)}
            {s && s.writeEnabled === false ? <p className="note warn small">{t("isp.writesLocked")}</p> : null}
          </>
        )}
      </section>
      <section className="glass pad stack workspace-main" aria-labelledby="isp-pay">
        <h2 id="isp-pay">{t("isp.payments")}</h2>
        {report.isLoading ? <Loading><SkMetrics n={3} cols="three" /></Loading> : report.error ? <p className="note warn small">{t("isp.paymentsUnavailable")}</p> : (
          <div className="metrics three">
            <Metric label={t("isp.paidToday")} value={report.data ? `${mmk(report.data.today_mmk)} ${t("c.mmk")}` : null} />
            <Metric label={t("isp.paidTotal")} value={report.data ? `${mmk(report.data.amount_mmk)} ${t("c.mmk")}` : null} sub={`${report.data?.payment_count ?? 0} ${t("isp.records")}`} />
            <Metric label={t("isp.pendingPay")} value={report.data ? mmk(report.data.pending_count) : null} />
          </div>
        )}
        <p className="soft small">{t("isp.noGatewayPay")}</p>
      </section>
      <div className="workspace-rail workspace-stack">
      <section className="stack" aria-label={t("isp.manageTitle")}>
        <h2 className="group-title">{t("isp.manageTitle")}</h2>
        <div className="linklist">
          {staff ? <LinkRow href="/app/pppoe" icon="users" title={t("isp.l.subs")} detail={t("isp.l.subsHelp")} /> : null}
          <LinkRow href="/app/customers?type=pppoe" icon="list" title={t("isp.l.list")} detail={t("isp.l.listHelp")} />
          {staff ? <LinkRow href="/app/network/pppoe" icon="gear" title={t("isp.l.server")} detail={t("isp.l.serverHelp")} /> : null}
          <LinkRow href="/app/revenue" icon="cash" title={t("isp.l.revenue")} detail={t("isp.l.revenueHelp")} />
        </div>
      </section>
      <section className="stack" aria-label={t("isp.planned")}>
        <h2 className="group-title">{t("isp.planned")}</h2>
        <div className="linklist">
          {["billing", "radius", "fttx"].map((k) => (
            <div key={k} className="linkrow planned" aria-disabled="true"><IconTile name="spark" /><span className="linkrow-text"><b>{t("isp.p." + k)}</b><span className="soft small">{t("isp.p." + k + "Help")}</span></span><span className="chip idle">{t("isp.plannedChip")}</span></div>
          ))}
        </div>
      </section>
      </div>
    </>
  );
}

type CustType = "all" | "hotspot" | "pppoe";
type SubFilter = "all" | "connected" | "suspended" | "expired";

/** Customers: Hotspot users and PPPoE subscribers, always labelled by service. */
export function Customers({ params }: { params: URLSearchParams }) {
  const { t } = useI18n();
  const initial = params.get("type");
  const [type, setType] = useState<CustType>(initial === "pppoe" || initial === "hotspot" ? initial : "all");
  const [q, setQ] = useState("");
  const [filter, setFilter] = useState<SubFilter>("all");
  const routers = useRouters();
  const pppoe = usePppoe(routers.gateways);
  const report = useVoucherReport();
  const rows = useMemo(() => {
    const now = Date.now() / 1000, needle = q.trim().toLowerCase();
    return (pppoe.data?.rows ?? []).filter((r) => {
      if (needle && !(`${r.username} ${r.profile} ${r.routerName}`.toLowerCase().includes(needle))) return false;
      const st = subscriberState(r, now).key;
      return filter === "all" || (filter === "connected" ? !!r.session : filter === "suspended" ? st === "isp.s.suspended" : st === "isp.s.expired");
    });
  }, [pppoe.data, q, filter]);
  return (
    <>
      <header className="ws-head"><h1>{t("cu.title")}</h1><p className="ws-lead">{t("cu.lead")}</p></header>
      <Segmented<CustType> label={t("cu.type")} value={type} onChange={setType} options={[["all", t("cu.all")], ["hotspot", t("cu.hotspot")], ["pppoe", t("cu.pppoe")]]} />
      {type !== "pppoe" && (
        <section className={`glass pad stack workspace-rail${type === "hotspot" ? " customer-panel-solo" : ""}`} aria-labelledby="cu-hs">
          <div className="row"><h2 id="cu-hs">{t("cu.hotspotUsers")}</h2><span className="chip">{t("cu.hotspot")}</span></div>
          <p className="soft small">{t("cu.hotspotHelp")}</p>
          {report.data ? <div className="metrics"><Metric label={t("cu.activeCodes")} value={mmk(report.data.activated_count)} sub={t(report.data.activated_count === 0 ? "cu.noActiveUsage" : "cu.activeCodesHint")} /><Metric label={t("hm.startedToday")} value={mmk(report.data.activation_today)} /></div> : report.isLoading ? <Loading><SkMetrics n={2} /></Loading> : null}
          <div className="actions-row"><a className="btn small" href="/app/guests">{t("cu.openSessions")}</a><a className="btn small quiet" href="/app/vouchers">{t("cu.findCode")}</a></div>
        </section>
      )}
      {type !== "hotspot" && (
        <section className={`glass pad stack workspace-main${type === "pppoe" ? " customer-panel-solo" : ""}`} aria-labelledby="cu-pp">
          <div className="row"><h2 id="cu-pp">{t("cu.subscribers")}</h2><span className="chip">{t("cu.pppoe")}</span></div>
          <div className="toolbar">
            <input className="input" type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("cu.search")} aria-label={t("cu.search")} />
            <select className="input" style={{ flex: "0 1 180px" }} value={filter} onChange={(e) => setFilter(e.target.value as SubFilter)} aria-label={t("cu.filter")}>
              <option value="all">{t("cu.f.all")}</option><option value="connected">{t("isp.connected")}</option><option value="suspended">{t("isp.suspended")}</option><option value="expired">{t("isp.expired")}</option>
            </select>
          </div>
          {routers.gateways?.length === 0 ? <p className="soft small">{t("isp.noGatewayHelp")}</p> : routers.error && !routers.data ? <Recovery title={t("isp.unavailable")} onRetry={() => void routers.mutate()} /> : pppoe.isLoading || !routers.gateways ? <Loading><SkList n={3} /></Loading> : pppoe.error ? <Recovery title={t("isp.unavailable")} onRetry={() => void pppoe.mutate()} /> : rows.length === 0 ? (
            <p className="soft small" role="status">{pppoe.data?.total ? t("cu.noMatch") : t("cu.noSubs")}</p>
          ) : (
            <ul className="list-table" aria-label={t("cu.subscribers")}>
              {rows.slice(0, 200).map((r) => {
                const st = subscriberState(r);
                return (
                  <li key={r.routerId + r.id} className="sub-row">
                    <b>{r.username}</b>
                    <StatePill tone={st.tone}>{t(st.key)}</StatePill>
                    <span className="meta">
                      <span>{t("cu.plan")}: {r.profile || "—"}</span>
                      <span>{r.session ? `${t("isp.connected")} · ${duration(r.session.uptime_seconds)} · ↓${bytes(r.session.download_bytes)} ↑${bytes(r.session.upload_bytes)}` : t("cu.noSession")}</span>
                      {r.observation?.expires_at ? <span>{t("cu.expires")}: {new Date(r.observation.expires_at * 1000).toLocaleDateString()}</span> : null}
                      <span>{r.routerName}</span>
                      <Ago at={r.session_observed_at ?? r.observed_at} prefix={t("ws.reported")} />
                    </span>
                  </li>
                );
              })}
            </ul>
          )}
          {rows.length > 200 ? <p className="soft small">{t("cu.more")}</p> : null}
          <div className="actions-row"><a className="btn small" href="/app/pppoe">{t("cu.manageSubs")}</a></div>
        </section>
      )}
      {asArray(pppoe.data?.failed).length ? <p className="note warn small">{t("isp.partial").replace("{n}", String(pppoe.data?.failed.length))}</p> : null}
    </>
  );
}
