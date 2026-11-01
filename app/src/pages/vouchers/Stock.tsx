import { useState } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { asArray, num } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";

interface Pending { batch_id: string; verified_count: number; total_count: number }
interface StockBatch { batch_id: string; device_id?: string; plan_label?: string; total_count?: number; activated_count?: number; installed_count?: number; shop_name?: string; created_at?: string }
interface Batches { batches?: Pending[]; stock_batches?: StockBatch[] }
interface BatchVoucher { id: string; code: string; status?: string; duration_minutes?: number }
interface BatchDetail { data_mb?: number; vouchers?: BatchVoucher[] }
interface Usage { voucher_id: string; exists?: boolean; uptime_seconds?: number; bytes_used?: number }

const fmtMB = (bytes: number) => (bytes / 1_000_000).toLocaleString(undefined, { maximumFractionDigits: bytes < 10_000_000 ? 1 : 0 });

/** Codes of one batch. Usage is read live from the router on request and is not stored by the app. */
function Codes({ batch }: { batch: StockBatch }) {
  const { t } = useI18n();
  const { data, error } = useSWR<BatchDetail>(`/guest-vouchers/batches/${encodeURIComponent(batch.batch_id)}`, fetcher);
  const [usage, setUsage] = useState<Record<string, Usage> | null>(null);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  async function refresh() {
    if (!batch.device_id) return;
    setBusy(true); setFailed(false);
    try {
      const r = await api.post<{ synced?: Usage[] }>("/guest-vouchers/sync", { device_id: batch.device_id });
      setUsage(Object.fromEntries(asArray<Usage>(r.synced).map((u) => [u.voucher_id, u])));
    } catch (e) { if (isUnauthorized(e)) throw e; setFailed(true); }
    setBusy(false);
  }
  const vouchers = asArray<BatchVoucher>(data?.vouchers);
  const limitBytes = (num(data?.data_mb) ?? 0) * 1_000_000;
  return (
    <details className="codes">
      <summary>{t("v.codes")}</summary>
      {error ? <Note kind="bad">{t("c.loadFailed")}</Note> : !data ? <Skeleton h={60} /> : (
        <div className="stack">
          <div className="row">
            <button className="pill" type="button" disabled={busy || !batch.device_id} onClick={() => void refresh()}>{busy ? t("v.refreshing") : t("v.refresh")}</button>
            <span className="soft small">{t("v.liveNote")}</span>
          </div>
          {failed ? <Note kind="bad">{t("v.refreshFailed")}</Note> : null}
          {vouchers.map((v) => {
            const u = usage?.[v.id];
            const limitSec = (num(v.duration_minutes) ?? 0) * 60;
            const bytes = num(u?.bytes_used) ?? 0, sec = num(u?.uptime_seconds) ?? 0;
            return (
              <div className="rowitem stack" key={v.id}>
                <div className="row split"><b className="num">{v.code}</b><span className="chip">{v.status || "—"}</span></div>
                {u ? (
                  <div className="row small">
                    <span className="soft">{t("v.dataUsed")} <b className="num">{fmtMB(bytes)}{limitBytes ? ` / ${fmtMB(limitBytes)}` : ""} MB</b></span>
                    <span className="soft">{t("v.timeUsed")} <b className="num">{Math.floor(sec / 60)}{limitSec ? ` / ${Math.round(limitSec / 60)}` : ""} {t("v.min")}</b></span>
                  </div>
                ) : null}
              </div>
            );
          })}
        </div>
      )}
    </details>
  );
}

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
                <div className="row tk-head"><h2>{b.plan_label || "—"}</h2><b className="num tk-pct">{pct}%</b></div>
                <span className="chip ok">{t("v.onRouter")} <b className="num">{installed}/{total}</b></span>
              </div>
              <div className="ticket-stub">
                <div className="row small"><span className="soft">{t("v.used")}</span><b className="num">{used} / {total}</b></div>
                <div className="bar" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}><i style={{ width: pct + "%" }} /></div>
                <div className="row small"><span className="soft">{t("v.shop")}</span><b>{b.shop_name || t("v.noShop")}</b></div>
                <Codes batch={b} />
              </div>
            </article>
          );
        })}
      </div>
    </div>
  );
}
