import { api } from "../api.js";
import { h, asArray } from "../dom.js";
import { t } from "../i18n.js";

const LEGACY = "/app.html";

function lastSeen(value) {
  if (!value) return t("r.never");
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? t("r.never") : d.toLocaleString();
}

export default {
  titleKey: "title.router",
  tab: "router",
  async render() {
    const res = await api.get("/routers");
    const list = asArray(res && res.routers);
    const out = h("div", { class: "stack" });
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
