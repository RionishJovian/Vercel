import { useState } from "react";
import useSWR from "swr";
import { api, ApiError, fetcher, isUnauthorized } from "../lib/api";
import { asArray, type Me } from "../lib/types";
import { useI18n } from "../i18n";
import { BackLink, Note } from "../components/ui";
import { Skeleton } from "../components/Skeleton";

interface Incident { id: string; severity?: string; kind?: string; subject_label?: string; subject_id?: string; detail?: string; opened_at?: string; suggested_action?: { kind?: string } }
const FIX = new Set(["reboot", "restore_backup", "reprovision_peer"]);
const HIGH_RISK = new Set(["reboot", "restore_backup"]);
const ADMIN = ["ADMIN", "OWNER", "DEVELOPER"];
export const tone = (s?: string) => ({ critical: "bad", warning: "warn", warn: "warn" } as Record<string, string>)[String(s).toLowerCase()] ?? "";

export function FixButton({ inc, canFix, onDone }: { inc: Incident; canFix: boolean; onDone: () => void }) {
  const { t } = useI18n();
  const kind = inc.suggested_action?.kind ?? "";
  const [s, setS] = useState<"idle" | "busy" | "queued" | "offline" | "manual" | "retry">("idle");
  if (FIX.has(kind)) {
    if (!canFix) return <span className="soft small">{t("inc.adminNeeded")}</span>;
    const go = async () => {
      if (HIGH_RISK.has(kind) && !window.confirm(`${t("inc.confirmFix")} ${kind}`)) return;
      setS("busy");
      try { await api.post(`/incidents/${encodeURIComponent(inc.id)}/action`, {}); setS("queued"); onDone(); }
      catch (e) { if (isUnauthorized(e)) throw e; setS(e instanceof ApiError && e.status === 503 ? "offline" : e instanceof ApiError && e.status === 400 ? "manual" : "retry"); }
    };
    return <button className="pill warn" type="button" disabled={s === "busy" || s === "queued" || s === "offline" || s === "manual"} onClick={() => void go()}>{t("inc.fix." + s)}</button>;
  }
  return kind === "review" || kind === "none" ? <span className="soft small">{t("inc.manual")}</span> : null;
}

export function Incidents({ me }: { me: Me }) {
  const { t } = useI18n();
  const canManage = ADMIN.includes(me.user?.user_role ?? "");
  const { data, error, isLoading, mutate } = useSWR<{ incidents?: Incident[] }>("/incidents", fetcher);
  const [busy, setBusy] = useState(""), [err, setErr] = useState("");
  const back = <BackLink href="#/more">{t("tab.more")}</BackLink>;
  if (isLoading) return <>{back}<Skeleton h={260} /></>;
  if (error) return <>{back}<Note kind="bad">{t("c.loadFailed")}</Note></>;
  const list = asArray<Incident>(data?.incidents);
  async function act(id: string, kind: "acknowledge" | "resolve") {
    setBusy(id + kind); setErr("");
    try { await api.post(`/incidents/${encodeURIComponent(id)}/${kind}`); await mutate(); }
    catch (e) { if (isUnauthorized(e)) throw e; setErr(e instanceof Error ? e.message : t("c.loadFailed")); }
    setBusy("");
  }
  return (
    <>
      {back}<h1>{t("inc.title")}</h1><p className="soft">{t("inc.lead")}</p>
      {err && <Note kind="bad">{err}</Note>}
      {!list.length ? <Note kind="ok">{t("inc.none")}</Note> : (
        <div className="stack">
          {list.map((i) => (
            <article className="glass pad stack" key={i.id}>
              <div className="row"><span className={"chip " + tone(i.severity)}>{i.severity || "info"}</span><span className="soft small">{i.opened_at ? new Date(i.opened_at).toLocaleString() : ""}</span></div>
              <b>{i.subject_label || i.subject_id || "—"}</b>
              <p className="soft small num">{i.kind}</p>
              {i.detail && <p className="small">{i.detail.replace(/\s+/g, " ").slice(0, 160)}</p>}
              {canManage ? (
                <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
                  <button className="pill" type="button" disabled={!!busy} onClick={() => void act(i.id, "acknowledge")}>{t("inc.ack")}</button>
                  <button className="pill" type="button" disabled={!!busy} onClick={() => void act(i.id, "resolve")}>{t("inc.resolve")}</button>
                  <FixButton inc={i} canFix={canManage} onDone={() => void mutate()} />
                </div>
              ) : <span className="soft small">{t("inc.readOnly")}</span>}
            </article>
          ))}
        </div>
      )}
    </>
  );
}
