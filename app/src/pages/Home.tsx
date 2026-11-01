import useSWR from "swr";
import { fetcher } from "../lib/api";
import { useFleet } from "../lib/useFleet";
import { asArray, mmk, num, type Me, type Router, type VoucherReport } from "../lib/types";
import { useI18n } from "../i18n";
import { Ring } from "../components/Ring";
import { Skeleton } from "../components/Skeleton";
import { useCountUp } from "../lib/useCountUp";

/** Today against the owner's own 7-day daily average. Half the dial is "on pace". Returns null when there is nothing to compare. */
export function pace(today: number, last7: number): { ratio: number; fraction: number } | null {
  const avg = last7 / 7;
  if (!(avg > 0)) return null;
  const ratio = today / avg;
  return { ratio, fraction: Math.min(ratio, 2) / 2 };
}

/** Stock split from the report's own counts: ready, reserved and used. Nothing is estimated. */
function StockMeter({ rep }: { rep?: VoucherReport }) {
  if (!rep) return null;
  const a = num(rep.available_count) ?? 0, r = num(rep.reserved_count) ?? 0, u = num(rep.activated_count) ?? 0;
  const total = a + r + u;
  if (!total) return null;
  const seg = (n: number, cls: string) => (n ? <i className={cls} style={{ flexGrow: n }} /> : null);
  return <div className="meter slim" role="img" aria-label={`${a} / ${r} / ${u}`}>{seg(a, "s-available")}{seg(r, "s-reserved")}{seg(u, "s-activated")}</div>;
}

export function Home({ me }: { me: Me }) {
  const { t } = useI18n();
  const report = useSWR<VoucherReport>("/voucher-report", fetcher);
  const fleet = useFleet();
  const routers = useSWR<{ routers?: Router[] }>("/routers", fetcher);
  const todayShown = useCountUp(num(report.data?.activation_today));
  const name = me.user?.full_name || (me.user?.email ?? "").split("@")[0];

  const rep = report.data;
  const today = num(rep?.activation_today), last7 = num(rep?.activation_last7), last30 = num(rep?.activation_last30);
  const p = today !== null && last7 !== null ? pace(today, last7) : null;
  const up = num(fleet.data?.up), total = num(fleet.data?.total_peers);
  const list = routers.data ? asArray<Router>(routers.data.routers) : null;
  const allUp = total !== null && total > 0 && up === total;

  return (
    <>
      <div className="greet">
        <h1>{t("h.greet")} <span className="accent">{name}</span></h1>
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
                <span className="num hero-figure">{mmk(todayShown ?? 0)}</span>
                <span className="mute small">{t("c.mmk")}</span>
              </Ring>
              <p className="pace">{p ? <><b className="num">{Math.round(p.ratio * 100)}%</b> {t("home.pace")}</> : t("home.paceNone")}</p>
              <div className="capsules">
                <div className="capsule"><span className="mute small">{t("home.week")}</span><b className="num">{mmk(last7 ?? 0)}</b></div>
                <div className="capsule"><span className="mute small">{t("home.month")}</span><b className="num">{mmk(last30 ?? 0)}</b></div>
              </div>
              <p className="mute small center">{t("h.revenueHelp")}</p>
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
                <StockMeter rep={rep} />
              </>
            )}
          </section>
        </div>
      </div>
    </>
  );
}
