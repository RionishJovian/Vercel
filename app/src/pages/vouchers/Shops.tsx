import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Field, Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";

interface Shop { id: string; shop_name: string; code_prefix: string; phone?: string }

export function Shops() {
  const { t } = useI18n();
  const { data, error, isLoading, mutate } = useSWR<{ resellers?: Shop[] } | Shop[]>("/voucher-resellers", fetcher);
  const [msg, setMsg] = useState(""), [busy, setBusy] = useState(false);
  if (isLoading) return <Skeleton h={220} />;
  if (error) return <Note kind="bad">{t("c.loadFailed")}</Note>;
  const list = asArray<Shop>(Array.isArray(data) ? data : data?.resellers);

  async function add(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget, f = new FormData(form);
    const body: Record<string, string> = { shop_name: String(f.get("name")).trim(), contact_name: String(f.get("contact")).trim(), code_prefix: String(f.get("prefix")).trim().toUpperCase() };
    const phone = String(f.get("phone")).trim(); if (phone) body.phone = phone;
    setBusy(true); setMsg("");
    try { await api.post("/voucher-resellers", body); form.reset(); void mutate(); } catch (err) { if (isUnauthorized(err)) throw err; setMsg(t("v.shops.failed")); }
    setBusy(false);
  }
  return (
    <div className="stack">
      {!list.length && <Note>{t("v.shops.empty")}</Note>}
      {list.map((s) => <div className="rowitem" key={s.id}><div className="stack" style={{ gap: 2 }}><b>{s.shop_name}</b>{s.phone && <span className="soft small">{s.phone}</span>}</div><span className="chip num">{s.code_prefix}</span></div>)}
      <form className="glass pad stack" onSubmit={add}>
        <Field label={t("v.shops.name")} htmlFor="sh-name"><input className="input" id="sh-name" name="name" maxLength={120} required /></Field>
        <Field label={t("v.shops.contact")} htmlFor="sh-contact"><input className="input" id="sh-contact" name="contact" maxLength={120} required /></Field>
        <Field label={t("v.shops.prefix")} htmlFor="sh-prefix"><input className="input" id="sh-prefix" name="prefix" minLength={2} maxLength={4} pattern="[A-Za-z]{2,4}" required style={{ textTransform: "uppercase" }} /></Field>
        <Field label={t("v.shops.phone")} htmlFor="sh-phone"><input className="input" id="sh-phone" name="phone" type="tel" maxLength={40} /></Field>
        {msg && <Note kind="bad">{msg}</Note>}
        <button className="btn" type="submit" disabled={busy}>{t("v.shops.add")}</button>
      </form>
    </div>
  );
}
