import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Field, Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import "../../components/tables.css";
import { DEFAULT_LAYOUT, isThermal, printCodes, receipt, type Layout, type PrintVoucher } from "./printCodes";
import { qrPath } from "./qr";

const PAPER = [58, 80, 210];

export function Preview({ voucher, layout, firstUse }: { voucher: PrintVoucher; layout: Layout; firstUse: string }) {
  const r = receipt(voucher, layout, firstUse);
  const qr = r.qr ? qrPath(r.code) : null;
  return (
    <div className="pl-paper" data-narrow={layout.paper_width_mm < 70 ? "" : undefined} aria-label={r.code} role="img">
      {r.business ? <b className="pl-biz">{r.business}</b> : null}
      {qr ? <svg className="pl-qr" viewBox={`0 0 ${qr.size} ${qr.size}`} aria-hidden="true"><path d={qr.d} /></svg> : null}
      <b className="pl-code">{r.code}</b>
      {r.rows.length ? <dl className="pl-rows">{r.rows.map(([k, v]) => <div key={k}><dt>{k}:</dt> <dd>{v}</dd></div>)}</dl> : null}
      {r.terms ? <p className="pl-terms">{r.terms}</p> : null}
    </div>
  );
}

function LayoutForm({ initial, onSaved }: { initial: Layout; onSaved: () => void }) {
  const { t } = useI18n();
  const [d, setD] = useState<Layout>(initial);
  const [msg, setMsg] = useState<{ kind: "ok" | "bad"; text: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const set = <K extends keyof Layout>(k: K, v: Layout[K]) => { setD((p) => ({ ...p, [k]: v })); setMsg(null); };
  const demo: PrintVoucher = { code: "DEMO-1234", plan_label: t("pl.demoPlan"), price_mmk: 1500, duration_minutes: 60 };
  const firstUse = t("v.validFirstUse");
  const paper = PAPER.includes(d.paper_width_mm) ? PAPER : [...PAPER, d.paper_width_mm].sort((a, b) => a - b);

  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const body: Record<string, unknown> = { ...d, business_name: d.business_name.trim(), wifi_name: d.wifi_name.trim(), support_contact: d.support_contact.trim(), terms: d.terms.trim() };
    delete body.business_id; delete body.created_at; delete body.updated_at;
    setBusy(true);
    try { await api.put("/voucher-print-layout", body); setMsg({ kind: "ok", text: t("p.saved") }); onSaved(); }
    catch (err) { if (isUnauthorized(err)) throw err; setMsg({ kind: "bad", text: t("p.saveFailed") }); }
    setBusy(false);
  }
  return (
    <div className="pl-layout">
      <form className="glass pad stack" onSubmit={(e) => void save(e)}>
        <div><h3>{t("pl.card")}</h3><p className="soft small">{t("pl.cardLead")}</p></div>
        <div className="pl-fields">
          <Field label={t("p.biz")} htmlFor="pl-biz"><input className="input" id="pl-biz" maxLength={120} required value={d.business_name} onChange={(e) => set("business_name", e.target.value)} /></Field>
          <Field label={t("p.wifi")} htmlFor="pl-wifi"><input className="input" id="pl-wifi" maxLength={120} required value={d.wifi_name} onChange={(e) => set("wifi_name", e.target.value)} /></Field>
          <Field label={t("p.support")} htmlFor="pl-support"><input className="input" id="pl-support" maxLength={200} required value={d.support_contact} onChange={(e) => set("support_contact", e.target.value)} /></Field>
          <Field label={t("pl.paper")} htmlFor="pl-paper">
            <select className="input" id="pl-paper" value={String(d.paper_width_mm)} onChange={(e) => set("paper_width_mm", Number(e.target.value))}>
              {paper.map((mm) => <option key={mm} value={mm}>{mm === 58 ? t("pl.p58") : mm === 80 ? t("pl.p80") : mm === 210 ? t("pl.a4") : t("pl.other").replace("{n}", String(mm))}</option>)}
            </select>
          </Field>
        </div>
        {isThermal(d.paper_width_mm) ? null : (
          <Field label={t("p.cols")} htmlFor="pl-cols"><select className="input" id="pl-cols" value={String(d.a4_columns)} onChange={(e) => set("a4_columns", Number(e.target.value))}>{[1, 2, 3, 4, 5, 6, 7, 8].map((n) => <option key={n} value={n}>{n}</option>)}</select></Field>
        )}
        <Field label={t("pl.terms")} htmlFor="pl-terms"><textarea className="input pl-area" id="pl-terms" maxLength={2000} rows={3} required value={d.terms} onChange={(e) => set("terms", e.target.value)} /></Field>
        <div className="pl-toggles">
          <label className="pl-toggle"><span>{t("pl.qr")}</span><input type="checkbox" checked={d.show_qr} onChange={(e) => set("show_qr", e.target.checked)} /></label>
          <label className="pl-toggle"><span>{t("p.showPrice")}</span><input type="checkbox" checked={d.show_price} onChange={(e) => set("show_price", e.target.checked)} /></label>
          <label className="pl-toggle"><span>{t("pl.showExpiry")}</span><input type="checkbox" checked={d.show_expiry} onChange={(e) => set("show_expiry", e.target.checked)} /></label>
        </div>
        {msg && <Note kind={msg.kind}>{msg.text}</Note>}
        <div className="pl-actions">
          <button className="btn" type="submit" disabled={busy}>{t("p.save")}</button>
          <button className="pill" type="button" onClick={() => printCodes([demo], d, firstUse)}>{t("pl.test")}</button>
        </div>
      </form>
      <aside className="glass pad stack pl-side">
        <div><h3>{t("pl.preview")}</h3><p className="soft small">{t("pl.previewLead")}</p></div>
        <Preview voucher={demo} layout={d} firstUse={firstUse} />
      </aside>
    </div>
  );
}

export function Print() {
  const { t } = useI18n();
  const res = useSWR<{ layout?: Partial<Layout> }>("/voucher-print-layout", fetcher, { revalidateOnFocus: false });
  return (
    <div className="stack">
      <div className="pl-head">
        <div><h2>{t("pl.title")}</h2><p className="soft small">{t("pl.lead")}</p></div>
        <a className="pill" href="/app/vouchers">{t("pl.back")}</a>
      </div>
      {res.isLoading ? <Skeleton h={420} /> : res.error || !res.data ? (
        <div className="stack"><Note kind="bad">{t("c.loadFailed")}</Note><button className="pill" type="button" onClick={() => void res.mutate()}>{t("pl.reload")}</button></div>
      ) : <LayoutForm initial={{ ...DEFAULT_LAYOUT, ...res.data.layout }} onSaved={() => void res.mutate()} />}
    </div>
  );
}
