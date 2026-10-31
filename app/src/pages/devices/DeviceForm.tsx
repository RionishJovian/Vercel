import { useState, type FormEvent } from "react";
import { api, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Field, Note } from "../../components/ui";
import type { RouterRow } from "../routers";

export interface DeviceRec extends RouterRow { device_model?: string; management_ip?: string; mac_address?: string; vendor?: string; category?: string }
export const LAYERS = ["MAIN_GATEWAY", "DOWNSTREAM_SWITCH", "ACCESS_POINT", "CONTROLLER"] as const;
export const CATEGORY: Record<string, string> = { MAIN_GATEWAY: "gateway", DOWNSTREAM_SWITCH: "switch", ACCESS_POINT: "ap", CONTROLLER: "controller" };
const errText = (e: unknown) => { const e2 = e as { status?: number; data?: { reason?: string; error?: string } } | null; return e2?.status === 403 ? "OWNER/ADMIN required." : e2?.data?.reason || e2?.data?.error || (e instanceof Error ? e.message : "") || "failed"; };

/** Create or edit one device record. A record alone does not connect a device or grant management access. */
export function DeviceForm({ rec, layers, onDone, onCancel, blocked }: { rec?: DeviceRec; layers: readonly string[]; onDone: () => void; onCancel: () => void; blocked?: (layer: string) => string }) {
  const { t } = useI18n();
  const [layer, setLayer] = useState(rec?.deployment_layer ?? layers[0]);
  const [msg, setMsg] = useState(""), [busy, setBusy] = useState(false);
  const tunnel = rec?.transport === "tunnel";
  const stop = blocked?.(layer) ?? "";
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget), s = (k: string) => String(f.get(k) ?? "").trim();
    if (!s("name")) { setMsg(t("dm.nameNeeded")); return; }
    const body = rec
      ? { hardware_name: s("name"), device_model: s("model") || null, deployment_layer: layer, management_ip: s("ip") || null, vendor: rec.vendor, category: rec.category, transport: rec.transport }
      : { hardware_name: s("name"), device_model: s("model") || null, deployment_layer: layer, management_ip: s("ip") || null, mac_address: s("mac") || null, vendor: "mikrotik", category: CATEGORY[layer], transport: "direct" };
    setBusy(true); setMsg("");
    try { if (rec) await api.put(`/routers/${encodeURIComponent(rec.device_id)}`, body); else await api.post("/routers", body); onDone(); }
    catch (err) { if (isUnauthorized(err)) throw err; setMsg(errText(err)); setBusy(false); }
  }
  return (
    <form className="glass pad stack" onSubmit={save}>
      <h2>{rec ? t("dm.edit") : t("dm.add")}</h2>
      <Field label={t("dm.name")} htmlFor="dm-name"><input className="input" id="dm-name" name="name" required maxLength={120} defaultValue={rec?.hardware_name ?? ""} /></Field>
      <Field label={t("dm.model")} htmlFor="dm-model"><input className="input" id="dm-model" name="model" maxLength={120} defaultValue={rec?.device_model ?? ""} /></Field>
      {layers.length > 1 && <Field label={t("dm.layer")} htmlFor="dm-layer"><select className="input" id="dm-layer" value={layer} onChange={(e) => setLayer(e.target.value)}>{layers.map((l) => <option key={l} value={l}>{t("dm.l." + l)}</option>)}</select></Field>}
      <Field label={t("hub.mgmtIp")} htmlFor="dm-ip"><input className="input" id="dm-ip" name="ip" readOnly={tunnel} defaultValue={rec?.management_ip ?? ""} /></Field>
      {tunnel && <p className="soft small">{t("dm.tunnelIp")}</p>}
      {!rec && <Field label={t("dm.mac")} htmlFor="dm-mac"><input className="input" id="dm-mac" name="mac" maxLength={17} placeholder="AA:BB:CC:DD:EE:FF" /></Field>}
      {stop && <Note kind="warn">{stop}</Note>}
      {msg && <Note kind="bad">{msg}</Note>}
      <div className="row" style={{ justifyContent: "flex-start" }}><button className="btn" type="submit" disabled={busy || !!stop}>{rec ? t("pf.save") : t("dm.create")}</button><button className="btn quiet" type="button" onClick={onCancel}>{t("c.cancel")}</button></div>
    </form>
  );
}
