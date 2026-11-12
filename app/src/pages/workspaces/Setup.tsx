import useSWR from "swr";
import { ApiError, fetcher } from "../../lib/api";
import { asArray, num } from "../../lib/types";
import { connectionIsFresh, type Conn, type Site } from "../../lib/siteState";
import { useI18n } from "../../i18n";
import { Icon } from "../../components/icons";
import { Ago, StatePill, type Tone } from "../../components/ui";
import { Bar, Loading, SkSteps } from "../../components/Skeleton";
import { useFleetSummary, useRouters, useVoucherReport } from "./data";

type StepState = "done" | "todo" | "manual" | "blocked";
interface Step { id: string; title: string; help: string; href: string; action: string; state: StepState; evidence?: string; at?: string }

/**
 * Guided Hotspot setup computed from real records. A step is "done" only when the API shows it;
 * steps the cloud cannot observe (internet on the router, guest page on the router) are "check on router",
 * never ticked automatically. Nothing here changes the router: each step opens its existing screen.
 */
export function useSetupProgress() {
  const sites = useSWR<{ sites?: Site[] }>("/sites", fetcher, { shouldRetryOnError: false });
  const conns = useSWR<{ connections?: Conn[] }>("/router-connections", fetcher, { shouldRetryOnError: false });
  const routers = useRouters();
  const fleet = useFleetSummary();
  const report = useVoucherReport();
  const portal = useSWR<unknown>("/portal/config", fetcher, { shouldRetryOnError: false });
  const loading = sites.isLoading || conns.isLoading || routers.isLoading || report.isLoading || portal.isLoading;

  const siteList = asArray<Site>(sites.data?.sites);
  const connList = asArray<Conn>(conns.data?.connections);
  const fresh = connList.find((c) => connectionIsFresh(c));
  const gateways = routers.gateways ?? [];
  const rep = report.data;
  const codes = rep ? (num(rep.available_count) ?? 0) + (num(rep.reserved_count) ?? 0) + (num(rep.activated_count) ?? 0) + (num(rep.expired_count) ?? 0) : 0;
  const portalSaved = !!portal.data && !(portal.error instanceof ApiError && portal.error.status === 404);
  const firstUse = rep ? (num(rep.activated_count) ?? 0) > 0 : false;

  const steps: Step[] = [
    { id: "site", title: "su.s.site", help: "su.h.site", href: "#/sites", action: "su.a.site", state: siteList.length ? "done" : "todo", evidence: siteList[0]?.name },
    { id: "hub", title: "su.s.hub", help: "su.h.hub", href: "#/hub", action: "su.a.hub", state: fresh ? "done" : !siteList.length ? "blocked" : "todo", at: fresh?.cloud_last_seen_at },
    { id: "router", title: "su.s.router", help: "su.h.router", href: "#/hub", action: "su.a.router", state: gateways.length && fleet.tone === "ok" ? "done" : !fresh && !gateways.length ? "blocked" : "todo", evidence: gateways[0]?.hardware_name },
    { id: "internet", title: "su.s.internet", help: "su.h.internet", href: "#/network/new", action: "su.a.internet", state: gateways.length ? "manual" : "blocked" },
    { id: "plan", title: "su.s.plan", help: "su.h.plan", href: "#/vouchers/create", action: "su.a.plan", state: codes > 0 ? "done" : "todo", evidence: codes ? String(codes) : undefined },
    { id: "portal", title: "su.s.portal", help: "su.h.portal", href: "#/vouchers/guest", action: "su.a.portal", state: portalSaved ? "manual" : "todo" },
    { id: "test", title: "su.s.test", help: "su.h.test", href: "#/guests", action: "su.a.test", state: firstUse ? "done" : codes ? "todo" : "blocked" },
  ];
  // Internet and the guest page on the router cannot be observed from the cloud, so they never count toward progress.
  const checkable = steps.filter((s) => s.id !== "internet" && s.id !== "portal");
  const done = checkable.filter((s) => s.state === "done").length;
  const next = steps.find((s) => s.state === "todo") ?? steps.find((s) => s.state === "manual");
  const failed = [sites, conns, report].some((r) => r.error && !(r.error instanceof ApiError && r.error.status === 404)) || !!routers.error;
  return { loading, steps, done, total: checkable.length, complete: !failed && done === checkable.length, next, failed };
}

const TONE: Record<StepState, Tone> = { done: "ok", todo: "info", manual: "warn", blocked: "idle" };

export function Setup() {
  const { t } = useI18n();
  const p = useSetupProgress();
  return (
    <>
      <header className="ws-head">
        <h1>{t("su.title")}</h1>
        <p className="ws-lead">{t("su.lead")}</p>
      </header>
      {p.loading ? <Loading className="sk-page"><div className="glass pad stack"><Bar w="45%" /><span className="progress" /><div className="dude-tip"><span className="skeleton" style={{ width: 40, height: 40, borderRadius: "50%", flex: "none" }} /><span style={{ flex: 1 }}><Bar w="95%" size=".875rem" /><Bar w="90%" size=".875rem" /><Bar w="60%" size=".875rem" /></span></div></div><SkSteps n={7} /></Loading> : (
        <>
          <section className="glass pad stack">
            <div className="row"><b>{p.complete ? t("su.ready") : t("su.progress").replace("{done}", String(p.done)).replace("{total}", String(p.total))}</b>{p.complete ? <StatePill tone="ok">{t("su.siteReady")}</StatePill> : null}</div>
            <div className="progress" role="progressbar" aria-valuemin={0} aria-valuemax={p.total} aria-valuenow={p.done} aria-label={t("su.title")}><i style={{ width: `${(p.done / Math.max(1, p.total)) * 100}%` }} /></div>
            {p.failed ? <p className="note warn small">{t("su.partial")}</p> : null}
            <div className="dude-tip"><img src="/v2/magic-dude.png" alt="" width={40} height={40} /><p className="small">{t("su.dude")}</p></div>
          </section>
          <ol className="steps">
            {p.steps.map((s, i) => {
              const isNext = p.next?.id === s.id;
              return (
                <li key={s.id} className="step" data-state={isNext ? "next" : s.state === "done" ? "done" : undefined}>
                  <span className="step-n" aria-hidden="true">{s.state === "done" ? <Icon name="check" /> : i + 1}</span>
                  <div className="step-body">
                    <div className="row"><b>{t(s.title)}</b><StatePill tone={TONE[s.state]}>{t("su.st." + s.state)}</StatePill></div>
                    <p className="soft small">{t(s.help)}</p>
                    {s.evidence ? <p className="small"><span className="mute">{t("su.evidence")}</span> {s.evidence}</p> : null}
                    {s.at ? <Ago at={s.at} prefix={t("ws.reported")} /> : null}
                    {s.state !== "done" && s.state !== "blocked" ? <div className="actions-row"><a className={"btn small" + (isNext ? "" : " quiet")} href={s.href}>{t(s.action)}</a></div> : null}
                    {s.state === "blocked" ? <p className="mute small">{t("su.blockedHelp")}</p> : null}
                  </div>
                </li>
              );
            })}
          </ol>
          <p className="soft small">{t("su.noLocalDiscovery")}</p>
        </>
      )}
    </>
  );
}
