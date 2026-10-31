import { api } from "../api.js";
import { h, mmk, asArray } from "../dom.js";
import { t } from "../i18n.js";

const n = (v) => (typeof v === "number" && Number.isFinite(v) ? v : 0);
const tile = (label, value) => h("div", { class: "tile" }, h("span", { class: "label" }, label), h("span", { class: "num" }, value));

export default {
  titleKey: "title.revenue",
  tab: "home",
  async render() {
    let r;
    try { r = await api.get("/voucher-report"); } catch (e) {
      if (e && e.message === "unauthorized") throw e;
      return h("div", { class: "note bad", role: "alert" }, t("rv.failed"));
    }
    const out = h("div", { class: "stack" });
    out.append(h("a", { class: "pill", href: "#/home", style: "align-self:flex-start" }, "← " + t("tab.home")));
    out.append(h("p", { class: "soft small" }, t("rv.help")));
    out.append(h("section", { class: "glass" },
      h("h2", { class: "num" }, t("rv.used")),
      h("div", { class: "big" }, mmk(n(r.activation_today)) + " ", h("span", { style: "font-size:22px" }, t("c.mmk"))),
      h("span", { class: "soft small" }, t("h.today")),
      h("div", { class: "grid2" },
        tile(t("h.last7"), mmk(n(r.activation_last7)) + " " + t("c.mmk")),
        tile(t("h.last30"), mmk(n(r.activation_last30)) + " " + t("c.mmk"))),
      tile(t("rv.all"), mmk(n(r.activation_value)) + " " + t("c.mmk")),
      n(r.activation_time_unknown_count) ? h("p", { class: "note warn" }, n(r.activation_time_unknown_count) + " " + t("rv.unknown")) : null));

    out.append(h("section", { class: "glass" },
      h("h2", { class: "num" }, t("rv.counts")),
      h("div", { class: "grid2" },
        tile(t("rv.available"), mmk(n(r.available_count))),
        tile(t("rv.reserved"), mmk(n(r.reserved_count))),
        tile(t("rv.activated"), mmk(n(r.activated_count))),
        tile(t("rv.expired"), mmk(n(r.expired_count))),
        tile(t("rv.revoked"), mmk(n(r.revoked_count))))));

    const gws = asArray(r.gateways);
    if (gws.length) {
      out.append(h("section", { class: "glass" }, h("h2", { class: "num" }, t("rv.byRouter")),
        gws.map((g) => h("div", { class: "stack", style: "border-top:1px solid var(--line-soft);padding-top:10px" },
          h("b", null, g.gateway_name || "—"),
          h("div", { class: "row small" }, h("span", { class: "soft" }, t("rv.available")), h("b", { class: "mono" }, mmk(n(g.available_count)))),
          h("div", { class: "row small" }, h("span", { class: "soft" }, t("rv.activated")), h("b", { class: "mono" }, mmk(n(g.activated_count)))),
          h("div", { class: "row small" }, h("span", { class: "soft" }, t("rv.used")), h("b", { class: "mono" }, mmk(n(g.activation_value)) + " " + t("c.mmk")))))));
    }
    return out;
  },
};
