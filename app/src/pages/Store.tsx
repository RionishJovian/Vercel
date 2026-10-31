import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, ApiError, fetcher, isUnauthorized } from "../lib/api";
import { asArray, mmk, type Me } from "../lib/types";
import { useI18n } from "../i18n";
import { BackLink, Field, Note } from "../components/ui";
import { Skeleton } from "../components/Skeleton";
import { AccountPass, type Pass } from "./store/AccountPass";

interface Addon { id: string; kind: string; period: string; price_mmk: number; name_en?: string; name_my?: string; description_en?: string; description_my?: string }
interface Req { id: string; addon_id: string; status: "pending" | "approved" | "rejected"; bank_reference: string; price_mmk: number; decision_note?: string; name_en?: string; name_my?: string }
interface Bank { bank_name?: string; holder_name?: string; account_number?: string }

export function useLocalized() {
  const { lang } = useI18n();
  return (o: { name_en?: string; name_my?: string; description_en?: string; description_my?: string }, f: "name" | "description") => {
    const en = o[`${f}_en`], my = o[`${f}_my`];
    return (lang === "my" ? my || en : en || my) ?? "";
  };
}
const statusChip = (s: Req["status"]) => (s === "approved" ? "ok" : s === "rejected" ? "bad" : "warn");

function AddonCard({ a, pending, canBuy, onDone }: { a: Addon; pending: boolean; canBuy: boolean; onDone: () => void }) {
  const { t } = useI18n();
  const loc = useLocalized();
  const [open, setOpen] = useState(false), [msg, setMsg] = useState<{ kind: "warn" | "bad"; text: string } | null>(null), [busy, setBusy] = useState(false);
  async function send(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const ref = String(new FormData(e.currentTarget).get("ref")).trim();
    setBusy(true);
    try { await api.post(`/store/addons/${encodeURIComponent(a.id)}/requests`, { bank_reference: ref, idempotency_key: crypto.randomUUID() }); onDone(); }
    catch (err) { if (isUnauthorized(err)) throw err; const dup = err instanceof ApiError && err.status === 409; setMsg({ kind: dup ? "warn" : "bad", text: dup ? t("st.pending") : t("st.failed") }); setBusy(false); }
  }
  const desc = loc(a, "description");
  return (
    <section className="glass pad stack">
      <div className="row"><h2>{loc(a, "name")}</h2><span className="chip">{t("st.kind." + a.kind)}</span></div>
      {desc && <p className="soft small">{desc}</p>}
      <div className="row"><b className="num price">{mmk(a.price_mmk)} {t("c.mmk")}</b><span className="soft small">{t("st.period." + a.period)}</span></div>
      <button className="btn" type="button" disabled={!canBuy || pending} onClick={() => setOpen((v) => !v)}>{pending ? t("st.status.pending") : t("st.request")}</button>
      {open && !pending && (
        <form className="stack" onSubmit={send}>
          <Field label={t("st.ref")} htmlFor={"ref-" + a.id}><input className="input" id={"ref-" + a.id} name="ref" minLength={4} maxLength={80} required placeholder={t("st.refHint")} /></Field>
          {msg && <Note kind={msg.kind}>{msg.text}</Note>}
          <button className="btn quiet" type="submit" disabled={busy}>{t("st.send")}</button>
        </form>
      )}
    </section>
  );
}

