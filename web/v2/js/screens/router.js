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

export default {
  titleKey: "title.router",
  tab: "router",
  async render(ctx) {
    const res = await api.get("/routers");
    const list = asArray(res && res.routers);
    const onMap = ctx.sub === "map";
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
          h("h2", { class: "num" }, d.hardware_name || d.device_id),
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
