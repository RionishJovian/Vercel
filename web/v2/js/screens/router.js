import { api } from "../api.js";
import { h, asArray } from "../dom.js";
import { t } from "../i18n.js";
import renderMap from "./map.js";

const LEGACY = "/app.html";

function lastSeen(value) {
  if (!value) return t("r.never");
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? t("r.never") : d.toLocaleString();
}

const pct = (v) => (typeof v === "number" ? Math.round(v) + "%" : "—");
const bytes = (v) => (typeof v === "number" ? (v / 1048576).toFixed(0) + " MB" : "—");
const rate = (v) => (typeof v === "number" ? (v >= 1e6 ? (v / 1e6).toFixed(1) + " Mbps" : Math.round(v / 1e3) + " kbps") : "—");
const kv = (k, v) => h("div", { class: "row small" }, h("span", { class: "soft" }, k), h("b", { class: "mono" }, v));
function uptime(sec) {
  if (typeof sec !== "number") return "—";
  const d = Math.floor(sec / 86400), hr = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
  return (d ? d + "d " : "") + hr + "h " + m + "m";
}

// Read-only live health. Network writes stay held server-side until a physical
// router acceptance test has passed, so nothing here changes the router.
async function renderDetail(id, router) {
  const out = h("div", { class: "stack" }, h("a", { class: "pill", href: "#/router", style: "align-self:flex-start" }, "← " + t("r.ws.list")));
  out.append(h("h2", { class: "num" }, (router && router.hardware_name) || id));
  let v;
  try { v = (await api.get("/routers/" + encodeURIComponent(id) + "/telemetry")).telemetry; } catch (e) {
    if (e && e.message === "unauthorized") throw e;
    out.append(h("div", { class: "note warn", role: "alert" }, t("r.telFail")), h("a", { class: "pill", href: "#/router/" + id }, t("c.retry")));
    return out;
  }
  const snap = v && v.snapshot;
  if (!snap) { out.append(h("div", { class: "note warn" }, t("r.telFail"))); return out; }
  const r = snap.resources || {};
  out.append(h("section", { class: "glass" },
    h("div", { class: "row" }, h("h3", null, t("r.health")), h("span", { class: "chip " + (v.state === "live" || v.state === "syncing" ? "ok" : "warn") }, v.state || "—")),
    kv(t("r.board"), (r.board_name || "—") + (r.routeros_version ? " · " + r.routeros_version : "")),
    kv("CPU", pct(r.cpu_percent)),
    kv(t("r.memory"), bytes(Number(r.memory_used_bytes)) + " / " + bytes(Number(r.memory_total_bytes))),
    kv(t("r.uptime"), uptime(r.uptime_seconds)),
    kv(t("r.guests"), snap.hotspot_sessions == null ? "—" : String(snap.hotspot_sessions)),
    kv(t("r.leases"), snap.dhcp_bound_leases == null ? "—" : String(snap.dhcp_bound_leases))));
  const ifs = asArray(snap.interfaces);
  if (ifs.length) {
    out.append(h("section", { class: "glass" }, h("h3", null, t("r.ports")),
      ifs.map((i) => h("div", { class: "row small" },
        h("span", null, i.name, " ", h("span", { class: "chip " + (i.disabled ? "bad" : i.running ? "ok" : "warn") }, i.disabled ? t("r.off") : i.running ? t("c.online") : t("r.down"))),
        h("span", { class: "mono soft" }, "↓ " + rate(i.rx_bps) + " ↑ " + rate(i.tx_bps))))));
  }
  const issues = asArray(snap.issues);
  if (issues.length) out.append(h("div", { class: "note warn" }, issues.join(" · ")));
  out.append(h("p", { class: "soft small" }, t("r.readOnly")));
  return out;
}

export default {
  titleKey: "title.router",
  tab: "router",
  async render(ctx) {
    const res = await api.get("/routers");
    const list = asArray(res && res.routers);
    const onMap = ctx.sub === "map";
    if (ctx.sub && !onMap) return renderDetail(ctx.sub, list.find((d) => d.device_id === ctx.sub));
    const nav = h("div", { class: "tabs-ws" },
      h("a", { href: "#/router", "aria-current": onMap ? null : "page" }, t("r.ws.list")),
      h("a", { href: "#/router/map", "aria-current": onMap ? "page" : null }, t("r.ws.map")));
    if (onMap) {
      const gateways = list.filter((d) => d.deployment_layer === "MAIN_GATEWAY");
      if (!gateways.length) return h("div", { class: "stack" }, nav, h("div", { class: "note warn" }, t("v.noGateway")));
      return h("div", { class: "stack" }, nav, await renderMap(ctx, gateways));
    }
    const out = h("div", { class: "stack" }, nav);
    if (!list.length) out.append(h("div", { class: "note" }, t("r.empty")));
    for (const d of list) {
      out.append(h("section", { class: "glass" },
        h("div", { class: "row wrap" },
          h("h2", { class: "num" }, h("a", { href: "#/router/" + encodeURIComponent(d.device_id), style: "color:inherit;text-decoration:none" }, d.hardware_name || d.device_id)),
          h("span", { class: "chip " + (d.is_online ? "ok" : "bad") }, d.is_online ? t("c.online") : t("c.offline"))),
        h("div", { class: "row wrap" }, h("span", { class: "chip" }, d.transport === "wireguard" ? "Magic Hub" : (d.transport || "—")),
          d.deployment_layer === "MAIN_GATEWAY" ? h("span", { class: "chip" }, "Gateway") : null),
        h("p", { class: "soft small" }, t("r.lastSeen") + ": " + lastSeen(d.last_seen_at))));
    }
    out.append(h("section", { class: "glass" }, h("h2", { class: "num" }, t("r.tools")), h("p", { class: "soft small" }, t("r.toolsHelp")),
      h("a", { class: "pill on", href: LEGACY + "#gateways" }, t("c.openLegacy"))));
    return out;
  },
};
