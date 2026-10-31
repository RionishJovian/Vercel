import { api } from "../api.js";
import { h, mmk, asArray } from "../dom.js";
import { t, getLang } from "../i18n.js";

const pick = (a, en, my) => (getLang() === "my" ? a[my] || a[en] : a[en] || a[my]);
const note = (kind, text) => h("div", { class: "note " + kind, role: kind === "bad" ? "alert" : "status" }, text);
const statusChip = (s) => h("span", { class: "chip " + (s === "approved" ? "ok" : s === "rejected" ? "bad" : "warn") }, t("st.status." + s));

export default {
  titleKey: "title.store",
  tab: "more",
  async render(ctx) {
    let data;
    try { data = await api.get("/store/addons"); } catch (e) {
      if (e && e.message === "unauthorized") throw e;
      return h("div", { class: "stack" }, h("a", { class: "pill", href: "#/more", style: "align-self:flex-start" }, "← " + t("tab.more")), note("warn", t("st.unavailable")));
    }
    let banks = [];
    try { const co = await api.get("/me/app-pass/checkout-options"); banks = asArray(co && co.bank_accounts); } catch (e) { if (e && e.message === "unauthorized") throw e; }
    const addons = asArray(data.addons);
    const requests = asArray(data.requests);
    const canBuy = ["OWNER", "ADMIN", "DEVELOPER"].includes(ctx.me && ctx.me.user && ctx.me.user.user_role);
    const pendingIds = new Set(requests.filter((r) => r.status === "pending").map((r) => r.addon_id));
    const out = h("div", { class: "stack" }, h("a", { class: "pill", href: "#/more", style: "align-self:flex-start" }, "← " + t("tab.more")), h("p", { class: "soft small" }, t("st.help")));

    if (banks.length) {
      out.append(h("section", { class: "glass" }, h("h2", { class: "num" }, t("st.pay")),
        banks.map((b) => h("div", { class: "row small wrap" }, h("span", { class: "soft" }, (b.bank_name || "") + " · " + (b.holder_name || "")), h("b", { class: "mono" }, b.account_number || "")))));
    }
    if (!addons.length) out.append(note("", t("st.empty")));
    for (const a of addons) {
      const msg = h("div", { class: "stack" });
      const ref = h("input", { class: "input", id: "ref-" + a.id, maxlength: "80", minlength: "4", required: true, placeholder: t("st.refHint") });
      const submit = h("button", { class: "btn", type: "submit" }, t("st.send"));
      const form = h("form", { class: "stack", hidden: true, onsubmit: async (e) => {
        e.preventDefault();
        if (!form.reportValidity()) return;
        submit.disabled = true;
        try {
          await api.post("/store/addons/" + encodeURIComponent(a.id) + "/requests", { bank_reference: ref.value.trim(), idempotency_key: crypto.randomUUID() });
          ctx.refresh();
        } catch (err) {
          if (err && err.message === "unauthorized") throw err;
          msg.replaceChildren(note(err && err.status === 409 ? "warn" : "bad", err && err.status === 409 ? t("st.pending") : t("st.failed")));
          submit.disabled = false;
        }
      } }, h("label", { class: "field", for: "ref-" + a.id }, h("span", null, t("st.ref")), ref), msg, submit);
      const open = h("button", { class: "pill on", type: "button", disabled: !canBuy || pendingIds.has(a.id) ? true : null, onclick: () => { form.hidden = !form.hidden; } },
        pendingIds.has(a.id) ? t("st.status.pending") : t("st.request"));
      out.append(h("section", { class: "glass" },
        h("div", { class: "row wrap" }, h("h2", { class: "num" }, pick(a, "name_en", "name_my")), h("span", { class: "chip" }, t("st.kind." + a.kind))),
        a.description_en || a.description_my ? h("p", { class: "soft small" }, pick(a, "description_en", "description_my")) : null,
        h("div", { class: "row" }, h("b", { class: "big", style: "font-size:26px" }, mmk(a.price_mmk) + " " + t("c.mmk")), h("span", { class: "soft small" }, t("st.period." + a.period))),
        open, form));
    }
    if (requests.length) {
      out.append(h("section", { class: "glass" }, h("h2", { class: "num" }, t("st.requests")),
        requests.map((r) => h("div", { class: "stack", style: "border-top:1px solid var(--line-soft);padding-top:10px" },
          h("div", { class: "row wrap" }, h("b", null, pick(r, "name_en", "name_my")), statusChip(r.status)),
          h("div", { class: "row small" }, h("span", { class: "soft mono" }, r.bank_reference), h("span", { class: "soft" }, mmk(r.price_mmk) + " " + t("c.mmk"))),
          r.decision_note ? h("p", { class: "soft small" }, r.decision_note) : null))));
    }
    return out;
  },
};
