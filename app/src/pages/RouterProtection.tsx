import { useEffect, useState } from "react";
import useSWR from "swr";
import { Note } from "../components/ui";
import { api, fetcher, isUnauthorized } from "../lib/api";
import { errorText } from "../lib/errors";
import { useI18n } from "../i18n";
import { asArray, type Router } from "../lib/types";
import { objectsOf, type Inventory } from "./network/schema";

type Request = { kind: "firewall" | "shield" | "guest_vpn"; id: string; values: Record<string, string> };
type Review = { steps: string[]; warnings: string[] | null; confirmation: string };
const disabled = (value?: string) => value === "yes" || value === "true";

function ProtectionSwitches({ router, onLock }: { router: Router; onLock: (locked: boolean) => void }) {
  const { t } = useI18n();
  const base = "/routers/" + encodeURIComponent(router.device_id) + "/network";
  const { data, error, mutate } = useSWR<{ inventory: Inventory }>(base + "/inventory", fetcher, { shouldRetryOnError: false });
  const [port, setPort] = useState(""), [binding, setBinding] = useState("");
  const [busy, setBusy] = useState(false), [notice, setNotice] = useState("");
  const [plan, setPlan] = useState<{ request: Request; review: Review } | null>(null);
  useEffect(() => { onLock(busy || !!plan); return () => onLock(false); }, [busy, plan, onLock]);
  if (error) return <><Note kind="warn">{errorText(error, t("c.loadFailed"))}</Note><button type="button" className="pill" onClick={() => void mutate()}>{t("c.retry")}</button></>;
  if (!data) return <Note>{t("r.checking")}</Note>;
  const inv = data.inventory;
  const filters = objectsOf(inv, "filters");
  const binds = objectsOf(inv, "bindings").filter(o => o.values.type === "bypassed" && (
    disabled(o.values.disabled) ? o.values.comment?.startsWith("mm-shield:") : !/^mm-/i.test(o.values.comment ?? "")
  ));
  const rule = filters.find(o => o.values.comment === "mm-protection-invalid-" + port);
  const vpn = filters.find(o => o.values.comment === "mm-guest-vpn-udp");
  const exemption = binds.find(o => o.id === binding);
  const firewallOn = !!rule && !disabled(rule.values.disabled);
  const shieldOn = !!exemption && disabled(exemption.values.disabled);
  const vpnOn = !!vpn && !disabled(vpn.values.disabled);
  const hotspotReady = inv.available?.hotspots === true && objectsOf(inv, "hotspots").filter(o => !disabled(o.values.disabled)).length === 1;
  const locked = busy || !!plan || inv.write_enabled !== true || !!inv.pending_confirm;
  const describe = (key: string, on: boolean, unavailable: boolean, request: Request) => (
    <article className="glass pad stack protection-card">
      <div className="row">
        <h3>{t("prot." + key)}</h3>
        <button type="button" role="switch" aria-checked={on} aria-label={t("prot." + key)} className="protection-switch" disabled={locked || unavailable}
          onClick={() => void preview(request)}><span /></button>
      </div>
      <p className="soft small">{t("prot." + key + "Help")}</p>
      <p className="small">{t(unavailable ? "c.unavailable" : on ? "prot.on" : "prot.off")}</p>
    </article>
  );
  async function preview(request: Request) {
    setBusy(true); setNotice("");
    try {
      const reply = await api.post<{ review?: Review }>(base + "/preview", request);
      if (!reply.review?.confirmation) throw new Error(t("c.actionFailed"));
      setPlan({ request, review: reply.review });
    } catch (e) { if (isUnauthorized(e)) throw e; setNotice(errorText(e, t("c.actionFailed"))); }
    finally { setBusy(false); }
  }
  async function apply() {
    if (!plan || busy) return;
    setBusy(true); setNotice("");
    try {
      const reply = await api.post<{ result?: { verified?: boolean } }>(base + "/apply", { ...plan.request, confirmation: plan.review.confirmation });
      if (!reply.result?.verified) throw new Error(t("c.actionFailed"));
      setNotice(t("prot.verified"));
    } catch (e) { if (isUnauthorized(e)) throw e; setNotice(errorText(e, t("c.actionFailed"))); }
    finally { setPlan(null); await mutate(); setBusy(false); }
  }
  return <>
    <p className="soft small">{router.hardware_name || router.device_id}</p>
    {inv.write_enabled !== true && <Note kind="warn">{t("net.writesOff")}</Note>}
    <div className="protection-grid">
      <div className="stack">
        <label className="field"><span>{t("prot.port")}</span><select className="input" value={port} disabled={busy || !!plan} onChange={e => setPort(e.target.value)}>
          <option value="">{t("net.choosePort")}</option>{(inv.interfaces ?? []).map(i => <option key={i.name}>{i.name}</option>)}
        </select></label>
        {describe("firewall", firewallOn, !port || inv.available?.filters !== true, { kind: "firewall", id: rule?.id ?? "", values: { interface: port, disabled: firewallOn ? "yes" : "no" } })}
      </div>
      <div className="stack">
        <label className="field"><span>{t("prot.binding")}</span><select className="input" value={binding} disabled={busy || !!plan} onChange={e => setBinding(e.target.value)}>
          <option value="">{t("net.chooseRecord")}</option>{binds.map(o => <option key={o.id} value={o.id}>{[o.values["mac-address"], o.values.address, o.values.server].filter(Boolean).join(" · ")}</option>)}
        </select></label>
        {describe("shield", shieldOn, !exemption || !hotspotReady, { kind: "shield", id: binding, values: { disabled: shieldOn ? "no" : "yes" } })}
      </div>
      {describe("vpn", vpnOn, !hotspotReady || inv.available?.filters !== true, { kind: "guest_vpn", id: "", values: { disabled: vpnOn ? "yes" : "no" } })}
    </div>
    {plan && <div className="note warn stack">
      <h3>{t("prot.review")}</h3>
      <ul>{plan.review.steps.map(step => <li key={step}>{step}</li>)}</ul>
      {(plan.review.warnings ?? []).map(w => <p key={w}>{w}</p>)}
      <div className="row"><button type="button" className="pill" disabled={busy} onClick={() => setPlan(null)}>{t("c.cancel")}</button>
        <button type="button" className="btn" disabled={busy} onClick={() => void apply()}>{t("prot.apply")}</button></div>
    </div>}
    {notice && <p role="status">{notice}</p>}
  </>;
}

