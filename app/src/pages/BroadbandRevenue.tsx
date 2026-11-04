import useSWR from "swr";
import { fetcher } from "../lib/api";
import { mmk } from "../lib/types";
import { useI18n } from "../i18n";

export function BroadbandRevenue() {
 const { t } = useI18n();
 const { data, error } = useSWR<{ amount_mmk: number; today_mmk: number; payment_count: number; pending_count: number }>("/broadband/report", fetcher, { shouldRetryOnError: false });
 return <section className="glass pad stack" aria-label={t("bb.report")}>
  <h2>{t("bb.report")}</h2>
  {error ? <p className="note warn">{t("bb.loadFailed")}</p> : !data ? <p>…</p> : <>
   <div className="row"><span>{t("bb.today")}: <b>{mmk(data.today_mmk)} {t("c.mmk")}</b></span><span>{t("bb.total")}: <b>{mmk(data.amount_mmk)} {t("c.mmk")}</b></span></div>
   {data.pending_count > 0 && <p className="note warn">{t("bb.pending")}: {data.pending_count}</p>}
  </>}
  <p className="soft small">{t("bb.reportHelp")}</p>
 </section>;
}
