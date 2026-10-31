import { api } from "../api.js";
import { h, mmk, asArray } from "../dom.js";
import { t } from "../i18n.js";
import guestPage from "./guestpage.js";

const LEGACY = "/app.html";
const TASKS = ["stock", "create", "shops", "print"];
const PENDING_KEY = "mm2_gv_pending";

function readPending() { try { return JSON.parse(localStorage.getItem(PENDING_KEY) || "null"); } catch (_) { return null; } }
function writePending(v) { try { if (v) localStorage.setItem(PENDING_KEY, JSON.stringify(v)); else localStorage.removeItem(PENDING_KEY); } catch (_) { /* storage blocked */ } }
const rethrow = (e) => { if (e && e.message === "unauthorized") throw e; };
const note = (kind, text) => h("div", { class: "note " + kind, role: kind === "bad" ? "alert" : "status" }, text);

function workspaceNav(guest) {
  return h("div", { class: "tabs-ws" },
    h("a", { href: "#/vouchers", "aria-current": guest ? null : "page" }, t("v.ws.vouchers")),
    h("a", { href: "#/vouchers/guest", "aria-current": guest ? "page" : null }, t("v.ws.guest")));
}
function taskNav(current) {
  return h("div", { class: "scroll-x" }, TASKS.map((k) =>
    h("a", { class: "pill", href: "#/vouchers/" + k, "aria-current": k === current ? "page" : null }, t("v.t." + k))));
}

async function stock(ctx) {
  let data;
  try { data = await api.get("/guest-vouchers/batches"); } catch (e) { rethrow(e); return note("bad", t("c.loadFailed")); }
  const incomplete = asArray(data && data.batches);
  const batches = asArray(data && data.stock_batches).slice().sort((a, b) => String(b.created_at).localeCompare(String(a.created_at)));
  const out = h("div", { class: "stack" });

  if (incomplete.length) {
    const box = h("section", { class: "glass" }, h("h2", { class: "num" }, t("v.pendingTitle")), h("p", { class: "soft small" }, t("v.pendingBody")));
    for (const b of incomplete) {
      const btn = h("button", { class: "pill warn", type: "button", onclick: async () => {
        btn.disabled = true;
        btn.textContent = t("v.retrying");
        try { await api.post("/guest-vouchers/batches/" + encodeURIComponent(b.batch_id) + "/retry", {}); } catch (e) { rethrow(e); }
        ctx.refresh();
      } }, t("v.retry"));
      box.append(h("div", { class: "row wrap" },
        h("span", { class: "mono small" }, b.verified_count + " / " + b.total_count), btn));
    }
    out.append(box);
  }

  if (!batches.length && !incomplete.length) return h("div", { class: "stack" }, note("", t("v.empty")));
  for (const b of batches) {
    const total = Number(b.total_count) || 0;
    const used = Number(b.activated_count) || 0;
    out.append(h("section", { class: "glass" },
      h("div", { class: "row" },
        h("h2", { class: "num" }, b.plan_label || "—"),
        h("span", { class: "chip ok" }, t("v.onRouter") + " " + (b.installed_count || 0) + "/" + total)),
      h("div", { class: "stack" },
        h("div", { class: "row small" }, h("span", { class: "soft" }, t("v.used")), h("b", { class: "mono" }, used + " / " + total)),
        h("div", { class: "bar", "aria-hidden": "true" }, h("i", { style: "width:" + (total ? Math.round((used / total) * 100) : 0) + "%" }))),
      h("div", { class: "row small", style: "border-top:1px solid var(--line-soft);padding-top:10px" },
        h("span", { class: "soft" }, t("v.shop")), h("b", null, b.shop_name || t("v.noShop")))));
  }
  return out;
}

const DEFAULT_LAYOUT = { business_name: "", wifi_name: "", support_contact: "", terms: "", show_price: true, show_expiry: true, show_qr: false, a4_columns: 3, a4_rows_per_page: 8, thermal_columns: 1, thermal_rows_per_strip: 1, paper_width_mm: 210 };

async function loadLayout() {
  try {
    const res = await api.get("/voucher-print-layout");
    return Object.assign({}, DEFAULT_LAYOUT, (res && res.layout) || {});
  } catch (e) { rethrow(e); return Object.assign({}, DEFAULT_LAYOUT); }
}

