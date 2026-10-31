import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Field, Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { DEFAULT_LAYOUT, printCodes, type Layout, type PrintVoucher } from "./printCodes";

interface Batch { batch_id: string; plan_label?: string; total_count?: number }

export function Print() {
  const { t } = useI18n();
  const layoutRes = useSWR<{ layout?: Partial<Layout> }>("/voucher-print-layout", fetcher);
  const batchRes = useSWR<{ stock_batches?: Batch[] }>("/guest-vouchers/batches", fetcher);
  const [layoutMsg, setLayoutMsg] = useState<{ kind: "ok" | "bad"; text: string } | null>(null);
  const [printMsg, setPrintMsg] = useState<{ kind: "warn" | "bad"; text: string } | null>(null);
  const [busy, setBusy] = useState(false);
  if (layoutRes.isLoading || batchRes.isLoading) return <Skeleton h={420} />;
  const layout: Layout = { ...DEFAULT_LAYOUT, ...(layoutRes.data?.layout ?? {}) };
  const batches = asArray<Batch>(batchRes.data?.stock_batches);

  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const body: Record<string, unknown> = { ...layout, business_name: String(f.get("biz")).trim(), wifi_name: String(f.get("wifi")).trim(), support_contact: String(f.get("support")).trim(), terms: String(f.get("terms")).trim(), a4_columns: Number(f.get("cols")), show_price: f.get("price") === "on", show_expiry: f.get("expiry") === "on" };
    delete body.business_id; delete body.created_at; delete body.updated_at;
    setBusy(true);
    try { await api.put("/voucher-print-layout", body); setLayoutMsg({ kind: "ok", text: t("p.saved") }); void layoutRes.mutate(); } catch (err) { if (isUnauthorized(err)) throw err; setLayoutMsg({ kind: "bad", text: t("p.saveFailed") }); }
    setBusy(false);
  }
  async function printBatch(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const id = String(new FormData(e.currentTarget).get("batch"));
    setBusy(true); setPrintMsg(null);
    try {
      const res = await api.get<{ state?: string; vouchers?: PrintVoucher[] }>("/guest-vouchers/batches/" + encodeURIComponent(id));
      const codes = asArray<PrintVoucher>(res?.vouchers);
      if (res?.state === "ready" && codes.length) printCodes(codes, layout, t("v.validFirstUse")); else setPrintMsg({ kind: "warn", text: t("p.notReady") });
    } catch (err) { if (isUnauthorized(err)) throw err; setPrintMsg({ kind: "bad", text: t("c.loadFailed") }); }
    setBusy(false);
  }
  return (
    <div className="stack">
      {batches.length ? (
        <form className="glass pad stack" onSubmit={printBatch}>
          <h2>{t("p.printBatch")}</h2>
          <Field label={t("p.pick")} htmlFor="pl-batch"><select className="input" id="pl-batch" name="batch">{batches.map((b) => <option key={b.batch_id} value={b.batch_id}>{(b.plan_label || "—") + " · " + b.total_count}</option>)}</select></Field>
          {printMsg && <Note kind={printMsg.kind}>{printMsg.text}</Note>}
          <button className="btn" type="submit" disabled={busy}>{t("p.printBtn")}</button>
        </form>
      ) : <Note>{t("p.noBatches")}</Note>}
      <form className="glass pad stack" onSubmit={save} key={JSON.stringify(layoutRes.data)}>
        <h2>{t("p.layout")}</h2>
        <Field label={t("p.biz")} htmlFor="pl-biz"><input className="input" id="pl-biz" name="biz" maxLength={120} required defaultValue={layout.business_name} /></Field>
        <Field label={t("p.wifi")} htmlFor="pl-wifi"><input className="input" id="pl-wifi" name="wifi" maxLength={120} required defaultValue={layout.wifi_name} /></Field>
        <Field label={t("p.support")} htmlFor="pl-support"><input className="input" id="pl-support" name="support" maxLength={200} required defaultValue={layout.support_contact} /></Field>
        <Field label={t("p.terms")} htmlFor="pl-terms"><textarea className="input" id="pl-terms" name="terms" maxLength={2000} rows={3} required defaultValue={layout.terms} style={{ padding: "12px 16px", minHeight: 96 }} /></Field>
        <Field label={t("p.cols")} htmlFor="pl-cols"><select className="input" id="pl-cols" name="cols" defaultValue={String(layout.a4_columns)}>{[2, 3, 4, 5].map((n) => <option key={n} value={n}>{n}</option>)}</select></Field>
        <label className="check"><span>{t("p.showPrice")}</span><input type="checkbox" name="price" defaultChecked={layout.show_price} /></label>
        <label className="check"><span>{t("p.showExpiry")}</span><input type="checkbox" name="expiry" defaultChecked={layout.show_expiry} /></label>
        {layoutMsg && <Note kind={layoutMsg.kind}>{layoutMsg.text}</Note>}
        <button className="btn" type="submit" disabled={busy}>{t("p.save")}</button>
      </form>
    </div>
  );
}
