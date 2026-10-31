import { useEffect, useRef, useState, type FormEvent } from "react";
import { api, isUnauthorized } from "../../lib/api";
import { asArray } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import type { RouterRow } from "../routers";

interface Iface { name: string; type?: string; running?: boolean }
interface Defaults { name?: string; gateway_cidr?: string; pool_start?: string; pool_end?: string }
interface Approved { request: Record<string, unknown>; steps: string[]; confirmation: string }
const LAN_TYPES = ["ether", "bridge", "bond", "vlan", "wlan", "wifi", "wireless"];
const errText = (e: unknown) => (e as { data?: { error?: string } } | null)?.data?.error || (e instanceof Error ? e.message : "") || "failed";

/** Creates a separate customer LAN with DHCP and an optional Hotspot. It never touches the uplink, default route or management. */
export function Wizard({ router, canEdit }: { router: RouterRow; canEdit: boolean }) {
  const { t } = useI18n();
  const [load, setLoad] = useState<{ k: "loading" } | { k: "error"; msg: string } | { k: "ready"; ifaces: Iface[]; d: Defaults }>({ k: "loading" });
  const [approved, setApproved] = useState<Approved | null>(null), [status, setStatus] = useState<{ kind: "" | "ok" | "warn"; text: string }>({ kind: "", text: "" });
  const [busy, setBusy] = useState(false), [applying, setApplying] = useState(false);
  const form = useRef<HTMLFormElement>(null);
  const base = `/routers/${encodeURIComponent(router.device_id)}/gateway/`;

  useEffect(() => {
    let live = true; setLoad({ k: "loading" });
    Promise.all([api.get<{ interfaces?: Iface[] }>(base + "interfaces"), api.get<{ defaults?: Defaults }>(base + "defaults")])
      .then(([i, d]) => { if (live) setLoad({ k: "ready", ifaces: asArray<Iface>(i.interfaces), d: d.defaults ?? {} }); })
      .catch((e) => { if (isUnauthorized(e)) throw e; if (live) setLoad({ k: "error", msg: errText(e) }); });
    return () => { live = false; };
  }, [base]);

  if (!canEdit) return <Note>{t("nw.needAdmin")}</Note>;
  if (load.k === "loading") return <Skeleton h={320} />;
  if (load.k === "error") return <Note kind="warn">{t("nw.readFail")} {load.msg}</Note>;
  const { ifaces, d } = load;
  const opt = (list: Iface[]) => [<option key="" value="">{t("nw.chooseIface")}</option>, ...list.map((i) => <option key={i.name} value={i.name}>{i.name} · {i.type || "interface"}{i.running ? ` · ${t("net.running")}` : ""}</option>)];
  const payload = () => {
    const f = new FormData(form.current!), s = (k: string) => String(f.get(k) ?? "").trim();
    return { name: s("name"), wan_interface: String(f.get("wan_interface")), lan_interface: String(f.get("lan_interface")), vlan_id: Number(f.get("vlan_id")), gateway_cidr: s("gateway_cidr"), pool_start: s("pool_start"), pool_end: s("pool_end"), dns_servers: s("dns_servers").split(",").map((x) => x.trim()).filter(Boolean), hotspot: f.has("hotspot"), radius_address: s("radius_address"), radius_secret: String(f.get("radius_secret") ?? "") };
  };
  const invalidate = () => { if (applying) return; setApproved(null); setStatus({ kind: "", text: t("nw.changed") }); };

  async function preview(e: FormEvent) {
    e.preventDefault();
    if (applying || busy || !form.current?.reportValidity()) return;
    setApproved(null); setBusy(true); setStatus({ kind: "", text: t("nw.checking") });
    try {
      const request = payload();
      const r = await api.post<{ plan: { steps?: string[]; confirmation: string } }>(base + "preview", request);
      setApproved({ request, steps: asArray<string>(r.plan.steps), confirmation: r.plan.confirmation }); setStatus({ kind: "", text: "" });
    } catch (err) { if (isUnauthorized(err)) throw err; setStatus({ kind: "warn", text: errText(err) }); }
    setBusy(false);
  }
  async function apply() {
    if (applying || !approved || !window.confirm(`${t("nw.confirm")} ${router.hardware_name ?? ""}`)) return;
    const request = { ...approved.request, confirmation: approved.confirmation };
    setApproved(null); setApplying(true); setStatus({ kind: "", text: t("nw.applying") });
    try {
      const r = await api.post<{ result?: { verified?: boolean; backup?: string } }>(base + "apply", request);
      if (r.result?.verified !== true) throw new Error(t("nw.notVerified"));
      if (form.current) (form.current.elements.namedItem("radius_secret") as HTMLInputElement).value = "";
      setStatus({ kind: "ok", text: `${t("nw.applied")} ${t("nw.backup")}: ${r.result.backup ?? ""}. ${t("nw.testGuest")}` });
    } catch (err) {
      if (isUnauthorized(err)) throw err;
      const rolled = (err as { data?: { result?: { rolled_back?: boolean } } } | null)?.data?.result?.rolled_back;
      setStatus({ kind: "warn", text: `${errText(err)} ${rolled ? t("nw.rolledBack") : t("nw.inspect")}` });
    }
    setApplying(false);
  }

  return (
    <form ref={form} className="stack" onSubmit={preview} onInput={invalidate} onChange={invalidate}>
      <p className="soft small">{t("nw.lead")}</p>
      <fieldset className="glass pad stack fs" disabled={applying}>
        <legend>{t("nw.networkName")}</legend>
        <label className="field"><span>{t("nw.name")}</span><input className="input" name="name" required maxLength={24} pattern="[A-Za-z0-9][A-Za-z0-9_\-]*" defaultValue={d.name || "guest-lan"} /></label>
        <label className="field"><span>{t("nw.uplink")}</span><select className="input" name="wan_interface" required>{opt(ifaces.filter((i) => i.type !== "loopback"))}</select></label>
        <label className="field"><span>{t("nw.lan")}</span><select className="input" name="lan_interface" required>{opt(ifaces.filter((i) => LAN_TYPES.includes(i.type ?? "")))}</select></label>
        <p className="soft small">{t("nw.lanHelp")}</p>
        <label className="field"><span>{t("nw.vlan")}</span><input className="input" name="vlan_id" type="number" min={0} max={4094} defaultValue={0} /></label>
        <p className="soft small">{t("nw.vlanHelp")}</p>
      </fieldset>
      <fieldset className="glass pad stack fs" disabled={applying}>
        <legend>{t("nw.addresses")}{d.gateway_cidr ? ` · ${d.gateway_cidr} (${t("nw.suggested")})` : ""}</legend>
        <p className="soft small">{t("nw.addrHelp")}</p>
        <label className="field"><span>{t("nw.gateway")}</span><input className="input" name="gateway_cidr" required defaultValue={d.gateway_cidr || ""} placeholder="192.168.50.1/24" /></label>
        <div className="two">
          <label className="field"><span>{t("nw.poolStart")}</span><input className="input" name="pool_start" required defaultValue={d.pool_start || ""} placeholder="192.168.50.10" /></label>
          <label className="field"><span>{t("nw.poolEnd")}</span><input className="input" name="pool_end" required defaultValue={d.pool_end || ""} placeholder="192.168.50.250" /></label>
        </div>
        <label className="field"><span>{t("nw.dns")}</span><input className="input" name="dns_servers" required defaultValue="1.1.1.1, 8.8.8.8" /></label>
      </fieldset>
      <fieldset className="glass pad stack fs" disabled={applying}>
        <legend>{t("nw.hotspotTitle")}</legend>
        <label className="check"><span>{t("nw.hotspot")}</span><input type="checkbox" name="hotspot" defaultChecked /></label>
        <label className="field"><span>{t("nw.radius")}</span><input className="input" name="radius_address" placeholder="10.0.0.10" autoComplete="off" /></label>
        <label className="field"><span>{t("nw.radiusSecret")}</span><input className="input" name="radius_secret" type="password" minLength={8} maxLength={128} autoComplete="new-password" /></label>
        <p className="soft small">{t("nw.radiusHelp")}</p>
      </fieldset>
      <button className="btn" type="submit" disabled={busy || applying}>{t("nw.preview")}</button>
      {status.text && <p className={"note " + status.kind} role="status" aria-live="polite">{status.text}</p>}
      {approved && (
        <div className="glass pad stack" role="region" aria-label={t("net.reviewTitle")}>
          <h3>{t("nw.reviewFor")} {router.hardware_name}</h3>
          <ol className="steps">{approved.steps.map((s, i) => <li key={i}>{s}</li>)}</ol>
          <p className="soft small">{t("nw.notReplaced")}</p>
          <button className="btn" type="button" disabled={applying} onClick={() => void apply()}>{t("nw.apply")}</button>
        </div>
      )}
    </form>
  );
}