// Printed cards are read by guests, so labels are Burmese + English.
function validity(minutes) {
  const m = Number(minutes) || 0;
  if (m && m % 1440 === 0) return m / 1440 + " ရက် / day" + (m / 1440 > 1 ? "s" : "");
  if (m && m % 60 === 0) return m / 60 + " နာရီ / hour" + (m / 60 > 1 ? "s" : "");
  return m ? m + " မိနစ် / min" : "";
}

function printCodes(vouchers, layout) {
  const L = Object.assign({}, DEFAULT_LAYOUT, layout || {});
  const root = h("div", { class: "print-root", style: "grid-template-columns:repeat(" + Math.max(1, Math.min(8, Number(L.a4_columns) || 3)) + ",1fr)" },
    vouchers.map((v) => h("div", { class: "card" },
      L.business_name ? h("span", null, h("b", { class: "biz" }, L.business_name)) : null,
      L.wifi_name ? h("span", null, "Wi-Fi: " + L.wifi_name) : null,
      h("b", { class: "code" }, v.code),
      L.show_price ? h("span", null, mmk(v.price_mmk) + " MMK") : null,
      L.show_expiry && v.duration_minutes ? h("span", null, validity(v.duration_minutes) + " · " + t("v.validFirstUse")) : null,
      L.terms ? h("span", { class: "terms" }, L.terms) : null,
      L.support_contact ? h("span", null, L.support_contact) : null)));
  document.body.append(root);
  document.body.classList.add("printing");
  const done = () => { document.body.classList.remove("printing"); root.remove(); window.removeEventListener("afterprint", done); };
  window.addEventListener("afterprint", done);
  window.print();
  setTimeout(() => { if (root.isConnected) done(); }, 60000);
}

async function create(ctx) {
  const [routers, config, shops] = await Promise.allSettled([api.get("/routers"), api.get("/guest-vouchers/config"), api.get("/voucher-resellers")]);
  for (const r of [routers, config, shops]) if (r.status === "rejected") rethrow(r.reason);
  const gateways = routers.status === "fulfilled" ? asArray(routers.value && routers.value.routers).filter((d) => d.deployment_layer === "MAIN_GATEWAY") : [];
  const max = config.status === "fulfilled" && config.value && config.value.max_batch_size ? config.value.max_batch_size : 200;
  const shopList = shops.status === "fulfilled" ? asArray(shops.value && (shops.value.resellers || shops.value)).filter((s) => s.active !== false) : [];
  if (!gateways.length) return note("warn", t("v.noGateway"));

  const result = h("div", { class: "stack" });
  const gateway = h("select", { class: "input", id: "gv-gw", required: true }, gateways.map((g) => h("option", { value: g.device_id }, g.hardware_name || g.device_id)));
  const label = h("input", { class: "input", id: "gv-label", type: "text", maxlength: "80", required: true, placeholder: "1 day" });
  const duration = h("select", { class: "input", id: "gv-dur" }, ["60", "1440", "10080", "43200"].map((m) => h("option", { value: m, selected: m === "1440" ? true : null }, t("v.d." + m))));
  const data = h("input", { class: "input", id: "gv-data", type: "number", inputmode: "numeric", min: "1", step: "1", required: true, value: "1024" });
  const price = h("input", { class: "input", id: "gv-price", type: "number", inputmode: "numeric", min: "0", step: "1", required: true, value: "0" });
  const count = h("input", { class: "input", id: "gv-count", type: "number", inputmode: "numeric", min: "1", max: String(max), step: "1", required: true, value: "50" });
  const shop = h("select", { class: "input", id: "gv-shop" }, h("option", { value: "" }, t("v.noShop")), shopList.map((s) => h("option", { value: s.id }, s.shop_name)));
  const submit = h("button", { class: "btn", type: "submit" }, t("v.create"));

  const showResponse = (res) => {
    result.replaceChildren();
    if (res && res.state === "ready") {
      writePending(null);
      const codes = asArray(res.vouchers);
      result.append(note("ok", t("v.ready")), note("warn", t("v.codesOnce")),
        h("button", { class: "pill on", type: "button", onclick: async () => printCodes(codes, await loadLayout()) }, t("v.print")),
        h("div", { class: "glass" }, codes.map((v) => h("div", { class: "row" }, h("b", { class: "mono" }, v.code), h("span", { class: "soft small" }, t("v.validFirstUse"))))));
    } else {
      result.append(note("warn", t("v.notReady")));
    }
  };

  const form = h("form", { class: "glass", onsubmit: async (e) => {
    e.preventDefault();
    if (!form.reportValidity()) return;
    const body = { device_id: gateway.value, plan_label: label.value.trim(), duration_minutes: Number(duration.value),
      data_mb: Number(data.value), price_mmk: Number(price.value), count: Number(count.value) };
    if (shop.value) body.reseller_id = shop.value;
    const saved = readPending();
    if (saved && Object.keys(body).some((k) => saved[k] !== body[k])) { result.replaceChildren(note("warn", t("v.pendingFirst"))); return; }
    body.batch_id = (saved && saved.batch_id) || crypto.randomUUID();
    // Persist the identity BEFORE the request so a lost response or reload retries the same batch and never makes new codes.
    writePending(body);
    submit.disabled = true;
    result.replaceChildren(note("", t("v.creating")));
    try {
      const res = await api.post("/guest-vouchers", body);
      showResponse(res);
    } catch (err) {
      rethrow(err);
      result.replaceChildren(note("bad", t("v.createFailed")));
    } finally {
      submit.disabled = false;
    }
  } },
    h("label", { class: "field", for: "gv-gw" }, h("span", null, t("v.f.gateway")), gateway),
    h("label", { class: "field", for: "gv-label" }, h("span", null, t("v.f.label")), label),
    h("label", { class: "field", for: "gv-dur" }, h("span", null, t("v.f.duration")), duration),
    h("label", { class: "field", for: "gv-data" }, h("span", null, t("v.f.data")), data),
    h("label", { class: "field", for: "gv-price" }, h("span", null, t("v.f.price")), price),
    h("label", { class: "field", for: "gv-count" }, h("span", null, t("v.f.count")), count),
    h("label", { class: "field", for: "gv-shop" }, h("span", null, t("v.f.shop")), shop),
    submit);
  return h("div", { class: "stack" }, form, result);
}

