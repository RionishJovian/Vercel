import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { FilterBar, Loading, Page, Pager, Select, usePaged, when } from "./Paged";

interface Finding { severity?: string; runtime_error?: boolean; title?: string; subtitle?: string; router?: string; suggestion?: string }
interface Run { generated_at?: string; kind?: string; max_severity?: string; router_count?: number; findings?: Finding[]; error_findings?: number; node?: { node_id?: string; node_status?: string; source?: string } }
interface Res { scans?: Run[]; kinds?: string[]; severities?: string[]; severity_counts?: Record<string, number>; crash_artifact_scans?: number; total?: number; has_more?: boolean }
const tone = (s?: string) => (s === "critical" ? "bad" : s === "warning" ? "warn" : s === "ok" ? "ok" : "");

export function Scans() {
  const { t } = useI18n();
  const p = usePaged<Res>("/fleet-scan-runs", { kind: "", severity: "", findings_only: "" });
  const runs = asArray<Run>(p.data?.scans);
  const crashes = Number(p.data?.crash_artifact_scans ?? 0);
  return (
    <Page title={t("dev.sc.title")} lead={t("dev.sc.lead")}>
      <FilterBar onApply={p.apply} onReset={p.reset}>
        <div className="two">
          <Select label={t("dev.kind")} value={p.draft.kind} onChange={(v) => p.setDraft("kind", v)} all={t("dev.allKinds")} options={asArray<string>(p.data?.kinds)} />
          <Select label={t("dev.sc.worst")} value={p.draft.severity} onChange={(v) => p.setDraft("severity", v)} all={t("ev.allSeverities")} options={asArray<string>(p.data?.severities).map((s): [string, string] => [s, `${s}${p.data?.severity_counts?.[s] ? ` (${p.data.severity_counts[s]})` : ""}`])} />
        </div>
        <label className="check"><span>{t("dev.sc.only")}</span><input type="checkbox" checked={p.draft.findings_only === "1"} onChange={(e) => p.setDraft("findings_only", e.target.checked ? "1" : "")} /></label>
      </FilterBar>
      {crashes > 0 && <Note kind="warn">{crashes} {t("dev.sc.crash")}</Note>}
      <Loading loading={p.isLoading && !p.data} error={p.error} text={t("dev.sc.failed")} />
      {p.data && (<>
        {!runs.length ? <Note>{t("dev.sc.none")}</Note> : <div className="stack">{runs.map((r, i) => {
          const fs = asArray<Finding>(r.findings);
          return (
            <article className="glass pad stack" key={i}>
              <div className="row"><span className={"chip " + tone(r.max_severity)}>{r.max_severity || "info"}</span><span className="soft small num">{when(r.generated_at)}</span></div>
              <div className="row small"><b>{r.kind}</b><span className="soft">{r.router_count} {t("dev.sc.routers")}</span></div>
              {fs.slice(0, 3).map((f, k) => (
                <div className="stack" key={k} style={{ gap: 2 }}>
                  <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}><span className={"chip " + (f.runtime_error ? "warn" : tone(f.severity))} title={f.runtime_error ? t("dev.sc.errTitle") : undefined}>{f.runtime_error ? t("dev.sc.scanError") : f.severity || "info"}</span><span className="small">{f.title || t("dev.sc.untitled")}</span>{f.router && <span className="soft small">{f.router}</span>}</div>
                  {f.subtitle && <span className="soft small">{f.subtitle}</span>}{f.suggestion && <span className="soft small">{f.suggestion}</span>}
                </div>
              ))}
              {fs.length > 3 && <span className="soft small">+{fs.length - 3} {t("dev.sc.more")}</span>}
              {!fs.length && (r.node && (r.node.node_status || r.node.node_id) ? <span className="small"><span className={"chip " + tone(r.max_severity)}>{r.max_severity}</span> {r.node.node_status || r.node.node_id} <span className="soft">{r.node.source}</span></span> : <span className="soft small">{t("dev.sc.noFindings")}</span>)}
              {Number(r.error_findings) > 0 && <span className="soft small">{r.error_findings} / {fs.length} {t("dev.sc.areErrors")}</span>}
            </article>
          );
        })}</div>}
        <Pager offset={p.offset} shown={runs.length} total={p.data.total ?? 0} hasMore={p.data.has_more} onNewer={p.newer} onOlder={p.older} filtered={p.filtered} unit={t("dev.sc.unit")} />
      </>)}
    </Page>
  );
}
