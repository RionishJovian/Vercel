import useSWR from "swr";
import { fetcher } from "../../lib/api";
import { keepPolling } from "../../lib/polling";
import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import type { RouterRow } from ".";
import { Capabilities } from "./Capabilities";
import { CAPsMANInventoryPanel } from "./CAPsMANInventoryPanel";
import { FirmwareUpgrade } from "./FirmwareUpgrade";
import { firmwareText, healthLabel, healthValue, softwareText, type Firmware, type Health } from "./portInfo";

interface Telemetry { state?: string; snapshot?: { read_at?: string; resources?: Record<string, unknown> & { board_name?: string; routeros_version?: string; cpu_percent?: number; memory_used_bytes?: number | string; memory_total_bytes?: number | string; uptime_seconds?: number }; hotspot_sessions?: number | null; dhcp_bound_leases?: number | null; issues?: string[]; health?: Health[] | null; firmware?: Firmware | null } }

const pct = (v: unknown) => (typeof v === "number" ? Math.round(v) + "%" : "—");
const mb = (v: unknown) => (Number.isFinite(Number(v)) ? (Number(v) / 1048576).toFixed(0) + " MB" : "—");
function uptime(sec: unknown) {
  if (typeof sec !== "number") return "—";
  const d = Math.floor(sec / 86400), h = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
  return (d ? d + "d " : "") + h + "h " + m + "m";
}
const Kv = ({ k, v }: { k: string; v: string }) => <div className="row small"><span className="soft">{k}</span><b className="num">{v}</b></div>;

/** Telemetry is read-only; editing is available in existing reviewed, server-gated workflows. */
export function Detail({ id, router, canReview }: { id: string; router?: RouterRow; canReview: boolean }) {
  const { t, lang } = useI18n();
  const { data, error, isLoading, mutate } = useSWR<{ telemetry?: Telemetry }>(`/routers/${encodeURIComponent(id)}/telemetry`, fetcher, keepPolling(15000));
  const tele = data?.telemetry, snap = tele?.snapshot, r = snap?.resources ?? {};
  const words = { updateAvail: t("r.updateAvail"), noNewer: t("r.noNewer"), fwUpgrade: t("r.fwUpgrade"), full: t("r.full"), half: t("r.half") };
  return (
    <>
      <h1>{router?.hardware_name || id}</h1>
      {router?.device_model && <p className="rc-model">{router.device_model}</p>}
      {/* Outside the loading/error branches so a restart (when telemetry briefly fails) does not close it. */}
      <FirmwareUpgrade id={id} fw={snap?.firmware ?? undefined} onDone={() => void mutate()} />
      {isLoading ? <Skeleton h={300} /> : error || !snap ? (
        <><Note kind="warn">{t("r.telFail")}</Note><button className="pill" type="button" onClick={() => void mutate()}>{t("c.retry")}</button></>
      ) : (
        <>
          <section className="glass pad stack">
            <div className="row"><h2>{t("r.health")}</h2><span className={"chip " + (tele?.state === "live" || tele?.state === "syncing" ? "ok" : "warn")}>{tele?.state || "—"}</span></div>
            <div className="kv-list">
            <Kv k={t("r.board")} v={(r.board_name || "—") + (r.routeros_version ? " " + r.routeros_version : "")} />
            <Kv k="CPU" v={pct(r.cpu_percent)} />
            <Kv k={t("r.memory")} v={`${mb(r.memory_used_bytes)} / ${mb(r.memory_total_bytes)}`} />
            <Kv k={t("r.uptime")} v={uptime(r.uptime_seconds)} />
            {asArray<Health>(snap.health).map((h) => <Kv key={h.name} k={healthLabel(h.name)} v={healthValue(h)} />)}
            {snap.firmware && <Kv k={t("r.software")} v={softwareText(r.routeros_version, snap.firmware, words)} />}
            {firmwareText(snap.firmware ?? undefined, words) && <Kv k={t("r.firmware")} v={firmwareText(snap.firmware ?? undefined, words)} />}
            <Kv k={t("r.guests")} v={snap.hotspot_sessions == null ? "—" : String(snap.hotspot_sessions)} />
            <Kv k={t("r.leases")} v={snap.dhcp_bound_leases == null ? "—" : String(snap.dhcp_bound_leases)} />
            </div>
          </section>
          <section className="glass pad stack" aria-label={lang === "my" ? "Router ပြင်ဆင်မှုများ" : "Router configuration"}>
            <h2>{lang === "my" ? "Router ပြင်ဆင်မှုများ" : "Router configuration"}</h2>
            <p className="soft small">{lang === "my"
              ? "ဤစာမျက်နှာရှိ အခြေအနေပြချက်များမှာ ဖတ်ရန်သာဖြစ်သည်။ Router ပြင်ဆင်မှုများကို အောက်ပါ လုပ်ငန်းစဉ်များမှ စစ်ဆေး၊ အတည်ပြုပြီးမှ အသုံးပြုနိုင်သည်။ ဆာဗာက ပိတ်ထားသော လုပ်ဆောင်ချက်များကို ဤစာမျက်နှာက မဖွင့်နိုင်ပါ။"
              : "Health readings are read-only, not the entire router. Use the existing configuration workflows below: they require a reviewed change, owner confirmation and server-side permission. Disabled capabilities remain disabled."}</p>
            <div className="scroll-x" aria-label={lang === "my" ? "ပြင်ဆင်မှု လင့်ခ်များ" : "Configuration workspaces"}>
              <a className="pill" href={"/app/net?router=" + encodeURIComponent(id)}>{t("home.managePorts")}</a>
              <a className="pill" href={"/app/network?router=" + encodeURIComponent(id)}>{t("title.network")}</a>
              <a className="pill" href="/app/topology">{t("tp.title")}</a>
              <a className="pill" href="/app/hub">{t("hub.title")}</a>
            </div>
          </section>
          {asArray<string>(snap.issues).length > 0 && <Note kind="warn">{asArray<string>(snap.issues).join(" · ")}</Note>}
          <Capabilities id={id} />
          <CAPsMANInventoryPanel key={id} id={id} siteId={router?.site_id} canReview={canReview} />
        </>
      )}
    </>
  );
}