async function shops(ctx) {
  let list = [];
  try { const res = await api.get("/voucher-resellers"); list = asArray(res && (res.resellers || res)); } catch (e) { rethrow(e); return note("bad", t("c.loadFailed")); }
  const msg = h("div", { class: "stack" });
  const name = h("input", { class: "input", id: "sh-name", maxlength: "120", required: true });
  const contact = h("input", { class: "input", id: "sh-contact", maxlength: "120", required: true });
  const prefix = h("input", { class: "input", id: "sh-prefix", minlength: "2", maxlength: "4", required: true, pattern: "[A-Za-z]{2,4}", style: "text-transform:uppercase" });
  const phone = h("input", { class: "input", id: "sh-phone", type: "tel", maxlength: "40" });
  const submit = h("button", { class: "btn", type: "submit" }, t("v.shops.add"));
  const form = h("form", { class: "glass", onsubmit: async (e) => {
    e.preventDefault();
    if (!form.reportValidity()) return;
    const body = { shop_name: name.value.trim(), contact_name: contact.value.trim(), code_prefix: prefix.value.trim().toUpperCase() };
    if (phone.value.trim()) body.phone = phone.value.trim();
    submit.disabled = true;
    try { await api.post("/voucher-resellers", body); ctx.refresh(); } catch (err) { rethrow(err); msg.replaceChildren(note("bad", t("v.shops.failed"))); submit.disabled = false; }
  } },
    h("label", { class: "field", for: "sh-name" }, h("span", null, t("v.shops.name")), name),
    h("label", { class: "field", for: "sh-contact" }, h("span", null, t("v.shops.contact")), contact),
    h("label", { class: "field", for: "sh-prefix" }, h("span", null, t("v.shops.prefix")), prefix),
    h("label", { class: "field", for: "sh-phone" }, h("span", null, t("v.shops.phone")), phone),
    msg, submit);
  const out = h("div", { class: "stack" });
  if (!list.length) out.append(note("", t("v.shops.empty")));
  for (const s of list) out.append(h("section", { class: "glass" }, h("div", { class: "row" }, h("h2", { class: "num" }, s.shop_name), h("span", { class: "chip mono" }, s.code_prefix)), s.phone ? h("p", { class: "soft small" }, s.phone) : null));
  out.append(form);
  return out;
}

