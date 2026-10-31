import { useState } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { asArray, num } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";

interface Pending { batch_id: string; verified_count: number; total_count: number }
interface StockBatch { batch_id: string; plan_label?: string; total_count?: number; activated_count?: number; installed_count?: number; shop_name?: string; created_at?: string }
interface Batches { batches?: Pending[]; stock_batches?: StockBatch[] }

export function Stock() {
  const { t } = useI18n();
  const { data, error, isLoading, mutate } = useSWR<Batches>("/guest-vouchers/batches", fetcher);
  const [retrying, setRetrying] = useState<string | null>(null);
  if (isLoading) return <Skeleton h={220} />;
  if (error) return <Note kind="bad">{t("c.loadFailed")}</Note>;
  const incomplete = asArray<Pending>(data?.batches);
  const batches = asArray<StockBatch>(data?.stock_batches).slice().sort((a, b) => String(b.created_at).localeCompare(String(a.created_at)));
  if (!batches.length && !incomplete.length) return <Note>{t("v.empty")}</Note>;

  async function retry(id: string) {
    setRetrying(id);
    try { await api.post(`/guest-vouchers/batches/${encodeURIComponent(id)}/retry`, {}); } catch (e) { if (isUnauthorized(e)) throw e; }
    setRetrying(null); void mutate();
  }
  return (
    <div className="stack">
      {incomplete.length > 0 && (
        <section className="glass pad stack">
          <h2>{t("v.pendingTitle")}</h2><p className="soft small">{t("v.pendingBody")}</p>
          {incomplete.map((b) => (
            <div className="row" key={b.batch_id}>
              <b className="num">{b.verified_count} / {b.total_count}</b>
              <button className="pill warn" type="button" disabled={retrying === b.batch_id} onClick={() => void retry(b.batch_id)}>{retrying === b.batch_id ? t("v.retrying") : t("v.retry")}</button>
            </div>
          ))}
        </section>
      )}
      <div className="tickets">
        {batches.map((b) => {
          const total = num(b.total_count) ?? 0, used = num(b.activated_count) ?? 0, installed = num(b.installed_count) ?? 0;
          const pct = total ? Math.round((used / total) * 100) : 0;
          return (
            <article className="ticket" key={b.batch_id}>
              <div className="ticket-main">
                <h2>{b.plan_label || "—"}</h2>
                <span className="chip ok">{t("v.onRouter")} <b className="num">{installed}/{total}</b></span>
              </div>
              <div className="ticket-stub">
                <div className="row small"><span className="soft">{t("v.used")}</span><b className="num">{used} / {total}</b></div>
                <div className="bar" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}><i style={{ width: pct + "%" }} /></div>
                <div className="row small"><span className="soft">{t("v.shop")}</span><b>{b.shop_name || t("v.noShop")}</b></div>
              </div>
            </article>
          );
        })}
      </div>
    </div>
  );
}
