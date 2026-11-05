import { useState } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";

interface Alert { id?: string; kind?: string; hardware_name?: string; company_name?: string; device_id?: string; offline_since?: string; detail?: string; created_at?: string; read_at?: string }
export interface RouterAlertsData { alerts?: Alert[]; unread?: number }

const when = (value?: string) => (value ? new Date(value).toLocaleString() : "");

/** Connected routers that stayed down, across all accounts. Only the platform owner sees this. */
export function RouterAlerts() {
  const { t } = useI18n();
  const { data, error, isLoading, mutate } = useSWR<RouterAlertsData>("/platform/router-alerts", fetcher, { refreshInterval: 60000, shouldRetryOnError: false });
  const [busy, setBusy] = useState(false), [failed, setFailed] = useState(false);
  if (isLoading) return <Skeleton h={200} />;
  if (error) return <Note kind="bad">{t("c.loadFailed")}</Note>;
  const list = asArray<Alert>(data?.alerts);
  async function markRead() {
    setBusy(true); setFailed(false);
    try { await api.post("/platform/router-alerts/read", {}); await mutate(); }
    catch (e) { if (isUnauthorized(e)) throw e; setFailed(true); }
    setBusy(false);
  }
  return (
    <div className="stack">
      <p className="soft small">{t("ra.lead")}</p>
      {failed && <Note kind="bad">{t("ra.markFailed")}</Note>}
      {(data?.unread ?? 0) > 0 && <button className="pill" type="button" disabled={busy} onClick={() => void markRead()}>{t("ra.markRead")}</button>}
      {!list.length ? <Note>{t("ra.none")}</Note> : list.map((a, i) => (
        <div className="glass pad stack" key={a.id ?? i}>
          <div className="row"><span className={"chip " + (a.kind === "recovered" ? "ok" : "bad")}>{t(a.kind === "recovered" ? "ra.recovered" : "ra.offline")}</span><span className="soft small">{when(a.created_at)}{a.read_at ? "" : " " + t("ra.new")}</span></div>
          <b><a className="link" href={"#/router/" + encodeURIComponent(a.device_id ?? "")}>{a.hardware_name || t("hub.unnamed")}</a></b>
          <span className="soft small">{a.company_name}</span>
          <span className="soft small">{t("ra.detected")}: {when(a.offline_since)}</span>
          {a.detail && <span className="soft small">{a.detail}</span>}
        </div>
      ))}
    </div>
  );
}
