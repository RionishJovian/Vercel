import { useState } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../lib/api";
import { asArray } from "../lib/types";
import { useI18n } from "../i18n";
import { BackLink, Note } from "../components/ui";
import { Skeleton } from "../components/Skeleton";

interface Adv { id: string; title: string; fix_unavailable?: boolean }
interface Dev { device_id: string; hardware_name?: string; posture?: string; routeros_version?: string; routeros_channel?: string; unpatchable?: boolean; exploited?: boolean; fix_first?: string; advisories?: Adv[]; version_checked_at?: string }
interface Posture { devices?: Dev[]; count?: number; affected?: number; safe?: number; unknown?: number; unpatchable?: number; exploited?: number; controls_enabled?: boolean }
const errText = (e: unknown) => (e as { data?: { error?: string } } | null)?.data?.error || (e instanceof Error ? e.message : "") || "failed";
// Worst first: an unfixable finding outranks a fixable one, which outranks "could not tell", which outranks fine.
export const rank = (d: Dev) => (d.posture === "affected" ? (d.unpatchable ? 0 : 1) : d.posture === "unknown" ? 2 : 3);

function Row({ d, controls }: { d: Dev; controls: boolean }) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(""), [res, setRes] = useState<{ bad: boolean; text: string; detail?: string } | null>(null);
  const [harden, setHarden] = useState<string | null>(null);
  const id = encodeURIComponent(d.device_id);
  async function run(kind: string, fn: () => Promise<{ bad: boolean; text: string; detail?: string }>) {
    setBusy(kind); setRes(null);
    try { setRes(await fn()); } catch (e) { if (isUnauthorized(e)) throw e; setRes({ bad: true, text: errText(e) }); }
    setBusy("");
  }
  const verify = () => run("verify", async () => { const r = await api.post<{ reachable?: boolean; detail?: string; state?: string }>(`/devices/${id}/security/verify`, {}); return { bad: !r.reachable, text: r.reachable ? t("sec.verified") : t("sec.verifyFail"), detail: r.detail || `${t("sec.state")} ${r.state || "unknown"}` }; });
  const backup = () => run("backup", async () => { const r = await api.post<{ id?: string }>(`/devices/${id}/backups`, { label: "Pre-security-action" }); return { bad: false, text: t("sec.backedUp"), detail: `ID: ${r.id}` }; });
  const doHarden = () => run("harden", async () => {
    const r = await api.post<{ verified?: boolean; verification_detail?: string; rollback_detail?: string }>(`/devices/${id}/security/harden`, { confirm: harden });
    setHarden(null);
    return { bad: !r.verified, text: r.verified ? t("sec.hardened") : t("sec.hardenFail"), detail: r.verified ? r.verification_detail : r.rollback_detail || r.verification_detail };
  });
  const tone = d.posture === "affected" ? "bad" : d.posture === "safe" ? "ok" : "";
  const fix = d.posture !== "affected" ? "" : d.fix_first ? `${t("sec.upgrade")} ≥ ${d.fix_first}` : t("sec.closeService");
  return (
    <article className="glass pad stack">
      <div className="row"><b>{d.hardware_name}</b><span className={"chip " + tone}>{t("sec.p." + (d.posture || "unknown"))}</span></div>
      <p className="soft small num">{d.routeros_version || "—"} {d.routeros_channel ? `· ${d.routeros_channel}` : ""}</p>
      <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
        {d.posture === "affected" && d.unpatchable && <span className="chip warn">{d.fix_first ? t("sec.partlyUnfixable") : t("sec.noFix")}</span>}
        {d.exploited && <span className="chip bad">{t("sec.exploited")}</span>}
        {(d.advisories?.length ?? 0) > 0 && <span className="chip" title={asArray<Adv>(d.advisories).map((a) => `${a.id} — ${a.title}${a.fix_unavailable ? " (no fix)" : ""}`).join("\n")}>{d.advisories!.length} {t("sec.matching")}</span>}
      </div>
      {fix && <p className="small">{fix}</p>}
      <p className="soft small">{t("sec.checked")}: {d.version_checked_at ? new Date(d.version_checked_at).toLocaleString() : "—"}</p>
      {!controls ? <span className="soft small">{t("sec.controlOff")}</span> : (
        <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
          <button className="pill" type="button" disabled={!!busy} onClick={() => void verify()}>{busy === "verify" ? t("sec.checking") : t("sec.verify")}</button>
          <button className="pill" type="button" disabled={!!busy} onClick={() => void backup()}>{busy === "backup" ? t("sec.capturing") : t("sec.backup")}</button>
          {d.posture === "affected" && <button className="pill warn" type="button" disabled={!!busy} onClick={() => setHarden("")}>{t("sec.mitigate")}</button>}
        </div>
      )}
      {harden !== null && (
        <div className="note warn stack">
          <p className="small">{t("sec.hardenWarn")}</p>
          <input className="input" value={harden} onChange={(e) => setHarden(e.target.value)} placeholder="HARDEN" aria-label={t("sec.typeHarden")} autoCapitalize="characters" />
          <div className="row" style={{ justifyContent: "flex-start" }}><button className="pill" type="button" onClick={() => setHarden(null)}>{t("c.cancel")}</button><button className="pill danger" type="button" disabled={harden !== "HARDEN" || !!busy} onClick={() => void doHarden()}>{busy === "harden" ? t("sec.applying") : t("sec.mitigate")}</button></div>
        </div>
      )}
      {res && <Note kind={res.bad ? "bad" : "ok"}><b>{res.text}</b>{res.detail && <details><summary>{t("sec.details")}</summary><code>{res.detail}</code></details>}</Note>}
    </article>
  );
}

export function Security() {
  const { t } = useI18n();
  const { data, error, isLoading } = useSWR<Posture>("/security/posture", fetcher);
  const back = <BackLink href="#/more">{t("tab.more")}</BackLink>;
  if (isLoading) return <>{back}<Skeleton h={260} /></>;
  if (error || !data) return <>{back}<Note kind="bad">{t("c.loadFailed")}</Note></>;
  const devices = asArray<Dev>(data.devices).slice().sort((a, b) => rank(a) - rank(b));
  const stat = (k: string, v?: number) => <div className="capsule" key={k}><span className="soft small">{t("sec.s." + k)}</span><b className="num">{v ?? 0}</b></div>;
  return (
    <>
      {back}<h1>{t("sec.title")}</h1><p className="soft">{t("sec.lead")}</p>
      {!devices.length ? <Note>{t("sec.none")}</Note> : (
        <>
          <div className="capsules wrap">{stat("affected", data.affected)}{stat("safe", data.safe)}{stat("unknown", data.unknown)}{stat("unpatchable", data.unpatchable)}{stat("exploited", data.exploited)}</div>
          {(data.unpatchable ?? 0) > 0 && <Note kind="warn">{data.unpatchable} / {data.count} {t("sec.unpatchableNote")}</Note>}
          {(data.exploited ?? 0) > 0 && <Note kind="bad">{data.exploited} {t("sec.exploitedNote")}</Note>}
          <div className="stack">{devices.map((d) => <Row key={d.device_id} d={d} controls={!!data.controls_enabled} />)}</div>
        </>
      )}
    </>
  );
}
