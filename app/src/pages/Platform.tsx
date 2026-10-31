import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, ApiError, fetcher, isUnauthorized } from "../lib/api";
import { asArray, mmk, type Me } from "../lib/types";
import { useI18n } from "../i18n";
import { BackLink, Field, Note } from "../components/ui";
import { Skeleton } from "../components/Skeleton";

interface PReq { id: string; business_id: string; company_name?: string; name_en?: string; bank_reference: string; price_mmk: number }
interface PAddon { id: string; addon_key: string; name_en: string; active: boolean; price_mmk: number }

function Pending({ r, onDone }: { r: PReq; onDone: () => void }) {
  const { t } = useI18n();
  const [note, setNote] = useState(""), [busy, setBusy] = useState(false);
  async function decide(decision: "approve" | "reject") {
    setBusy(true);
    try { await api.post(`/platform/store/requests/${encodeURIComponent(r.id)}/decision`, { decision, note: note.trim() }); onDone(); } catch (e) { if (isUnauthorized(e)) throw e; setBusy(false); }
  }
  return (
    <div className="stack rq">
      <div className="row"><b>{r.company_name || r.business_id}</b><span className="chip">{r.name_en}</span></div>
      <div className="row small"><span className="soft num">{r.bank_reference}</span><span>{mmk(r.price_mmk)} {t("c.mmk")}</span></div>
      <input className="input" maxLength={500} aria-label={t("pf.note")} placeholder={t("pf.note")} value={note} onChange={(e) => setNote(e.target.value)} />
      <div className="row" style={{ justifyContent: "flex-start" }}>
        <button className="pill on" type="button" disabled={busy} onClick={() => void decide("approve")}>{t("pf.approve")}</button>
        <button className="pill danger" type="button" disabled={busy} onClick={() => void decide("reject")}>{t("pf.reject")}</button>
      </div>
    </div>
  );
}

function NewAddon({ onDone }: { onDone: () => void }) {
  const { t } = useI18n();
  const [msg, setMsg] = useState(""), [busy, setBusy] = useState(false);
  async function add(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget), v = (k: string) => String(f.get(k)).trim();
    setBusy(true); setMsg("");
    try { await api.post("/platform/store/addons", { addon_key: v("key"), kind: f.get("kind"), name_en: v("en"), name_my: v("my"), description_en: v("den"), description_my: v("dmy"), price_mmk: Number(f.get("price")), period: f.get("period"), active: true }); onDone(); }
    catch (err) { if (isUnauthorized(err)) throw err; setMsg((err instanceof ApiError && (err.data as { error?: string } | null)?.error) || t("st.failed")); setBusy(false); }
  }
  return (
    <form className="glass pad stack" onSubmit={add}>
      <h2>{t("pf.new")}</h2>
      <Field label={t("pf.key")}><input className="input" name="key" maxLength={40} pattern="[a-z0-9_]{3,40}" required /></Field>
      <Field label={t("pf.nameEn")}><input className="input" name="en" maxLength={120} required /></Field>
      <Field label={t("pf.nameMy")}><input className="input" name="my" maxLength={120} required /></Field>
      <Field label={t("pf.descEn")}><input className="input" name="den" maxLength={600} /></Field>
      <Field label={t("pf.descMy")}><input className="input" name="dmy" maxLength={600} /></Field>
      <Field label={t("pf.price")}><input className="input" name="price" type="number" min={0} step={1} inputMode="numeric" required /></Field>
      <div className="two">
        <Field label={t("pf.kind")}><select className="input" name="kind">{["router_slot", "server", "feature"].map((k) => <option key={k} value={k}>{t("st.kind." + k)}</option>)}</select></Field>
        <Field label={t("pf.period")}><select className="input" name="period">{["one_time", "monthly", "annual"].map((p) => <option key={p} value={p}>{t("st.period." + p)}</option>)}</select></Field>
      </div>
      {msg && <Note kind="bad">{msg}</Note>}
      <button className="btn" type="submit" disabled={busy}>{t("pf.add")}</button>
    </form>
  );
}

export function Platform({ me }: { me: Me }) {
  const { t } = useI18n();
  const isDev = me.user?.user_role === "DEVELOPER";
  const reqs = useSWR<{ requests?: PReq[] }>(isDev ? "/platform/store/requests?status=pending" : null, fetcher, { shouldRetryOnError: false });
  const adds = useSWR<{ addons?: PAddon[] }>(isDev ? "/platform/store/addons" : null, fetcher, { shouldRetryOnError: false });
  const back = <BackLink href="#/more">{t("tab.more")}</BackLink>;
  if (!isDev) return <>{back}<Note kind="warn">{t("pf.denied")}</Note></>;
  if (reqs.isLoading || adds.isLoading) return <>{back}<Skeleton h={300} /></>;
  if (reqs.error || adds.error) return <>{back}<Note kind="bad">{t("st.unavailable")}</Note></>;
  const pending = asArray<PReq>(reqs.data?.requests), addons = asArray<PAddon>(adds.data?.addons);
  const refresh = () => { void reqs.mutate(); void adds.mutate(); };
  return (
    <>
      {back}<h1>{t("title.platform")}</h1>
      <section className="glass pad stack"><h2>{t("pf.pending")}</h2>{pending.length ? pending.map((r) => <Pending key={r.id} r={r} onDone={refresh} />) : <p className="soft small">{t("pf.none")}</p>}</section>
      <section className="glass pad stack"><h2>{t("pf.catalog")}</h2>
        {addons.map((a) => <div className="row small" key={a.id}><span>{a.name_en} <span className="soft">{a.addon_key}</span></span><span className={"chip " + (a.active ? "ok" : "")}>{a.active ? t("pf.active") : t("pf.hidden")} {mmk(a.price_mmk)}</span></div>)}
      </section>
      <NewAddon onDone={refresh} />
    </>
  );
}
