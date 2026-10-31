import { useState, type ReactNode } from "react";
import useSWR from "swr";
import { fetcher } from "../../lib/api";
import { useI18n } from "../../i18n";
import { BackLink, Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";

export type Filters = Record<string, string>;
export const tok = (n: unknown) => Number(n || 0).toLocaleString();
export const when = (v?: string | null) => (v ? new Date(v).toLocaleString() : "—");

/** Filtered, paged list over a read-only endpoint. Applying a filter returns to the first page. */
export function usePaged<T>(path: string, initial: Filters, limit = 50) {
  const [applied, setApplied] = useState<Filters>(initial);
  const [draft, setDraft] = useState<Filters>(initial);
  const [offset, setOffset] = useState(0);
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(applied)) if (v) qs.set(k, v);
  qs.set("limit", String(limit)); qs.set("offset", String(offset));
  const res = useSWR<T>(`${path}?${qs.toString()}`, fetcher, { keepPreviousData: true, shouldRetryOnError: false });
  return {
    ...res, draft, setDraft: (k: string, v: string) => setDraft((d) => ({ ...d, [k]: v })), applied, offset,
    apply: () => { setOffset(0); setApplied(draft); },
    reset: () => { setDraft(initial); setApplied(initial); setOffset(0); },
    newer: () => setOffset(Math.max(0, offset - limit)), older: () => setOffset(offset + limit), limit,
    filtered: Object.values(applied).some(Boolean),
  };
}

export function Page({ title, lead, children }: { title: string; lead?: string; children: ReactNode }) {
  const { t } = useI18n();
  return <><BackLink href="#/more">{t("tab.more")}</BackLink><h1>{title}</h1>{lead && <p className="soft">{lead}</p>}{children}</>;
}

export function FilterBar({ children, onApply, onReset }: { children: ReactNode; onApply: () => void; onReset: () => void }) {
  const { t } = useI18n();
  return (
    <form className="glass pad stack" onSubmit={(e) => { e.preventDefault(); onApply(); }}>
      {children}
      <div className="row" style={{ justifyContent: "flex-start" }}><button className="btn" type="submit">{t("ev.apply")}</button><button className="btn quiet" type="button" onClick={onReset}>{t("ev.reset")}</button></div>
    </form>
  );
}
export function Select({ label, value, onChange, all, options }: { label: string; value: string; onChange: (v: string) => void; all: string; options: Array<string | [string, string]> }) {
  return <label className="field"><span>{label}</span><select className="input" value={value} onChange={(e) => onChange(e.target.value)}><option value="">{all}</option>{options.map((o) => { const [v, l] = Array.isArray(o) ? o : [o, o]; return <option key={v} value={v}>{l}</option>; })}</select></label>;
}
export function Pager({ offset, shown, total, hasMore, onNewer, onOlder, filtered, unit }: { offset: number; shown: number; total: number; hasMore?: boolean; onNewer: () => void; onOlder: () => void; filtered: boolean; unit: string }) {
  const { t } = useI18n();
  return (
    <>
      <p className="soft small" role="status">{t("ev.showing")} {shown ? offset + 1 : 0}–{offset + shown} / {tok(total)} {unit}{filtered ? ` (${t("ev.filtered")})` : ""}</p>
      <div className="row"><button className="pill" type="button" disabled={offset === 0} onClick={onNewer}>{t("ev.newer")}</button><button className="pill" type="button" disabled={!hasMore} onClick={onOlder}>{t("ev.older")}</button></div>
    </>
  );
}
export function Loading({ error, loading, text }: { error?: unknown; loading: boolean; text: string }) {
  if (loading) return <Skeleton h={200} />;
  return error ? <Note kind="bad">{text}</Note> : null;
}
