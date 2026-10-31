import { useState } from "react";
import { api, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";

interface Menu { supported?: boolean; configured?: boolean; count?: number }
interface Caps { device_id?: string; detected?: boolean; board_name?: string; routeros_version?: string; dhcp_server?: Menu; hotspot?: Menu; radius?: Menu; modern_wifi?: { count?: number }; legacy_wireless?: { count?: number }; modern_capsman?: { supported?: boolean }; legacy_capsman?: { supported?: boolean }; webfig?: { https_enabled?: boolean; https_port?: number; trusted_only?: boolean }; error?: string }
const TIMEOUT = 30000;

/** What the router reports it can do. Read on request because it probes the live router. */
export function Capabilities({ id }: { id: string }) {
  const { t } = useI18n();
  const [s, setS] = useState<{ k: "idle" | "loading" } | { k: "error"; msg: string } | { k: "ready"; caps: Caps }>({ k: "idle" });
  async function read() {
    setS({ k: "loading" });
    try {
      const r = await Promise.race([api.get<{ capabilities?: Caps } & Caps>(`/routers/${encodeURIComponent(id)}/capabilities`), new Promise<never>((_, rej) => setTimeout(() => rej(new Error("timeout")), TIMEOUT))]);
      const caps = r.capabilities ?? r;
      if (caps.device_id !== id || caps.detected !== true) throw new Error(t("cap.unverified"));
      setS({ k: "ready", caps });
    } catch (e) {
      if (isUnauthorized(e)) throw e;
      const m = (e as { data?: { error?: string } } | null)?.data?.error || (e instanceof Error ? e.message : "");
      setS({ k: "error", msg: m === "timeout" ? t("cap.timeout") : m || t("cap.failed") });
    }
  }
  const menu = (m?: Menu) => (!m || !m.supported ? t("cap.notDetected") : m.configured ? `${m.count} ${t("cap.configured")}` : t("cap.availableNot"));
  const c = s.k === "ready" ? s.caps : null;
  const radios = c ? Number(c.modern_wifi?.count ?? 0) + Number(c.legacy_wireless?.count ?? 0) : 0;
  const capsman = !!(c && (c.modern_capsman?.supported || c.legacy_capsman?.supported));
  return (
    <section className="glass pad stack">
      <h2>{t("cap.title")}</h2><p className="soft small">{t("cap.lead")}</p>
      <button className="pill" type="button" disabled={s.k === "loading"} onClick={() => void read()}>{s.k === "loading" ? t("cap.reading") : t("cap.read")}</button>
      {s.k === "error" && <Note kind="warn">{s.msg}</Note>}
      {c && (
        <dl className="facts">
          <dt>{t("r.board")}</dt><dd>{c.board_name || t("cap.reported")}</dd><dt>RouterOS</dt><dd>{c.routeros_version || t("c.unknown")}</dd>
          <dt>{t("cap.dhcp")}</dt><dd>{menu(c.dhcp_server)}</dd><dt>Hotspot</dt><dd>{menu(c.hotspot)}</dd><dt>RADIUS</dt><dd>{menu(c.radius)}</dd>
          <dt>{t("cap.radios")}</dt><dd>{radios || t("cap.none")}</dd><dt>CAPsMAN</dt><dd>{capsman ? t("cap.capsmanYes") : t("cap.notDetected")}</dd>
          <dt>{t("cap.webfig")}</dt><dd>{c.webfig?.https_enabled ? t("cap.enabled") : t("cap.notEnabled")}{c.webfig?.https_port ? ` · ${c.webfig.https_port}` : ""}</dd>
          <dt>{t("cap.trusted")}</dt><dd>{c.webfig?.trusted_only ? t("cap.yes") : t("cap.attention")}</dd>
        </dl>
      )}
      {c && <p className="soft small">{radios ? t("cap.localRadios") : t("cap.externalAps")}</p>}
    </section>
  );
}
