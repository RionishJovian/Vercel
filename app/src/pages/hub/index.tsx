import { Icon } from "../../components/icons";
import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, ApiError, fetcher, isUnauthorized } from "../../lib/api";
import { useFleet } from "../../lib/useFleet";
import { asArray, type Me } from "../../lib/types";
import { useI18n } from "../../i18n";
import { BackLink, Field, Note, PageHead } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { Enrollment, Script } from "./Enrollment";
import { Maintenance } from "./Maintenance";
import { WebFig } from "./WebFig";
import { DeviceForm, LAYERS } from "../devices/DeviceForm";
import { enrollmentUnavailableKey } from "./messages";
import { ConnectionCheck } from "./ConnectionCheck";
import type { RouterRow } from "../routers";
import { errorText } from "../../lib/errors";
import { ageShort } from "../../lib/age";

interface GW extends RouterRow { site_id?: string | null; management_ip?: string; vendor?: string; device_model?: string; management_connected?: boolean }
interface Capability { available?: boolean; lookup_enabled?: boolean; code?: string }
const EDITORS = ["OWNER", "ADMIN", "DEVELOPER"];

// Platform Admin only. Erases the history that normally keeps a record, so the exact name must
// be typed. The server checks the role and the name again; this form only asks.
function ForceDelete({ r, onDone }: { r: GW; onDone: (res: { router_cleanup_script?: string } | null) => void }) {
  const { t } = useI18n();
  const word = (r.hardware_name ?? "").trim() || r.device_id;
  const [typed, setTyped] = useState(""), [busy, setBusy] = useState(false), [err, setErr] = useState("");
  async function go() {
    setBusy(true); setErr("");
    try {
      const res = await api.del<{ router_cleanup_script?: string; forced?: boolean }>(`/routers/${encodeURIComponent(r.device_id)}?force=true&confirm=${encodeURIComponent(word)}`, AbortSignal.timeout(60_000));
      // A server that has not been updated yet ignores force=true and answers "kept" with a 200.
      // That is not a deletion, so never drop the card on it.
      if (res?.forced !== true) { setBusy(false); setErr(t("hub.forceNotApplied")); return; }
      onDone(res);
    } catch (e) {
      setBusy(false);
      setErr(isUnauthorized(e) ? t("hub.delSignedOut") : errorText(e, t("hub.delFail")));
    }
  }
  return (
    <div className="note bad stack" role="group" aria-label={t("hub.forceTitle")}>
      <b>{t("hub.forceTitle")}</b><p className="small">{t("hub.forceBody")}</p>
      <Field label={`${t("hub.forceType")} ${word}`} htmlFor={"force-" + r.device_id}><input className="input" id={"force-" + r.device_id} value={typed} autoComplete="off" spellCheck={false} onChange={(e) => setTyped(e.target.value)} /></Field>
      {err && <Note kind="bad">{err}</Note>}
      <div className="row" style={{ justifyContent: "flex-start" }}><button className="pill danger" type="button" disabled={busy || typed !== word} aria-busy={busy} onClick={() => void go()}>{busy ? t("hub.deleting") : t("hub.forceDelete")}</button></div>
    </div>
  );
}

