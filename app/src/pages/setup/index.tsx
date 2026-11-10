import { useState } from "react";
import useSWR from "swr";
import { fetcher } from "../../lib/api";
import { useFleet } from "../../lib/useFleet";
import { asArray, type Me } from "../../lib/types";
import { useI18n } from "../../i18n";
import { BackLink, Note, PageHead } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { AddGateway } from "../hub";
import { Enrollment } from "../hub/Enrollment";
import { enrollmentUnavailableKey } from "../hub/messages";
import { setupStatus, type GatewayLike } from "./status";

interface GW extends GatewayLike { hardware_name?: string; deployment_layer?: string }
interface Capability { available?: boolean; code?: string }
const EDITORS = ["OWNER", "ADMIN", "DEVELOPER"];
const KEY = "mm_selected_router";
const read = () => { try { return sessionStorage.getItem(KEY) ?? ""; } catch { return ""; } };

/** Quick setup: one guided path from an empty account to a working guest Wi-Fi page. It hosts the existing screens; it adds no router writes of its own. */
export function QuickSetup({ me }: { me: Me }) {
  const { t } = useI18n();
  const canEdit = EDITORS.includes(me.user?.user_role ?? "");
  const sitesRes = useSWR<{ sites?: Array<{ site_id: string; name: string }> }>("/sites", fetcher, { shouldRetryOnError: false });
  const routers = useSWR<{ routers?: GW[] }>("/routers", fetcher);
  const fleet = useFleet({ shouldRetryOnError: false });
  const cap = useSWR<Capability>(canEdit ? "/vps/enrollment-capability" : null, fetcher, { shouldRetryOnError: false });
  const [picked, setPicked] = useState(read());
  const back = <BackLink href="#/router">{t("title.router")}</BackLink>;
  if (routers.isLoading || sitesRes.isLoading) return <>{back}<Skeleton h={300} /></>;
  if (routers.error || sitesRes.error) return <>{back}<Note kind="bad">{t("c.loadFailed")}</Note></>;

  const sites = asArray<{ site_id: string; name: string }>(sitesRes.data?.sites);
  const gateways = asArray<GW>(routers.data?.routers).filter((r) => r.deployment_layer === "MAIN_GATEWAY");
  const states = new Map(asArray<{ device_id?: string; state?: string }>(fleet.data?.peers).map((p) => [p.device_id, String(p.state ?? "unknown").toLowerCase()]));
  const status = setupStatus({ sites: sites.length, gateways, selected: picked, states });
  const router = status.router as GW | undefined;
  const current = status.steps.find((s) => s.state === "current")?.key;
  const refresh = () => { void routers.mutate(); void fleet.mutate(); };
  const pick = (id: string) => { setPicked(id); try { if (id) sessionStorage.setItem(KEY, id); } catch { /* ignore */ } };
  const capability = cap.data ?? { available: false, code: "capability_unavailable" };

  return (
    <>
      {back}<PageHead title={t("qs.title")} help={t("qs.lead")} />
      <ol className="wiz-rail" aria-label={t("qs.progress")}>
        {status.steps.map((s, n) => (
          <li key={s.key} data-state={s.state}>
            <span className="wiz-dot" aria-current={s.state === "current" ? "step" : undefined}>
              <span className="wiz-num" aria-hidden="true">{s.state === "done" ? "✓" : n + 1}</span>
              <span className="wiz-name">{t("qs.s." + s.key)}</span>
            </span>
          </li>
        ))}
      </ol>
      {gateways.length > 1 && (
        <label className="field"><span>{t("net.working")}</span>
          <select className="input" value={router?.device_id ?? ""} onChange={(e) => pick(e.target.value)}>
            <option value="">{t("net.chooseRouter")}</option>
            {gateways.map((g) => <option key={g.device_id} value={g.device_id}>{g.hardware_name || g.device_id}</option>)}
          </select>
        </label>
      )}

      {current === "site" && <Note kind="warn">{t("qs.needSite")} <a className="link" href="#/sites">{t("title.sites")}</a></Note>}

      {current === "connect" && !canEdit && <Note>{t("nw.needAdmin")}</Note>}
      {current === "connect" && canEdit && (!gateways.length ? (
        <AddGateway sites={sites} onDone={(id) => { if (id) pick(id); refresh(); }} />
      ) : !router ? (
        <Note>{t("net.pickOnce")}</Note>
      ) : capability.available ? (
        <Enrollment id={router.device_id} onVerified={refresh} />
      ) : <Note>{t(enrollmentUnavailableKey(capability.code))}</Note>)}

      {!current && router && (
        <section className="glass pad stack" aria-live="polite">
          <Note kind="ok">{t("hub.connected")}</Note>
          <p className="soft small">{status.live ? t("qs.live") : t("qs.notLive")}</p>
          <a className="btn" href="#/network/new">{t("qs.nextHotspot")}</a>
        </section>
      )}
    </>
  );
}
