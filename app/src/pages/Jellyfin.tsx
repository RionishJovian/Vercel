import { useCallback, useEffect, useRef, useState } from "react";
import useSWR from "swr";
import { api, ApiError, fetcher, isUnauthorized } from "../lib/api";
import { asArray, type Me } from "../lib/types";
import { useI18n } from "../i18n";
import { Note, WorkspaceTabs } from "../components/ui";
import { Skeleton } from "../components/Skeleton";
import { JellyfinDesign } from "./jellyfin/JellyfinDesign";

const PARTS = ["dash", "vouchers", "guest"] as const;
type Part = (typeof PARTS)[number];
const n = (v: unknown) => (typeof v === "number" && Number.isFinite(v) ? v.toLocaleString("en-US") : "—");
const Kv = ({ k, v }: { k: string; v: string }) => <div className="row small"><span className="soft">{k}</span><b className="num">{v}</b></div>;

interface Job { state?: string; result?: Record<string, unknown>; error_code?: string }
/** Queue one job and poll it until the owner's PC has answered (or about 60 s pass). */
async function runJob(operation: string, payload: Record<string, unknown> = {}): Promise<Record<string, unknown>> {
  const id = crypto.randomUUID();
  await api.post("/media/jobs", { id, operation, payload });
  for (let i = 0; i < 30; i++) {
    await new Promise((r) => setTimeout(r, 2000));
    const j = await api.get<Job>("/media/jobs/" + id);
    if (j.state === "succeeded") return j.result ?? {};
    if (["failed", "cancelled", "expired"].includes(j.state ?? "")) throw new Error(j.error_code || j.state);
  }
  throw new Error("timeout");
}

interface Connection { server_name?: string; revoked_at?: string | null; last_seen_at?: string }
type Dash = { server?: { ServerName?: string; Version?: string }; active_sessions?: number; counts?: Record<string, number> };
type Lib = { Name?: string; CollectionType?: string; Locations?: string[] };

function Live() {
  const { t } = useI18n();
  const [state, setState] = useState<{ kind: "loading" } | { kind: "err" } | { kind: "ok"; d: Dash; libs: Lib[] }>({ kind: "loading" });
  const [scan, setScan] = useState<"idle" | "busy" | "done" | "failed">("idle");
  const load = useCallback(async () => {
    setState({ kind: "loading" });
    try {
      const [dash, libs] = await Promise.all([runJob("dashboard"), runJob("libraries")]);
      setState({ kind: "ok", d: (dash.dashboard as Dash) ?? {}, libs: asArray<Lib>(libs.libraries) });
    } catch (e) { if (isUnauthorized(e)) throw e; setState({ kind: "err" }); }
  }, []);
  useEffect(() => { void load(); }, [load]);
  return (
    <section className="glass pad stack">
      <h2>{t("j.status")}</h2>
      {state.kind === "loading" && <p className="soft">{t("j.checking")}</p>}
      {state.kind === "err" && <><Note kind="warn">{t("j.noAnswer")}</Note><button className="pill" type="button" onClick={() => void load()}>{t("c.retry")}</button></>}
      {state.kind === "ok" && (() => {
        const { d, libs } = state, c = d.counts ?? {};
        return (<>
          <Kv k={t("j.server")} v={(d.server?.ServerName ?? "—") + (d.server?.Version ? " " + d.server.Version : "")} />
          <Kv k={t("j.watching")} v={n(d.active_sessions)} />
          <Kv k={t("j.movies")} v={n(c.MovieCount ?? c.movies)} />
          <Kv k={t("j.series")} v={n(c.SeriesCount ?? c.series)} />
          <h3>{t("j.libraries")}</h3>
          {!libs.length && <p className="soft">{t("j.noLibraries")}</p>}
          {libs.map((l, i) => <div className="row" key={i}><span>{l.Name || "—"} <span className="chip">{l.CollectionType || "—"}</span></span><span className="soft small">{l.Locations?.[0] ?? ""}</span></div>)}
          <button className="pill" type="button" disabled={scan === "busy" || scan === "done"} onClick={async () => {
            if (!window.confirm(t("j.scanConfirm"))) return;
            setScan("busy");
            try { await runJob("scan", { confirm: true }); setScan("done"); } catch { setScan("failed"); }
          }}>{scan === "done" ? t("j.scanned") : scan === "failed" ? t("j.failed") : t("j.scan")}</button>
        </>);
      })()}
    </section>
  );
}

