import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Field, Note } from "../../components/ui";
import { FilterBar, Loading, Page, Pager, Select, tok, usePaged, when } from "./Paged";

interface Key { created_at?: string; key_name?: string; key_kind?: string; attributes?: string; price_coins?: number; status?: string; expires_at?: string }
interface Res { keys?: Key[]; kinds?: string[]; total?: number; has_more?: boolean; summary?: { activations?: number; coins_spent?: number; active?: number; expired?: number; consumed?: number; by_kind?: Array<{ name: string; count?: number; coins_spent?: number }> } }

export function UnlockKeys() {
  const { t } = useI18n();
  const p = usePaged<Res>("/unlock-keys", { kind: "", status: "", q: "" });
  const s = p.data?.summary ?? {}, keys = asArray<Key>(p.data?.keys);
  return (
    <Page title={t("dev.uk.title")} lead={t("dev.uk.lead")}>
      <Loading loading={p.isLoading && !p.data} error={p.error} text={t("dev.uk.failed")} />
      {p.data && (<>
        <section className="glass pad stack"><h2>{t("dev.uk.spent")}</h2><p className="num small">{tok(s.activations)} {t("dev.uk.bought")} · {tok(s.coins_spent)} {t("dev.uk.coins")} · {tok(s.active)} {t("dev.uk.active")} · {tok(s.expired)} {t("dev.uk.expired")} · {tok(s.consumed)} {t("dev.uk.consumed")}</p>
          {asArray<{ name: string; count?: number; coins_spent?: number }>(s.by_kind).map((g) => <div className="row small" key={g.name}><span>{g.name}</span><span className="num">{tok(g.count)} / {tok(g.coins_spent)}</span></div>)}</section>
        <FilterBar onApply={p.apply} onReset={p.reset}>
          <div className="two"><Select label={t("dev.kind")} value={p.draft.kind} onChange={(v) => p.setDraft("kind", v)} all={t("dev.allKinds")} options={asArray<string>(p.data.kinds)} /><Select label={t("dev.status")} value={p.draft.status} onChange={(v) => p.setDraft("status", v)} all={t("dev.anyStatus")} options={["active", "expired", "consumed"]} /></div>
          <Field label={t("dev.search")}><input className="input" value={p.draft.q} placeholder={t("dev.uk.search")} onChange={(e) => p.setDraft("q", e.target.value)} /></Field>
        </FilterBar>
        {!keys.length ? <Note>{t("dev.uk.none")}</Note> : <div className="stack">{keys.map((k, i) => (
          <div className="glass pad stack" key={i}>
            <div className="row"><b>{k.key_name || k.key_kind}</b><span className={"chip " + (k.status === "active" ? "ok" : k.status === "consumed" ? "" : "warn")}>{k.status}</span></div>
            <p className="soft small">{k.key_kind} · {k.attributes}</p>
            <div className="row small"><span className="soft num">{when(k.created_at)}</span><span className="num">{tok(k.price_coins)} {t("dev.uk.coins")}</span></div>
            <span className="soft small">{t("dev.uk.expires")}: {when(k.expires_at)}</span>
          </div>
        ))}</div>}
        <Pager offset={p.offset} shown={keys.length} total={p.data.total ?? 0} hasMore={p.data.has_more} onNewer={p.newer} onOlder={p.older} filtered={p.filtered} unit={t("dev.uk.unit")} />
      </>)}
    </Page>
  );
}