function Card({ r, state, canEdit, platform, cap, openId, setOpen, onChanged, onDeleted }: { r: GW; state: string; canEdit: boolean; platform: boolean; cap: Capability; openId: string; setOpen: (id: string) => void; onChanged: () => void; onDeleted: (id: string) => void }) {
  const { t } = useI18n();
  const [del, setDel] = useState<{ confirm: boolean; busy: boolean; err?: string; kept?: boolean; conflict?: boolean }>({ confirm: false, busy: false });
  const [editing, setEditing] = useState(false), [maint, setMaint] = useState(false);
  const [cleanup, setCleanup] = useState(""), [kept, setKept] = useState(false), [partial, setPartial] = useState("");
  async function remove() {
    setDel({ confirm: true, busy: true });
    try {
      // A Hub that never answers must not leave the button spinning for good.
      const res = await api.del<{ router_cleanup_script?: string; record_kept?: boolean }>(`/routers/${encodeURIComponent(r.device_id)}`, AbortSignal.timeout(60_000));
      setDel({ confirm: false, busy: false });
      if (res?.record_kept) setKept(true);
      // The cleanup script lives in this card, so the card stays until Done is pressed.
      if (res?.router_cleanup_script) setCleanup(res.router_cleanup_script);
      else if (res?.record_kept) {
        // Nothing to paste, but the press must still answer: the record stays because of its
        // history. A second press on an already ended router changes nothing else on the card.
        setDel({ confirm: true, busy: false, kept: true });
        onChanged();
      }
      else onDeleted(r.device_id);
    }
    catch (e) {
      // A throw here would vanish: this runs from a click handler, so nothing would show.
      if (isUnauthorized(e)) { setDel({ confirm: true, busy: false, err: t("hub.delSignedOut") }); return; }
      // Keep the confirmation open so the reason shows next to the button that was pressed.
      // The connection ended but the record did not change. This is the only copy of the script.
      const script = e instanceof ApiError ? (e.data as { router_cleanup_script?: string } | null)?.router_cleanup_script : undefined;
      setDel({ confirm: !script, busy: false, err: errorText(e, t("hub.delFail")), conflict: e instanceof ApiError && e.status === 409 });
      if (script) { setPartial(script); onChanged(); }
    }
  }
  function forced(res: { router_cleanup_script?: string } | null) {
    setDel({ confirm: false, busy: false });
    setKept(false);
    if (res?.router_cleanup_script) setCleanup(res.router_cleanup_script); else onDeleted(r.device_id);
  }
  const tone = state === "up" ? "ok" : state === "stale" ? "warn" : state === "offline" ? "bad" : "";
  if (partial) return (
    <article className="glass pad stack gw-card">
      <h3>{t("hub.partialTitle")}</h3>
      <p className="soft small">{t("hub.partialBody")}</p>
      <Script title={t("hub.setupTitle")} text={partial} />
      <button className="btn" type="button" onClick={() => setPartial("")}>{t("hub.delDone")}</button>
    </article>
  );
  if (cleanup) return (
    <article className="glass pad stack gw-card">
      <h3>{t(kept ? "hub.keptTitle" : "hub.delDoneTitle")}</h3>
      <p className="soft small">{t(kept ? "hub.keptBody" : "hub.delDoneBody")}</p>
      <Script title={t("hub.setupTitle")} text={cleanup} />
      <button className="btn" type="button" onClick={() => { setCleanup(""); if (kept) onChanged(); else onDeleted(r.device_id); }}>{t("hub.delDone")}</button>
    </article>
  );
  return (
    <article className="glass pad stack gw-card" data-tone={tone || undefined}>
      <div className="row"><div className="who"><span className={"medal sm" + (state === "up" ? " on" : "")}><Icon name="router" /></span><div className="stack" style={{ gap: 2 }}><h2>{r.hardware_name || t("hub.unnamed")}</h2><span className="rc-model">{[r.vendor, r.device_model].filter(Boolean).join(" ")}</span></div></div><span className={"chip " + tone}>{t("fl.state." + (["up", "stale", "offline"].includes(state) ? state : "unknown"))}</span></div>
      <dl className="spec">
        <dt>{t("hub.transport")}</dt><dd><span className="mono">{r.transport || "direct"}</span></dd>
        <dt>{t("hub.mgmtIp")}</dt><dd>{r.management_ip ? <span className="num">{r.management_ip}</span> : <span className="nil">{t("hub.notRegistered")}</span>}</dd>
        <dt>{t("hub.lastSignal")}</dt><dd>{r.last_seen_at ? <><span className="num">{ageShort(r.last_seen_at) ?? "—"}</span><span className="soft small" title={new Date(r.last_seen_at).toLocaleString()}>{new Date(r.last_seen_at).toLocaleString()}</span></> : <span className="nil">{t("hub.noSignal")}</span>}</dd>
      </dl>
      {canEdit && cap.lookup_enabled && <ConnectionCheck id={r.device_id} />}
      {kept && !del.kept && <Note kind="ok">{t("hub.keptNote")}</Note>}
      {r.management_connected ? <Note kind="ok">{t("hub.connected")}</Note> : canEdit && (cap.available ? (
        <>
          <button className="btn" type="button" aria-expanded={openId === r.device_id} onClick={() => setOpen(openId === r.device_id ? "" : r.device_id)}>{t("hub.connect")}</button>
          {openId === r.device_id && <Enrollment id={r.device_id} onVerified={onChanged} />}
        </>
      ) : <Note>{t(enrollmentUnavailableKey(cap.code))}</Note>)}
      <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
        <a className="pill" href={"/app/router/" + encodeURIComponent(r.device_id)}>{t("hub.health")}</a>
        <button className="pill" type="button" aria-expanded={maint} onClick={() => setMaint((v) => !v)}>{t("mt.title")}</button>
        {canEdit && !editing && <button className="pill" type="button" onClick={() => setEditing(true)}>{t("dm.edit")}</button>}
        {canEdit && !del.confirm && <button className="pill danger" type="button" onClick={() => setDel({ confirm: true, busy: false })}>{t("hub.delete")}</button>}
      </div>
      {canEdit && r.management_connected && <WebFig id={r.device_id} />}
      {maint && <Maintenance id={r.device_id} name={r.hardware_name ?? ""} canEdit={canEdit} />}
      {editing && <DeviceForm rec={r} layers={LAYERS} onDone={() => { setEditing(false); onChanged(); }} onCancel={() => setEditing(false)} />}
      {del.confirm && del.kept && (
        <div className="note ok stack" role="status" aria-label={t("hub.keptNowTitle")}>
          <b>{t("hub.keptNowTitle")}</b><p className="small">{t("hub.keptNote")}</p>
          <div className="row" style={{ justifyContent: "flex-start" }}><button className="pill" type="button" onClick={() => setDel({ confirm: false, busy: false })}>{t("hub.delDone")}</button></div>
          {platform && <ForceDelete r={r} onDone={forced} />}
        </div>
      )}
      {del.confirm && !del.kept && (
        <div className="note warn stack" role="alertdialog" aria-label={t("hub.delTitle")}>
          <b>{t("hub.delTitle")}</b><p className="small"><b>{r.hardware_name || t("hub.unnamed")}</b></p><p className="small">{t("hub.delBody")}</p><p className="small">{t("hub.delNoReset")}</p><p className="small"><b>{t("hub.delPermanent")}</b></p>
          {del.err && <Note kind="bad">{del.err}</Note>}
          <div className="row" style={{ justifyContent: "flex-start" }}>
            <button className="pill" type="button" disabled={del.busy} onClick={() => setDel({ confirm: false, busy: false })}>{t("c.cancel")}</button>
            <button className="pill danger" type="button" disabled={del.busy} aria-busy={del.busy} onClick={() => void remove()}>{del.busy ? t("hub.deleting") : t("hub.delete")}</button>
          </div>
          {platform && del.conflict && <ForceDelete r={r} onDone={forced} />}
        </div>
      )}
      {!del.confirm && del.err && <Note kind="bad">{del.err}</Note>}
    </article>
  );
}

