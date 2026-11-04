import { useState } from "react";
import { api, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { Migration } from "./Migration";
import { errorText } from "../../lib/errors";


/** Recovery and maintenance for one gateway. Reboot is queued for approval; restore lives in Backups. */
export function Maintenance({ id, name, canEdit }: { id: string; name: string; canEdit: boolean }) {
  const { t } = useI18n();
  const [confirm, setConfirm] = useState(false), [busy, setBusy] = useState(false), [msg, setMsg] = useState<{ bad: boolean; text: string } | null>(null);
  async function reboot() {
    setBusy(true); setMsg(null);
    try { const r = await api.post<{ status?: string }>(`/topology/${encodeURIComponent(id)}/reboot`, {}); setMsg({ bad: false, text: r?.status || t("mt.queued") }); setConfirm(false); }
    catch (e) { if (isUnauthorized(e)) throw e; setMsg({ bad: true, text: errorText(e, t("mt.failed")) }); }
    setBusy(false);
  }
  return (
    <div className="stack enroll">
      <h3>{t("mt.title")}</h3><p className="soft small">{t("mt.lead")}</p>
      <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}><a className="pill" href="#/backups">{t("bk.title")}</a><a className="pill" href="#/events">{t("ev.title")}</a><a className="pill" href="#/topology">{t("tp.title")}</a></div>
      {canEdit && (confirm ? (
        <div className="note warn stack"><p className="small">{t("mt.rebootConfirm")}</p><div className="row" style={{ justifyContent: "flex-start" }}><button className="pill" type="button" onClick={() => setConfirm(false)}>{t("c.cancel")}</button><button className="pill danger" type="button" disabled={busy} onClick={() => void reboot()}>{t("mt.reboot")}</button></div></div>
      ) : <button className="pill danger" type="button" onClick={() => setConfirm(true)}>{t("mt.reboot")}</button>)}
      {msg && <Note kind={msg.bad ? "bad" : "ok"}>{msg.text}</Note>}
      <Migration id={id} name={name} canEdit={canEdit} />
    </div>
  );
}
