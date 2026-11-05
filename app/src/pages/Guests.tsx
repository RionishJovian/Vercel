import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../lib/api";
import { asArray, type Me, type Router } from "../lib/types";
import { useI18n } from "../i18n";
import { BackLink, Note, PageHead } from "../components/ui";
import { Skeleton } from "../components/Skeleton";
import { HeadCheck } from "../components/DenseTable";
import { fmtBytes, fmtDuration, fmtUptime, NEAR_EXPIRY_SECONDS } from "./guestFormat";

interface RouterX extends Router { site_id?: string | null }
interface Session { id?: string; values?: Record<string, string> }
type Source = { gateway: RouterX; ok: true; sessions: Session[]; writable: boolean } | { gateway: RouterX; ok: false };
type Readable = Extract<Source, { ok: true }>;
interface Observations { inventory?: { available?: Record<string, boolean>; objects?: Record<string, Session[]>; write_enabled?: boolean } }
interface Row { key: string; s: Readable; session: Session; left: number | null; near: boolean }
type Filter = "all" | "active" | "near";
const EDITORS = ["OWNER", "ADMIN", "DEVELOPER"];
const CODE_CHUNK = 200;
const msgOf = (e: unknown) => { const d = (e as { data?: { error?: string } } | null)?.data; return d?.error || (e instanceof Error ? e.message : "") || "failed"; };

/** Expiry the app holds for each code. Time left is not part of the router read, so a code with no record shows no time. */
async function readExpiries(codes: string[]): Promise<Map<string, string>> {
  const out = new Map<string, string>();
  const usable = [...new Set(codes.filter((c) => c && !c.includes(",")))];
  for (let i = 0; i < usable.length; i += CODE_CHUNK) {
    const chunk = usable.slice(i, i + CODE_CHUNK);
    const res = await api.get<{ vouchers?: Array<{ code: string; expires_at?: string }> }>(`/vouchers?page=1&per_page=${CODE_CHUNK}&codes=${encodeURIComponent(chunk.join(","))}`);
    for (const v of asArray<{ code: string; expires_at?: string }>(res.vouchers)) if (v.expires_at) out.set(v.code, v.expires_at);
  }
  return out;
}