function AddGateway({ sites, onDone }: { sites: Array<{ site_id: string; name: string }>; onDone: (id: string) => void }) {
  const { t } = useI18n();
  const [msg, setMsg] = useState(""), [busy, setBusy] = useState(false);
  async function add(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    // React clears currentTarget once the handler yields, so keep the form for after the awaits.
    const form = e.currentTarget, f = new FormData(form), name = String(f.get("name")).trim(), site = String(f.get("site"));
    if (!name) { setMsg(t("hub.nameNeeded")); return; }
    if (!sites.some((s) => s.site_id === site)) { setMsg(t("hub.siteNeeded")); return; }
    setBusy(true); setMsg("");
    try {
      const res = await api.post<{ router?: { device_id?: string } }>("/routers", { hardware_name: name, device_model: null, deployment_layer: "MAIN_GATEWAY", management_ip: null, mac_address: null, vendor: "mikrotik", category: "gateway", transport: "tunnel" });
      const id = res.router?.device_id;
      if (id) { try { await api.put(`/vps/peers/${encodeURIComponent(id)}/site`, { site_id: site }); } catch (err) { if (isUnauthorized(err)) throw err; setMsg(errorText(err, t("hub.siteFail"))); } }
      form.reset(); onDone(id ?? "");
    } catch (err) { if (isUnauthorized(err)) throw err; setMsg(err instanceof ApiError && err.status === 403 ? t("hub.forbidden") : errorText(err, t("hub.addFail"))); }
    setBusy(false);
  }
  return (
    <form className="glass pad stack" onSubmit={add}>
      <h2>{t("hub.add")}</h2>
      {!sites.length ? <Note kind="warn">{t("hub.needSite")} <a className="link" href="/app/sites">{t("title.sites")}</a></Note> : (
        <>
          <Field label={t("hub.rbName")} htmlFor="gw-name"><input className="input" id="gw-name" name="name" maxLength={120} required /></Field>
          <Field label={t("hub.site")} htmlFor="gw-site"><select className="input" id="gw-site" name="site" required defaultValue=""><option value="" disabled>{t("hub.chooseSite")}</option>{sites.map((s) => <option key={s.site_id} value={s.site_id}>{s.name}</option>)}</select></Field>
          <p className="soft small">{t("hub.autoAddress")}</p>
          {msg && <Note kind="bad">{msg}</Note>}
          <button className="btn" type="submit" disabled={busy}>{t("hub.create")}</button>
        </>
      )}
    </form>
  );
}

