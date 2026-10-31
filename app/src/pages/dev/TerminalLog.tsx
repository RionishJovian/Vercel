import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Field, Note } from "../../components/ui";
import { FilterBar, Loading, Page, Pager, Select, tok, usePaged, when } from "./Paged";

interface Entry { created_at?: string; script?: string; body_unparsed?: boolean; source?: string; status_known?: boolean; status?: number; duration_known?: boolean; duration_ms?: number; response_snippet?: string }
interface Res { entries?: Entry[]; sources?: string[]; total?: number; has_more?: boolean }

export function TerminalLog() {
  const { t } = useI18n();
  const p = usePaged<Res>("/terminal-history", { source: "", status: "", q: "" });
  const rows = asArray<Entry>(p.data?.entries);
  return (
    <Page title={t("dev.tl.title")} lead={t("dev.tl.lead")}>
      <Note>{t("dev.tl.redacted")}</Note>
      <FilterBar onApply={p.apply} onReset={p.reset}>
        <Select label={t("dev.source")} value={p.draft.source} onChange={(v) => p.setDraft("source", v)} all={t("dev.allSources")} options={asArray<string>(p.data?.sources)} />
        <Field label={t("dev.status")}><input className="input" inputMode="numeric" value={p.draft.status} placeholder="200" onChange={(e) => p.setDraft("status", e.target.value)} /></Field>
        <Field label={t("dev.tl.contains")}><input className="input" value={p.draft.q} onChange={(e) => p.setDraft("q", e.target.value)} /></Field>
      </FilterBar>
      <Loading loading={p.isLoading && !p.data} error={p.error} text={t("dev.tl.failed")} />
      {p.data && (<>
        {!rows.length ? <Note>{t("dev.tl.none")}</Note> : <div className="stack">{rows.map((e, i) => (
          <div className="glass pad stack" key={i}>
            <div className="row"><span className="soft small num">{when(e.created_at)}</span>{e.status_known ? <span className={"chip " + (Number(e.status) < 400 ? "ok" : "warn")}>{e.status}</span> : <span className="soft small">—</span>}</div>
            <code className="small" style={{ overflowWrap: "anywhere" }}>{e.script}</code>{e.body_unparsed && <span className="soft small">({t("dev.tl.raw")})</span>}
            <div className="row small"><span className="soft">{e.source || t("dev.tl.manual")}</span><span className="num">{e.duration_known ? tok(e.duration_ms) + " ms" : "—"}</span></div>
            {e.response_snippet && <code className="small soft" style={{ overflowWrap: "anywhere" }}>{e.response_snippet.slice(0, 120)}</code>}
          </div>
        ))}</div>}
        <Pager offset={p.offset} shown={rows.length} total={p.data.total ?? 0} hasMore={p.data.has_more} onNewer={p.newer} onOlder={p.older} filtered={p.filtered} unit={t("dev.tl.unit")} />
      </>)}
    </Page>
  );
}
