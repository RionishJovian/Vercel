import { h } from "../dom.js";
import { t } from "../i18n.js";

const LEGACY = "/app.html";
const PARTS = ["dash", "vouchers", "guest"];

function nav(current) {
  return h("div", { class: "tabs-ws" },
    h("a", { href: "#/jellyfin/dash", "aria-current": current === "dash" ? "page" : null }, t("j.ws.dash")),
    h("a", { href: "#/jellyfin/vouchers", "aria-current": current === "vouchers" ? "page" : null }, t("j.ws.vouchers")),
    h("a", { href: "#/jellyfin/guest", "aria-current": current === "guest" ? "page" : null }, t("j.ws.guest")));
}

export default {
  titleKey: "title.jellyfin",
  tab: "jellyfin",
  async render(ctx) {
    const part = PARTS.includes(ctx.sub) ? ctx.sub : "dash";
    let body;
    if (part === "vouchers") {
      // Honest state: no Jellyfin voucher backend exists yet.
      body = h("section", { class: "glass" }, h("h2", { class: "num" }, t("j.notReady")), h("p", { class: "soft" }, t("j.notReadyBody")));
    } else {
      const target = part === "guest" ? "#jellyfin-portal" : "#media";
      body = h("section", { class: "glass" },
        h("h2", { class: "num" }, t("j.status")),
        h("p", { class: "soft" }, t("j.dashHelp")),
        h("a", { class: "pill on", href: LEGACY + target }, t("c.openLegacy")));
    }
    return h("div", { class: "stack" }, nav(part), body);
  },
};
