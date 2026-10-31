import { useState, type FormEvent } from "react";
import { api, isUnauthorized } from "../../lib/api";
import { asArray, type Me } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Field, Note } from "../../components/ui";
import { FilterBar, Loading, Page, Pager, Select, tok, usePaged, when } from "./Paged";

interface Ov { id: string; locale: string; namespace: string; key: string; value: string; updated_at?: string }
interface Res { overrides?: Ov[]; locales?: string[]; namespaces?: string[]; by_locale?: Record<string, number>; by_namespace?: Record<string, number>; total?: number; has_more?: boolean }
const errText = (e: unknown) => (e as { data?: { error?: string } } | null)?.data?.error || (e instanceof Error && !e.message.startsWith("http-") ? e.message : "") || "failed";

function Editor({ item, locales, namespaces, onDone }: { item: Ov | null; locales: string[]; namespaces: string[]; onDone: () => void }) {
  const { t } = useI18n();
  const [msg, setMsg] = useState(""), [busy, setBusy] = useState(false);
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const body = { locale: String(f.get("locale")), namespace: String(f.get("ns")), key: String(f.get("key")).trim(), value: String(f.get("value")) };
    if (!body.key) { setMsg(t("dev.tr.keyNeeded")); return; }
    if (!body.value.trim()) { setMsg(t("dev.tr.valueNeeded")); return; }
    setBusy(true); setMsg("");
    try { if (item) await api.put("/i18n-overrides/" + item.id, body); else await api.post("/i18n-overrides", body); onDone(); }
    catch (err) { if (isUnauthorized(err)) throw err; setMsg(errText(err)); setBusy(false); }
  }
  return (
    <form className="glass pad stack" onSubmit={save}>
      <h2>{item ? t("dev.tr.edit") : t("dev.tr.add")}</h2>
      <div className="two">
        <Field label={t("dev.tr.locale")}><select className="input" name="locale" defaultValue={item?.locale ?? locales[0]}>{locales.map((l) => <option key={l}>{l}</option>)}</select></Field>
        <Field label={t("dev.tr.namespace")}><select className="input" name="ns" defaultValue={item?.namespace ?? namespaces[0]}>{namespaces.map((l) => <option key={l}>{l}</option>)}</select></Field>
      </div>
      <Field label={t("dev.tr.key")}><input className="input" name="key" defaultValue={item?.key ?? ""} /></Field>
      <Field label={t("dev.tr.value")}><input className="input" name="value" defaultValue={item?.value ?? ""} /></Field>
      {msg && <Note kind="bad">{msg}</Note>}
      <div className="row" style={{ justifyContent: "flex-start" }}><button className="btn" type="submit" disabled={busy}>{item ? t("pf.save") : t("dev.tr.create")}</button><button className="btn quiet" type="button" onClick={onDone}>{t("c.cancel")}</button></div>
    </form>
  );
}

export function Translations({ me }: { me: Me }) {
  const { t } = useI18n();
  const isDev = me.user?.user_role === "DEVELOPER";
  const p = usePaged<Res>("/i18n-overrides", { locale: "", namespace: "", q: "" }, 100);
  const [edit, setEdit] = useState<Ov | "new" | null>(null), [err, setErr] = useState("");
  const items = asArray<Ov>(p.data?.overrides), locales = asArray<string>(p.data?.locales).length ? asArray<string>(p.data?.locales) : ["en", "my"], namespaces = asArray<string>(p.data?.namespaces).length ? asArray<string>(p.data?.namespaces) : ["ui", "copy"];
  const facet = (o?: Record<string, number>) => (o && Object.keys(o).length ? Object.entries(o).map(([k, v]) => `${k} ${v}`).join(", ") : t("dev.none"));
  async function remove(id: string) { try { await api.del("/i18n-overrides/" + id); void p.mutate(); } catch (e) { if (isUnauthorized(e)) throw e; setErr(errText(e)); } }
  return (
    <Page title={t("dev.tr.title")}>
      <Note kind="warn"><b>{t("dev.tr.global")}</b> {isDev ? t("dev.tr.careful") : t("dev.tr.devOnly")}</Note>
      {err && <Note kind="bad">{err}</Note>}
      {isDev && !edit && <button className="btn" type="button" onClick={() => setEdit("new")}>{t("dev.tr.add")}</button>}
      {isDev && edit && <Editor item={edit === "new" ? null : edit} locales={locales} namespaces={namespaces} onDone={() => { setEdit(null); void p.mutate(); }} />}
      <Loading loading={p.isLoading && !p.data} error={p.error} text={t("dev.tr.failed")} />
      {p.data && (<>
        <p className="soft small">{tok(p.data.total)} {t("dev.tr.overrides")} · {t("dev.tr.byLocale")}: {facet(p.data.by_locale)} · {t("dev.tr.byNs")}: {facet(p.data.by_namespace)}</p>
        <FilterBar onApply={p.apply} onReset={p.reset}>
          <div className="two"><Select label={t("dev.tr.locale")} value={p.draft.locale} onChange={(v) => p.setDraft("locale", v)} all={t("dev.tr.allLocales")} options={locales} /><Select label={t("dev.tr.namespace")} value={p.draft.namespace} onChange={(v) => p.setDraft("namespace", v)} all={t("dev.tr.allNs")} options={namespaces} /></div>
          <Field label={t("dev.search")}><input className="input" value={p.draft.q} placeholder={t("dev.tr.searchHint")} onChange={(e) => p.setDraft("q", e.target.value)} /></Field>
        </FilterBar>
        {!items.length ? <Note>{t("dev.tr.none")}</Note> : <div className="stack">{items.map((o) => (
          <div className="glass pad stack" key={o.id}>
            <div className="row"><span className="chip num">{o.locale} / {o.namespace}</span><span className="soft small num">{when(o.updated_at)}</span></div>
            <p className="small soft" style={{ overflowWrap: "anywhere" }}>{o.key}</p><p style={{ overflowWrap: "anywhere" }}>{o.value}</p>
            {isDev && <div className="row" style={{ justifyContent: "flex-start" }}><button className="pill" type="button" onClick={() => setEdit(o)}>{t("site.edit")}</button><button className="pill danger" type="button" onClick={() => void remove(o.id)}>{t("dev.delete")}</button></div>}
          </div>
        ))}</div>}
        <Pager offset={p.offset} shown={items.length} total={p.data.total ?? 0} hasMore={p.data.has_more} onNewer={p.newer} onOlder={p.older} filtered={p.filtered} unit={t("dev.tr.overrides")} />
      </>)}
    </Page>
  );
}
