import { useEffect, useMemo, useState } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { asArray, num } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { HeadCheck, Pager } from "../../components/DenseTable";
import { DEFAULT_LAYOUT, printCodes, type Layout, type PrintVoucher } from "./printCodes";

const DESKTOP_PAGE = 100, PHONE_PAGE = 25;
const STATUSES = ["unused", "active", "used", "expired", "revoked", "banned"] as const;
const TONE: Record<string, string> = { active: "ok", used: "", expired: "warn", revoked: "bad", banned: "bad" };

interface Pending { batch_id: string; verified_count: number; total_count: number }
interface StockBatch { batch_id: string; device_id?: string; plan_label?: string; total_count?: number; activated_count?: number; installed_count?: number; shop_name?: string; created_at?: string }
interface Batches { batches?: Pending[]; stock_batches?: StockBatch[] }
export interface Voucher extends PrintVoucher { id: string; status?: string; issue_batch_id?: string; install_state?: string; reseller_shop?: string }
interface Usage { voucher_id?: string; bytes_used?: number; uptime_seconds?: number; observed_at?: string }
interface PageRes { vouchers?: Voucher[]; total?: number; status_counts?: Record<string, number>; usage?: Record<string, Usage>; data_limits_mb?: Record<string, number>; handed_out?: Record<string, string> | null; hand_out_enabled?: boolean; router_delete_enabled?: boolean }

const fmtMB = (bytes: number) => (bytes / 1_000_000).toLocaleString(undefined, { maximumFractionDigits: bytes < 10_000_000 ? 1 : 0 });
const bump = (s: string, n: number | string) => s.replace("{n}", String(n));

function listKey(page: number, perPage: number, f: { status: string; q: string; batch: string }) {
  const p = new URLSearchParams({ page: String(page), per_page: String(perPage) });
  if (f.status) p.set("status", f.status);
  if (f.q) p.set("q", f.q);
  if (f.batch) p.set("batch", f.batch);
  return "/vouchers?" + p.toString();
}

