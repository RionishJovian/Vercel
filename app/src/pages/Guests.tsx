import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../lib/api";
import { asArray, type Me, type Router } from "../lib/types";
import { useVisibleInterval } from "../lib/polling";
import { useI18n } from "../i18n";
import { BackLink, Note, PageHead } from "../components/ui";
import { Skeleton } from "../components/Skeleton";
import { errorText } from "../lib/errors";
import { HeadCheck } from "../components/DenseTable";
import { fmtBytes, fmtDuration, fmtUptime, IDLE_SECONDS, NEAR_EXPIRY_SECONDS, parseSeconds } from "./guestFormat";

interface RouterX extends Router { site_id?: string | null }
interface Session { id?: string; values?: Record<string, string> }
type Source = { gateway: RouterX; ok: true; sessions: Session[]; writable: boolean; times: Session[] | null; bindings: Session[] | null } | { gateway: RouterX; ok: false };
type Readable = Extract<Source, { ok: true }>;
interface Observations { guest_actions_enabled?: boolean; inventory?: { available?: Record<string, boolean>; objects?: Record<string, Session[]> } }
interface Row { key: string; s: Readable; session: Session; left: number | null; near: boolean; idle: boolean; over: boolean }
type Filter = "all" | "active" | "idle" | "near" | "over" | "banned";
interface VoucherFact { expires_at?: string; over: boolean }
const BAN_COMMENT = "mega-magic:ban";
const EDITORS = ["OWNER", "ADMIN", "DEVELOPER"];
const CODE_CHUNK = 200;

/** What the app holds about each code: time limit and saved use against the data limit. */
async function readVoucherFacts(codes: string[]): Promise<Map<string, VoucherFact>> {
  const out = new Map<string, VoucherFact>();
  const usable = [...new Set(codes.filter((c) => c && !c.includes(",")))];
  for (let i = 0; i < usable.length; i += CODE_CHUNK) {
    const chunk = usable.slice(i, i + CODE_CHUNK);
    const res = await api.get<{ vouchers?: Array<{ id: string; code: string; expires_at?: string }>; usage?: Record<string, { bytes_used?: number }>; data_limits_mb?: Record<string, number> }>(`/vouchers?page=1&per_page=${CODE_CHUNK}&codes=${encodeURIComponent(chunk.join(","))}`);
    for (const v of asArray<{ id: string; code: string; expires_at?: string }>(res.vouchers)) {
      const limit = res.data_limits_mb?.[v.id] ?? 0, used = res.usage?.[v.id]?.bytes_used ?? 0;
      out.set(v.code, { expires_at: v.expires_at, over: limit > 0 && used >= limit * 1_000_000 });
    }
  }
  return out;
}

/** Each read asks the router, so this is slower than the port rates. */
const GUESTS_REFRESH_MS = 30000;

