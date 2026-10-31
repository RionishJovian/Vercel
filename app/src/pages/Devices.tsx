import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../lib/api";
import { asArray, type Me } from "../lib/types";
import { useI18n } from "../i18n";
import { BackLink, Note } from "../components/ui";
import { Skeleton } from "../components/Skeleton";
import type { RouterRow } from "./routers";

interface Inv { available?: Record<string, boolean>; objects?: Record<string, Array<{ values?: Record<string, string> }>> }
interface Label { mac: string; name: string; kind: string }
export interface Row { mac: string; ip: string; name: string; type: string; sources: string[]; state: string }
const SOURCE_NAME: Record<string, string> = { discovery_leases: "DHCP lease", arp: "ARP table", neighbors: "Router neighbor" };
const KINDS = ["other", "switch", "ap", "computer", "printer", "controller"];
const EDITORS = ["OWNER", "ADMIN", "DEVELOPER"];
const MAC = /^([0-9A-F]{2}:){5}[0-9A-F]{2}$/;

/** What the router reported in its discovery tables. Online status is never inferred from a table entry. */
export function discoveryRows(inv: Inv): Row[] {
  const rows = new Map<string, Row>();
  for (const source of ["discovery_leases", "arp", "neighbors"]) {
    if (inv.available?.[source] !== true) continue;
    for (const o of asArray<{ values?: Record<string, string> }>(inv.objects?.[source])) {
      const v = o.values ?? {}, mac = String(v["mac-address"] ?? "").toUpperCase(), ip = v.address ?? "", key = mac || ip;
      if (!key) continue;
      const row = rows.get(key) ?? { mac, ip, name: "", type: "", sources: [], state: "unverified" };
      row.ip = ip || row.ip; row.name = v["host-name"] || v.identity || row.name; row.type = v.platform || v.board || row.type; row.sources.push(SOURCE_NAME[source]);
      if (source === "discovery_leases") row.state = v.status === "bound" ? "bound" : "dhcp:" + (v.status || "");
      rows.set(key, row);
    }
  }
  return [...rows.values()];
}

function LabelForm({ row, label, routerId, canEdit }: { row: Row; label?: Label; routerId: string; canEdit: boolean }) {
  const { t } = useI18n();
  const [msg, setMsg] = useState(""), [busy, setBusy] = useState(false);
  const shown = label?.name || row.name || t("dv.unidentified");
  if (!canEdit || !MAC.test(row.mac)) return <b>{shown}</b>;
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget), name = String(f.get("name")).trim(), kind = String(f.get("kind"));
    setBusy(true); setMsg(t("dv.saving"));
    try {
      const saved = await api.put<{ labels?: Label[] }>(`/routers/${encodeURIComponent(routerId)}/discovery-labels`, { mac: row.mac, name, kind });
      if (!asArray<Label>(saved.labels).some((l) => l.mac === row.mac && l.name === name && l.kind === kind)) throw new Error("unconfirmed");
      setMsg(t("dv.saved"));
    } catch (err) { if (isUnauthorized(err)) throw err; setMsg(t("dv.saveFail")); }
    setBusy(false);
  }
  return (
    <form className="stack" onSubmit={save}>
      <label className="field"><span>{t("dv.name")}</span><input className="input" name="name" maxLength={120} required defaultValue={label?.name || row.name} /></label>
      <label className="field"><span>{t("dv.type")}</span><select className="input" name="kind" defaultValue={label?.kind || "other"}>{KINDS.map((k) => <option key={k} value={k}>{t("dv.k." + k)}</option>)}</select></label>
      <button className="pill" type="submit" disabled={busy}>{t("dv.save")}</button>
      {msg && <span className="soft small" role="status">{msg}</span>}
    </form>
  );
}