export function Hub({ me }: { me: Me }) {
  const { t } = useI18n();
  const canEdit = EDITORS.includes(me.user?.user_role ?? ""), platform = me.user?.user_role === "DEVELOPER";
  const routers = useSWR<{ routers?: GW[] }>("/routers", fetcher);
  const fleet = useFleet({ shouldRetryOnError: false });
  const sitesRes = useSWR<{ sites?: Array<{ site_id: string; name: string }> }>("/sites", fetcher, { shouldRetryOnError: false });
  const cap = useSWR<Capability>(canEdit ? "/vps/enrollment-capability" : null, fetcher, { shouldRetryOnError: false });
  const [openId, setOpen] = useState("");
  const back = <BackLink href="/app/more">{t("tab.more")}</BackLink>;
  if (routers.isLoading) return <>{back}<Skeleton h={300} /></>;
  if (routers.error) return <>{back}<Note kind="bad">{t("c.loadFailed")}</Note></>;
  const gateways = asArray<GW>(routers.data?.routers).filter((r) => r.deployment_layer === "MAIN_GATEWAY");
  const states = new Map(asArray<{ device_id?: string; state?: string }>(fleet.data?.peers).map((p) => [p.device_id, String(p.state ?? "unknown").toLowerCase()]));
  const sites = asArray<{ site_id: string; name: string }>(sitesRes.data?.sites);
  const capability: Capability = cap.data ?? { available: false, lookup_enabled: false, code: "capability_unavailable" };
  const refresh = () => { void routers.mutate(); void fleet.mutate(); };
  // Drop the deleted card at once, then confirm against the server.
  const removed = (id: string) => { void routers.mutate((cur) => cur && { ...cur, routers: asArray<GW>(cur.routers).filter((x) => x.device_id !== id) }, { revalidate: true }); void fleet.mutate(); };
  return (
    <>
      {back}<PageHead title={t("hub.title")} help={t("hub.lead")} />
      {!gateways.length && <Note>{t("hub.none")}</Note>}
      <div className="router-grid workspace-main">{gateways.map((r) => <Card key={r.device_id} r={r} state={states.get(r.device_id) ?? "unknown"} canEdit={canEdit} platform={platform} cap={capability} openId={openId} setOpen={setOpen} onChanged={refresh} onDeleted={removed} />)}</div>
      {canEdit && <div className="workspace-rail"><AddGateway sites={sites} onDone={(id) => { refresh(); if (id) setOpen(id); }} /></div>}
    </>
  );
}
