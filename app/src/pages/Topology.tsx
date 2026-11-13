import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../lib/api";
import { asArray, type Me } from "../lib/types";
import { useI18n } from "../i18n";
import { BackLink, Note, PageHead } from "../components/ui";
import { Skeleton } from "../components/Skeleton";
import { project, type DeviceRecord, type GatewayRef, type Graph, type Label, type ObsInventory, type TNode } from "./topology/project";
import { DeviceControls, RouterPorts } from "./topology/Controls";
import { PendingPanel } from "./network/NetworkEditor";
import { errorText } from "../lib/errors";
import { useFleet, type FleetPeer } from "../lib/useFleet";
import { statusFacts } from "../components/StatusDetail";

const EDITORS = ["OWNER", "ADMIN", "DEVELOPER"];
interface Dev extends GatewayRef, DeviceRecord { site_id?: string | null; deployment_layer?: string; category?: string }
interface TreeNode { device?: Dev; children?: TreeNode[] }
interface Topo { roots?: TreeNode[]; orphans?: Dev[] }
const asset = (name: string) => `${import.meta.env.BASE_URL}${name.startsWith("models/") ? "" : "topology/"}${name}`;
const MAX = 120;

function Card({ n, root, on, selected, onPick }: { n: TNode; root?: boolean; on?: boolean; selected: boolean; onPick: () => void }) {
  const active = on ?? /associated|session/.test(n.state);
  return (
    <button type="button" className={"nt-device" + (root ? " nt-router" : "") + (selected ? " is-selected" : "")} aria-pressed={selected} onClick={onPick}>
      <span className="nt-image"><img src={asset(n.image || n.kind + ".svg")} alt="" width={96} height={96} /></span>
      <strong>{n.name}</strong>
      <span className={"nt-state" + (active ? " nt-active" : "")}>{n.state}</span>
      <small className="num">{n.ip || n.model || n.mac || ""}</small>
    </button>
  );
}

