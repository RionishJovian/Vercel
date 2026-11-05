import { useEffect, useRef } from "react";
import { useI18n } from "../i18n";
import "./tables.css";

/** Page numbers to show; null marks a gap. Up to seven pages are all listed. */
export function pageWindow(current: number, pages: number): Array<number | null> {
  if (pages <= 7) return Array.from({ length: pages }, (_, i) => i + 1);
  const keep = new Set([1, pages, current - 1, current, current + 1]);
  const out: Array<number | null> = [];
  let last = 0;
  for (let p = 1; p <= pages; p++) {
    if (!keep.has(p)) continue;
    if (p - last > 1) out.push(null);
    out.push(p);
    last = p;
  }
  return out;
}

export function Pager({ page, perPage, total, shown, onPage }: { page: number; perPage: number; total: number; shown: number; onPage: (p: number) => void }) {
  const { t } = useI18n();
  const pages = Math.max(1, Math.ceil(total / perPage));
  const first = total ? (page - 1) * perPage + 1 : 0;
  const summary = t("vt.pageOf").replace("{p}", String(page)).replace("{a}", String(first)).replace("{b}", String(first ? first + shown - 1 : 0)).replace("{n}", String(total));
  return (
    <nav className="dt-pager" aria-label={t("vt.page")}>
      <span className="soft small">{summary}</span>
      <div className="dt-pages">
        <button className="dt-page" type="button" disabled={page <= 1} onClick={() => onPage(page - 1)}>{t("vt.prev")}</button>
        {pageWindow(page, pages).map((p, i) => p === null
          ? <span className="soft dt-gap" key={"gap" + i} aria-hidden="true">…</span>
          : <button className="dt-page" type="button" key={p} aria-current={p === page ? "page" : undefined} aria-label={`${t("vt.page")} ${p}`} onClick={() => onPage(p)}><span className="dt-wide">{t("vt.page")} </span>{p}</button>)}
        <button className="dt-page" type="button" disabled={page >= pages} onClick={() => onPage(page + 1)}>{t("vt.next")}</button>
      </div>
    </nav>
  );
}

/** A select-all checkbox that shows the mixed state when only some rows are chosen. */
export function HeadCheck({ label, all, some, onChange }: { label: string; all: boolean; some: boolean; onChange: (on: boolean) => void }) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => { if (ref.current) ref.current.indeterminate = some && !all; }, [some, all]);
  return <input ref={ref} type="checkbox" aria-label={label} checked={all} onChange={(e) => onChange(e.target.checked)} />;
}
