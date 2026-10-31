import { api } from "../api.js";
import { h, mmk, asArray } from "../dom.js";
import { t } from "../i18n.js";

const num = (v) => (typeof v === "number" && Number.isFinite(v) ? v : null);
const stat = (label, value, extra) => h("div", { class: "tile" }, h("span", { class: "label" }, label), h("span", { class: "num" }, value), extra || null);

export default {
  titleKey: "title.home",
  tab: "home",
  async render(ctx) {
    const [fleet, routers, report] = await Promise.allSettled([api.get("/fleet/health"), api.get("/routers"), api.get("/voucher-report")]);
    for (const r of [fleet, routers, report]) if (r.status === "rejected" && r.reason && r.reason.message === "unauthorized") throw r.reason;

    const user = (ctx.me && ctx.me.user) || {};
    const name = user.full_name || (user.email || "").split("@")[0];
    const rep = report.status === "fulfilled" ? report.value : null;
    const flt = fleet.status === "fulfilled" ? fleet.value : null;
    const list = routers.status === "fulfilled" ? asArray(routers.value && routers.value.routers) : null;

    const out = h("div", { class: "stack" });
    out.append(h("div", { class: "stack" },
      h("span", { class: "eyebrow" }, t("h.eyebrow")),
      h("h1", { class: "big" }, t("h.greet") + " ", h("span", { style: "color:#9CC7AB" }, name)),
      h("p", { class: "soft" }, t("h.sub"))));

    if (list && list.length === 0) {
      out.append(h("div", { class: "note warn" }, t("h.noRouter") + " ", h("a", { href: "#/router" }, t("tab.router"))));
    }

    // Voucher value used (first use), never presented as cash received.
    const rev = h("section", { class: "glass" },
      h("h2", { class: "num" }, t("h.revenue")),
      h("p", { class: "soft small" }, t("h.revenueHelp")),
      h("a", { class: "pill", href: "#/revenue", style: "align-self:flex-start" }, t("rv.open")));
    if (rep && num(rep.activation_today) !== null) {
      rev.append(
        h("div", { class: "big" }, mmk(rep.activation_today) + " ", h("span", { style: "font-size:22px" }, t("c.mmk"))),
        h("div", { class: "grid2" },
          stat(t("h.last7"), mmk(rep.activation_last7) + " " + t("c.mmk")),
          stat(t("h.last30"), mmk(rep.activation_last30) + " " + t("c.mmk"))));
    } else {
      rev.append(h("div", { class: "note bad", role: "status" }, t("h.reportFailed")));
    }
    out.append(rev);

    const online = flt ? num(flt.up) : null;
    const total = flt ? num(flt.total_peers) : null;
    const status = h("section", { class: "glass" },
      h("h2", { class: "num" }, t("h.routers")),
      flt && online !== null && total !== null
        ? h("div", { class: "row" }, h("span", { class: "big" }, online + " / " + total), h("span", { class: "chip " + (total > 0 && online === total ? "ok" : "warn") }, total > 0 && online === total ? t("c.online") : t("c.offline")))
        : h("div", { class: "note bad", role: "status" }, t("h.fleetFailed")));
    out.append(status);

    out.append(h("section", { class: "glass" },
      h("h2", { class: "num" }, t("h.left")),
      h("div", { class: "grid2" },
        stat(t("h.internet"), rep && num(rep.available_count) !== null ? mmk(rep.available_count) : t("c.unavailable")),
        stat(t("h.jellyfin"), t("h.jellyfinSoon")))));
    return out;
  },
};