export function Topology({ me }: { me: Me }) {
  const { t, lang } = useI18n();
  const canEdit = EDITORS.includes(me.user?.user_role ?? "");
  const [view, setView] = useState<"map" | "tree">(() => {
    try { return sessionStorage.getItem("mm_topology_view") === "tree" ? "tree" : "map"; } catch { return "map"; }
  });
  const switchView = (next: "map" | "tree") => {
    setView(next);
    try { sessionStorage.setItem("mm_topology_view", next); } catch { /* optional preference */ }
  };
  const topo = useSWR<Topo>("/topology/map", fetcher);
  const fleet = useFleet({ shouldRetryOnError: false });
  const sitesRes = useSWR<{ sites?: Array<{ site_id: string; name: string }> }>("/sites", fetcher, { shouldRetryOnError: false });
  const [site, setSite] = useState(() => { try { return sessionStorage.getItem("mm_selected_site") ?? ""; } catch { return ""; } });
  const [routerId, setRouterId] = useState(() => { try { return sessionStorage.getItem("mm_selected_router") ?? ""; } catch { return ""; } });
  const [graph, setGraph] = useState<Graph | null>(null);
  const [selected, setSelected] = useState(""), [query, setQuery] = useState("");
  const [status, setStatus] = useState(""), [reading, setReading] = useState(false);
  const [guestActions, setGuestActions] = useState(false);
  const [pendingAt, setPendingAt] = useState(""), [pendingNote, setPendingNote] = useState("");
  const rev = useRef(0);

  const all = useMemo(() => {
    const out: Dev[] = [];
    const walk = (n: TreeNode) => { if (n.device) out.push(n.device); asArray<TreeNode>(n.children).forEach(walk); };
    asArray<TreeNode>(topo.data?.roots).forEach(walk); out.push(...asArray<Dev>(topo.data?.orphans));
    return out;
  }, [topo.data]);
  const gateways = all.filter((d) => (d.deployment_layer === "MAIN_GATEWAY" || d.category === "gateway") && (!site || d.site_id === site));
  const device = gateways.length === 1 ? gateways[0] : gateways.find((d) => d.device_id === routerId);

  const allRef = useRef(all); allRef.current = all;
  const read = useCallback(async (d: Dev) => {
    const mine = ++rev.current;
    setReading(true); setStatus(t("tp.reading"));
    try {
      const id = encodeURIComponent(d.device_id);
      const [obs, labels] = await Promise.all([api.get<{ inventory?: ObsInventory; guest_actions_enabled?: boolean }>(`/routers/${id}/observations`), api.get<{ labels?: Label[] }>(`/routers/${id}/discovery-labels`).catch(() => ({ labels: [] as Label[] }))]);
      if (mine !== rev.current) return;
      // Only records for this router's site: another site's record must not name a device here.
      const records = allRef.current.filter((r) => r.device_id !== d.device_id && r.deployment_layer !== "MAIN_GATEWAY" && (!r.site_id || r.site_id === d.site_id));
      const g = project(d, obs.inventory ?? {}, asArray<Label>(labels.labels), records);
      setGraph(g); setGuestActions(obs.guest_actions_enabled === true); setPendingAt(g.pendingAt); setStatus(g.unavailable.length ? `${t("tp.someUnavailable")} ${g.unavailable.map((s) => s.replaceAll("_", " ")).join(", ")}.` : t("tp.done"));
    } catch (e) { if (isUnauthorized(e)) throw e; if (mine === rev.current) setStatus(errorText(e, t("tp.failed"))); }
    finally { if (mine === rev.current) setReading(false); }
  }, [t]);
  useEffect(() => {
    if (!device) { ++rev.current; setGraph(null); setStatus(""); setReading(false); return; }
    setGraph(project(device)); setSelected(device.device_id); setPendingAt(""); void read(device);
  }, [device?.device_id, read]); // eslint-disable-line react-hooks/exhaustive-deps

  const back = <BackLink href="#/more">{t("tab.more")}</BackLink>;
  if (topo.isLoading) return <>{back}<Skeleton h={360} /></>;
  if (topo.error) return <>{back}<Note kind="bad">{t("c.loadFailed")}</Note></>;
  const q = query.trim().toLowerCase();
  const filtered = graph ? graph.devices.filter((n) => [n.name, n.ip, n.mac, n.model, ...n.ports].join(" ").toLowerCase().includes(q)) : [];
  const visible = filtered.slice(0, MAX), up = visible.filter((n) => n.upstream), down = visible.filter((n) => !n.upstream);
  // Report only the gateway interface where a device was observed. A learned MAC
  // is not evidence of a physical parent switch, so never fabricate a hop.
  const groups = new Map<string, TNode[]>();
  for (const node of down) {
    const port = node.ports[0] || (lang === "my" ? "ချိတ်ဆက်မှု မသိရသေး" : "Attachment unverified");
    groups.set(port, [...(groups.get(port) ?? []), node]);
  }
  const sel = graph && (selected === graph.root.id ? graph.root : graph.devices.find((n) => n.id === selected));
  const pickSite = (v: string) => { setSite(v); setRouterId(""); try { sessionStorage.setItem("mm_selected_site", v); sessionStorage.removeItem("mm_selected_router"); } catch { /* ignore */ } };
  const pickRouter = (v: string) => { setRouterId(v); try { if (v) sessionStorage.setItem("mm_selected_router", v); } catch { /* ignore */ } };
  const branch = (n: TNode) => (
    <div className={"nt-branch" + (n.direct ? "" : " nt-unplaced")} key={n.id}><span className="nt-link-label">{n.ports.join(" / ") || n.connection}</span><Card n={n} selected={selected === n.id} onPick={() => setSelected(n.id)} /></div>
  );
  // The router's connection state is the shared one. Reading its details is a separate fact, shown below.
  const rootPeer = asArray<FleetPeer>(fleet.data?.peers).find((p) => p.device_id === device?.device_id);
  const rootState = String(rootPeer?.state ?? "unknown").toLowerCase();
  const rootLabel = t("fl.state." + (["up", "stale", "offline"].includes(rootState) ? rootState : "unknown"));
  const rootFacts = sel && graph && sel.id === graph.root.id ? statusFacts(t, rootPeer) : [];
  const facts: Array<[string, string | undefined]> = sel ? [...rootFacts, [t("tp.model"), sel.model], [t("dv.reports"), sel.platform], ["IP", sel.ip], ["MAC", sel.mac], [t("tp.connection"), sel.connection], [t("tp.iface"), sel.ports.join(", ")], [t("tp.signal"), sel.signal], [t("tp.reading2"), graph?.observedAt ? new Date(graph.observedAt).toLocaleString() : t("tp.unavailable")], [t("tp.sources"), sel.sources.map((s) => s.replaceAll("_", " ")).join(", ")]] : [];

  const reload = () => { if (device) void read(device); };
  const ctx = device && graph ? { routerId: device.device_id, routerName: device.hardware_name ?? device.device_id, locked: !graph.controlsEnabled || !!pendingAt, guestLocked: !guestActions, onChanged: reload, onPending: (at: string) => { setPendingNote(""); setPendingAt(at); } } : null;
  return (
    <>
      {back}<PageHead title={t("tp.title")} help={t("tp.lead")} />
      <div className="glass pad stack">
        <div className="two">
          <label className="field"><span>{t("gu.site")}</span><select className="input" value={site} onChange={(e) => pickSite(e.target.value)}><option value="">{t("tp.allSites")}</option>{asArray<{ site_id: string; name: string }>(sitesRes.data?.sites).map((s) => <option key={s.site_id} value={s.site_id}>{s.name}</option>)}</select></label>
          {gateways.length > 1 && <label className="field"><span>{t("v.f.gateway")}</span><select className="input" value={device?.device_id ?? ""} onChange={(e) => pickRouter(e.target.value)}><option value="">{t("net.chooseRouter")}</option>{gateways.map((d) => <option key={d.device_id} value={d.device_id}>{d.hardware_name}</option>)}</select></label>}
        </div>
        <div className="row nt-view-toggle" role="group" aria-label={lang === "my" ? "ကွန်ရက်ပုံစံ" : "Topology view"}>
          <button type="button" className={"pill" + (view === "map" ? " on" : "")} aria-pressed={view === "map"} onClick={() => switchView("map")}>{lang === "my" ? "မြေပုံ" : "Map"}</button>
          <button type="button" className={"pill" + (view === "tree" ? " on" : "")} aria-pressed={view === "tree"} onClick={() => switchView("tree")}>{lang === "my" ? "သစ်ပင်ပုံစံ" : "Tree"}</button>
        </div>
        <input className="input" type="search" placeholder={t("tp.find")} aria-label={t("tp.find")} value={query} onChange={(e) => setQuery(e.target.value)} />
        <div className="row"><p className="soft small" role="status">{status}</p>{device && <button className="pill" type="button" disabled={reading} onClick={reload}>{t("gu.refresh")}</button>}</div>
      </div>
      {pendingNote && !pendingAt && <Note>{pendingNote}</Note>}
      {device && !!pendingAt && <PendingPanel routerId={device.device_id} revertAt={pendingAt} onDone={(message) => { setPendingAt(""); setPendingNote(message); reload(); }} />}
      {!gateways.length ? <Note>{t("tp.noGateway")} <a className="link" href="#/hub">{t("hub.title")}</a></Note> : !graph ? <Note>{t("tp.pick")}</Note> : (
        <div className="nt-layout">
          <section className="glass nt-stage" aria-label={t("tp.title")} aria-busy={reading}>
            {view === "map" ? <>
              {up.length > 0 && <div className="nt-upstream">{up.map(branch)}</div>}
              <div className="nt-root"><Card n={{ ...graph.root, state: rootLabel }} root on={rootState === "up"} selected={selected === graph.root.id} onPick={() => setSelected(graph.root.id)} /></div>
              <div className="nt-children">{down.map(branch)}</div>
              {!down.length && <p className="nt-none soft small">{t("tp.noDownstream")}</p>}
            </> : <div className="nt-tree">
              <div className="nt-tree-root"><Card n={{ ...graph.root, state: rootLabel }} root on={rootState === "up"} selected={selected === graph.root.id} onPick={() => setSelected(graph.root.id)} /></div>
              {up.length > 0 && <section className="nt-tree-section" aria-label={lang === "my" ? "အဝင်ချိတ်ဆက်မှု" : "Upstream"}>
                <h3>{lang === "my" ? "အဝင်ချိတ်ဆက်မှု" : "Upstream — observed"}</h3>
                <div className="nt-tree-items">{up.map((n) => <div className="nt-tree-item" key={n.id}>{branch(n)}</div>)}</div>
              </section>}
              {groups.size > 0 ? <div className="nt-tree-section">
                <h3>{lang === "my" ? "အောက်ဆင့် ကိရိယာများ" : "Downstream — grouped by observed gateway interface"}</h3>
                {[...groups].map(([port, nodes]) => <details className="nt-tree-group" key={port} open>
                  <summary>{port} <span className="soft small num">· {nodes.length}</span></summary>
                  <div className="nt-tree-items">{nodes.map((n) => <div className="nt-tree-item" key={n.id}>{branch(n)}</div>)}</div>
                </details>)}
              </div> : <p className="soft small">{t("tp.noDownstream")}</p>}
              <p className="soft small">{lang === "my" ? "ဆက်စပ်မှုများသည် RouterOS မှ လေ့လာရရှိထားသော အချက်အလက်များသာဖြစ်ပြီး switch ချိတ်ဆက်ပုံကို အတည်မပြုနိုင်သေးပါ။" : "Groups show observed gateway interfaces, not verified switch-to-AP cabling. Unknown attachments are never invented."}</p>
            </div>}
            {filtered.length > MAX && <p className="soft small">{t("tp.truncated")}</p>}
          </section>
          <aside className="glass pad stack nt-inspector" aria-live="polite">
            {sel ? (<><h2>{sel.name}</h2><p className="soft small">{sel.id === graph.root.id ? rootLabel : sel.state}</p><dl className="facts">{facts.filter(([, v]) => v).map(([k, v]) => <div key={k} style={{ display: "contents" }}><dt>{k}</dt><dd>{v}</dd></div>)}</dl>
              {sel.record && <p className="small">{t("dc.record")}: <a className="link" href="#/managed-devices">{sel.record.hardware_name || t("hub.unnamed")}</a></p>}
              {sel.id === graph.root.id && <a className="btn quiet" href="#/hub">{t("tp.routerSettings")}</a>}
              {canEdit && ctx && (sel.id === graph.root.id ? <RouterPorts ports={graph.ports} ctx={ctx} /> : <DeviceControls key={sel.id} node={sel} ports={graph.ports} ctx={ctx} />)}</>) : <p className="soft small">{t("tp.select")}</p>}
          </aside>
        </div>
      )}
      <p className="soft small">{t("tp.note")}</p>
    </>
  );
}
