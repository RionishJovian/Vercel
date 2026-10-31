import useSWR from "swr";
import { fetcher } from "../../lib/api";
import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { tone } from "../Incidents";

interface Inc { id?: string; severity?: string; kind?: string; business_id?: string; subject_label?: string; subject_id?: string; opened_at?: string }

export function PlatformIncidents() {
  const { t } = useI18n();
  const inc = useSWR<{ incidents?: Inc[] } | Inc[]>("/platform/incidents", fetcher, { shouldRetryOnError: false });
  const counts = useSWR<{ counts?: { open?: number; resolved?: number } }>("/platform/incidents/counts", fetcher, { shouldRetryOnError: false });
  const biz = useSWR<{ businesses?: Array<{ business_id: string; company_name?: string }> }>("/platform/businesses", fetcher, { shouldRetryOnError: false });
  if (inc.isLoading) return <Skeleton h={200} />;
  if (inc.error) return <Note kind="bad">{t("c.loadFailed")}</Note>;
  const list = asArray<Inc>(Array.isArray(inc.data) ? inc.data : inc.data?.incidents);
  const names = new Map(asArray<{ business_id: string; company_name?: string }>(biz.data?.businesses).map((b) => [b.business_id, b.company_name]));
  const c = counts.data?.counts ?? {};
  return (
    <>
      <div className="capsules"><div className="capsule"><span className="soft small">{t("pi.open")}</span><b className="num">{c.open ?? 0}</b></div><div className="capsule"><span className="soft small">{t("pi.resolved")}</span><b className="num">{c.resolved ?? 0}</b></div></div>
      {!list.length ? <Note>{t("pi.none")}</Note> : <div className="stack">{list.map((i, k) => (
        <div className="glass pad stack" key={i.id ?? k}><div className="row"><span className={"chip " + tone(i.severity)}>{i.severity || "info"}</span><span className="soft small">{i.opened_at ? new Date(i.opened_at).toLocaleString() : ""}</span></div>
          <b>{names.get(i.business_id ?? "") ?? i.business_id ?? "—"}</b><p className="soft small">{i.subject_label || i.subject_id || "—"} · {i.kind}</p></div>
      ))}</div>}
    </>
  );
}
