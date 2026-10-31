import useSWR from "swr";
import { fetcher } from "../lib/api";
import { asArray, mmk, num, type VoucherReport } from "../lib/types";
import { useI18n } from "../i18n";
import { Icon } from "../components/icons";
import { Skeleton } from "../components/Skeleton";
import { RevenuePayments } from "./RevenuePayments";
import type { Me } from "../lib/types";

const STATES = [
  ["available_count", "rv.available", "s-available"], ["reserved_count", "rv.reserved", "s-reserved"], ["activated_count", "rv.activated", "s-activated"],
  ["expired_count", "rv.expired", "s-expired"], ["revoked_count", "rv.revoked", "s-revoked"],
] as const;

export function Revenue({ me }: { me: Me }) {
  const { t } = useI18n();
  const { data: r, error, isLoading } = useSWR<VoucherReport>("/voucher-report", fetcher);
  if (isLoading) return <Skeleton h={360} />;
  if (error || !r) return <p className="note bad" role="alert">{t("rv.failed")}</p>;
  const counts = STATES.map(([k, label, cls]) => ({ label, cls, n: num(r[k]) ?? 0 }));
  const total = counts.reduce((s, c) => s + c.n, 0);
  const gateways = asArray<NonNullable<VoucherReport["gateways"]>[number]>(r.gateways);
  return (
    <>
      <a className="backlink" href="#/home"><Icon name="back" />{t("rv.back")}</a>
      <div className="stack"><h1>{t("rv.title")}</h1><p className="soft">{t("rv.help")}</p></div>

      <section className="glass pad ledger" aria-labelledby="rv-used">
        <h2 id="rv-used" className="mute small">{t("rv.used")}</h2>
        <p className="num ledger-figure">{mmk(num(r.activation_today) ?? 0)}<small>{t("c.mmk")}</small></p>
        <p className="mute small">{t("h.today")}</p>
        <div className="capsules">
          <div className="capsule"><span className="mute small">{t("h.last7")}</span><b className="num">{mmk(num(r.activation_last7) ?? 0)}</b></div>
          <div className="capsule"><span className="mute small">{t("h.last30")}</span><b className="num">{mmk(num(r.activation_last30) ?? 0)}</b></div>
          <div className="capsule"><span className="mute small">{t("rv.all")}</span><b className="num">{mmk(num(r.activation_value) ?? 0)}</b></div>
        </div>
        {(num(r.activation_time_unknown_count) ?? 0) > 0 ? <p className="note warn">{r.activation_time_unknown_count} {t("rv.unknown")}</p> : null}
      </section>

      <section className="glass pad" aria-labelledby="rv-counts">
        <h2 id="rv-counts">{t("rv.counts")}</h2>
        <div className="meter" role="img" aria-label={counts.map((c) => `${t(c.label)} ${c.n}`).join(", ")}>
          {total > 0 ? counts.filter((c) => c.n > 0).map((c) => <i key={c.cls} className={c.cls} style={{ flexGrow: c.n }} />) : <i className="s-empty" style={{ flexGrow: 1 }} />}
        </div>
        <ul className="legend">
          {counts.map((c) => <li key={c.cls}><i className={c.cls} /><span className="soft">{t(c.label)}</span><b className="num">{mmk(c.n)}</b></li>)}
        </ul>
      </section>

      {gateways.length > 0 ? (
        <section className="stack" aria-labelledby="rv-by">
          <h2 id="rv-by">{t("rv.byRouter")}</h2>
          {gateways.map((g, i) => (
            <div className="rowitem split" key={i}>
              <b>{g.gateway_name || "—"}</b>
              <span className="soft small">{t("rv.available")} <b className="num">{mmk(num(g.available_count) ?? 0)}</b></span>
              <span className="soft small">{t("rv.activated")} <b className="num">{mmk(num(g.activated_count) ?? 0)}</b></span>
              <span className="num">{mmk(num(g.activation_value) ?? 0)} {t("c.mmk")}</span>
            </div>
          ))}
        </section>
      ) : null}
      <RevenuePayments canDecide={["ADMIN", "OWNER", "DEVELOPER"].includes(me.user?.user_role ?? "")} />
    </>
  );
}
