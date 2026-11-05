import { useEffect, useState } from "react";
import useSWR from "swr";
import { fetcher } from "../../lib/api";
import { asArray, type Me } from "../../lib/types";
import { useI18n } from "../../i18n";
import { BackLink, Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { NetworkEditor } from "./NetworkEditor";
import { Wizard } from "./Wizard";
import { workspaceInfo, workspaces, type Workspace } from "./schema";
import type { RouterRow } from "../routers";
import { Broadband } from "./Broadband";

const EDITORS = ["OWNER", "ADMIN", "DEVELOPER"];
const KEY = "mm_selected_router";
const read = () => { try { return sessionStorage.getItem(KEY) ?? ""; } catch { return ""; } };

/** Internet setup: one router is chosen once (or auto-picked when there is one) and remembered across these pages. */
export function Network({ me, sub, params }: { me: Me; sub: string; params?: URLSearchParams }) {
  const { t, tn } = useI18n();
  const canEdit = EDITORS.includes(me.user?.user_role ?? "");
  const { data, error, isLoading } = useSWR<{ routers?: RouterRow[] }>("/routers", fetcher);
  const gateways = asArray<RouterRow>(data?.routers).filter((r) => r.deployment_layer === "MAIN_GATEWAY");
  const [id, setId] = useState(read());
  const [broadbandBusy, setBroadbandBusy] = useState(false);
  useEffect(() => { if (gateways.length && !gateways.some((g) => g.device_id === id)) setId(gateways.length === 1 ? gateways[0].device_id : ""); }, [gateways, id]);
  const pickRouter = (v: string) => { setId(v); try { if (v) sessionStorage.setItem(KEY, v); } catch { /* ignore */ } };
  const back = <BackLink href="#/router">{t("title.router")}</BackLink>;
  if (isLoading) return <>{back}<Skeleton h={300} /></>;

  const isNew = sub === "new";
  const ws = !sub ? "wan" : (workspaces as readonly string[]).includes(sub) ? (sub as Workspace) : null;
  const router = gateways.find((g) => g.device_id === id);
  // Why the forms are locked, when they are. They are always shown either way.
  const reason = error ? t("c.loadFailed") : !gateways.length ? t("net.noRouterBanner") : !router ? t("net.pickOnce") : undefined;
  return (
    <>
      {back}<h1>{isNew ? t("nw.title") : sub && ws ? tn(workspaceInfo[ws][0]) : t("net.title")}</h1>
      {!sub && <p className="soft">{t("net.lead")}</p>}
      {gateways.length > 1 && <label className="field"><span>{t("net.working")}</span><select className="input" value={id} disabled={broadbandBusy} onChange={(e) => pickRouter(e.target.value)}><option value="">{t("net.chooseRouter")}</option>{gateways.map((g) => <option key={g.device_id} value={g.device_id}>{g.hardware_name}</option>)}</select></label>}
      {gateways.length === 1 && <p className="soft small">{gateways[0].hardware_name}</p>}
      {isNew ? (
        <Wizard key={router?.device_id ?? "none"} router={router} canEdit={canEdit} reason={reason} />
      ) : !ws ? (
        <div className="stack">{reason && <Note kind="bad">{reason}</Note>}<a className="rowitem menu-link" href="#/network/new"><span className="stack" style={{ gap: 2 }}><b>{t("nw.title")}</b><span className="soft small">{t("nw.menuHelp")}</span></span><span aria-hidden="true" className="soft">›</span></a>{workspaces.map((k) => <a key={k} className="rowitem menu-link" href={"#/network/" + k}><span className="stack" style={{ gap: 2 }}><b>{workspaceInfo[k][0]}</b><span className="soft small">{workspaceInfo[k][1]}</span></span><span aria-hidden="true" className="soft">›</span></a>)}</div>
      ) : (
        <>
          <div className="scroll-x"><a className="pill" href="#/network/new">{t("nw.title")}</a>{workspaces.map((k) => <a key={k} className="pill" href={"#/network/" + k} aria-current={k === ws ? "page" : undefined}>{tn(workspaceInfo[k][0])}</a>)}</div>
          <NetworkEditor key={(router?.device_id ?? "none") + ws} workspace={ws} router={router} canEdit={canEdit} reason={reason} prefill={params} />
          {ws === "pppoe" && router && <Broadband key={router.device_id} routerId={router.device_id} routerName={router.hardware_name ?? router.device_id} canEdit={canEdit} onBusyChange={setBroadbandBusy} />}
        </>
      )}
    </>
  );
}
