import useSWR from "swr";
import { fetcher } from "../lib/api";
import { asArray, mmk, num, type Fleet, type Me, type Router, type VoucherReport } from "../lib/types";
import { useI18n } from "../i18n";
import { Ring } from "../components/Ring";
import { Skeleton } from "../components/Skeleton";

/** Today against the owner's own 7-day daily average. Half the dial is "on pace". Returns null when there is nothing to compare. */
export function pace(today: number, last7: number): { ratio: number; fraction: number } | null {
  const avg = last7 / 7;
  if (!(avg > 0)) return null;
  const ratio = today / avg;
  return { ratio, fraction: Math.min(ratio, 2) / 2 };
}

export function Home({ me }: { me: Me }) {
  const { t } = useI18n();
  const report = useSWR<VoucherReport>("/voucher-report", fetcher);
  const fleet = useSWR<Fleet>("/fleet/health", fetcher, { refreshInterval: 30000 });
  const routers = useSWR<{ routers?: Router[] }>("/routers", fetcher);
  const name = me.user?.full_name || (me.user?.email ?? "").split("@")[0];

  const rep = report.data;
  const today = num(rep?.activation_today), last7 = num(rep?.activation_last7), last30 = num(rep?.activation_last30);
  const p = today !== null && last7 !== null ? pace(today, last7) : null;
  const up = num(fleet.data?.up), total = num(fleet.data?.total_peers);
  const list = routers.data ? asArray<Router>(routers.data.routers) : null;
  const allUp = total !== null && total > 0 && up === total;

  return (
    <>
      <div className="stack greet">
        <h1>{t("h.greet")} <span className="accent">{name}</span></h1>
        <p className="soft">{t("home.sub")}</p>
      </div>

      <div className="home-grid">
        <section className="glass instrument" aria-labelledby="inst-title">
          <h2 id="inst-title" className="sr-only">{t("h.revenue")}</h2>
          {report.isLoading ? <Skeleton h={300} /> : today === null ? (
            <p className="note bad" role="status">{t("h.reportFailed")}</p>
          ) : (
            <>
              <Ring fraction={p ? p.fraction : 0} label={p ? `${Math.round(p.ratio * 100)}% ${t("home.pace")}` : t("home.paceNone")}>
                <span className="mute small">{t("home.today")}</span>
                <span className="num hero-figure">{mmk(today)}</span>
                <span className="mute small">{t("c.mmk")}</span>
              </Ring>
              <p className="pace">{p ? <><b className="num">{Math.round(p.ratio * 100)}%</b> {t("home.pace")}</> : t("home.paceNone")}</p>
              <p className="soft small center">{t("h.revenueHelp")}</p>
              <div className="capsules">
                <div className="capsule"><span className="mute small">{t("home.week")}</span><b className="num">{mmk(last7 ?? 0)}</b></div>
                <div className="capsule"><span className="mute small">{t("home.month")}</span><b className="num">{mmk(last30 ?? 0)}</b></div>
              </div>
              <a className="btn quiet" href="#/revenue">{t("rv.open")}</a>
            </>
          )}
        </section>

        <div className="stack side">
          <section className="glass pad">
            <div className="row"><h2>{t("home.routers")}</h2>
              {total === null ? null : <span className={"chip " + (allUp ? "ok" : "warn")}>{allUp ? t("home.allOnline") : t("home.someOffline")}</span>}
            </div>
            {fleet.isLoading ? <Skeleton h={64} /> : total === null ? <p className="note bad" role="status">{t("h.fleetFailed")}</p> : (
              <div className="leds" aria-label={`${up} / ${total}`}>
                {Array.from({ length: Math.min(total, 12) }, (_, i) => <i key={i} className={i < (up ?? 0) ? "on" : "off"} />)}
                <b className="num">{up} / {total}</b>
              </div>
            )}
            {list && list.length === 0 ? <p className="note warn">{t("home.noRouters")} <a className="link" href="#/router">{t("home.addRouter")}</a></p> : null}
          </section>
          <section className="glass pad">
            <h2>{t("home.vouchers")}</h2>
            {report.isLoading ? <Skeleton h={64} /> : (
              <>
                <p className="num stock-figure">{rep && num(rep.available_count) !== null ? mmk(rep.available_count) : t("c.unavailable")}</p>
                <p className="soft small">{t("home.stockHelp")}</p>
              </>
            )}
          </section>
        </div>
      </div>
    </>
  );
}
