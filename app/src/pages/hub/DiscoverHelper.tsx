import { useEffect, useState } from "react";
import useSWR from "swr";
import { api, ApiError, fetcher, isUnauthorized } from "../../lib/api";
import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";

export interface Candidate {
  device_key: string; macs?: string[]; identity?: string; board?: string; platform?: string; version?: string; interface?: string;
  claimed_ip?: string; source_ip?: string; ip_mismatch?: boolean; ip_changed?: boolean; heard_count: number; last_seen: string; selected: boolean;
  status: string; existing_router?: { device_id: string; name: string };
}
export interface SessionView { session_id: string; state: "open" | "paired" | "cancelled" | "closed" | "expired"; site_id?: string; expires_at: string; paired_at?: string; pairing_expires_at?: string; candidates: Candidate[] }
interface Selection { device_key: string; suggested_name: string; device_model?: string; local_address?: string; site_id?: string; existing_router?: { device_id: string; name: string } }
/** What the add-router form is prefilled with. Every value is an unverified hint from nearby discovery. */
export interface DiscoveryPrefill { sessionId: string; deviceKey: string; name: string; model: string; address: string; siteId: string }

const mmss = (ms: number) => { const s = Math.max(0, Math.ceil(ms / 1000)); return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`; };

function useNow(active: boolean) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => { if (!active) return; const t = window.setInterval(() => setNow(Date.now()), 1000); return () => window.clearInterval(t); }, [active]);
  return now;
}

function Card({ c, onSelect, busy }: { c: Candidate; onSelect: (c: Candidate) => void; busy: boolean }) {
  const { t } = useI18n();
  const unknown = t("dn.h.unknown");
  const ip = c.source_ip || c.claimed_ip || "";
  return (
    <article className="glass pad stack" aria-label={c.identity || c.board || t("dn.h.nearby")}>
      <h4>{t("dn.h.nearby")}</h4>
      <dl className="facts">
        <dt>{t("dn.h.device")}</dt><dd>{c.platform ? `${c.platform} RouterBOARD` : unknown}</dd>
        <dt>{t("dn.h.model")}</dt><dd>{c.board || unknown}</dd>
        <dt>{t("dn.h.ip")}</dt><dd className="num">{ip || unknown}</dd>
        <dt>RouterOS</dt><dd>{c.version || unknown}</dd>
        <dt>{t("dn.h.how")}</dt><dd>MNDP</dd>
        <dt>{t("dn.h.status")}</dt><dd>{t("dn.h.statusText")}</dd>
      </dl>
      {c.ip_mismatch && <Note kind="warn">{t("dn.h.mismatch")}</Note>}
      {c.ip_changed && <Note kind="warn">{t("dn.h.changed")}</Note>}
      {c.existing_router
        ? <Note kind="warn">{t("dn.h.existing")} <a className="link" href={"#/router/" + encodeURIComponent(c.existing_router.device_id)}>{c.existing_router.name}</a></Note>
        : <button className="btn" type="button" disabled={busy} onClick={() => onSelect(c)}>{t("dn.h.select")}</button>}
    </article>
  );
}

/**
 * Automatic discovery with a helper program on a laptop that is on the router's own network.
 * Hidden unless the server has discovery switched on. Everything shown is an unverified observation: only the
 * existing one-paste Magic Hub connection can verify a router.
 */
export function DiscoverHelper({ sites, onSelected }: { sites: Array<{ site_id: string; name: string }>; onSelected: (p: DiscoveryPrefill) => void }) {
  const { t } = useI18n();
  const cap = useSWR<{ enabled?: boolean }>("/discovery/capability", fetcher, { shouldRetryOnError: false });
  const [site, setSite] = useState(""), [busy, setBusy] = useState(false), [err, setErr] = useState("");
  const [session, setSession] = useState<{ id: string; code: string } | null>(null), [copied, setCopied] = useState(false);
  const view = useSWR<SessionView>(session ? `/discovery/sessions/${session.id}` : null, fetcher, {
    shouldRetryOnError: false, refreshInterval: (d) => (d && (d.state === "open" || d.state === "paired") ? 3000 : 0),
  });
  const v = view.data;
  const live = !!v && (v.state === "open" || v.state === "paired");
  const now = useNow(live);
  if (cap.error || !cap.data?.enabled) return null;

  const chosenSite = site || (sites.length === 1 ? sites[0].site_id : "");
  async function start() {
    setBusy(true); setErr("");
    try {
      const r = await api.post<{ session_id: string; pairing_code: string }>("/discovery/sessions", chosenSite ? { site_id: chosenSite } : {});
      setSession({ id: r.session_id, code: r.pairing_code }); setCopied(false);
    } catch (e) {
      if (isUnauthorized(e)) throw e;
      setErr(e instanceof ApiError && e.status === 409 ? t("dn.h.alreadyOpen") : t("dn.h.startFail"));
    }
    setBusy(false);
  }
  async function cancel() {
    if (!session) return;
    try { await api.del(`/discovery/sessions/${session.id}`); } catch (e) { if (isUnauthorized(e)) throw e; }
    void view.mutate();
  }
  async function select(c: Candidate) {
    if (!session) return;
    setBusy(true); setErr("");
    try {
      const r = await api.post<Selection>(`/discovery/sessions/${session.id}/select`, { device_key: c.device_key });
      onSelected({ sessionId: session.id, deviceKey: r.device_key, name: r.suggested_name, model: r.device_model ?? "", address: r.local_address ?? "", siteId: r.site_id ?? "" });
    } catch (e) { if (isUnauthorized(e)) throw e; setErr(t("dn.h.selectFail")); }
    setBusy(false);
  }
  async function copy() {
    if (!session) return;
    try { await navigator.clipboard.writeText(`magic-discover --pair ${session.code}`); setCopied(true); } catch { setCopied(false); }
  }

  const cands = asArray<Candidate>(v?.candidates);
  const pairLeft = v?.pairing_expires_at ? Date.parse(v.pairing_expires_at) - now : 0;
  const ended = !!v && !live;
  return (
    <section className="stack" aria-labelledby="dnh-title">
      <h4 id="dnh-title">{t("dn.h.title")}</h4>
      <p className="soft small">{t("dn.h.lead")}</p>
      {err && <Note kind="bad">{err}</Note>}
      {!session || ended ? (
        <>
          {ended && <Note kind="warn">{t("dn.h.end." + v.state)}</Note>}
          {sites.length > 1 && (
            <label className="field"><span>{t("hub.site")}</span>
              <select className="input" value={site} onChange={(e) => setSite(e.target.value)}>
                <option value="">{t("hub.chooseSite")}</option>{sites.map((s) => <option key={s.site_id} value={s.site_id}>{s.name}</option>)}
              </select>
            </label>
          )}
          <button className="btn quiet" type="button" disabled={busy} onClick={() => void start()}>{t("dn.h.start")}</button>
          <p className="soft small">{t("dn.h.noDownload")}</p>
        </>
      ) : (
        <>
          <p className="soft small">{t("dn.h.runIt")}</p>
          <pre className="input script num" aria-label={t("dn.h.command")}>magic-discover --pair {session.code}</pre>
          <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
            <button className="pill" type="button" onClick={() => void copy()}>{copied ? t("hub.copied") : t("hub.copy")}</button>
            <button className="pill" type="button" onClick={() => void cancel()}>{t("dn.h.cancel")}</button>
          </div>
          {v?.state === "open" && (pairLeft > 0
            ? <p className="note" role="status">{t("dn.h.waiting")} {t("dn.h.codeLeft")} <b className="num">{mmss(pairLeft)}</b></p>
            : <Note kind="warn">{t("dn.h.codeExpired")}</Note>)}
          {v?.state === "paired" && <p className="note ok" role="status">{t("dn.h.paired")}</p>}
          {v?.state === "paired" && !cands.length && <p className="soft small">{t("dn.h.none")}</p>}
        </>
      )}
      {cands.map((c) => <Card key={c.device_key} c={c} onSelect={(x) => void select(x)} busy={busy} />)}
    </section>
  );
}
