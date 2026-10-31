import { api } from "../api.js";
import { h, asArray } from "../dom.js";
import { t, getLang } from "../i18n.js";

const HEX6 = /^#[0-9a-fA-F]{6}$/;
const rethrow = (e) => { if (e && e.message === "unauthorized") throw e; };
const note = (kind, text) => h("div", { class: "note " + kind, role: kind === "bad" ? "alert" : "status" }, text);
const hexOr = (v, fallback) => (HEX6.test(String(v || "").trim()) ? String(v).trim() : fallback);
// Configs saved before access-method selection existed have no flags: treat as both enabled.
const methodOn = (cfg, key) => (!cfg || typeof cfg[key] !== "boolean" ? true : cfg[key]);

function planRow(p, onChange) {
  const cell = (k, type, value, label) => h("label", { class: "field" }, h("span", null, label),
    h("input", { class: "input", type, "data-k": k, value: value, min: type === "number" ? "0" : null, inputmode: type === "number" ? "numeric" : null, oninput: onChange }));
  const row = h("div", { class: "glass plan-row" },
    cell("key", "text", p.key || "", t("g.plan.key")),
    cell("label", "text", p.label || "", t("g.plan.label")),
    cell("minutes", "number", p.minutes || 0, t("g.plan.minutes")),
    cell("amount", "number", p.amount || 0, t("g.plan.price")),
    cell("data_mb", "number", p.data_mb || 0, t("g.plan.data")),
    h("button", { class: "pill danger", type: "button", onclick: () => { row.remove(); onChange(); } }, t("g.plan.remove")));
  return row;
}

