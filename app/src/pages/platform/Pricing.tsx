import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Field, Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";

type P = { monthly_standard_mmk?: number | null; annual_standard_mmk?: number | null; monthly_promo_mmk?: number | null; annual_promo_mmk?: number | null; starts_on?: string | null; ends_on?: string | null; label?: string | null; active?: boolean };

export function Pricing() {
  const { t } = useI18n();
  const { data, isLoading, error, mutate } = useSWR<{ pricing?: P; published?: boolean }>("/platform/pricing", fetcher, { shouldRetryOnError: false });
  const [msg, setMsg] = useState<{ bad: boolean; text: string } | null>(null), [busy, setBusy] = useState(false);
  if (isLoading) return <Skeleton h={260} />;
  if (error) return <Note kind="bad">{t("c.loadFailed")}</Note>;
  const p = data?.pricing ?? {};
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const num = (k: string) => { const raw = String(f.get(k) ?? "").trim(); if (!raw) return null; const v = Number(raw); return Number.isSafeInteger(v) ? v : NaN; };
    const body = { active: f.get("active") === "on", starts_on: String(f.get("starts") ?? "") || null, ends_on: String(f.get("ends") ?? "") || null, label: String(f.get("label") ?? "").trim() || null, monthly_promo_mmk: num("mp"), monthly_standard_mmk: num("ms"), annual_promo_mmk: num("ap"), annual_standard_mmk: num("as") };
    if (Object.values(body).some((v) => typeof v === "number" && !Number.isSafeInteger(v))) { setMsg({ bad: true, text: t("pp.whole") }); return; }
    setBusy(true); setMsg(null);
    try { await api.put("/platform/pricing", body); setMsg({ bad: false, text: t("pp.published") }); void mutate(); }
    catch (err) { if (isUnauthorized(err)) throw err; setMsg({ bad: true, text: (err as { data?: { error?: string } }).data?.error || t("pp.failed") }); }
    setBusy(false);
  }
  return (
    <form className="glass pad stack" onSubmit={save} key={JSON.stringify(p)}>
      <h2>{t("pp.title")}</h2><p className="soft small">{t("pp.lead")}</p>
      <div className="two"><Field label={t("pp.ms")}><input className="input" name="ms" inputMode="numeric" defaultValue={p.monthly_standard_mmk ?? ""} /></Field><Field label={t("pp.as")}><input className="input" name="as" inputMode="numeric" defaultValue={p.annual_standard_mmk ?? ""} /></Field></div>
      <div className="two"><Field label={t("pp.mp")}><input className="input" name="mp" inputMode="numeric" defaultValue={p.monthly_promo_mmk ?? ""} /></Field><Field label={t("pp.ap")}><input className="input" name="ap" inputMode="numeric" defaultValue={p.annual_promo_mmk ?? ""} /></Field></div>
      <div className="two"><Field label={t("pp.starts")}><input className="input" name="starts" type="date" defaultValue={p.starts_on ?? ""} /></Field><Field label={t("pp.ends")}><input className="input" name="ends" type="date" defaultValue={p.ends_on ?? ""} /></Field></div>
      <Field label={t("pp.label")}><input className="input" name="label" maxLength={120} defaultValue={p.label ?? ""} placeholder="Launch offer" /></Field>
      <label className="check"><span>{t("pp.active")}</span><input type="checkbox" name="active" defaultChecked={!!p.active} /></label>
      {msg && <Note kind={msg.bad ? "bad" : "ok"}>{msg.text}</Note>}
      <button className="btn" type="submit" disabled={busy}>{t("pp.publish")}</button>
    </form>
  );
}