async function printPane(ctx) {
  const [layoutRes, batchRes] = await Promise.allSettled([api.get("/voucher-print-layout"), api.get("/guest-vouchers/batches")]);
  for (const r of [layoutRes, batchRes]) if (r.status === "rejected") rethrow(r.reason);
  const layout = Object.assign({}, DEFAULT_LAYOUT, layoutRes.status === "fulfilled" && layoutRes.value ? layoutRes.value.layout : {});
  const batches = batchRes.status === "fulfilled" ? asArray(batchRes.value && batchRes.value.stock_batches) : [];
  const msg = h("div", { class: "stack" });

  const biz = h("input", { class: "input", id: "pl-biz", maxlength: "120", required: true, value: layout.business_name });
  const wifi = h("input", { class: "input", id: "pl-wifi", maxlength: "120", required: true, value: layout.wifi_name });
  const support = h("input", { class: "input", id: "pl-support", maxlength: "200", required: true, value: layout.support_contact });
  const terms = h("textarea", { class: "input", id: "pl-terms", maxlength: "2000", rows: "3", required: true, style: "padding:12px 16px;min-height:96px" }, layout.terms);
  const cols = h("select", { class: "input", id: "pl-cols" }, [2, 3, 4, 5].map((n) => h("option", { value: String(n), selected: Number(layout.a4_columns) === n ? true : null }, String(n))));
  const showPrice = h("input", { type: "checkbox", id: "pl-price", checked: layout.show_price ? true : null });
  const showExpiry = h("input", { type: "checkbox", id: "pl-expiry", checked: layout.show_expiry ? true : null });
  const save = h("button", { class: "btn", type: "submit" }, t("p.save"));
  const form = h("form", { class: "glass", onsubmit: async (e) => {
    e.preventDefault();
    if (!form.reportValidity()) return;
    save.disabled = true;
    const body = Object.assign({}, layout, { business_name: biz.value.trim(), wifi_name: wifi.value.trim(), support_contact: support.value.trim(), terms: terms.value.trim(),
      a4_columns: Number(cols.value), show_price: showPrice.checked, show_expiry: showExpiry.checked });
    delete body.business_id; delete body.created_at; delete body.updated_at;
    try { await api.put("/voucher-print-layout", body); Object.assign(layout, body); msg.replaceChildren(note("ok", t("p.saved"))); } catch (err) { rethrow(err); msg.replaceChildren(note("bad", t("p.saveFailed"))); }
    save.disabled = false;
  } },
    h("h2", { class: "num" }, t("p.layout")),
    h("label", { class: "field", for: "pl-biz" }, h("span", null, t("p.biz")), biz),
    h("label", { class: "field", for: "pl-wifi" }, h("span", null, t("p.wifi")), wifi),
    h("label", { class: "field", for: "pl-support" }, h("span", null, t("p.support")), support),
    h("label", { class: "field", for: "pl-terms" }, h("span", null, t("p.terms")), terms),
    h("label", { class: "field", for: "pl-cols" }, h("span", null, t("p.cols")), cols),
    h("label", { class: "row", for: "pl-price" }, h("span", null, t("p.showPrice")), showPrice),
    h("label", { class: "row", for: "pl-expiry" }, h("span", null, t("p.showExpiry")), showExpiry),
    msg, save);

  const pick = h("select", { class: "input", id: "pl-batch" }, batches.map((b) => h("option", { value: b.batch_id }, (b.plan_label || "—") + " · " + b.total_count)));
  const printMsg = h("div", { class: "stack" });
  const printBtn = h("button", { class: "btn", type: "button", onclick: async () => {
    printBtn.disabled = true;
    try {
      const res = await api.get("/guest-vouchers/batches/" + encodeURIComponent(pick.value));
      const codes = asArray(res && res.vouchers);
      if (res && res.state === "ready" && codes.length) printCodes(codes, layout); else printMsg.replaceChildren(note("warn", t("p.notReady")));
    } catch (err) { rethrow(err); printMsg.replaceChildren(note("bad", t("c.loadFailed"))); }
    printBtn.disabled = false;
  } }, t("p.printBtn"));
  const printBox = batches.length
    ? h("section", { class: "glass" }, h("h2", { class: "num" }, t("p.printBatch")), h("label", { class: "field", for: "pl-batch" }, h("span", null, t("p.pick")), pick), printMsg, printBtn)
    : note("", t("p.noBatches"));
  return h("div", { class: "stack" }, printBox, form);
}

export default {
  titleKey: "title.vouchers",
  tab: "vouchers",
  async render(ctx) {
    if (ctx.sub === "guest") return h("div", { class: "stack" }, workspaceNav(true), await guestPage(ctx));
    const sub = TASKS.includes(ctx.sub) ? ctx.sub : "stock";
    const pane = sub === "stock" ? await stock(ctx) : sub === "create" ? await create(ctx) : sub === "shops" ? await shops(ctx) : await printPane(ctx);
    return h("div", { class: "stack" }, workspaceNav(), taskNav(sub), h("p", { class: "soft small" }, t("v.hint")), pane);
  },
};
