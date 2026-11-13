import { useRef, useState } from "react";
import { api, ApiError, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { lookupError } from "./messages";

interface Peer {
 handshake_status?: string; tunnel_address?: string; last_handshake_at?: string;
 connection?: {management?: string; setup?: string};
}
const setupKeys=new Map<string,string>([["pending","hub.progress.pending"],["expired","hub.progress.expired"],["repair_pending","hub.progress.repairPending"],["incomplete","hub.progress.incomplete"],["not_started","hub.progress.notStarted"],["not_required","hub.progress.notRequired"]]);

export function ConnectionCheck({id}:{id:string}) {
 const {t}=useI18n();
 const [s,setS]=useState<{busy:boolean;peer?:Peer;key?:string}>({busy:false});
 const pending=useRef(false);
 async function check() {
  if(pending.current)return;
  pending.current=true;setS({busy:true});
  try {setS({busy:false,peer:await api.get<Peer>(`/vps/peers/${encodeURIComponent(id)}/live`)});}
  catch(e){if(isUnauthorized(e))throw e;setS({busy:false,key:lookupError(e instanceof ApiError?e.status:undefined,e instanceof ApiError?(e.data as {code?:string}|null)?.code:undefined,e instanceof Error?e.message:"")});}
  finally {pending.current=false;}
 }
 const p=s.peer;
 const management=p?.connection?.management;
 const setup=p?.connection?.setup;
 return <div className="stack">
  <button className="pill" type="button" disabled={s.busy} onClick={()=>void check()}>{s.busy?t("hub.checkingHub"):t("hub.check")}</button>
  {s.key&&<Note kind="warn">{t(s.key)}</Note>}
  {p&&<div className="note" role="status">
   <b>{p.handshake_status==="recent_handshake"?t("hub.hs.recent"):p.handshake_status==="inactive"?t("hub.hs.inactive"):t("hub.hs.none")}</b>
   <dl className="facts"><dt>{t("hub.addr")}</dt><dd>{p.tunnel_address||t("hub.notReported")}</dd><dt>{t("hub.lastHs")}</dt><dd className="tnum">{p.last_handshake_at?new Date(p.last_handshake_at).toLocaleString():t("hub.never")}</dd></dl>
   <dl className="facts"><dt>{t("hub.progress.management")}</dt><dd>{t(management==="saved"?"hub.progress.saved":management==="missing"?"hub.progress.missing":"hub.progress.unknown")}</dd><dt>{t("hub.progress.setup")}</dt><dd>{t(setupKeys.get(setup??"")??"hub.progress.unknown")}</dd></dl>
   <p className="soft small">{t("hub.lookupNote")}</p>
  </div>}
 </div>;
}
