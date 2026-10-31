import { api } from "../api.js";
import { h, asArray } from "../dom.js";
import { t, getLang } from "../i18n.js";

const rows = (inv, key) => (inv && inv.available && inv.available[key] === true ? asArray(inv.objects && inv.objects[key]).map((o) => o.values || {}) : []);
const macOf = (v) => (/^([0-9a-f]{2}:){5}[0-9a-f]{2}$/i.test(String(v)) ? String(v).toUpperCase() : "");
const yes = (v) => v === "true" || v === "yes";

// Read-only projection. A forwarding-table entry is not proof of a cable, so
// every device keeps the way it was observed and nothing is invented.
export function project(inv) {
  const own = new Set(rows(inv, "interfaces").map((v) => macOf(v["mac-address"])).filter(Boolean));
  const found = new Map();
  for (const key of ["discovery_leases", "arp", "neighbors", "bridge_hosts", "wifi_clients", "hotspot_sessions"]) {
    for (const v of rows(inv, key)) {
      if (key === "bridge_hosts" && yes(v.local)) continue;
      const mac = macOf(v["mac-address"]);
      if (!mac || own.has(mac) || parseInt(mac.slice(0, 2), 16) & 1) continue;
      if (!found.has(mac)) found.set(mac, { mac, name: "", ip: "", port: "", how: "" });
      const d = found.get(mac);
      if (v.address) d.ip = v.address;
      const label = v.identity || v["host-name"];
      if (label) d.name = label;
      if (key === "bridge_hosts" && v["on-interface"]) { d.port = d.port || v["on-interface"]; d.how = d.how || "learned"; }
      if (key === "wifi_clients" && v.interface) { d.port = v.interface; d.how = "wifi"; }
      if (key === "neighbors" && v.interface) { d.port = d.port || v.interface; d.how = d.how || "neighbor"; }
    }
  }
  const ports = new Map();
  const unplaced = [];
  for (const d of found.values()) {
    if (!d.name) d.name = d.ip || d.mac;
    if (d.port) { if (!ports.has(d.port)) ports.set(d.port, []); ports.get(d.port).push(d); } else unplaced.push(d);
  }
  return { ports: [...ports.entries()].sort((a, b) => a[0].localeCompare(b[0], undefined, { numeric: true })), unplaced, total: found.size };
}

const howText = (d) => (d.how === "wifi" ? t("map.how.wifi") : d.how === "neighbor" ? t("map.how.neighbor") : d.how === "learned" ? (getLang() === "my" ? d.port + " " + t("map.how.learned") : t("map.how.learned") + " " + d.port) : t("map.how.unknown"));

export default async function renderMap(ctx, gateways) {
  const id = ctx.params.get("id") || gateways[0].device_id;
  const gw = gateways.find((g) => g.device_id === id) || gateways[0];
  let inv;
  try { inv = await api.get("/routers/" + encodeURIComponent(gw.device_id) + "/network/inventory"); } catch (e) {
    if (e && e.message === "unauthorized") throw e;
    return h("div", { class: "note bad", role: "alert" }, t("map.failed"));
  }
  const model = project(inv);
  const out = h("div", { class: "stack" });
  const inspector = h("section", { class: "glass", "aria-live": "polite" }, h("p", { class: "soft small" }, t("map.pick")));
  let selected = null;

  function show(node, el) {
    if (selected) selected.removeAttribute("aria-pressed");
    selected = el; el.setAttribute("aria-pressed", "true");
    inspector.replaceChildren(
      h("span", { class: "label" }, t("map.props")),
      h("h2", { class: "num" }, node.name),
      h("div", { class: "row small" }, h("span", { class: "soft" }, "IP"), h("b", { class: "mono" }, node.ip || "—")),
      h("div", { class: "row small" }, h("span", { class: "soft" }, "MAC"), h("b", { class: "mono" }, node.mac)),
      h("div", { class: "row small" }, h("span", { class: "soft" }, t("map.seen")), h("b", null, howText(node))),
      h("p", { class: "soft small" }, t("map.note")));
  }
  const nodeBtn = (d) => { const b = h("button", { type: "button", class: "mapnode dev" }, h("b", null, d.name), h("span", { class: "soft small" }, howText(d))); b.addEventListener("click", () => show(d, b)); return b; };

  if (gateways.length > 1) {
    const sel = h("select", { class: "input", "aria-label": t("v.f.gateway"), onchange: () => { location.hash = "#/router/map?id=" + encodeURIComponent(sel.value); } },
      gateways.map((g) => h("option", { value: g.device_id, selected: g.device_id === gw.device_id ? true : null }, g.hardware_name || g.device_id)));
    out.append(sel);
  }
  out.append(h("section", { class: "glass" },
    h("div", { class: "row wrap" }, h("div", { class: "mapnode router" }, h("b", null, gw.hardware_name || "RouterBOARD"), h("span", { class: "soft small" }, model.total + " " + t("map.devices"))),
      h("span", { class: "chip " + (gw.is_online ? "ok" : "bad") }, gw.is_online ? t("c.online") : t("c.offline"))),
    model.total === 0 ? h("div", { class: "note" }, t("map.empty")) : null,
    model.ports.map(([port, devs]) => h("div", { class: "stack" },
      h("div", { class: "row" }, h("span", { class: "chip mono" }, port), h("span", { class: "soft small" }, devs.length)),
      h("div", { class: "stack", style: "padding-left:14px;border-left:2px solid var(--line)" }, devs.map(nodeBtn)))),
    model.unplaced.length ? h("div", { class: "stack" }, h("span", { class: "label" }, t("map.unplaced")), model.unplaced.map(nodeBtn)) : null),
    inspector);
  return out;
}
