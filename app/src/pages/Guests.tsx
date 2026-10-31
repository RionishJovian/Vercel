import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../lib/api";
import { asArray, type Me, type Router } from "../lib/types";
import { useI18n } from "../i18n";
import { BackLink, Note } from "../components/ui";
import { Skeleton } from "../components/Skeleton";

interface RouterX extends Router { site_id?: string | null }
interface Session { id?: string; values?: Record<string, string> }
type Source = { gateway: RouterX; ok: true; sessions: Session[]; writable: boolean } | { gateway: RouterX; ok: false };
interface Observations { inventory?: { available?: Record<string, boolean>; objects?: Record<string, Session[]>; write_enabled?: boolean } }
const EDITORS = ["OWNER", "ADMIN", "DEVELOPER"];
const msgOf = (e: unknown) => { const d = (e as { data?: { error?: string } } | null)?.data; return d?.error || (e instanceof Error ? e.message : "") || "failed"; };

export function Guests({ me }: { me: Me }) {
  const { t } = useI18n();
  const routers = useSWR<{ routers?: RouterX[] }>("/routers", fetcher);
  const sitesRes = useSWR<{ sites?: Array<{ site_id: string; name: string }> }>("/sites", fetcher, { shouldRetryOnError: false });
  const gateways = useMemo(() => asArray<RouterX>(routers.data?.routers).filter((r) => r.deployment_layer === "MAIN_GATEWAY"), [routers.data]);
  const siteNames = useMemo(() => new Map(asArray<{ site_id: string; name: string }>(sitesRes.data?.sites).map((s) => [s.site_id, s.name])), [sitesRes.data]);
  const [sources, setSources] = useState<Source[]>([]);
  const [reading, setReading] = useState(false), [busy, setBusy] = useState(false);
  const [status, setStatus] = useState(""), [site, setSite] = useState(""), [device, setDevice] = useState(""), [q, setQ] = useState("");
  const alive = useRef(true);
  useEffect(() => () => { alive.current = false; }, []);
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
    try { await Promise.all(Array.from({ length: Math.min(4, gateways.length) }, worker)); if (alive.current) { setSources(next); setStatus(t("gu.done")); } }
    finally { if (alive.current) setReading(false); }
  }, [gateways, t]);
  useEffect(() => { if (routers.data) void reload(); }, [routers.data, reload]);

  if (routers.isLoading) return <><BackLink href="#/more">{t("tab.more")}</BackLink><Skeleton h={300} /></>;
  const siteIds = [...new Set(gateways.map((g) => g.site_id || "unassigned"))];
  const scoped = sources.filter((s) => (!site || (s.gateway.site_id || "unassigned") === site) && (!device || s.gateway.device_id === device));
  const available = scoped.filter((s): s is Extract<Source, { ok: true }> => s.ok);
  const all = available.flatMap((s) => s.sessions.map((session) => ({ s, session })));
  const needle = q.trim().toLowerCase();
  const shown = all.filter(({ session }) => Object.values(session.values ?? {}).some((v) => String(v).toLowerCase().includes(needle)));
  const deviceChoices = gateways.filter((g) => !site || (g.site_id || "unassigned") === site);

  async function disconnect(s: Extract<Source, { ok: true }>, session: Session) {
    const v = session.values ?? {};
    if (busy || reading || !window.confirm(`${t("gu.confirm")} ${v.user ?? ""} / ${s.gateway.hardware_name ?? ""}`)) return;
    setBusy(true); setStatus(t("gu.disconnecting"));
    let note = "";
    try {
      const r = await api.post<{ verified?: boolean }>(`/routers/${encodeURIComponent(s.gateway.device_id)}/guests/disconnect`, { id: session.id, mac: v["mac-address"], address: v.address, user: v.user, confirm: true });
      if (r.verified !== true) throw new Error("not verified");
      note = t("gu.disconnected");
    } catch (e) { if (isUnauthorized(e)) throw e; note = `${t("gu.failed")} ${msgOf(e)}`; }
    setBusy(false); await reload(); setStatus(note);
  }

  return (
    <>
      <BackLink href="#/more">{t("tab.more")}</BackLink>
      <h1>{t("gu.title")}</h1><p className="soft">{t("gu.lead")}</p>
      <div className="capsules">
        <div className="capsule"><span className="soft small">{available.length < scoped.length ? t("gu.sessionsPartial") : t("gu.sessions")}</span><b className="num">{available.length ? all.length : "—"}</b></div>
        <div className="capsule"><span className="soft small">{t("gu.sources")}</span><b className="num">{available.length} / {scoped.length}</b></div>
      </div>
      <div className="glass pad stack">
        <div className="two">
          <select className="input" aria-label={t("gu.site")} value={site} onChange={(e) => { setSite(e.target.value); setDevice(""); }}><option value="">{t("gu.allSites")}</option>{siteIds.map((id) => <option key={id} value={id}>{siteNames.get(id) ?? (id === "unassigned" ? t("gu.unassigned") : id)}</option>)}</select>
          <select className="input" aria-label={t("v.f.gateway")} value={device} onChange={(e) => setDevice(e.target.value)}><option value="">{t("gu.allGateways")}</option>{deviceChoices.map((g) => <option key={g.device_id} value={g.device_id}>{g.hardware_name}</option>)}</select>
        </div>
        <input className="input" type="search" placeholder={t("gu.search")} aria-label={t("gu.search")} value={q} onChange={(e) => setQ(e.target.value)} />
        <button className="btn quiet" type="button" disabled={reading || busy} onClick={() => void reload()}>{t("gu.refresh")}</button>
        <p className="soft small" role="status">{status}</p>
      </div>
      {reading && !sources.length ? <Skeleton h={200} /> : shown.length === 0 ? <Note>{all.length ? t("gu.noMatch") : t("gu.empty")}</Note> : (
        <div className="stack">
          {shown.map(({ s, session }, i) => {
            const v = session.values ?? {};
            return (
              <div className="glass pad stack" key={(session.id ?? "") + i}>
                <div className="row"><b>{v.user || t("gu.guest")}</b><span className="chip ok">{t("gu.active")}</span></div>
                <p className="soft small num">{v.address} · {v["mac-address"]}</p>
                <div className="row small"><span className="soft">{siteNames.get(s.gateway.site_id ?? "") ?? s.gateway.hardware_name}</span><span className="num">{v.uptime} · ↓{v["bytes-out"] ?? "—"} ↑{v["bytes-in"] ?? "—"}</span></div>
                {canEdit && s.writable && <button className="pill danger" type="button" disabled={busy || reading} onClick={() => void disconnect(s, session)}>{t("gu.disconnect")}</button>}
              </div>
            );
          })}
        </div>
      )}
      {scoped.some((s) => !s.ok) && <Note kind="warn">{t("gu.unavailable")}</Note>}
      {(!canEdit || all.some(({ s }) => !s.writable)) && <p className="soft small">{t("gu.held")}</p>}
    </>
  );
}
