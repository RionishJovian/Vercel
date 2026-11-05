import useSWR from "swr";
import { fetcher } from "../lib/api";
import { mmk } from "../lib/types";
import { useI18n } from "../i18n";
import { Note, SectionHead } from "../components/ui";

export function BroadbandRevenue() {
 const { t } = useI18n();
 const { data, error, isValidating, mutate } = useSWR<{ amount_mmk: number; today_mmk: number; payment_count: number; pending_count: number }>("/broadband/report", fetcher, { shouldRetryOnError: false });
 return <section className="glass pad stack" aria-label={t("bb.report")}>
  <div className="row" style={{ flexWrap: "wrap" }}><SectionHead title={t("bb.report")} help={t("bb.reportHelp")} /><button className="pill" type="button" disabled={isValidating} onClick={() => void mutate()}>{t("bb.reload")}</button></div>
  {error && <Note kind="warn"><p>{t("bb.reportFailed")}</p>{data && <p>{t("bb.savedReport")}</p>}</Note>}
  {!data && !error && <p>…</p>}
  {data && <>
   <div className="row"><span>{t("bb.today")}: <b>{mmk(data.today_mmk)} {t("c.mmk")}</b></span><span>{t("bb.total")}: <b>{mmk(data.amount_mmk)} {t("c.mmk")}</b></span></div>
   {data.pending_count > 0 && <p className="note warn">{t("bb.pending")}: {data.pending_count}</p>}
  </>}
 </section>;
}