export function Guests({ me }: { me: Me }) {
  const { t } = useI18n();
  const routers = useSWR<{ routers?: RouterX[] }>("/routers", fetcher);
  const sitesRes = useSWR<{ sites?: Array<{ site_id: string; name: string }> }>("/sites", fetcher, { shouldRetryOnError: false });
  const gateways = useMemo(() => asArray<RouterX>(routers.data?.routers).filter((r) => r.deployment_layer === "MAIN_GATEWAY"), [routers.data]);
  const siteNames = useMemo(() => new Map(asArray<{ site_id: string; name: string }>(sitesRes.data?.sites).map((s) => [s.site_id, s.name])), [sitesRes.data]);
  const [sources, setSources] = useState<Source[]>([]);
  const [facts, setFacts] = useState<Map<string, VoucherFact> | null>(null);
  const [readAt, setReadAt] = useState<number | null>(null);
  const [reading, setReading] = useState(false), [busy, setBusy] = useState(false);
  const [status, setStatus] = useState(""), [site, setSite] = useState(""), [device, setDevice] = useState(""), [q, setQ] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const alive = useRef(true), refreshing = useRef(false), generation = useRef(0);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const canEdit = EDITORS.includes(me.user?.user_role ?? "");

  // A background refresh changes only the figures. It leaves the owner's selection
  // and any message alone, and quietly keeps the last reading if it fails.
  const reload = useCallback(async (background = false) => {
    if (!gateways.length) { setSources([]); if (!background) setStatus(t("gu.noSources")); return; }
    if (!background) { setReading(true); setStatus(t("gu.refreshing")); }
    const started = background ? generation.current : ++generation.current;
    const next: Source[] = new Array(gateways.length);
    let cursor = 0;
    async function worker() {
      while (cursor < gateways.length && alive.current) {
        const i = cursor++, gateway = gateways[i];
        try {
          const { inventory, guest_actions_enabled } = await api.get<Observations>(`/routers/${encodeURIComponent(gateway.device_id)}/observations`);
          if (inventory?.available?.hotspot_sessions !== true) throw new Error("unavailable");
          next[i] = { gateway, ok: true, sessions: asArray<Session>(inventory.objects?.hotspot_sessions), writable: guest_actions_enabled === true,
            times: inventory.available?.hotspot_session_times === true ? asArray<Session>(inventory.objects?.hotspot_session_times) : null,
            bindings: inventory.available?.hotspot_bindings === true ? asArray<Session>(inventory.objects?.hotspot_bindings) : null };
        } catch (e) { if (isUnauthorized(e)) throw e; next[i] = { gateway, ok: false }; }
      }
    }
    try {
      await Promise.all(Array.from({ length: Math.min(4, gateways.length) }, worker));
      if (!alive.current) return;
      let known: Map<string, VoucherFact> | null = null;
      try { known = await readVoucherFacts(next.flatMap((s) => (s?.ok ? s.sessions.map((x) => x.values?.user ?? "") : []))); }
      catch (e) { if (isUnauthorized(e)) throw e; }
      if (!alive.current || (background && generation.current !== started)) return;
      setSources(next); setFacts(known); setReadAt(Date.now());
      if (!background) { setStatus(""); setPicked(new Set()); }
    }
    catch (e) { if (!background || isUnauthorized(e)) throw e; }
    finally { if (alive.current && !background) setReading(false); }
  }, [gateways, t]);
  useEffect(() => { if (routers.data) void reload(); }, [routers.data, reload]);
  useVisibleInterval(() => { if (!reading && !busy && !refreshing.current) { refreshing.current = true; void reload(true).finally(() => { refreshing.current = false; }); } }, GUESTS_REFRESH_MS, !!routers.data);

  if (routers.isLoading) return <><BackLink href="/app/more">{t("tab.more")}</BackLink><Skeleton h={300} /></>;
  const siteIds = [...new Set(gateways.map((g) => g.site_id || "unassigned"))];
  const scoped = sources.filter((s) => (!site || (s.gateway.site_id || "unassigned") === site) && (!device || s.gateway.device_id === device));
  const available = scoped.filter((s): s is Readable => s.ok);
  const needle = q.trim().toLowerCase();
  const all: Row[] = available.flatMap((s) => s.sessions.map((session, i) => {
    const fact = facts?.get(session.values?.user ?? "");
    const time = s.times?.find((x) => x.id === session.id)?.values;
    const appLeft = fact?.expires_at && readAt ? Math.max(0, (new Date(fact.expires_at).getTime() - readAt) / 1000) : null;
    const routerLeft = parseSeconds(time?.["session-time-left"]);
    const lefts = [appLeft, routerLeft].filter((n): n is number => n !== null);
    const left = lefts.length ? Math.min(...lefts) : null;
    return { key: `${s.gateway.device_id}|${session.id ?? i}`, s, session, left, near: left !== null && left <= NEAR_EXPIRY_SECONDS, idle: (parseSeconds(time?.["idle-time"]) ?? 0) >= IDLE_SECONDS, over: fact?.over === true };
  }));
  const found = all.filter(({ session }) => Object.values(session.values ?? {}).some((v) => String(v).toLowerCase().includes(needle)));
  const plainActive = (r: Row) => !r.near && !r.idle && !r.over;
  const bans = available.flatMap((s) => asArray<Session>(s.bindings).filter((b) => b.values?.type === "blocked" && b.values?.disabled !== "true").map((binding) => ({ s, binding })));
  const counts = { all: found.length, active: found.filter(plainActive).length, idle: found.filter((r) => r.idle).length, near: found.filter((r) => r.near).length, over: found.filter((r) => r.over).length, banned: bans.length };
  const shown = found.filter((r) => (filter === "all" ? true : filter === "active" ? plainActive(r) : filter === "banned" ? false : r[filter]));
  const timesRead = available.length > 0 && available.every((s) => s.times !== null), bindingsRead = available.length > 0 && available.every((s) => s.bindings !== null), factsRead = facts !== null;
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
    } catch (e) { if (isUnauthorized(e)) throw e; return errorText(e, ""); }
  }
  async function setBan(s: Readable, mac: string, block: boolean) {
    if (busy || reading || !window.confirm(t(block ? "lu.banConfirm" : "lu.unbanConfirm").replace("{mac}", mac))) return;
    setBusy(true); setStatus(t("gu.disconnecting"));
    let note: string;
    try {
      const res = await api.post<{ verified?: boolean }>(`/routers/${encodeURIComponent(s.gateway.device_id)}/guests/${block ? "block" : "unblock"}`, { mac, confirm: true });
      if (res.verified !== true) throw new Error("not verified");
      note = t(block ? "lu.bannedDone" : "lu.unbanned");
    } catch (e) { if (isUnauthorized(e)) throw e; note = `${t("lu.banFailed")} ${errorText(e, "")}`; }
    setBusy(false); await reload(); setStatus(note);
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
  const chip = (key: Filter, label: string, tone: string, read: boolean, n: number) => (read
    ? <button className="dt-filter" type="button" data-tone={tone} aria-pressed={filter === key} onClick={() => setFilter(key)}>{label} · <span className="num">{n}</span></button>
    : <button className="dt-filter" type="button" disabled title={t("lu.notRead")}>{label} · —</button>);

  return (
    <>
      <BackLink href="/app/more">{t("tab.more")}</BackLink>
      <PageHead title={t("gu.title")} help={t("gu.lead")} />
      <div className="glass pad stack workspace-rail">
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
          {chip("active", t("lu.active"), "ok", available.length > 0, counts.active)}
          {chip("idle", t("lu.idle"), "soft", timesRead, counts.idle)}
          {chip("near", t("lu.near"), "warn", available.length > 0, counts.near)}
          {chip("over", t("lu.over"), "bad", factsRead && available.length > 0, counts.over)}
          {chip("banned", t("lu.banned"), "bad", bindingsRead, counts.banned)}
        </div>
        <p className="soft small num">
          {siteLabel}{readAt ? ` · ${t("lu.updated")} ${new Date(readAt).toLocaleTimeString()}` : ""}
          {available.length < scoped.length ? ` · ${t("gu.sources")} ${available.length} / ${scoped.length}` : ""}
        </p>
        <p className="soft small" role="status">{status}</p>
      </div>
      <div className="workspace-main workspace-stack">
      {filter === "banned" ? (
        bans.length === 0 ? <Note>{t("lu.noBans")}</Note> : (
          <div className="dt-wrap"><div className="dt-scroll">
            <table className="dt">
              <thead><tr><th scope="col">{t("lu.mac")}</th><th scope="col">{t("lu.banNote")}</th><th scope="col" className="dt-right">{t("vt.actions")}</th></tr></thead>
              <tbody>
                {bans.map(({ s, binding }, i) => {
                  const v = binding.values ?? {}, mine = v.comment === BAN_COMMENT, can = canEdit && s.writable && mine && !!v["mac-address"];
                  return (
                    <tr key={s.gateway.device_id + (binding.id ?? i)}>
                      <td data-cell="primary" className="dt-mono num">{v["mac-address"] || "—"}</td>
                      <td data-cell="line">{mine ? t("lu.banByApp") : v.comment || t("lu.banOther")}</td>
                      <td data-cell="actions" className="dt-right"><div className="dt-actions"><button className="dt-btn" type="button" disabled={!can || busy || reading} title={can ? undefined : mine ? kickNote : t("lu.banOther")} onClick={() => void setBan(s, v["mac-address"] ?? "", false)}>{t("lu.unban")}</button></div></td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div></div>
        )
      ) : reading && !sources.length ? <Skeleton h={200} /> : shown.length === 0 ? <Note>{all.length ? t("gu.noMatch") : t("gu.empty")}</Note> : (
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
                      <td data-cell="status">{r.over ? <span className="dt-status" data-tone="bad">{t("lu.over")}</span> : r.near ? <span className="dt-status" data-tone="warn">{t("lu.near")}</span> : r.idle ? <span className="dt-status">{t("lu.idle")}</span> : <span className="dt-status" data-tone="ok" title={t("gu.active")}>{t("lu.active")}</span>}</td>
                      <td data-cell="primary"><span className="dt-code tnum">{v.user || t("gu.guest")}</span>{gateways.length > 1 ? <span className="dt-sub">{r.s.gateway.hardware_name}</span> : null}</td>
                      <td data-cell="half-a" className="dt-mono num">{v.address || "—"}</td>
                      <td data-cell="half-b" className="dt-mono num">{v["mac-address"] || "—"}</td>
                      <td data-cell="half-a" data-label={t("lu.uptime")} className="num">{fmtUptime(v.uptime)}</td>
                      <td data-cell="half-b" data-label={t("lu.left")} className="num">{r.left === null ? "—" : fmtDuration(r.left)}</td>
                      <td data-cell="line" data-label={t("lu.downUp")} className="num">{fmtBytes(v["bytes-out"])} ↓ / {fmtBytes(v["bytes-in"])} ↑</td>
                      <td data-cell="actions" className="dt-right">
                        <div className="dt-actions">
                          <button className="dt-btn" type="button" disabled={!kickable(r) || busy || reading} title={kickable(r) ? undefined : kickNote} onClick={() => void kickOne(r)}>{t("lu.kick")}</button>
                          <button className="dt-btn danger" type="button" disabled={!kickable(r) || busy || reading || !v["mac-address"]} title={kickable(r) ? undefined : kickNote} onClick={() => void setBan(r.s, v["mac-address"] ?? "", true)}>{t("lu.ban")}</button>
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
      </div>
    </>
  );
}
