import { useEffect, useState } from "react";
import useSWR from "swr";
import { fetcher } from "../../lib/api";
import { asArray, type Me } from "../../lib/types";
import { useI18n } from "../../i18n";
import { BackLink, Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { NetworkEditor } from "./NetworkEditor";
import { workspaceInfo, workspaces, type Workspace } from "./schema";
import type { RouterRow } from "../routers";

const EDITORS = ["OWNER", "ADMIN", "DEVELOPER"];
const KEY = "mm_selected_router";
const read = () => { try { return sessionStorage.getItem(KEY) ?? ""; } catch { return ""; } };

/** Internet setup: one router is chosen once (or auto-picked when there is one) and remembered across these pages. */
export function Network({ me, sub }: { me: Me; sub: string }) {
  const { t } = useI18n();
  const canEdit = EDITORS.includes(me.user?.user_role ?? "");
  const { data, error, isLoading } = useSWR<{ routers?: RouterRow[] }>("/routers", fetcher);
  const gateways = asArray<RouterRow>(data?.routers).filter((r) => r.deployment_layer === "MAIN_GATEWAY");
  const [id, setId] = useState(read());
  useEffect(() => { if (gateways.length && !gateways.some((g) => g.device_id === id)) setId(gateways.length === 1 ? gateways[0].device_id : ""); }, [gateways, id]);
  const pickRouter = (v: string) => { setId(v); try { if (v) sessionStorage.setItem(KEY, v); } catch { /* ignore */ } };
  const back = <BackLink href="#/more">{t("tab.more")}</BackLink>;
  if (isLoading) return <>{back}<Skeleton h={300} /></>;
  if (error) return <>{back}<Note kind="bad">{t("c.loadFailed")}</Note></>;

  const ws = (workspaces as readonly string[]).includes(sub) ? (sub as Workspace) : null;
  const router = gateways.find((g) => g.device_id === id);
  return (
    <>
      {back}<h1>{ws ? workspaceInfo[ws][0] : t("net.title")}</h1>
      {!ws && <p className="soft">{t("net.lead")}</p>}
      {gateways.length > 1 && <label className="field"><span>{t("net.working")}</span><select className="input" value={id} onChange={(e) => pickRouter(e.target.value)}><option value="">{t("net.chooseRouter")}</option>{gateways.map((g) => <option key={g.device_id} value={g.device_id}>{g.hardware_name}</option>)}</select></label>}
      {gateways.length === 1 && <p className="soft small">{gateways[0].hardware_name}</p>}
      {!ws ? (
        <div className="stack">{workspaces.map((k) => <a key={k} className="rowitem menu-link" href={"#/network/" + k}><span className="stack" style={{ gap: 2 }}><b>{workspaceInfo[k][0]}</b><span className="soft small">{workspaceInfo[k][1]}</span></span><span aria-hidden="true" className="soft">›</span></a>)}</div>
      ) : (
        <>
          <div className="scroll-x">{workspaces.map((k) => <a key={k} className="pill" href={"#/network/" + k} aria-current={k === ws ? "page" : undefined}>{workspaceInfo[k][0]}</a>)}</div>
          {!gateways.length ? <Note>{t("net.noRouter")} <a className="link" href="#/hub">{t("hub.title")}</a></Note>
            : !router ? <Note>{t("net.pickOnce")}</Note>
            : <NetworkEditor key={router.device_id + ws} workspace={ws} router={router} canEdit={canEdit} />}
        </>
      )}
    </>
  );
}
