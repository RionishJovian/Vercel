import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { FilterBar, Loading, Page, Pager, Select, tok, usePaged, when } from "./Paged";

interface Group { name: string; events?: number; total_tokens?: number }
interface Ev { created_at?: string; feature?: string; model?: string; prompt_tokens?: number; completion_tokens?: number; total_tokens?: number; token_mismatch?: boolean }
interface Res { events?: Ev[]; total?: number; has_more?: boolean; summary?: { events?: number; total_tokens?: number; prompt_tokens?: number; completion_tokens?: number; cached_events?: number; mismatched_events?: number; by_feature?: Group[]; by_model?: Group[] } }

function Groups({ title, list }: { title: string; list: Group[] }) {
  const { t } = useI18n();
  return <section className="glass pad stack"><h2>{title}</h2>{!list.length ? <p className="soft small">{t("dev.none")}</p> : list.map((g) => <div className="row small" key={g.name}><span>{g.name}</span><span className="soft num">{tok(g.events)} / {tok(g.total_tokens)}</span></div>)}</section>;
}

export function AIUsage() {
  const { t } = useI18n();
  const p = usePaged<Res>("/ai-usage-events", { feature: "", model: "" });
  const s = p.data?.summary ?? {}, evs = asArray<Ev>(p.data?.events);
  const opts = (g?: Group[]) => asArray<Group>(g).map((x): [string, string] => [x.name, `${x.name} (${tok(x.total_tokens)})`]);
  return (
    <Page title={t("dev.ai.title")} lead={t("dev.ai.lead")}>
      <Loading loading={p.isLoading && !p.data} error={p.error} text={t("dev.ai.failed")} />
      {p.data && (<>
        <section className="glass pad stack"><p className="num"><b>{tok(s.events)}</b> {t("dev.ai.calls")} · <b>{tok(s.total_tokens)}</b> {t("dev.ai.tokens")} ({tok(s.prompt_tokens)} {t("dev.ai.prompt")} + {tok(s.completion_tokens)} {t("dev.ai.completion")}){Number(s.cached_events) > 0 ? ` · ${tok(s.cached_events)} ${t("dev.ai.cached")}` : ""}</p></section>
        {Number(s.mismatched_events) > 0 && <Note kind="warn">{tok(s.mismatched_events)} {t("dev.ai.mismatch")}</Note>}
        <div className="two"><Groups title={t("dev.ai.byFeature")} list={asArray<Group>(s.by_feature)} /><Groups title={t("dev.ai.byModel")} list={asArray<Group>(s.by_model)} /></div>
        <FilterBar onApply={p.apply} onReset={p.reset}>
          <div className="two"><Select label={t("dev.ai.feature")} value={p.draft.feature} onChange={(v) => p.setDraft("feature", v)} all={t("dev.ai.allFeatures")} options={opts(s.by_feature)} /><Select label={t("dev.ai.model")} value={p.draft.model} onChange={(v) => p.setDraft("model", v)} all={t("dev.ai.allModels")} options={opts(s.by_model)} /></div>
        </FilterBar>
        {!evs.length ? <Note>{t("dev.ai.none")}</Note> : <div className="stack">{evs.map((e, i) => (
          <div className="rowitem" key={i}><div className="stack" style={{ gap: 2 }}><b className="small">{e.feature}</b><span className="soft small">{e.model} · {when(e.created_at)}</span></div><span className="num small">{tok(e.prompt_tokens)} + {tok(e.completion_tokens)} = {tok(e.total_tokens)}{e.token_mismatch ? " ≠" : ""}</span></div>
        ))}</div>}
        <Pager offset={p.offset} shown={evs.length} total={p.data.total ?? 0} hasMore={p.data.has_more} onNewer={p.newer} onOlder={p.older} filtered={p.filtered} unit={t("dev.ai.unit")} />
      </>)}
    </Page>
  );
}
