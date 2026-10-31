import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { asArray, type Me } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Field, Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { Page, when } from "./Paged";

interface Tx { created_at?: string; kind?: string; delta?: number; balance_after?: number; note?: string }
interface Member { user_id: string; full_name?: string; email?: string }
const amt = (n: unknown) => Number(n || 0).toFixed(2);
const KINDS = ["agent_earn", "agent_reversal", "service_payment", "service_refund", "feature_key_purchase", "purchase_credit", "manual_credit", "manual_debit"];
const errText = (e: unknown) => (e as { data?: { error?: string } } | null)?.data?.error || (e instanceof Error ? e.message : "") || "failed";

export function Coins({ me }: { me: Me }) {
  const { t } = useI18n();
  const canAdjust = ["OWNER", "ADMIN", "DEVELOPER"].includes(me.user?.user_role ?? "");
  const wallet = useSWR<{ wallet?: { balance?: number } }>("/magic-coins/wallet", fetcher, { shouldRetryOnError: false });
  const txs = useSWR<{ transactions?: Tx[] }>("/magic-coins/transactions", fetcher, { shouldRetryOnError: false });
  const team = useSWR<{ team?: Member[] }>(canAdjust ? "/team" : null, fetcher, { shouldRetryOnError: false });
  const [msg, setMsg] = useState(""), [busy, setBusy] = useState(false);
  const list = asArray<Tx>(txs.data?.transactions), members = asArray<Member>(team.data?.team);
  async function adjust(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget, f = new FormData(form);
    const user = String(f.get("user")), amount = parseFloat(String(f.get("amount"))), note = String(f.get("note")).trim();
    if (!user) { setMsg(t("dev.co.pickUser")); return; }
    if (!(amount > 0)) { setMsg(t("dev.co.amountPositive")); return; }
    if (note.length < 3) { setMsg(t("dev.co.reason")); return; }
    setBusy(true); setMsg("");
    try { await api.post("/magic-coins/adjust", { user_id: user, amount, kind: String(f.get("kind")), note }); form.reset(); void wallet.mutate(); void txs.mutate(); }
    catch (err) { if (isUnauthorized(err)) throw err; setMsg(errText(err)); }
    setBusy(false);
  }
  return (
    <Page title={t("dev.co.title")} lead={t("dev.co.lead")}>
      {wallet.isLoading || txs.isLoading ? <Skeleton h={160} /> : (
        <div className="capsules"><div className="capsule"><span className="soft small">{t("dev.co.balance")}</span><b className="num">{amt(wallet.data?.wallet?.balance)}</b></div><div className="capsule"><span className="soft small">{t("dev.co.entries")}</span><b className="num">{list.length}</b></div></div>
      )}
      {canAdjust && (
        <form className="glass pad stack" onSubmit={adjust}>
          <h2>{t("dev.co.adjust")}</h2>
          <Field label={t("dev.co.user")}><select className="input" name="user" required>{members.length ? members.map((m) => <option key={m.user_id} value={m.user_id}>{m.full_name || m.email}</option>) : <option value="">{t("dev.co.noMembers")}</option>}</select></Field>
          <Field label={t("dev.co.type")}><select className="input" name="kind"><option value="manual_credit">{t("dev.co.credit")}</option><option value="manual_debit">{t("dev.co.debit")}</option></select></Field>
          <Field label={t("dev.co.amount")}><input className="input" name="amount" type="number" min="0.01" step="0.01" inputMode="decimal" placeholder="0.00" /></Field>
          <Field label={t("dev.co.why")}><input className="input" name="note" placeholder={t("dev.co.whyHint")} /></Field>
          {msg && <Note kind="bad">{msg}</Note>}
          <button className="btn" type="submit" disabled={busy}>{t("dev.co.apply")}</button>
        </form>
      )}
      <section className="stack"><h2>{t("dev.co.ledger")}</h2>
        {!list.length ? <Note>{t("dev.co.none")}</Note> : list.map((x, i) => (
          <div className="rowitem" key={i}><div className="stack" style={{ gap: 2 }}><b className="small">{KINDS.includes(x.kind ?? "") ? t("dev.co.k." + x.kind) : x.kind}</b><span className="soft small">{x.note} · {when(x.created_at)}</span></div><span className="num">{Number(x.delta) > 0 ? "+" : ""}{amt(x.delta)}<span className="soft small"> → {amt(x.balance_after)}</span></span></div>
        ))}
      </section>
    </Page>
  );
}
