import { api } from "../api.js";
import { h } from "../dom.js";
import { t } from "../i18n.js";
import { clickOn, setClick } from "../feedback.js";

export default {
  titleKey: "title.more",
  tab: "more",
  async render(ctx) {
    const user = (ctx.me && ctx.me.user) || {};
    const business = (ctx.me && ctx.me.business) || {};
    const out = h("div", { class: "stack" });
    out.append(h("section", { class: "glass" },
      h("span", { class: "label" }, t("m.account")),
      h("h2", { class: "num" }, user.full_name || user.email || "—"),
      h("p", { class: "soft small" }, user.email || ""),
      business.company_name ? h("p", { class: "soft small" }, business.company_name) : null,
      user.user_role ? h("span", { class: "chip" }, t("m.role") + ": " + user.user_role) : null));
    out.append(h("section", { class: "glass" },
      h("h2", { class: "num" }, t("m.store")), h("p", { class: "soft small" }, t("m.storeHelp")),
      h("a", { class: "pill on", href: "#/store" }, t("m.openStore"))));
    if (user.user_role === "DEVELOPER") {
      out.append(h("section", { class: "glass" }, h("h2", { class: "num" }, t("title.platform")), h("a", { class: "pill on", href: "#/platform" }, t("m.openPlatform"))));
    }
    const click = h("button", { class: "pill", type: "button", "aria-pressed": String(clickOn()) }, clickOn() ? t("m.clickOn") : t("m.clickOff"));
    click.addEventListener("click", () => { const on = !clickOn(); setClick(on); click.setAttribute("aria-pressed", String(on)); click.textContent = on ? t("m.clickOn") : t("m.clickOff"); });
    out.append(h("section", { class: "glass" }, h("h2", { class: "num" }, t("m.clickTitle")), h("p", { class: "soft small" }, t("m.clickHelp")), click));
    out.append(h("button", { class: "pill danger", type: "button", onclick: async () => {
      try { await api.post("/auth/logout", {}); } catch (_) { /* session may already be gone */ }
      ctx.signOut("");
    } }, t("c.signOut")));
    return out;
  },
};
