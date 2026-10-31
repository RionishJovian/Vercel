import { useState, type FormEvent } from "react";
import { api, isUnauthorized } from "../../lib/api";
import { asArray, type Me } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Field, Note } from "../../components/ui";
import { FilterBar, Loading, Page, Select, usePaged } from "./Paged";

interface Tpl { id: string; name: string; category: string; method: string; path: string; is_builtin?: boolean }
interface Res { templates?: Tpl[]; categories?: string[]; count?: number; builtin?: number; custom?: number }
const EDITORS = ["OWNER", "ADMIN", "DEVELOPER"];
const errText = (e: unknown) => (e as { data?: { error?: string } } | null)?.data?.error || (e instanceof Error ? e.message : "") || "failed";

export function Templates({ me }: { me: Me }) {
  const { t } = useI18n();
  const canEdit = EDITORS.includes(me.user?.user_role ?? "");
  const p = usePaged<Res>("/terminal-templates", { category: "", q: "" }, 1000);
  const [form, setForm] = useState(false), [msg, setMsg] = useState(""), [busy, setBusy] = useState(false), [confirmId, setConfirmId] = useState("");
  const list = asArray<Tpl>(p.data?.templates);
  async function create(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget), v = (k: string) => String(f.get(k) ?? "").trim();
    const body = { name: v("name"), category: v("category"), method: v("method"), path: v("path"), description: v("desc") || null, body: v("body") || null };
    if (!body.name || !body.category || !body.path) { setMsg(t("dev.tpl.required")); return; }
    setBusy(true); setMsg("");
    try { await api.post("/terminal-templates", body); setForm(false); void p.mutate(); } catch (err) { if (isUnauthorized(err)) throw err; setMsg(errText(err)); }
    setBusy(false);
  }
  async function remove(id: string) {
    try { await api.del("/terminal-templates/" + encodeURIComponent(id)); setConfirmId(""); void p.mutate(); } catch (err) { if (isUnauthorized(err)) throw err; setMsg(errText(err)); setConfirmId(""); }
  }
  return (
    <Page title={t("dev.tpl.title")} lead={t("dev.tpl.lead")}>
      <FilterBar onApply={p.apply} onReset={p.reset}>
        <Select label={t("dev.tpl.category")} value={p.draft.category} onChange={(v) => p.setDraft("category", v)} all={t("dev.allCategories")} options={asArray<string>(p.data?.categories)} />
        <Field label={t("dev.tpl.contains")}><input className="input" value={p.draft.q} placeholder="firewall" onChange={(e) => p.setDraft("q", e.target.value)} /></Field>
      </FilterBar>
      {canEdit && !form && <button className="btn" type="button" onClick={() => setForm(true)}>{t("dev.tpl.add")}</button>}
      {form && (
        <form className="glass pad stack" onSubmit={create}>
          <h2>{t("dev.tpl.new")}</h2>
          <Field label={t("dev.tpl.name")}><input className="input" name="name" required /></Field>
          <Field label={t("dev.tpl.category")}><input className="input" name="category" placeholder="network" required /></Field>
          <Field label={t("dev.tpl.method")}><select className="input" name="method">{["GET", "POST", "PATCH", "PUT", "DELETE"].map((m) => <option key={m}>{m}</option>)}</select></Field>
          <Field label={t("dev.tpl.path")}><input className="input" name="path" placeholder="/ip/firewall/filter" required /></Field>
          <Field label={t("dev.tpl.desc")}><input className="input" name="desc" /></Field>
          <Field label={t("dev.tpl.body")}><input className="input" name="body" /></Field>
          <div className="row" style={{ justifyContent: "flex-start" }}><button className="btn" type="submit" disabled={busy}>{t("dev.tpl.create")}</button><button className="btn quiet" type="button" onClick={() => setForm(false)}>{t("c.cancel")}</button></div>
        </form>
      )}
      {msg && <Note kind="bad">{msg}</Note>}
      <Loading loading={p.isLoading && !p.data} error={p.error} text={t("dev.tpl.failed")} />
      {p.data && (
        <>
          <p className="soft small" role="status">{p.data.count} {t("dev.tpl.templates")}: {p.data.builtin} {t("dev.tpl.builtin")}, {p.data.custom} {t("dev.tpl.custom")}{p.filtered ? ` (${t("ev.filtered")})` : ""}</p>
          {!list.length ? <Note>{t("dev.tpl.none")}</Note> : <div className="stack">{list.map((x) => (
            <div className="rowitem" key={x.id} style={{ alignItems: "flex-start" }}>
              <div className="stack" style={{ gap: 2, minWidth: 0 }}>
                <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}><b>{x.name}</b><span className={"chip " + (x.is_builtin ? "" : "ok")}>{x.is_builtin ? t("dev.tpl.builtin") : t("dev.tpl.custom")}</span><span className="chip num">{x.method}</span></div>
                <span className="soft small">{x.category}</span><code className="small" style={{ overflowWrap: "anywhere" }}>{x.path}</code>
              </div>
              {canEdit && !x.is_builtin && (confirmId === x.id ? <div className="row"><button className="pill danger" type="button" onClick={() => void remove(x.id)}>{t("dev.delete")}</button><button className="pill" type="button" onClick={() => setConfirmId("")}>{t("c.cancel")}</button></div> : <button className="pill danger" type="button" onClick={() => setConfirmId(x.id)}>{t("dev.delete")}</button>)}
            </div>
          ))}</div>}
        </>
      )}
    </Page>
  );
}