export function RouterProtection() {
  const { t } = useI18n();
  const { data, error } = useSWR<{ routers?: Router[] }>("/routers", fetcher);
  const gateways = asArray<Router>(data?.routers).filter(r => r.deployment_layer === "MAIN_GATEWAY");
  const [id, setId] = useState(() => { try { return sessionStorage.getItem("mm_selected_router") ?? ""; } catch { return ""; } });
  const [locked, setLocked] = useState(false);
  const router = gateways.find(r => r.device_id === id) ?? (gateways.length === 1 ? gateways[0] : undefined);
  return <section className="glass pad stack">
    <h2>{t("prot.title")}</h2>
    {error ? <Note kind="warn">{t("c.loadFailed")}</Note> : !data ? <Note>{t("r.checking")}</Note> : !gateways.length ? <Note>{t("v.noGateway")}</Note> : null}
    {gateways.length > 1 && <label className="field"><span>{t("net.working")}</span><select className="input" value={router?.device_id ?? ""} disabled={locked} onChange={e => { setId(e.target.value); try { sessionStorage.setItem("mm_selected_router", e.target.value); } catch { /* ignore */ } }}>
      <option value="">{t("net.chooseRouter")}</option>{gateways.map(r => <option key={r.device_id} value={r.device_id}>{r.hardware_name || r.device_id}</option>)}
    </select></label>}
    {router && <ProtectionSwitches key={router.device_id} router={router} onLock={setLocked} />}
  </section>;
}