function Dashboard() {
  const { t } = useI18n();
  const { data: c, error, mutate, isLoading } = useSWR<Connection>("/media/connection", fetcher, { shouldRetryOnError: false });
  const [code, setCode] = useState<string | null>(null);
  const [pairErr, setPairErr] = useState(false);
  const timer = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearInterval(timer.current), []);
  if (isLoading) return <Skeleton h={220} />;
  if (error instanceof ApiError && error.status === 403) return <Note kind="warn">{t("j.locked")}</Note>;
  const linked = !!(c?.server_name && !c.revoked_at);

  async function pair() {
    setPairErr(false);
    try {
      const r = await api.post<{ pairing_code: string }>("/media/pairing", {});
      setCode(r.pairing_code);
      let tries = 0;
      timer.current = window.setInterval(async () => {
        tries++;
        try { const x = await api.get<Connection>("/media/connection"); if (x?.server_name && !x.revoked_at) { window.clearInterval(timer.current); setCode(null); void mutate(); } } catch { /* keep waiting */ }
        if (tries > 100) window.clearInterval(timer.current);
      }, 6000);
    } catch { setPairErr(true); }
  }
  return (
    <>
      <section className="glass pad stack">
        <div className="row"><h2>{t("j.link")}</h2><span className={"chip " + (linked ? "ok" : "warn")}>{linked ? t("j.linked") : t("j.notLinked")}</span></div>
        {linked ? (<>
          <Kv k={t("j.server")} v={c?.server_name ?? "—"} /><Kv k={t("j.lastSeen")} v={c?.last_seen_at ? new Date(c.last_seen_at).toLocaleString() : "—"} />
          <button className="pill danger" type="button" onClick={async () => { if (!window.confirm(t("j.unlinkConfirm"))) return; try { await api.del("/media/connection"); } catch { /* shown on refresh */ } void mutate(); }}>{t("j.unlink")}</button>
        </>) : (<>
          <p className="soft">{t("j.pairHelp")}</p>
          <button className="btn" type="button" disabled={!!code} onClick={() => void pair()}>{code ? t("j.pairWait") : t("j.pair")}</button>
          {code && <p className="num pair-code" aria-live="polite">{code}</p>}
          {pairErr && <Note kind="bad">{t("j.failed")}</Note>}
          <p className="soft small">{t("j.pairExpiry")}</p>
        </>)}
      </section>
      {linked ? <Live /> : <section className="glass pad stack"><h2>{t("j.status")}</h2><p className="soft">{t("j.needLink")}</p></section>}
    </>
  );
}

export function Jellyfin({ sub, me }: { sub: string; me: Me }) {
  const { t } = useI18n();
  const part: Part = (PARTS as readonly string[]).includes(sub) ? (sub as Part) : "dash";
  return (
    <>
      <h1>{t("title.jellyfin")}</h1>
      <WorkspaceTabs items={PARTS.map((p) => ({ href: "#/jellyfin/" + p, label: t("j.ws." + p), current: p === part }))} />
      {part === "dash" ? <Dashboard /> : part === "vouchers" ? (
        <section className="glass pad stack"><h2>{t("j.notReady")}</h2><p className="soft">{t("j.notReadyBody")}</p></section>
      ) : (
        <JellyfinDesign me={me} />
      )}
    </>
  );
}
