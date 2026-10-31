import { api } from "../api.js";
import { h, mmk, asArray } from "../dom.js";
import { t } from "../i18n.js";

const note = (kind, text) => h("div", { class: "note " + kind, role: kind === "bad" ? "alert" : "status" }, text);
const rethrow = (e) => { if (e && e.message === "unauthorized") throw e; };

export default {
  titleKey: "title.platform",
  tab: "more",
  async render(ctx) {
    const role = ctx.me && ctx.me.user && ctx.me.user.user_role;
    const back = h("a", { class: "pill", href: "#/more", style: "align-self:flex-start" }, "← " + t("tab.more"));
    if (role !== "DEVELOPER") return h("div", { class: "stack" }, back, note("warn", t("pf.denied")));
    const [reqs, adds] = await Promise.allSettled([api.get("/platform/store/requests?status=pending"), api.get("/platform/store/addons")]);
    for (const r of [reqs, adds]) if (r.status === "rejected") rethrow(r.reason);
    if (reqs.status !== "fulfilled" || adds.status !== "fulfilled") return h("div", { class: "stack" }, back, note("bad", t("st.unavailable")));
    const out = h("div", { class: "stack" }, back);

    const pending = asArray(reqs.value.requests);
    out.append(h("section", { class: "glass" }, h("h2", { class: "num" }, t("pf.pending")),
      pending.length ? null : h("p", { class: "soft small" }, t("pf.none")),
      pending.map((r) => {
        const noteIn = h("input", { class: "input", maxlength: "500", "aria-label": t("pf.note"), placeholder: t("pf.note") });
        const decide = async (decision, btn) => {
          btn.disabled = true;
          try { await api.post("/platform/store/requests/" + encodeURIComponent(r.id) + "/decision", { decision, note: noteIn.value.trim() }); ctx.refresh(); } catch (e) { rethrow(e); btn.disabled = false; }
        };
        const ok = h("button", { class: "pill on", type: "button", onclick: () => decide("approve", ok) }, t("pf.approve"));
        const no = h("button", { class: "pill danger", type: "button", onclick: () => decide("reject", no) }, t("pf.reject"));
        return h("div", { class: "stack", style: "border-top:1px solid var(--line-soft);padding-top:10px" },
          h("div", { class: "row wrap" }, h("b", null, r.company_name || r.business_id), h("span", { class: "chip" }, r.name_en)),
          h("div", { class: "row small" }, h("span", { class: "mono soft" }, r.bank_reference), h("span", null, mmk(r.price_mmk) + " " + t("c.mmk"))),
          noteIn, h("div", { class: "row wrap" }, ok, no));
      })));

    const addons = asArray(adds.value.addons);
    out.append(h("section", { class: "glass" }, h("h2", { class: "num" }, t("pf.catalog")),
      addons.map((a) => h("div", { class: "row small wrap" }, h("span", null, a.name_en + " · " + a.addon_key), h("span", { class: "chip " + (a.active ? "ok" : "") }, (a.active ? t("pf.active") : t("pf.hidden")) + " · " + mmk(a.price_mmk))))));

    const f = {};
    const field = (id, label, attrs) => { f[id] = h("input", Object.assign({ class: "input", id: "pf-" + id, required: true }, attrs)); return h("label", { class: "field", for: "pf-" + id }, h("span", null, label), f[id]); };
    const kind = h("select", { class: "input", id: "pf-kind" }, ["router_slot", "server", "feature"].map((k) => h("option", { value: k }, t("st.kind." + k))));
    const period = h("select", { class: "input", id: "pf-period" }, ["one_time", "monthly", "annual"].map((p) => h("option", { value: p }, t("st.period." + p))));
    const msg = h("div", { class: "stack" });
    const submit = h("button", { class: "btn", type: "submit" }, t("pf.add"));
    const form = h("form", { class: "glass", onsubmit: async (e) => {
      e.preventDefault();
      if (!form.reportValidity()) return;
      submit.disabled = true;
      try {
        await api.post("/platform/store/addons", { addon_key: f.key.value.trim(), kind: kind.value, name_en: f.en.value.trim(), name_my: f.my.value.trim(),
          description_en: f.den.value.trim(), description_my: f.dmy.value.trim(), price_mmk: Number(f.price.value), period: period.value, active: true });
        ctx.refresh();
      } catch (err) { rethrow(err); msg.replaceChildren(note("bad", (err && err.data && err.data.error) || t("st.failed"))); submit.disabled = false; }
    } },
      h("h2", { class: "num" }, t("pf.new")),
      field("key", t("pf.key"), { maxlength: "40", pattern: "[a-z0-9_]{3,40}" }),
      field("en", t("pf.nameEn"), { maxlength: "120" }), field("my", t("pf.nameMy"), { maxlength: "120" }),
      field("den", t("pf.descEn"), { maxlength: "600", required: false }), field("dmy", t("pf.descMy"), { maxlength: "600", required: false }),
      field("price", t("pf.price"), { type: "number", min: "0", step: "1", inputmode: "numeric" }),
      h("label", { class: "field", for: "pf-kind" }, h("span", null, t("pf.kind")), kind),
      h("label", { class: "field", for: "pf-period" }, h("span", null, t("pf.period")), period), msg, submit);
    out.append(form);
    return out;
  },
};