export default async function guestPage() {
  let cfg = null;
  try {
    const res = await api.get("/portal/config");
    cfg = res && res.config ? res.config : res;
  } catch (e) {
    rethrow(e);
    // 404 simply means nothing saved yet: start from an empty form.
    if (e.status !== 404) return note("bad", t("c.loadFailed"));
  }
  cfg = cfg || {};

  const input = (id, value, extra) => h("input", Object.assign({ class: "input", id, type: "text", value: value || "" }, extra || {}));
  const field = (id, label, control) => h("label", { class: "field", for: id }, h("span", null, label), control);
  const name = input("gp-name", cfg.business_name, { maxlength: "120" });
  const welcome = input("gp-welcome", cfg.welcome_text, { maxlength: "240" });
  const terms = input("gp-terms", cfg.terms_text, { maxlength: "500" });
  const primary = input("gp-primary", hexOr(cfg.primary_hex, "#ffb547"), { type: "color" });
  const tint = input("gp-tint", hexOr(cfg.glass_tint_hex, "#7ad0ff"), { type: "color" });
  const bankName = input("gp-bank", cfg.bank_name, { maxlength: "120" });
  const bankHolder = input("gp-holder", cfg.bank_holder, { maxlength: "120" });
  const bankAcc = input("gp-acc", cfg.bank_account, { maxlength: "60", inputmode: "numeric" });
  const instructions = h("textarea", { class: "input", id: "gp-instr", maxlength: "600", rows: "4", style: "padding:12px 16px;min-height:104px" }, cfg.payment_instructions || "");
  const mmqr = input("gp-mmqr", cfg.mmqr_url, { maxlength: "500", placeholder: "https://…/mmqr.jpg" });
  const graceMin = input("gp-gmin", cfg.guest_grace_minutes ?? 10, { type: "number", min: "1", max: "1440", inputmode: "numeric" });
  const graceData = input("gp-gdata", cfg.guest_grace_data_mb ?? 50, { type: "number", min: "1", max: "10240", inputmode: "numeric" });
  const voucherOn = h("input", { type: "checkbox", id: "gp-voucher", checked: methodOn(cfg, "voucher_login_enabled") ? true : null });
  const paymentOn = h("input", { type: "checkbox", id: "gp-payment", checked: methodOn(cfg, "mobile_payment_enabled") ? true : null });
  const plansBox = h("div", { class: "stack" });
  const msg = h("div", { class: "stack" });
  const frame = h("iframe", { class: "gp-frame", sandbox: "allow-scripts", title: t("g.preview") });
  // The hosted payment page is a second guest screen (plans, bank details, MMQR). It is the production page in preview mode.
  const hosted = h("iframe", { class: "gp-frame", src: "/portal.html?preview=1", title: t("g.previewHosted"), hidden: true });
  let dest = "router";
  const pick = (value, label) => h("button", { class: "pill", type: "button", "aria-pressed": "false", onclick: () => { dest = value; syncDest(); refreshPreview(); } }, label);
  const destRouter = pick("router", t("g.destRouter")), destHosted = pick("hosted", t("g.destHosted"));
  const syncDest = () => {
    frame.hidden = dest !== "router"; hosted.hidden = dest !== "hosted";
    destRouter.classList.toggle("on", dest === "router"); destHosted.classList.toggle("on", dest === "hosted");
    destRouter.setAttribute("aria-pressed", String(dest === "router")); destHosted.setAttribute("aria-pressed", String(dest === "hosted"));
  };
  const previewNote = h("p", { class: "soft small", role: "status" });
  const save = h("button", { class: "btn", type: "submit" }, t("g.save"));

  const plans = () => Array.from(plansBox.children).map((row) => {
    const o = {};
    row.querySelectorAll("input[data-k]").forEach((i) => { o[i.dataset.k] = i.type === "number" ? parseInt(i.value || "0", 10) || 0 : i.value.trim(); });
    return o;
  });

  // The server renders the router voucher page, so the preview is the page guests get, not a second copy.
  // Empty fields get sample values in the preview only; they are never sent by Save.
  let timer = 0, rev = 0;
  const draft = () => {
    const entered = plans();
    const has = entered.some((p) => p.key || p.label || p.minutes || p.amount || p.data_mb);
    return {
      business_name: name.value.trim() || "Your WiFi", welcome_text: welcome.value.trim() || "Fast, secure internet for your stay.",
      terms_text: terms.value.trim() || "One plan connects one device.", primary_hex: primary.value, glass_tint_hex: tint.value,
      bank_name: bankName.value.trim(), bank_holder: bankHolder.value.trim(), bank_account: bankAcc.value.trim(), mmqr_url: mmqr.value.trim(),
      guest_grace_minutes: Number(graceMin.value) || 0, guest_grace_data_mb: Number(graceData.value) || 0,
      voucher_login_enabled: voucherOn.checked, mobile_payment_enabled: paymentOn.checked,
      guest_plans: has ? entered : [{ key: "preview-hour", label: "1 Hour WiFi", minutes: 60, amount: 1000, data_mb: 1024 }, { key: "preview-day", label: "Day Pass", minutes: 1440, amount: 3500, data_mb: 5120 }],
      consent_required: !!cfg.consent_required, consent_text: cfg.consent_text || "", preview_sample: !has,
    };
  };
  const pushHosted = () => {
    if (hosted.contentWindow && hosted.isConnected) hosted.contentWindow.postMessage({ type: "mm-portal-preview", config: Object.assign(draft(), { payment_instructions: instructions.value.trim() }), lang: getLang() === "my" ? "my" : "en" }, location.origin);
  };
  const onReady = (ev) => {
    if (!hosted.isConnected) { window.removeEventListener("message", onReady); return; }
    if (ev.origin === location.origin && ev.data && ev.data.type === "mm-portal-preview-ready") pushHosted();
  };
  window.addEventListener("message", onReady);
  const refreshPreview = () => {
    if (dest === "hosted") { previewNote.textContent = t("g.previewHostedNote"); pushHosted(); return; }
    const mine = ++rev;
    clearTimeout(timer);
    previewNote.textContent = t("g.previewLoading");
    timer = setTimeout(async () => {
      try {
        const res = await api.post("/portal/router-preview", draft());
        if (mine !== rev || !frame.isConnected) return;
        if (typeof res.html !== "string") throw new Error("no-html");
        frame.srcdoc = res.html;
        previewNote.textContent = t("g.previewNote");
      } catch (e) {
        if (e && e.message === "unauthorized") { location.reload(); return; }
        if (mine === rev && frame.isConnected) { frame.removeAttribute("srcdoc"); previewNote.textContent = t("g.previewFailed"); }
      }
    }, 300);
  };

  for (const p of asArray(cfg.guest_plans)) plansBox.append(planRow(p, refreshPreview));
  const addPlan = h("button", { class: "pill", type: "button", onclick: () => { plansBox.append(planRow({}, refreshPreview)); refreshPreview(); } }, t("g.plan.add"));

  const form = h("form", { class: "stack", onsubmit: async (e) => {
    e.preventDefault();
    if (!form.reportValidity()) return;
    if (!voucherOn.checked && !paymentOn.checked) { msg.replaceChildren(note("bad", t("g.needMethod"))); return; }
    const gm = Number(graceMin.value), gd = Number(graceData.value);
    if (!Number.isInteger(gm) || gm < 1 || gm > 1440 || !Number.isInteger(gd) || gd < 1 || gd > 10240) { msg.replaceChildren(note("bad", t("g.badGrace"))); return; }
    save.disabled = true;
    // The server merges, so send only what this screen owns. Media (Jellyfin) and consent settings stay untouched.
    const body = {
      business_name: name.value.trim(), welcome_text: welcome.value.trim(), terms_text: terms.value.trim(),
      primary_hex: primary.value, glass_tint_hex: tint.value,
      bank_name: bankName.value.trim(), bank_holder: bankHolder.value.trim(), bank_account: bankAcc.value.trim(),
      mmqr_url: mmqr.value.trim(), payment_instructions: instructions.value.trim(), guest_grace_minutes: gm, guest_grace_data_mb: gd,
      voucher_login_enabled: voucherOn.checked, mobile_payment_enabled: paymentOn.checked, guest_plans: plans().filter((p) => p.key || p.label || p.minutes || p.amount || p.data_mb),
    };
    try { await api.put("/portal/config", body); msg.replaceChildren(note("ok", t("g.saved")), note("warn", t("g.deployHint"))); }
    catch (err) { rethrow(err); msg.replaceChildren(note("bad", t("g.saveFailed"))); }
    save.disabled = false;
  } },
    h("section", { class: "glass stack" }, h("h2", { class: "num" }, t("g.brand")),
      field("gp-name", t("g.name"), name), field("gp-welcome", t("g.welcome"), welcome), field("gp-terms", t("g.terms"), terms),
      h("div", { class: "row" }, field("gp-primary", t("g.primary"), primary), field("gp-tint", t("g.tint"), tint))),
    h("section", { class: "glass stack" }, h("h2", { class: "num" }, t("g.methods")),
      h("label", { class: "row", for: "gp-voucher" }, h("span", null, t("g.voucherCode")), voucherOn),
      h("label", { class: "row", for: "gp-payment" }, h("span", null, t("g.mobilePay")), paymentOn),
      h("p", { class: "soft small" }, t("g.methodsHint"))),
    h("section", { class: "glass stack" }, h("h2", { class: "num" }, t("g.pay")),
      field("gp-bank", t("g.bank"), bankName), field("gp-holder", t("g.holder"), bankHolder), field("gp-acc", t("g.acc"), bankAcc), field("gp-mmqr", t("g.mmqr"), mmqr), field("gp-instr", t("g.instr"), instructions), h("p", { class: "soft small" }, t("g.instrHint")),
      h("div", { class: "row" }, field("gp-gmin", t("g.graceMin"), graceMin), field("gp-gdata", t("g.graceData"), graceData)),
      h("p", { class: "soft small" }, t("g.graceHint"))),
    h("section", { class: "stack" }, h("h2", { class: "num" }, t("g.plans")), plansBox, addPlan),
    msg, save);
  form.addEventListener("input", refreshPreview);
  form.addEventListener("change", refreshPreview);
  syncDest();
  refreshPreview();

  return h("div", { class: "stack" },
    h("p", { class: "soft small" }, t("g.hint")),
    h("section", { class: "glass stack" }, h("h2", { class: "num" }, t("g.preview")), h("div", { class: "row wrap" }, destRouter, destHosted), frame, hosted, previewNote),
    form,
    h("a", { class: "pill", href: "/app.html#portal" }, t("g.deploy")));
}
