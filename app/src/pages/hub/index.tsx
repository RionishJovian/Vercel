import { useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, ApiError, fetcher, isUnauthorized } from "../../lib/api";
import { asArray, type Me } from "../../lib/types";
import { useI18n } from "../../i18n";
import { BackLink, Field, Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { Enrollment } from "./Enrollment";
import { Maintenance } from "./Maintenance";
import { DeviceForm, LAYERS } from "../devices/DeviceForm";
import { enrollmentUnavailableKey, lookupError } from "./messages";
import type { RouterRow } from "../routers";

interface GW extends RouterRow { site_id?: string | null; management_ip?: string; vendor?: string; device_model?: string }
interface Capability { available?: boolean; lookup_enabled?: boolean; code?: string }
interface Peer { handshake_status?: string; tunnel_address?: string; last_handshake_at?: string }
const EDITORS = ["OWNER", "ADMIN", "DEVELOPER"];
const errText = (e: unknown, fallback: string) => { const d = (e as { data?: { reason?: string; error?: string } } | null)?.data; return d?.reason || d?.error || (e instanceof Error ? e.message : "") || fallback; };

function Lookup({ id }: { id: string }) {
  const { t } = useI18n();
  const [s, setS] = useState<{ busy: boolean; peer?: Peer; key?: string }>({ busy: false });
  async function check() {
    setS({ busy: true });
    try { setS({ busy: false, peer: await api.get<Peer>(`/vps/peers/${encodeURIComponent(id)}/live`) }); }
    catch (e) { if (isUnauthorized(e)) throw e; setS({ busy: false, key: lookupError(e instanceof ApiError ? e.status : undefined, (e as ApiError).data ? ((e as ApiError).data as { code?: string }).code : undefined, e instanceof Error ? e.message : "") }); }
  }
  const p = s.peer;
  return (
    <div className="stack">
      <button className="pill" type="button" disabled={s.busy} onClick={() => void check()}>{s.busy ? t("hub.checkingHub") : t("hub.check")}</button>
      {s.key && <Note kind="warn">{t(s.key)}</Note>}
      {p && (
        <div className="note" role="status">
          <b>{p.handshake_status === "recent_handshake" ? t("hub.hs.recent") : p.handshake_status === "inactive" ? t("hub.hs.inactive") : t("hub.hs.none")}</b>
          <dl className="facts"><dt>{t("hub.addr")}</dt><dd>{p.tunnel_address || t("hub.notReported")}</dd><dt>{t("hub.lastHs")}</dt><dd>{p.last_handshake_at ? new Date(p.last_handshake_at).toLocaleString() : t("hub.never")}</dd></dl>
          <p className="soft small">{t("hub.lookupNote")}</p>
        </div>
      )}
    </div>
  );
}

function Card({ r, state, canEdit, cap, openId, setOpen, onChanged }: { r: GW; state: string; canEdit: boolean; cap: Capability; openId: string; setOpen: (id: string) => void; onChanged: () => void }) {
  const { t } = useI18n();
  const [del, setDel] = useState<{ confirm: boolean; busy: boolean; err?: string }>({ confirm: false, busy: false });
  const [editing, setEditing] = useState(false), [maint, setMaint] = useState(false);
  async function remove() {
    setDel({ confirm: true, busy: true });
    try { await api.del(`/routers/${encodeURIComponent(r.device_id)}`); onChanged(); }
    catch (e) { if (isUnauthorized(e)) throw e; setDel({ confirm: false, busy: false, err: errText(e, t("hub.delFail")) }); }
  }
  const tone = state === "up" ? "ok" : state === "stale" ? "warn" : state === "offline" ? "bad" : "";
  return (
    <article className="glass pad stack gw-card">
      <div className="row"><div className="stack" style={{ gap: 2 }}><h2>{r.hardware_name || t("hub.unnamed")}</h2><span className="soft small">{[r.vendor, r.device_model].filter(Boolean).join(" ")}</span></div><span className={"chip " + tone}>{t("fl.state." + (["up", "stale", "offline"].includes(state) ? state : "unknown"))}</span></div>
      <dl className="facts">
        <dt>{t("hub.transport")}</dt><dd>{r.transport || "direct"}</dd>
        <dt>{t("hub.mgmtIp")}</dt><dd className="num">{r.management_ip || t("hub.notRegistered")}</dd>
        <dt>{t("hub.lastSignal")}</dt><dd>{r.last_seen_at ? new Date(r.last_seen_at).toLocaleString() : t("hub.noSignal")}</dd>
      </dl>
      {canEdit && cap.lookup_enabled && <Lookup id={r.device_id} />}
      {canEdit && (cap.available ? (
        <>
          <button className="btn" type="button" aria-expanded={openId === r.device_id} onClick={() => setOpen(openId === r.device_id ? "" : r.device_id)}>{t("hub.connect")}</button>
          {openId === r.device_id && <Enrollment id={r.device_id} onVerified={onChanged} />}
        </>
      ) : <Note>{t(enrollmentUnavailableKey(cap.code))}</Note>)}
      <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
        <a className="pill" href={"#/router/" + encodeURIComponent(r.device_id)}>{t("hub.health")}</a>
        <button className="pill" type="button" aria-expanded={maint} onClick={() => setMaint((v) => !v)}>{t("mt.title")}</button>
        {canEdit && !editing && <button className="pill" type="button" onClick={() => setEditing(true)}>{t("dm.edit")}</button>}
        {canEdit && !del.confirm && <button className="pill danger" type="button" onClick={() => setDel({ confirm: true, busy: false })}>{t("hub.delete")}</button>}
      </div>
      {maint && <Maintenance id={r.device_id} name={r.hardware_name ?? ""} canEdit={canEdit} />}
      {editing && <DeviceForm rec={r} layers={LAYERS} onDone={() => { setEditing(false); onChanged(); }} onCancel={() => setEditing(false)} />}
      {del.confirm && (
        <div className="note warn stack" role="alertdialog" aria-label={t("hub.delTitle")}>
          <b>{t("hub.delTitle")}</b><p className="small">{t("hub.delBody")}</p><p className="small">{t("hub.delNoReset")}</p><p className="small"><b>{t("hub.delPermanent")}</b></p>
          <div className="row" style={{ justifyContent: "flex-start" }}>
            <button className="pill" type="button" disabled={del.busy} onClick={() => setDel({ confirm: false, busy: false })}>{t("c.cancel")}</button>
            <button className="pill danger" type="button" disabled={del.busy} onClick={() => void remove()}>{t("hub.delete")}</button>
          </div>
        </div>
      )}
      {del.err && <Note kind="bad">{del.err}</Note>}
    </article>
  );
}

function AddGateway({ sites, onDone }: { sites: Array<{ site_id: string; name: string }>; onDone: (id: string) => void }) {
  const { t } = useI18n();
  const [msg, setMsg] = useState(""), [busy, setBusy] = useState(false);
  async function add(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget), name = String(f.get("name")).trim(), site = String(f.get("site"));
    if (!name) { setMsg(t("hub.nameNeeded")); return; }
    if (!sites.some((s) => s.site_id === site)) { setMsg(t("hub.siteNeeded")); return; }
    setBusy(true); setMsg("");
    try {
      const res = await api.post<{ router?: { device_id?: string } }>("/routers", { hardware_name: name, device_model: null, deployment_layer: "MAIN_GATEWAY", management_ip: null, mac_address: null, vendor: "mikrotik", category: "gateway", transport: "tunnel" });
      const id = res.router?.device_id;
      if (id) { try { await api.put(`/vps/peers/${encodeURIComponent(id)}/site`, { site_id: site }); } catch (err) { if (isUnauthorized(err)) throw err; setMsg(errText(err, t("hub.siteFail"))); } }
      e.currentTarget.reset(); onDone(id ?? "");
    } catch (err) { if (isUnauthorized(err)) throw err; setMsg(err instanceof ApiError && err.status === 403 ? t("hub.forbidden") : errText(err, t("hub.addFail"))); }
    setBusy(false);
  }
  return (
    <form className="glass pad stack" onSubmit={add}>
      <h2>{t("hub.add")}</h2>
      {!sites.length ? <Note kind="warn">{t("hub.needSite")} <a className="link" href="#/sites">{t("title.sites")}</a></Note> : (
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
  const canEdit = EDITORS.includes(me.user?.user_role ?? "");
  const routers = useSWR<{ routers?: GW[] }>("/routers", fetcher);
  const fleet = useSWR<{ peers?: Array<{ device_id?: string; state?: string }> }>("/fleet/health", fetcher, { shouldRetryOnError: false });
  const sitesRes = useSWR<{ sites?: Array<{ site_id: string; name: string }> }>("/sites", fetcher, { shouldRetryOnError: false });
  const cap = useSWR<Capability>(canEdit ? "/vps/enrollment-capability" : null, fetcher, { shouldRetryOnError: false });
  const [openId, setOpen] = useState("");
  const back = <BackLink href="#/more">{t("tab.more")}</BackLink>;
  if (routers.isLoading) return <>{back}<Skeleton h={300} /></>;
  if (routers.error) return <>{back}<Note kind="bad">{t("c.loadFailed")}</Note></>;
  const gateways = asArray<GW>(routers.data?.routers).filter((r) => r.deployment_layer === "MAIN_GATEWAY");
  const states = new Map(asArray<{ device_id?: string; state?: string }>(fleet.data?.peers).map((p) => [p.device_id, String(p.state ?? "unknown").toLowerCase()]));
  const sites = asArray<{ site_id: string; name: string }>(sitesRes.data?.sites);
  const capability: Capability = cap.data ?? { available: false, lookup_enabled: false, code: "capability_unavailable" };
  const refresh = () => { void routers.mutate(); void fleet.mutate(); };
  return (
    <>
      {back}<h1>{t("hub.title")}</h1><p className="soft">{t("hub.lead")}</p>
      {!gateways.length && <Note>{t("hub.none")}</Note>}
      <div className="router-grid">{gateways.map((r) => <Card key={r.device_id} r={r} state={states.get(r.device_id) ?? (r.is_online ? "up" : "unknown")} canEdit={canEdit} cap={capability} openId={openId} setOpen={setOpen} onChanged={refresh} />)}</div>
      {canEdit && <AddGateway sites={sites} onDone={(id) => { refresh(); if (id) setOpen(id); }} />}
    </>
  );
}