export function Stock() {
  const { t } = useI18n();
  const batchesRes = useSWR<Batches>("/guest-vouchers/batches", fetcher);
  const layoutRes = useSWR<{ layout?: Partial<Layout> }>("/voucher-print-layout", fetcher, { revalidateOnFocus: false });
  // A phone shows fewer rows a page: the pager sits under them, and they stack taller.
  const [PER_PAGE] = useState(() => (window.matchMedia?.("(max-width: 760px)").matches ? PHONE_PAGE : DESKTOP_PAGE));
  const [page, setPage] = useState(1), [status, setStatus] = useState(""), [batch, setBatch] = useState("");
  const [search, setSearch] = useState(""), [q, setQ] = useState("");
  const filters = useMemo(() => ({ status, q, batch }), [status, q, batch]);
  const list = useSWR<PageRes>(listKey(page, PER_PAGE, filters), fetcher, { keepPreviousData: true });
  const [picked, setPicked] = useState<Map<string, Voucher>>(new Map());
  const [live, setLive] = useState<Record<string, Usage>>({});
  const [busy, setBusy] = useState(false), [retrying, setRetrying] = useState<string | null>(null);
  const [msg, setMsg] = useState<{ kind: "" | "ok" | "warn" | "bad"; text: string } | null>(null);
  const [handed, setHanded] = useState<Record<string, boolean>>({});

  useEffect(() => {
    const id = setTimeout(() => { setQ(search.trim().slice(0, 64)); setPage(1); }, 300);
    return () => clearTimeout(id);
  }, [search]);

  const incomplete = asArray<Pending>(batchesRes.data?.batches);
  const batches = useMemo(() => asArray<StockBatch>(batchesRes.data?.stock_batches).slice().sort((a, b) => String(b.created_at).localeCompare(String(a.created_at))), [batchesRes.data]);
  const unfinished = useMemo(() => new Set(incomplete.map((b) => b.batch_id)), [incomplete]);
  const planByBatch = useMemo(() => new Map(batches.map((b) => [b.batch_id, b])), [batches]);
  const rows = asArray<Voucher>(list.data?.vouchers);
  const total = num(list.data?.total) ?? 0;
  const counts = list.data?.status_counts ?? {};
  const all = Object.values(counts).reduce((a, b) => a + b, 0);
  const pages = Math.max(1, Math.ceil(total / PER_PAGE));
  // Deleting the last rows of the last page must not strand the screen past the end.
  if (list.data && !list.isValidating && page > pages) setPage(pages);

  const layout: Layout = { ...DEFAULT_LAYOUT, ...(layoutRes.data?.layout ?? {}) };
  const firstUse = t("v.validFirstUse");
  /** Same guard as batch printing: never print a code the router has not verified, or one from an unfinished batch. */
  const canSlip = (v: Voucher) => !!batchesRes.data && !!layoutRes.data && !!v.issue_batch_id && v.install_state === "verified" && !unfinished.has(v.issue_batch_id) && (v.status === "unused" || v.status === "active");
  const routerDelete = list.data?.router_delete_enabled === true, handOutOn = list.data?.hand_out_enabled === true;
  const isHanded = (v: Voucher) => handed[v.id] ?? !!list.data?.handed_out?.[v.id];
  /** A code on the router is deleted by removing its login there; the voucher stays on record as revoked, which keeps batch recovery and sales history. */
  const canDelete = (v: Voucher) => (v.issue_batch_id
    ? routerDelete && v.status === "unused" && v.install_state === "verified" && !!batchesRes.data && !unfinished.has(v.issue_batch_id)
    : v.status !== "used");
  const deleteNote = (v: Voucher) => (v.issue_batch_id && !routerDelete ? t("vt.deleteOff") : t("vt.deleteUnused"));
  const canHandOut = (v: Voucher) => handOutOn && (v.status === "unused" || isHanded(v));
  const hidden = (v: Voucher) => !!v.install_state && v.install_state !== "verified";

  const pageIds = rows.map((v) => v.id);
  const pagePicked = pageIds.filter((id) => picked.has(id)).length;
  const selected = [...picked.values()];
  const slips = selected.filter(canSlip), deletable = selected.filter(canDelete);
  const toHandOut = selected.filter((v) => handOutOn && v.status === "unused" && !isHanded(v));

  function toggle(v: Voucher, on: boolean) { setPicked((m) => { const n = new Map(m); if (on) n.set(v.id, v); else n.delete(v.id); return n; }); }
  function togglePage(on: boolean) { setPicked((m) => { const n = new Map(m); for (const v of rows) { if (on) n.set(v.id, v); else n.delete(v.id); } return n; }); }
  function changeFilter(fn: () => void) { fn(); setPage(1); }

  async function selectAll() {
    setBusy(true); setMsg(null);
    try {
      const next = new Map(picked);
      for (let p = 1; ; p++) {
        const res = await api.get<PageRes>(listKey(p, 200, filters));
        const got = asArray<Voucher>(res.vouchers);
        for (const v of got) next.set(v.id, v);
        if (got.length < 200 || p * 200 >= (num(res.total) ?? 0)) break;
      }
      setPicked(next);
    } catch (e) { if (isUnauthorized(e)) throw e; setMsg({ kind: "bad", text: t("c.loadFailed") }); }
    setBusy(false);
  }
  async function remove(targets: Voucher[]) {
    const onRouter = targets.some((v) => v.issue_batch_id);
    if (!targets.length || !window.confirm(bump(t(onRouter ? "vt.deleteRouterConfirm" : "vt.deleteConfirm"), targets.length))) return;
    setBusy(true); setMsg(null);
    let ok = 0;
    const left = new Map(picked);
    for (const v of targets) {
      try {
        if (v.issue_batch_id) {
          const r = await api.post<{ verified?: boolean }>(`/guest-vouchers/codes/${encodeURIComponent(v.id)}/retire`, { confirm: true });
          if (r.verified !== true) continue;
        } else await api.del(`/vouchers/${encodeURIComponent(v.id)}`);
        ok++; left.delete(v.id);
      }
      catch (e) { if (isUnauthorized(e)) throw e; }
    }
    setPicked(left); setBusy(false);
    setMsg({ kind: ok === targets.length ? "ok" : "bad", text: bump(t("vt.deleted").replace("{ok}", String(ok)), targets.length) });
    void list.mutate(); void batchesRes.mutate();
  }
  async function setHandOut(targets: Voucher[], on: boolean) {
    setBusy(true); setMsg(null);
    let ok = 0;
    const next = { ...handed };
    for (const v of targets) {
      try { await api.put(`/vouchers/${encodeURIComponent(v.id)}/hand-out`, { handed_out: on }); next[v.id] = on; ok++; }
      catch (e) { if (isUnauthorized(e)) throw e; }
    }
    setHanded(next); setBusy(false);
    if (ok < targets.length) setMsg({ kind: "bad", text: t("vt.handOutFailed") });
    void list.mutate();
  }
  async function refreshUsage() {
    const devices = [...new Set(batches.map((b) => b.device_id).filter((d): d is string => !!d))];
    setBusy(true); setMsg(null);
    let failed = false;
    const next = { ...live };
    for (const device_id of devices) {
      try {
        const r = await api.post<{ synced?: Usage[] }>("/guest-vouchers/sync", { device_id });
        for (const u of asArray<Usage>(r.synced)) if (u.voucher_id) next[u.voucher_id] = u;
      } catch (e) { if (isUnauthorized(e)) throw e; failed = true; }
    }
    setLive(next); setBusy(false);
    setMsg(failed ? { kind: "bad", text: t("v.refreshFailed") } : { kind: "ok", text: t("vt.refreshed") });
    void list.mutate(); void batchesRes.mutate();
  }
  async function retry(id: string) {
    setRetrying(id);
    try { await api.post(`/guest-vouchers/batches/${encodeURIComponent(id)}/retry`, {}); } catch (e) { if (isUnauthorized(e)) throw e; }
    setRetrying(null); void batchesRes.mutate(); void list.mutate();
  }
  async function printBatch(id: string) {
    setBusy(true); setMsg(null);
    try {
      const res = await api.get<{ state?: string; vouchers?: PrintVoucher[] }>("/guest-vouchers/batches/" + encodeURIComponent(id));
      // A deleted, used or expired code stays in its batch's record but must not be printed.
      const codes = asArray<Voucher>(res?.vouchers).filter((v) => v.status === undefined || v.status === "unused" || v.status === "active");
      if (res?.state === "ready" && codes.length) printCodes(codes, layout, firstUse); else setMsg({ kind: "warn", text: t("p.notReady") });
    } catch (e) { if (isUnauthorized(e)) throw e; setMsg({ kind: "bad", text: t("c.loadFailed") }); }
    setBusy(false);
  }

  if (list.isLoading && !list.data) return <Skeleton h={320} />;
  if (list.error && !list.data) return <div className="stack"><Note kind="bad">{t("c.loadFailed")}</Note><button className="pill" type="button" onClick={() => void list.mutate()}>{t("pl.reload")}</button></div>;
  const filtered = !!(status || q || batch);
  const current = batch ? planByBatch.get(batch) : undefined;

  return (
    <div className="stack">
      {incomplete.length > 0 && (
        <section className="glass pad stack">
          <h2>{t("v.pendingTitle")}</h2><p className="soft small">{t("v.pendingBody")}</p>
          {incomplete.map((b) => (
            <div className="row" key={b.batch_id}>
              <b className="num">{b.verified_count} / {b.total_count}</b>
              <button className="pill warn" type="button" disabled={retrying === b.batch_id} onClick={() => void retry(b.batch_id)}>{retrying === b.batch_id ? t("v.retrying") : t("v.retry")}</button>
            </div>
          ))}
        </section>
      )}
      {!filtered && all === 0 ? <Note>{t("v.empty")}</Note> : (
        <>
          <div className="glass pad stack">
            <div className="dt-tools three">
              <input className="input" type="search" placeholder={t("vt.search")} aria-label={t("vt.search")} value={search} onChange={(e) => setSearch(e.target.value)} />
              <select className="input" aria-label={t("p.pick")} value={batch} onChange={(e) => { const v = e.target.value; changeFilter(() => setBatch(v)); }}>
                <option value="">{t("vt.allBatches")}</option>
                {batches.map((b) => <option key={b.batch_id} value={b.batch_id}>{`${b.plan_label || "—"} · ${b.total_count ?? 0}${b.shop_name ? " · " + b.shop_name : ""}`}</option>)}
              </select>
              <button className="dt-filter" type="button" disabled={busy || !batches.length} onClick={() => void refreshUsage()}>{busy ? t("v.refreshing") : t("v.refresh")}</button>
            </div>
            <div className="dt-filters" role="group" aria-label={t("vt.statusCol")}>
              <button className="dt-filter" type="button" aria-pressed={!status} onClick={() => changeFilter(() => setStatus(""))}>{t("vt.all")} · <span className="num">{all}</span></button>
              {STATUSES.map((s) => <button className="dt-filter" type="button" key={s} data-tone={TONE[s] || "soft"} aria-pressed={status === s} onClick={() => changeFilter(() => setStatus(s))}>{t("vt.status." + s)} · <span className="num">{counts[s] ?? 0}</span></button>)}
            </div>
            {current ? (
              <div className="dt-summary">
                <span><span className="soft">{t("v.onRouter")}</span> <b className="num">{current.installed_count ?? 0}/{current.total_count ?? 0}</b></span>
                <span><span className="soft">{t("v.used")}</span> <b className="num">{current.activated_count ?? 0}/{current.total_count ?? 0}</b></span>
                <span><span className="soft">{t("v.shop")}</span> <b>{current.shop_name || t("v.noShop")}</b></span>
                <button className="dt-btn" type="button" disabled={busy || !layoutRes.data} onClick={() => void printBatch(current.batch_id)}>{t("vt.printBatch")}</button>
              </div>
            ) : null}
            {msg && <Note kind={msg.kind}>{msg.text}</Note>}
          </div>

          <div className="dt-wrap">
            {selected.length > 0 && (
              <div className="dt-bulk" role="group" aria-label={bump(t("vt.selected"), selected.length)}>
                <b className="num">{bump(t("vt.selected"), selected.length)}</b>
                {total > selected.length && <button className="dt-btn" type="button" disabled={busy} onClick={() => void selectAll()}>{bump(t("vt.selectAll"), total)}</button>}
                <button className="dt-btn" type="button" disabled={busy || !slips.length} onClick={() => printCodes(slips, layout, firstUse)}>{bump(t("vt.slips"), slips.length)}</button>
                <button className="dt-btn warn" type="button" disabled={busy || !toHandOut.length} title={handOutOn ? undefined : t("vt.handOutOff")} onClick={() => void setHandOut(toHandOut, true)}>{bump(t("vt.handOutN"), toHandOut.length)}</button>
                <button className="dt-btn danger" type="button" disabled={busy || !deletable.length} title={deletable.length || routerDelete ? undefined : t("vt.deleteOff")} onClick={() => void remove(deletable)}>{bump(t("vt.deleteN"), deletable.length)}</button>
                <button className="dt-btn" type="button" onClick={() => setPicked(new Map())}>{t("vt.clear")}</button>
              </div>
            )}
            {rows.length === 0 ? <div className="pad"><Note>{t("vt.noMatch")}</Note></div> : (
              <div className="dt-scroll">
                <table className="dt">
                  <thead>
                    <tr>
                      <th scope="col" className="dt-check"><HeadCheck label={t("vt.selectPage")} all={pagePicked === rows.length} some={pagePicked > 0} onChange={togglePage} /></th>
                      <th scope="col">{t("vt.code")}</th><th scope="col">{t("vt.plan")}</th><th scope="col">{t("vt.statusCol")}</th>
                      <th scope="col">{t("v.dataUsed")}</th><th scope="col">{t("v.timeUsed")}</th><th scope="col" className="dt-right">{t("vt.actions")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((v) => {
                      const saved = list.data?.usage?.[v.id], u = live[v.id] ?? saved;
                      const limitMB = num(list.data?.data_limits_mb?.[v.id]) ?? 0, limitMin = num(v.duration_minutes) ?? 0;
                      const asOf = saved?.observed_at && !live[v.id] ? `${t("v.asOf")} ${new Date(saved.observed_at).toLocaleString()}` : undefined;
                      const secret = hidden(v), slip = canSlip(v), del = canDelete(v);
                      const shop = v.reseller_shop || (v.issue_batch_id ? planByBatch.get(v.issue_batch_id)?.shop_name : "");
                      const data = u ? `${fmtMB(num(u.bytes_used) ?? 0)}${limitMB ? ` / ${fmtMB(limitMB * 1_000_000)}` : ""} MB` : "—";
                      const time = u ? `${Math.floor((num(u.uptime_seconds) ?? 0) / 60)}${limitMin ? ` / ${limitMin}` : ""} ${t("v.min")}` : "—";
                      return (
                        <tr key={v.id} data-on={picked.has(v.id) ? "" : undefined}>
                          <td className="dt-check"><input type="checkbox" aria-label={`${t("vt.selectRow")} ${secret ? "" : v.code}`} checked={picked.has(v.id)} onChange={(e) => toggle(v, e.target.checked)} /></td>
                          <td data-cell="primary"><span className="dt-code" title={secret ? t("vt.hiddenCode") : undefined}>{secret ? "••••••••" : v.code}</span></td>
                          <td data-cell="line"><div className="dt-plan" title={shop ? `${v.plan_label} · ${shop}` : v.plan_label}>{v.plan_label || "—"}{shop ? <span className="soft"> · {shop}</span> : null}</div></td>
                          <td data-cell="status">{secret ? <span className="dt-status" data-tone="warn">{t("vt.notInstalled")}</span> : <span className="dt-status" data-tone={TONE[v.status ?? ""] || undefined}>{STATUSES.includes(v.status as (typeof STATUSES)[number]) ? t("vt.status." + v.status) : v.status || "—"}</span>}</td>
                          <td data-cell="half-a" data-label={t("v.dataUsed")} className="num" title={asOf}>{data}</td>
                          <td data-cell="half-b" data-label={t("v.timeUsed")} className="num" title={asOf}>{time}</td>
                          <td data-cell="actions" className="dt-right">
                            <div className="dt-actions">
                              <button className="dt-btn" type="button" disabled={!slip} title={slip ? undefined : t("vt.slipOff")} onClick={() => printCodes([v], layout, firstUse)}>{t("vt.slip")}</button>
                              <button className={"dt-btn warn" + (isHanded(v) ? " on" : "")} type="button" aria-pressed={isHanded(v)} disabled={!canHandOut(v) || busy} title={handOutOn ? undefined : t("vt.handOutOff")} onClick={() => void setHandOut([v], !isHanded(v))}>{isHanded(v) ? t("vt.handedOut") : t("vt.handOut")}</button>
                              <button className="dt-btn danger" type="button" disabled={!del || busy} title={del ? undefined : deleteNote(v)} onClick={() => void remove([v])}>{t("vt.delete")}</button>
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
            <Pager page={Math.min(page, pages)} perPage={PER_PAGE} total={total} shown={rows.length} onPage={setPage} />
          </div>
          <ul className="dt-foot">{handOutOn ? null : <li>{t("vt.handOutOff")}</li>}{routerDelete ? null : <li>{t("vt.deleteOff")}</li>}<li>{t("v.liveNote")}</li></ul>
        </>
      )}
    </div>
  );
}