export function Devices({ me }: { me: Me }) {
  const { t } = useI18n();
  const canEdit = EDITORS.includes(me.user?.user_role ?? "");
  const routers = useSWR<{ routers?: RouterRow[] }>("/routers", fetcher);
  const gateways = useMemo(() => asArray<RouterRow>(routers.data?.routers).filter((r) => r.deployment_layer === "MAIN_GATEWAY"), [routers.data]);
  const [id, setId] = useState(() => { try { return sessionStorage.getItem("mm_selected_router") ?? ""; } catch { return ""; } });
  const [state, setState] = useState<{ kind: "idle" | "loading" } | { kind: "error"; msg: string } | { kind: "ready"; rows: Row[]; labels: Label[]; partial: boolean; labelsFailed: boolean; at: string }>({ kind: "idle" });
  const rev = useRef(0);
  const selected = gateways.find((g) => g.device_id === id) ?? (gateways.length === 1 ? gateways[0] : undefined);

  const read = useCallback(async (rid: string) => {
    const mine = ++rev.current;
    setState({ kind: "loading" });
    try {
      const { inventory } = await api.get<{ inventory: Inv }>(`/routers/${encodeURIComponent(rid)}/observations`);
      const sources = ["discovery_leases", "arp", "neighbors"];
      if (!sources.some((k) => inventory.available?.[k] === true)) throw new Error(t("dv.noTables"));
      let labels: Label[] = [], labelsFailed = false;
      try { labels = asArray<Label>((await api.get<{ labels?: Label[] }>(`/routers/${encodeURIComponent(rid)}/discovery-labels`)).labels); } catch { labelsFailed = true; }
      if (mine === rev.current) setState({ kind: "ready", rows: discoveryRows(inventory), labels, partial: sources.some((k) => inventory.available?.[k] !== true), labelsFailed, at: new Date().toLocaleString() });
    } catch (e) { if (isUnauthorized(e)) throw e; if (mine === rev.current) setState({ kind: "error", msg: e instanceof Error && e.message ? e.message : t("dv.failed") }); }
  }, [t]);
  useEffect(() => { if (selected) void read(selected.device_id); else setState({ kind: "idle" }); }, [selected?.device_id, read]); // eslint-disable-line react-hooks/exhaustive-deps

  const back = <BackLink href="#/more">{t("tab.more")}</BackLink>;
  if (routers.isLoading) return <>{back}<Skeleton h={260} /></>;
  const byMac = new Map(state.kind === "ready" ? state.labels.map((l) => [l.mac, l]) : []);
  return (
    <>
      {back}<h1>{t("dv.title")}</h1><p className="soft">{t("dv.lead")}</p>
      {gateways.length > 1 && <label className="field"><span>{t("net.working")}</span><select className="input" value={selected?.device_id ?? ""} onChange={(e) => { setId(e.target.value); try { sessionStorage.setItem("mm_selected_router", e.target.value); } catch { /* ignore */ } }}><option value="">{t("net.chooseRouter")}</option>{gateways.map((g) => <option key={g.device_id} value={g.device_id}>{g.hardware_name}</option>)}</select></label>}
      {!gateways.length ? <Note>{t("net.noRouter")} <a className="link" href="#/hub">{t("hub.title")}</a></Note> : !selected ? <Note>{t("net.pickOnce")}</Note> : (
        <>
          <button className="pill" type="button" disabled={state.kind === "loading"} onClick={() => void read(selected.device_id)}>{t("gu.refresh")}</button>
          {state.kind === "loading" && <Skeleton h={160} />}
          {state.kind === "error" && <Note kind="warn">{state.msg} {t("dv.nothingChanged")}</Note>}
          {state.kind === "ready" && (
            <>
              <p className="soft small" role="status">{t("dv.read")} {state.at}. {state.partial ? t("dv.partial") : ""} {state.labelsFailed ? t("dv.labelsFailed") : ""}</p>
              {!state.rows.length ? <Note>{t("dv.none")}</Note> : (
                <div className="stack">
                  {state.rows.map((r) => (
                    <div className="glass pad stack" key={r.mac || r.ip}>
                      <LabelForm row={r} label={byMac.get(r.mac)} routerId={selected.device_id} canEdit={canEdit} />
                      {r.type && <p className="soft small">{t("dv.reports")}: {r.type}</p>}
                      <p className="small num">{r.ip} {r.mac}</p>
                      <p className="soft small">{r.state === "bound" ? t("dv.bound") : r.state.startsWith("dhcp:") ? `DHCP ${r.state.slice(5) || t("dv.statusNA")}` : t("dv.unverified")} · {r.sources.join(", ")}</p>
                    </div>
                  ))}
                </div>
              )}
            </>
          )}
        </>
      )}
    </>
  );
}