function AddonSection({ me }: { me: Me }) {
  const { t } = useI18n();
  const loc = useLocalized();
  const store = useSWR<{ addons?: Addon[]; requests?: Req[] }>("/store/addons", fetcher, { shouldRetryOnError: false });
  const banks = useSWR<{ bank_accounts?: Bank[] }>("/me/app-pass/checkout-options", fetcher, { shouldRetryOnError: false });
  if (store.isLoading) return <Skeleton h={300} />;
  if (store.error) return <Note kind="warn">{t("st.unavailable")}</Note>;
  const addons = asArray<Addon>(store.data?.addons), requests = asArray<Req>(store.data?.requests), bankList = asArray<Bank>(banks.data?.bank_accounts);
  const canBuy = ["OWNER", "ADMIN", "DEVELOPER"].includes(me.user?.user_role ?? "");
  const pendingIds = new Set(requests.filter((r) => r.status === "pending").map((r) => r.addon_id));
  return (
    <>
      <h2>{t("st.addons")}</h2><p className="soft">{t("st.help")}</p>
      {bankList.length > 0 && <section className="glass pad stack"><h2>{t("st.pay")}</h2>{bankList.map((b, i) => <div className="row small" key={i}><span className="soft">{b.bank_name} {b.holder_name}</span><b className="num">{b.account_number}</b></div>)}</section>}
      {!addons.length && <Note>{t("st.empty")}</Note>}
      <div className="store-grid">{addons.map((a) => <AddonCard key={a.id} a={a} pending={pendingIds.has(a.id)} canBuy={canBuy} onDone={() => void store.mutate()} />)}</div>
      {requests.length > 0 && (
        <section className="glass pad stack"><h2>{t("st.requests")}</h2>
          {requests.map((r) => (
            <div className="stack rq" key={r.id}>
              <div className="row"><b>{loc(r, "name")}</b><span className={"chip " + statusChip(r.status)}>{t("st.status." + r.status)}</span></div>
              <div className="row small"><span className="soft num">{r.bank_reference}</span><span className="soft">{mmk(r.price_mmk)} {t("c.mmk")}</span></div>
              {r.decision_note && <p className="soft small">{r.decision_note}</p>}
            </div>
          ))}
        </section>
      )}
    </>
  );
}

interface Purchase { id?: string; created_at?: string; status?: string; product?: string; amount_mmk?: number; bank_reference?: string }

function LegacyHistory() {
  const { t } = useI18n();
  const { data, error } = useSWR<{ purchases?: Purchase[] }>("/store/purchases", fetcher, { shouldRetryOnError: false });
  const list = asArray<Purchase>(data?.purchases);
  return (
    <section className="glass pad stack">
      <h2>{t("st.legacy")}</h2><p className="soft small">{t("st.legacyNote")}</p>
      {error ? <Note kind="warn">{t("st.legacyFail")}</Note> : !list.length ? <p className="soft small">{t("st.legacyNone")}</p> : list.map((p, i) => (
        <div className="rowitem" key={p.id ?? i}><div className="stack" style={{ gap: 2 }}><b className="small">{p.product ?? "—"}</b><span className="soft small">{p.created_at ? new Date(p.created_at).toLocaleString() : ""} · {p.status}</span></div><span className="num small">{p.amount_mmk != null ? mmk(p.amount_mmk) : ""}</span></div>
      ))}
    </section>
  );
}

/** Store: the account pass first (how access is kept), then optional add-ons. */
export function Store({ me }: { me: Me }) {
  const { t } = useI18n();
  const sub = useSWR<Pass & { mode?: string }>("/subscription", fetcher, { shouldRetryOnError: false });
  const own = useSWR<Pass & { scope?: string; authority?: string }>(sub.data && sub.data.mode !== "individual_pass" && sub.data.mode !== "subscription_only" ? "/me/app-pass" : null, fetcher, { shouldRetryOnError: false });
  const back = <BackLink href="#/more">{t("tab.more")}</BackLink>;
  if (sub.isLoading) return <>{back}<Skeleton h={300} /></>;
  if (sub.error || !sub.data) return <>{back}<Note kind="bad">{t("st.compat")}</Note></>;
  const mode = sub.data.mode;
  return (
    <>
      {back}<h1>{t("title.store")}</h1>
      {mode === "individual_pass" ? <AccountPass me={me} pass={sub.data} onRefresh={() => void sub.mutate()} />
        : mode === "subscription_only" ? <Note kind="warn">{t("st.subOnly")} <a className="link" href="/app.html#store">{t("c.openLegacy")}</a></Note>
        : (
          <>
            {own.data && own.data.scope === "account" && own.data.authority === "app_account_passes" ? (
              <section className="glass pad stack"><h2>{t("ap.individual")}</h2><p>{t("ap.k." + (own.data.kind ?? "none"))} · {t("ap.s." + (own.data.state ?? "unknown"))}</p><p className="soft small">{t("ap.activation")}: {own.data.activated_at ? new Date(own.data.activated_at).toLocaleString() : t("ap.notRecorded")} · {t("ap.expiry")}: {own.data.expires_at ? new Date(own.data.expires_at).toLocaleString() : t("ap.notRecorded")}</p></section>
            ) : null}
            <LegacyHistory />
          </>
        )}
      <AddonSection me={me} />
    </>
  );
}
