import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, ApiError, fetcher, isUnauthorized } from "../../lib/api";
import { asArray, type Router } from "../../lib/types";
import { useI18n } from "../../i18n";
import { errorText } from "../../lib/errors";
import { Field, Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { DEFAULT_LAYOUT, printCodes, type Layout, type PrintVoucher } from "./printCodes";

const PENDING_KEY = "mm2_gv_pending";
type Pending = Record<string, unknown> & { batch_id?: string };
const readPending = (): Pending | null => { try { return JSON.parse(localStorage.getItem(PENDING_KEY) || "null"); } catch { return null; } };
const writePending = (v: Pending | null) => { try { if (v) localStorage.setItem(PENDING_KEY, JSON.stringify(v)); else localStorage.removeItem(PENDING_KEY); } catch { /* storage blocked */ } };
const DURATIONS = ["60", "1440", "10080", "43200"];

interface Created { state?: string; vouchers?: Array<PrintVoucher & { code: string }> }

export function Create() {
  const { t } = useI18n();
  const routers = useSWR<{ routers?: Router[] }>("/routers", fetcher);
  const config = useSWR<{ max_batch_size?: number }>("/guest-vouchers/config", fetcher);
  const shops = useSWR<{ resellers?: Array<{ id: string; shop_name: string; active?: boolean }> }>("/voucher-resellers", fetcher);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<{ kind: "ready"; codes: NonNullable<Created["vouchers"]> } | { kind: "notReady" | "failed" | "creating" } | null>(null);
  const [saved, setSaved] = useState(readPending);
  const [selected, setSelected] = useState(() => String(readPending()?.device_id ?? ""));
  const [failure, setFailure] = useState("");
  const gateways = asArray<Router>(routers.data?.routers).filter(d => d.deployment_layer === "MAIN_GATEWAY");
  const gateway = gateways.find(g => g.device_id === selected) ?? gateways[0];
  const profiles = useSWR<{ profiles?: string[] }>(gateway ? `/routers/${encodeURIComponent(gateway.device_id)}/portal/profiles` : null, fetcher, { shouldRetryOnError: false });
  const ready = profiles.data?.profiles?.length === 1;
  function accept(res: Created) {
    if (res?.state !== "ready") { setResult({ kind: "notReady" }); return; }
    writePending(null); setSaved(null); setResult({ kind: "ready", codes: asArray(res.vouchers) });
  }
  async function retry(id: string) {
    setBusy(true); setFailure(""); setResult({ kind: "creating" });
    try {
      const path = "/guest-vouchers/batches/" + encodeURIComponent(id);
      let res: Created;
      try { res = await api.get<Created>(path); }
      catch (e) {
        // The browser records the request before sending it. A lost request
        // can leave only that local record; resend the same immutable batch.
        if (!(e instanceof ApiError) || e.status !== 404 || !saved || !ready) throw e;
        accept(await api.post<Created>("/guest-vouchers", saved));
        return;
      }
      accept(res.state === "ready" || res.state === "installing" ? res : await api.post<Created>(path + "/retry"));
    } catch (e) { if (isUnauthorized(e)) throw e; setFailure(errorText(e, t("v.createFailed"))); setResult({ kind: "failed" }); }
    finally { setBusy(false); }
  }

  if (routers.isLoading || config.isLoading || shops.isLoading) return <Skeleton h={420} />;
  const max = config.data?.max_batch_size ?? 200;
  const shopList = asArray<{ id: string; shop_name: string; active?: boolean }>(shops.data?.resellers).filter((s) => s.active !== false);
  if (!gateways.length) return <Note kind="warn">{t("v.noGateway")}</Note>;

  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!ready || busy || saved) return;
    const f = new FormData(e.currentTarget);
    const body: Record<string, unknown> = {
      device_id: f.get("gw"), plan_label: String(f.get("label")).trim(), duration_minutes: Number(f.get("dur")),
      data_mb: Number(f.get("data")), price_mmk: Number(f.get("price")), count: Number(f.get("count")),
    };
    if (f.get("shop")) body.reseller_id = f.get("shop");
    body.batch_id = crypto.randomUUID();
    // Persist the identity BEFORE the request: a lost response or reload retries the same batch and never makes new codes.
    writePending(body); setSaved(body); setFailure("");
    setBusy(true); setResult({ kind: "creating" });
    try {
      const res = await api.post<Created>("/guest-vouchers", body);
      accept(res);
    } catch (err) { if (isUnauthorized(err)) throw err; setFailure(errorText(err, t("v.createFailed"))); setResult({ kind: "failed" }); }
    finally { setBusy(false); }
  }

  async function print(codes: PrintVoucher[]) {
    let layout: Partial<Layout> = DEFAULT_LAYOUT;
    try { layout = { ...DEFAULT_LAYOUT, ...((await api.get<{ layout?: Layout }>("/voucher-print-layout")).layout ?? {}) }; } catch { /* default layout */ }
    printCodes(codes, layout, t("v.validFirstUse"));
  }

  return (
    <div className="stack">
      {profiles.error && <Note kind="warn">{errorText(profiles.error, t("c.loadFailed"))}</Note>}
      {profiles.data && !ready && <Note kind="warn">{t("v.hotspotRequired")} <a className="link" href={"/app/network/new?router=" + encodeURIComponent(gateway.device_id)}>{t("nw.title")}</a></Note>}
      {(!profiles.data || !ready) && <button className="pill" type="button" disabled={profiles.isValidating} onClick={() => void profiles.mutate()}>{t(profiles.isValidating ? "r.checking" : "c.retry")}</button>}
      {saved?.batch_id && <Note kind="warn">{t("v.pendingFirst")} <button type="button" className="pill" disabled={busy} onClick={() => void retry(saved.batch_id!)}>{t("v.retrySavedBatch")}</button></Note>}
      <form className="glass pad stack" onSubmit={submit}>
        <Field label={t("v.f.gateway")} htmlFor="gv-gw"><select className="input" id="gv-gw" name="gw" value={gateway.device_id} onChange={e => setSelected(e.target.value)} disabled={busy || !!saved} required>{gateways.map((g) => <option key={g.device_id} value={g.device_id}>{g.hardware_name || g.device_id}</option>)}</select></Field>
        <Field label={t("v.f.label")} htmlFor="gv-label"><input className="input" id="gv-label" name="label" maxLength={80} required placeholder="1 day" /></Field>
        <Field label={t("v.f.duration")} htmlFor="gv-dur"><select className="input" id="gv-dur" name="dur" defaultValue="1440">{DURATIONS.map((m) => <option key={m} value={m}>{t("v.d." + m)}</option>)}</select></Field>
        <div className="two">
          <Field label={t("v.f.data")} htmlFor="gv-data"><input className="input" id="gv-data" name="data" type="number" inputMode="numeric" min={1} step={1} required defaultValue={1024} /></Field>
          <Field label={t("v.f.price")} htmlFor="gv-price"><input className="input" id="gv-price" name="price" type="number" inputMode="numeric" min={0} step={1} required defaultValue={0} /></Field>
        </div>
        <div className="two">
          <Field label={t("v.f.count")} htmlFor="gv-count"><input className="input" id="gv-count" name="count" type="number" inputMode="numeric" min={1} max={max} step={1} required defaultValue={50} /></Field>
          <Field label={t("v.f.shop")} htmlFor="gv-shop"><select className="input" id="gv-shop" name="shop"><option value="">{t("v.noShop")}</option>{shopList.map((s) => <option key={s.id} value={s.id}>{s.shop_name}</option>)}</select></Field>
        </div>
        <button className="btn" type="submit" disabled={busy || !ready || !!saved}>{t("v.create")}</button>
      </form>
      {result?.kind === "creating" && <Note>{t("v.creating")}</Note>}
      {result?.kind === "failed" && <Note kind="bad">{failure || t("v.createFailed")}</Note>}
      {result?.kind === "notReady" && <Note kind="warn">{t("v.notReady")}</Note>}
      {result?.kind === "ready" && (
        <>
          <Note kind="ok">{t("v.ready")}</Note><Note kind="warn">{t("v.codesOnce")}</Note>
          <button className="btn" type="button" onClick={() => void print(result.codes)}>{t("v.print")}</button>
          <div className="glass pad stack">{result.codes.map((v) => <div className="row" key={v.code}><b className="num code">{v.code}</b><span className="soft small">{t("v.validFirstUse")}</span></div>)}</div>
        </>
      )}
    </div>
  );
}