export function Guests({ me }: { me: Me }) {
  const { t } = useI18n();
  const routers = useSWR<{ routers?: RouterX[] }>("/routers", fetcher);
  const sitesRes = useSWR<{ sites?: Array<{ site_id: string; name: string }> }>("/sites", fetcher, { shouldRetryOnError: false });
  const gateways = useMemo(() => asArray<RouterX>(routers.data?.routers).filter((r) => r.deployment_layer === "MAIN_GATEWAY"), [routers.data]);
  const siteNames = useMemo(() => new Map(asArray<{ site_id: string; name: string }>(sitesRes.data?.sites).map((s) => [s.site_id, s.name])), [sitesRes.data]);
  const [sources, setSources] = useState<Source[]>([]);
  const [expiries, setExpiries] = useState<Map<string, string>>(new Map());
  const [readAt, setReadAt] = useState<number | null>(null);
  const [reading, setReading] = useState(false), [busy, setBusy] = useState(false);
  const [status, setStatus] = useState(""), [site, setSite] = useState(""), [device, setDevice] = useState(""), [q, setQ] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const canEdit = EDITORS.includes(me.user?.user_role ?? "");

  const reload = useCallback(async () => {
    if (!gateways.length) { setSources([]); setStatus(t("gu.noSources")); return; }
    setReading(true); setStatus(t("gu.refreshing"));
    const next: Source[] = new Array(gateways.length);
    let cursor = 0;
    async function worker() {
      while (cursor < gateways.length && alive.current) {
        const i = cursor++, gateway = gateways[i];
        try {
          const { inventory } = await api.get<Observations>(`/routers/${encodeURIComponent(gateway.device_id)}/observations`);
          if (inventory?.available?.hotspot_sessions !== true) throw new Error("unavailable");
          next[i] = { gateway, ok: true, sessions: asArray<Session>(inventory.objects?.hotspot_sessions), writable: inventory.write_enabled === true };
        } catch (e) { if (isUnauthorized(e)) throw e; next[i] = { gateway, ok: false }; }
      }
    }
    try {
      await Promise.all(Array.from({ length: Math.min(4, gateways.length) }, worker));
      if (!alive.current) return;
      let known = new Map<string, string>();
      try { known = await readExpiries(next.flatMap((s) => (s?.ok ? s.sessions.map((x) => x.values?.user ?? "") : []))); }
      catch (e) { if (isUnauthorized(e)) throw e; }
      if (alive.current) { setSources(next); setExpiries(known); setReadAt(Date.now()); setStatus(""); setPicked(new Set()); }
    }
    finally { if (alive.current) setReading(false); }
  }, [gateways, t]);
  useEffect(() => { if (routers.data) void reload(); }, [routers.data, reload]);

  if (routers.isLoading) return <><BackLink href="#/more">{t("tab.more")}</BackLink><Skeleton h={300} /></>;
  const siteIds = [...new Set(gateways.map((g) => g.site_id || "unassigned"))];
  const scoped = sources.filter((s) => (!site || (s.gateway.site_id || "unassigned") === site) && (!device || s.gateway.device_id === device));
  const available = scoped.filter((s): s is Readable => s.ok);
  const needle = q.trim().toLowerCase();
  const all: Row[] = available.flatMap((s) => s.sessions.map((session, i) => {
    const expiry = expiries.get(session.values?.user ?? "");
    const left = expiry && readAt ? Math.max(0, (new Date(expiry).getTime() - readAt) / 1000) : null;
    return { key: `${s.gateway.device_id}|${session.id ?? i}`, s, session, left, near: left !== null && left <= NEAR_EXPIRY_SECONDS };
  }));
  const found = all.filter(({ session }) => Object.values(session.values ?? {}).some((v) => String(v).toLowerCase().includes(needle)));
  const counts = { all: found.length, near: found.filter((r) => r.near).length, active: found.filter((r) => !r.near).length };
  const shown = found.filter((r) => filter === "all" || (filter === "near" ? r.near : !r.near));
  const deviceChoices = gateways.filter((g) => !site || (g.site_id || "unassigned") === site);
  const selected = shown.filter((r) => picked.has(r.key));
  const kickable = (r: Row) => canEdit && r.s.writable;
  const kicks = selected.filter(kickable);
  // The server reports write_enabled=false until the router action passes acceptance, so this is the normal state today.
  const kickHeld = all.some(({ s }) => !s.writable);
  const kickNote = kickHeld ? t("lu.kickOff") : t("gu.held");
  const count = (n: number) => (available.length ? n : "—");
  const siteLabel = site ? (siteNames.get(site) ?? (site === "unassigned" ? t("gu.unassigned") : site)) : siteIds.length === 1 ? (siteNames.get(siteIds[0]) ?? gateways[0]?.hardware_name ?? "") : t("gu.allSites");

  async function disconnect(r: Row): Promise<string | null> {
    const v = r.session.values ?? {};
    try {
      const res = await api.post<{ verified?: boolean }>(`/routers/${encodeURIComponent(r.s.gateway.device_id)}/guests/disconnect`, { id: r.session.id, mac: v["mac-address"], address: v.address, user: v.user, confirm: true });
      if (res.verified !== true) throw new Error("not verified");
      return null;
    } catch (e) { if (isUnauthorized(e)) throw e; return msgOf(e); }
  }
  async function kickOne(r: Row) {
    const v = r.session.values ?? {};
    if (busy || reading || !window.confirm(`${t("gu.confirm")} ${v.user ?? ""} / ${r.s.gateway.hardware_name ?? ""}`)) return;
    setBusy(true); setStatus(t("gu.disconnecting"));
    const failure = await disconnect(r);
    setBusy(false); await reload();
    setStatus(failure === null ? t("gu.disconnected") : `${t("gu.failed")} ${failure}`);
  }
  async function kickMany(targets: Row[]) {
    if (busy || reading || !window.confirm(t("lu.kickConfirm").replace("{n}", String(targets.length)))) return;
    setBusy(true); setStatus(t("gu.disconnecting"));
    let ok = 0;
    for (const r of targets) if ((await disconnect(r)) === null) ok++;
    setBusy(false); await reload();
    setStatus(t("lu.kicked").replace("{ok}", String(ok)).replace("{n}", String(targets.length)));
  }
  async function copy(targets: Row[]) {
    const text = targets.map(({ session }) => { const v = session.values ?? {}; return [v.user, v.address, v["mac-address"]].filter(Boolean).join("\t"); }).join("\n");
    try { await navigator.clipboard.writeText(text); setStatus(t("lu.copied")); } catch { setStatus(t("lu.copyFailed")); }
  }
  function toggleAll(on: boolean) { setPicked(on ? new Set(shown.map((r) => r.key)) : new Set()); }
  function toggle(key: string, on: boolean) { setPicked((p) => { const n = new Set(p); if (on) n.add(key); else n.delete(key); return n; }); }
  const unread = (label: string) => <button className="dt-filter" type="button" disabled title={t("lu.notRead")}>{label} · —</button>;

  return (
    <>
      <BackLink href="#/more">{t("tab.more")}</BackLink>
      <PageHead title={t("gu.title")} help={t("gu.lead")} />
      <div className="glass pad stack">
        {gateways.length > 1 ? (
          <div className="two">
            <select className="input" aria-label={t("gu.site")} value={site} onChange={(e) => { setSite(e.target.value); setDevice(""); }}><option value="">{t("gu.allSites")}</option>{siteIds.map((id) => <option key={id} value={id}>{siteNames.get(id) ?? (id === "unassigned" ? t("gu.unassigned") : id)}</option>)}</select>
            <select className="input" aria-label={t("v.f.gateway")} value={device} onChange={(e) => setDevice(e.target.value)}><option value="">{t("gu.allGateways")}</option>{deviceChoices.map((g) => <option key={g.device_id} value={g.device_id}>{g.hardware_name}</option>)}</select>
          </div>
        ) : null}
        <div className="dt-tools">
          <input className="input" type="search" placeholder={t("gu.search")} aria-label={t("gu.search")} value={q} onChange={(e) => setQ(e.target.value)} />
          <button className="dt-filter" type="button" disabled={reading || busy} onClick={() => void reload()}>{t("gu.refresh")}</button>
        </div>
        <div className="dt-filters" role="group" aria-label={t("lu.status")}>
          <button className="dt-filter" type="button" aria-pressed={filter === "all"} onClick={() => setFilter("all")}>{t("lu.all")} · <span className="num">{count(counts.all)}</span></button>
          <button className="dt-filter" type="button" data-tone="ok" aria-pressed={filter === "active"} onClick={() => setFilter("active")}>{t("lu.active")} · <span className="num">{count(counts.active)}</span></button>
          {unread(t("lu.idle"))}
          <button className="dt-filter" type="button" data-tone="warn" aria-pressed={filter === "near"} onClick={() => setFilter("near")}>{t("lu.near")} · <span className="num">{count(counts.near)}</span></button>
          {unread(t("lu.over"))}
          {unread(t("lu.banned"))}
        </div>
        <p className="soft small">
          {siteLabel}{readAt ? ` · ${t("lu.updated")} ${new Date(readAt).toLocaleTimeString()}` : ""}
          {available.length < scoped.length ? ` · ${t("gu.sources")} ${available.length} / ${scoped.length}` : ""}
        </p>
        <p className="soft small" role="status">{status}</p>
      </div>
      {reading && !sources.length ? <Skeleton h={200} /> : shown.length === 0 ? <Note>{all.length ? t("gu.noMatch") : t("gu.empty")}</Note> : (
        <div className="dt-wrap">
          {selected.length > 0 && (
            <div className="dt-bulk" role="group" aria-label={t("vt.selected").replace("{n}", String(selected.length))}>
              <b className="num">{t("vt.selected").replace("{n}", String(selected.length))}</b>
              <button className="dt-btn" type="button" disabled={busy || reading || !kicks.length} title={kicks.length ? undefined : kickNote} onClick={() => void kickMany(kicks)}>{t("lu.kickN").replace("{n}", String(kicks.length))}</button>
              <button className="dt-btn" type="button" onClick={() => void copy(selected)}>{t("lu.copyN").replace("{n}", String(selected.length))}</button>
              <button className="dt-btn" type="button" onClick={() => setPicked(new Set())}>{t("vt.clear")}</button>
            </div>
          )}
          <div className="dt-scroll">
            <table className="dt">
              <thead>
                <tr>
                  <th scope="col" className="dt-check"><HeadCheck label={t("lu.selectAll")} all={selected.length === shown.length} some={selected.length > 0} onChange={toggleAll} /></th>
                  <th scope="col">{t("lu.status")}</th><th scope="col">{t("lu.voucher")}</th><th scope="col">{t("lu.ip")}</th><th scope="col">{t("lu.mac")}</th>
                  <th scope="col">{t("lu.uptime")}</th><th scope="col">{t("lu.left")}</th><th scope="col">{t("lu.downUp")}</th><th scope="col" className="dt-right">{t("vt.actions")}</th>
                </tr>
              </thead>
              <tbody>
                {shown.map((r) => {
                  const v = r.session.values ?? {};
                  return (
                    <tr key={r.key} data-on={picked.has(r.key) ? "" : undefined}>
                      <td className="dt-check"><input type="checkbox" aria-label={`${t("vt.selectRow")} ${v.user ?? ""}`} checked={picked.has(r.key)} onChange={(e) => toggle(r.key, e.target.checked)} /></td>
                      <td data-cell="status">{r.near ? <span className="dt-status" data-tone="warn">{t("lu.near")}</span> : <span className="dt-status" data-tone="ok" title={t("gu.active")}>{t("lu.active")}</span>}</td>
                      <td data-cell="primary"><span className="dt-code">{v.user || t("gu.guest")}</span>{gateways.length > 1 ? <span className="dt-sub">{r.s.gateway.hardware_name}</span> : null}</td>
                      <td data-cell="half-a" className="dt-mono">{v.address || "—"}</td>
                      <td data-cell="half-b" className="dt-mono">{v["mac-address"] || "—"}</td>
                      <td data-cell="half-a" data-label={t("lu.uptime")} className="num">{fmtUptime(v.uptime)}</td>
                      <td data-cell="half-b" data-label={t("lu.left")} className="num">{r.left === null ? "—" : fmtDuration(r.left)}</td>
                      <td data-cell="line" data-label={t("lu.downUp")} className="num">{fmtBytes(v["bytes-out"])} ↓ / {fmtBytes(v["bytes-in"])} ↑</td>
                      <td data-cell="actions" className="dt-right">
                        <div className="dt-actions">
                          <button className="dt-btn" type="button" disabled={!kickable(r) || busy || reading} title={kickable(r) ? undefined : kickNote} onClick={() => void kickOne(r)}>{t("lu.kick")}</button>
                          <button className="dt-btn danger" type="button" disabled title={t("lu.off")}>{t("lu.ban")}</button>
                          <button className="dt-btn" type="button" disabled title={t("lu.off")}>{t("lu.rate")}</button>
                          <button className="dt-btn" type="button" onClick={() => void copy([r])}>{t("lu.copy")}</button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
      {scoped.some((s) => !s.ok) && <Note kind="warn">{t("gu.unavailable")}</Note>}
      <ul className="dt-foot">
        <li>{t("lu.off")}</li>
        {kickHeld || !canEdit ? <li>{kickNote}</li> : null}
      </ul>
    </>
  );
}
