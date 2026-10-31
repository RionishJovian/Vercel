import { useState } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { asArray, type Me } from "../../lib/types";
import { useI18n } from "../../i18n";
import { BackLink, Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { DeviceForm, LAYERS, type DeviceRec } from "./DeviceForm";

interface Allowance { routers: number | null; controllers: number | null; sites: number | null; used_routers: number; used_controllers: number; used_sites: number; used_access_points: number; included_router_name?: string }
interface Request { id: string; kind: string; quantity?: number; user_email?: string; reason?: string; status: string; decided_at?: string }
const EDITORS = ["OWNER", "ADMIN", "DEVELOPER"];
const DOWNSTREAM = LAYERS.filter((l) => l !== "MAIN_GATEWAY");
const errText = (e: unknown) => (e as { data?: { error?: string } } | null)?.data?.error || (e instanceof Error ? e.message : "") || "failed";

function Row({ label, granted, used }: { label: string; granted: number | null; used: number }) {
  const { t } = useI18n();
  const limited = granted !== null, over = limited && used > granted, at = limited && used === granted;
  return <div className="row small"><b>{label}</b><span className="num soft">{t("dm.granted")} {limited ? granted : "—"} · {t("dm.inUse")} {used} · {t("dm.remaining")} {limited ? Math.max(0, granted - used) : "—"}</span><span className={"chip " + (over ? "bad" : at ? "warn" : "ok")}>{over ? t("dm.over") : at ? t("dm.atLimit") : t("dm.available")}</span></div>;
}

export function ManagedDevices({ me }: { me: Me }) {
  const { t } = useI18n();
  const canEdit = EDITORS.includes(me.user?.user_role ?? ""), canDecide = me.user?.user_role === "DEVELOPER";
  const sub = useSWR<{ mode?: string }>("/subscription", fetcher, { shouldRetryOnError: false });
  const capacity = sub.data?.mode !== "subscription_only";
  const routers = useSWR<{ routers?: DeviceRec[] }>("/routers", fetcher);
  const al = useSWR<{ allowance?: Allowance | null; kinds?: string[] }>(capacity ? "/device-allowances" : null, fetcher, { shouldRetryOnError: false });
  const rq = useSWR<{ requests?: Request[] }>(capacity ? "/device-requests" : null, fetcher, { shouldRetryOnError: false });
  const [form, setForm] = useState<{ rec?: DeviceRec } | null>(null), [confirmId, setConfirmId] = useState(""), [err, setErr] = useState(""), [reqOpen, setReqOpen] = useState(false), [reqMsg, setReqMsg] = useState("");
  const back = <BackLink href="#/more">{t("tab.more")}</BackLink>;
  if (routers.isLoading) return <>{back}<Skeleton h={260} /></>;
  if (routers.error) return <>{back}<Note kind="bad">{t("c.loadFailed")}</Note></>;
  const list = asArray<DeviceRec>(routers.data?.routers).filter((r) => r.deployment_layer !== "MAIN_GATEWAY");
  const a = al.data?.allowance ?? null, requests = asArray<Request>(rq.data?.requests), kinds = al.data?.kinds?.length ? al.data.kinds : ["routers", "controllers", "sites"];
  const exhausted = (layer: string) => {
    if (!capacity || !a) return "";
    const kind = layer === "CONTROLLER" ? "controllers" : layer === "DOWNSTREAM_SWITCH" ? "routers" : null;
    if (!kind) return "";
    const used = kind === "routers" ? a.used_routers : a.used_controllers, granted = kind === "routers" ? a.routers : a.controllers;
    return granted !== null && used >= granted ? `${t("dm.noCapacity")} ${kind}. ${t("dm.requestFirst")}` : "";
  };
  const refresh = () => { setForm(null); void routers.mutate(); void al.mutate(); };
  async function remove(id: string) { try { await api.del(`/routers/${encodeURIComponent(id)}`); setConfirmId(""); refresh(); } catch (e) { if (isUnauthorized(e)) throw e; setErr(errText(e)); setConfirmId(""); } }
  async function submitRequest(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget), quantity = parseInt(String(f.get("qty")), 10);
    if (!Number.isInteger(quantity) || quantity < 1 || quantity > 100) { setReqMsg(t("dm.qtyRange")); return; }
    try { await api.post("/device-requests", { kind: String(f.get("kind")), quantity, reason: String(f.get("reason")).trim() || null }); setReqOpen(false); setReqMsg(""); void rq.mutate(); } catch (er) { if (isUnauthorized(er)) throw er; setReqMsg(errText(er)); }
  }
  async function decide(id: string, status: "approved" | "rejected") { try { await api.post(`/device-requests/${encodeURIComponent(id)}/decide`, { status }); void rq.mutate(); void al.mutate(); } catch (e) { if (isUnauthorized(e)) throw e; setErr(errText(e)); } }

  return (
    <>
      {back}<h1>{t("dm.title")}</h1><p className="soft">{t("dm.lead")}</p>
      {err && <Note kind="bad">{err}</Note>}
      {canEdit && !form && <button className="btn" type="button" onClick={() => setForm({})}>{t("dm.add")}</button>}
      {form && <DeviceForm rec={form.rec} layers={form.rec ? LAYERS : DOWNSTREAM} blocked={form.rec ? undefined : exhausted} onDone={refresh} onCancel={() => setForm(null)} />}
      {!list.length ? <Note>{t("dm.none")}</Note> : (
        <div className="stack">{list.map((r) => (
          <article className="glass pad stack" key={r.device_id}>
            <div className="row"><b>{r.hardware_name || t("hub.unnamed")}</b><span className={"chip " + (r.is_online ? "ok" : "")}>{r.is_online ? t("c.online") : t("dm.unverified")}</span></div>
            <p className="soft small">{t("dm.l." + (r.deployment_layer ?? "DOWNSTREAM_SWITCH"))} · {[r.vendor, r.device_model].filter(Boolean).join(" ")} · {r.transport || "direct"}</p>
            <p className="small num">{r.management_ip || t("hub.notRegistered")}</p>
            {canEdit && (confirmId === r.device_id ? (
              <div className="note warn stack"><p className="small">{t("hub.delBody")}</p><p className="small"><b>{t("hub.delPermanent")}</b></p><div className="row" style={{ justifyContent: "flex-start" }}><button className="pill" type="button" onClick={() => setConfirmId("")}>{t("c.cancel")}</button><button className="pill danger" type="button" onClick={() => void remove(r.device_id)}>{t("hub.delete")}</button></div></div>
            ) : (
              <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}><button className="pill" type="button" disabled={!!form} onClick={() => setForm({ rec: r })}>{t("dm.edit")}</button><button className="pill danger" type="button" disabled={!!form} onClick={() => setConfirmId(r.device_id)}>{t("hub.delete")}</button></div>
            ))}
          </article>
        ))}</div>
      )}
      {capacity && a && (
        <section className="glass pad stack"><h2>{t("dm.allowances")}</h2>
          <Row label={t("dm.routers")} granted={a.routers} used={a.used_routers} /><Row label={t("dm.controllers")} granted={a.controllers} used={a.used_controllers} /><Row label={t("pa.sites")} granted={a.sites} used={a.used_sites} /><Row label={t("dm.aps")} granted={null} used={a.used_access_points} />
          {a.included_router_name && <p className="soft small">{t("dm.included")}: {a.included_router_name}</p>}
          <p className="soft small">{t("dm.allowNote")}</p>
        </section>
      )}
      {capacity && rq.data && (
        <section className="stack"><h2>{t("dm.requests")}</h2>
          {canDecide && requests.some((r) => r.status === "pending") && <Note kind="warn">{requests.filter((r) => r.status === "pending").length} {t("dm.awaiting")}</Note>}
          {!requests.length ? <p className="soft small">{t("dm.noRequests")}</p> : requests.map((r) => (
            <div className="glass pad stack" key={r.id}>
              <div className="row"><b>{r.kind} × {r.quantity || 1}</b><span className={"chip " + (r.status === "approved" ? "ok" : r.status === "rejected" ? "bad" : "warn")}>{t("st.status." + (["pending", "approved", "rejected"].includes(r.status) ? r.status : "pending"))}</span></div>
              <p className="soft small">{r.user_email} {r.reason ? `· ${r.reason}` : ""}</p>
              {canDecide && r.status === "pending" && <div className="row" style={{ justifyContent: "flex-start" }}><button className="pill on" type="button" onClick={() => void decide(r.id, "approved")}>{t("pf.approve")}</button><button className="pill danger" type="button" onClick={() => void decide(r.id, "rejected")}>{t("pf.reject")}</button></div>}
            </div>
          ))}
          {!reqOpen ? <button className="btn quiet" type="button" onClick={() => setReqOpen(true)}>{t("dm.requestCapacity")}</button> : (
            <form className="glass pad stack" onSubmit={submitRequest}>
              <h3>{t("dm.newRequest")}</h3>
              <label className="field"><span>{t("dm.capacity")}</span><select className="input" name="kind">{kinds.map((k) => <option key={k}>{k}</option>)}</select></label>
              <label className="field"><span>{t("dm.quantity")}</span><input className="input" name="qty" type="number" min={1} max={100} defaultValue={1} /></label>
              <label className="field"><span>{t("dm.reason")}</span><input className="input" name="reason" placeholder={t("dm.reasonHint")} /></label>
              {reqMsg && <Note kind="bad">{reqMsg}</Note>}
              <div className="row" style={{ justifyContent: "flex-start" }}><button className="btn" type="submit">{t("dm.submit")}</button><button className="btn quiet" type="button" onClick={() => setReqOpen(false)}>{t("c.cancel")}</button></div>
            </form>
          )}
        </section>
      )}
    </>
  );
}
