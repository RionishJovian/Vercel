/* Guest captive portal — MMQR buy flow.

   PREVIEW MODE (`?preview=1`) exists for the operator console's live preview:
   the page renders a draft config pushed in by the parent frame over
   postMessage and performs NO network calls of its own. Crucially its actions
   are inert — clicking a plan must never create a real guest-purchase request,
   which would otherwise pollute the owner's real queue with preview clicks.

   Branding: primary_hex / glass_tint_hex come from the owner and are injected
   into CSS custom properties. They are validated against a strict hex pattern
   here as well as on the server, because this is a public page: it must not
   assume the only way a value reached the database was the write path.

   i18n: default language is Burmese (my-MM); `en` is the fallback. `t(en,key)`
   returns the Burmese only when LANG==='my' AND the key is present/non-blank;
   otherwise the English passed in the call renders. Technical terms with no
   clean Burmese equivalent (MMQR, Plan, Receipt, Bank/transaction) stay in
   English per D2 — no machine transliteration.

   STATUS: token-level typos corrected (ဖုန်းနံပါတ်, အတည်ပြုပါ, မကြာမီ). Several
   strings remain FLAGGED FOR NATIVE REVIEW (marked inline); the malformed
   `waiting` string was removed so the safe English fallback renders instead. */
var API = "/api/v1";
var QS = new URLSearchParams(location.search);
var DEVICE = QS.get("deviceId") || "";
var PREVIEW = QS.get("preview") === "1";
var CLIENT_IP = (QS.get("ip") || QS.get("client_ip") || "").trim();

var LANG = (function () { try { return localStorage.getItem("mm_lang") || "my"; } catch (e) { return "my"; } })();
var RERENDER = null;
var GRACE_TIMER = null;
var CONFIG_PENDING = false;
var VOUCHER_PENDING = false;

var MM = {
  mediaTitle: "ရုပ်ရှင်းများ",
  mediaSub: "ဖွင့်ကြည့်မှုသည် ဤကွန်ရက်အတွင်းတွင်သာ ဖြစ်ပါသည်။ အင်တာနက်သို့ မတင်ပို့ပါ။",
  mediaOpenAll: "ရုပ်ရှင်းအားလုံး ဖွင့်ရန်",
  mediaOpenHint: "နောက်တစ်မျက်နှာတွင် ရုပ်ရှင်းရွေးချယ်ပါ။",
  mediaLoadFailed: "ရုပ်ရှင်းများ မဖွင့်နိုင်ပါ။ အထက်ရှိ လင့်မှတစ်ဆင့် အသုံးပြုနိုင်သေးသည်။",
  buy:      "WiFi ဝယ်ယူမှု",                                 // Buy WiFi  [needs native review]
  pay:      "ဘဏ်လွှဲ ငွေပေးချေမှု",                            // Pay by bank transfer  [needs native review]
  steps:    "1) Plan ရွေးပါ  2) MMQR ငွေပေးချေပါ  3) ဖုန်းနံပါတ် ထည့်ပါ", // [phone token fixed; native review]
  choose:   "Plan ရွေးပါ",                                    // Choose a plan (Plan kept English)
  trial:    "MMQR ဖြင့် ငွေပေးချေပါ — trial အချိန်အတွင်း ပြီးအောင်", // [needs native review]
  expired:  "Trial အချိန်ကုန်သွားပါပြီ",                        // [needs native review]
  afterPay: "ငွေပေးချေပြီးနောက် အောက်ပါကို ဖြည့်ပါ",           // verified
  phone:    "ငွေပေးချေခဲ့သော ဖုန်းနံပါတ်",                      // [phone token fixed; native review]
  last6:    "Bank/transaction နံပါတ် နောက်ဆုံး ၆ လုံး",         // kept English "Bank/transaction"
  receipt:  "Receipt ဓာတ်ပုံ (optional)",                      // Receipt kept English; [native review]
  verConfirm:"ငွေပေးချေပြီးပါပြီ — အတည်ပြုပါ",                 // [typo fixed; native review]
  requestRec:"လက်ခံရရှိပါပြီ — owner က မကြာမီ အတည်ပြုပါလိမ့်မည်", // [typo fixed; native review]
  flowChoose:"ရွေးပါ",            // Choose  [native review]
  flowPay:   "ပေးချေပါ",          // Pay  [native review]
  flowConfirm:"အတည်ပြုပါ",        // Confirm  [native review]
  flowLabel: "အဆင့်များ",         // Steps  [native review]
  amountDue: "ပေးရမည့် ပမာဏ",     // Amount to pay  [native review]
  copy:      "ကူးယူရန်",          // Copy  [native review]
  copied:    "ကူးပြီး",           // Copied  [native review]
  bankName:  "ဘဏ်",               // Bank  [native review]
  holderName:"အမည်",              // Name  [native review]
  accountNo: "အကောင့်နံပါတ်",     // Account number  [native review]
  pickPhoto: "ဓာတ်ပုံ ရွေးပါ",    // Choose photo  [native review]
  trialEndsIn:"ယာယီအင်တာနက် ကုန်ရန်", // Temporary internet ends in  [native review]
  passActive:"အသုံးပြုနိုင်ပါပြီ", // Pass active  [native review]
  timeLeft:  "ကျန်ရှိသော အချိန်", // Time left  [native review]
  passEnded: "Pass ကုန်ဆုံးပါပြီ" // Pass ended  [native review]
  // 'waiting' — malformed in the draft and deliberately REMOVED so the safe
  // English fallback renders until a native speaker supplies correct text.
};

function t(en, key) {
  if (LANG !== "my") return en;
  var v = MM[key];
  return v && v.trim() ? v : en;
}
function langLabel() { return LANG === "my" ? "English" : "မြန်မာ"; }
function applyLang() {
  document.documentElement.lang = LANG === "my" ? "my-MM" : "en";
  var el = document.getElementById("lang");
  if (el) el.textContent = langLabel();
}
function toggleLang() {
  if (VOUCHER_PENDING) return;
  LANG = LANG === "my" ? "en" : "my";
  try { localStorage.setItem("mm_lang", LANG); } catch (e) {}
  applyLang();
  if (RERENDER) RERENDER();
}
function esc(s) {
  return String(s==null?"":s).replace(/[&<>"']/g, function (ch) {
    return { "&":"&amp;", "<":"&lt;", ">":"&gt;", '"':"&quot;", "'":"&#39;" }[ch];
  });
}
function clientMac() {
  var raw = QS.get("mac") || QS.get("client_mac") || QS.get("clientMac") || "";
  var compact = raw.trim().replace(/[^0-9a-fA-F]/g, "").toUpperCase();
  if (compact.length !== 12) return PREVIEW ? "00:00:00:00:00:00" : "";
  return compact.match(/.{2}/g).join(":");
}

/* --- branding ------------------------------------------------------------ */

/** Only a literal hex colour may reach a stylesheet; anything else is dropped. */
function safeHex(v, fallback) {
  var s = String(v == null ? "" : v).trim();
  return /^#([0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/.test(s) ? s : fallback;
}

/** Pick the foreground with the higher WCAG contrast against a brand background. A mid colour such as cyan
 *  (#06B6D4) needs dark ink; white on it is only 2.4:1. */
function readableOn(hex) {
  var h = hex.replace("#", "");
  if (h.length === 3 || h.length === 4) { h = h[0]+h[0]+h[1]+h[1]+h[2]+h[2]; }
  function lin(v) { v = v / 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); }
  var L = 0.2126 * lin(parseInt(h.substring(0,2),16)) + 0.7152 * lin(parseInt(h.substring(2,4),16)) + 0.0722 * lin(parseInt(h.substring(4,6),16));
  return (L + 0.05) / 0.05518 > 1.05 / (L + 0.05) ? "#111" : "#fff";
}

/** The owner's colour recolours the primary button and a faint panel wash. Without one, the page keeps its
 *  Obsidian and Cyan defaults (cyan in Night, deep cyan in Day). */
function applyBranding(cfg) {
  var s = document.documentElement.style;
  var brand = safeHex(cfg.primary_hex, "");
  var tint = safeHex(cfg.glass_tint_hex, "");
  if (brand) { s.setProperty("--brand", brand); s.setProperty("--brand-ink", readableOn(brand)); }
  else { s.removeProperty("--brand"); s.removeProperty("--brand-ink"); }
  if (tint) s.setProperty("--tint", tint); else s.removeProperty("--tint");
}

/* --- data ---------------------------------------------------------------- */

async function loadConfig() {
  if (CONFIG_PENDING) return;
  CONFIG_PENDING = true;
  var root = document.getElementById("root");
  RERENDER = null;
  root.innerHTML = '<p class="msg" role="status">' + t("Loading hotspot…", "loadingHotspot") + '</p>';
  try {
    var res = await fetch(API + "/portal/" + encodeURIComponent(DEVICE) + "/config", { cache: "no-store" });
    if (!res.ok) throw new Error("config unavailable");
    var d = await res.json();
    var cfg = d && (d.config || d);
    if (!cfg || typeof cfg !== "object" || Array.isArray(cfg) || typeof cfg.business_name !== "string" || !cfg.business_name.trim()) throw new Error("invalid config");
    cfg.guest_voucher_access = d.guest_voucher_access === true;
    // The router login page links here with start=pay so a guest lands on the plans.
    if (QS.get("start") === "pay" && accessMethodEnabled(cfg, "mobile_payment_enabled")) renderPlans(cfg); else renderPortalHome(cfg);
  } catch (e) {
    renderConfigError();
  } finally {
    CONFIG_PENDING = false;
  }
}

function renderConfigError() {
  RERENDER = renderConfigError;
  document.getElementById("root").innerHTML =
    '<p class="msg" role="alert">' + t("Could not load this hotspot. Check your WiFi connection and try again.", "loadErr") + '</p>' +
    '<button class="btn" id="portal-retry" type="button">' + t("Try again", "retryHotspot") + '</button>';
  document.getElementById("portal-retry").addEventListener("click", loadConfig);
}

function accessMethodEnabled(cfg, key) {
  return typeof cfg[key] === "boolean" ? cfg[key] : true;
}

function portalHeading(cfg, titleKey, titleEnglish) {
  return '<div class="head">' +
    (cfg.business_name ? '<p class="biz">' + esc(cfg.business_name) + '</p>' : '') +
    '<h1>' + t(titleEnglish, titleKey) + '</h1>' +
    (cfg.welcome_text ? '<p class="welcome">' + esc(cfg.welcome_text) + '</p>' : '') +
    '</div>';
}

/** Choose, pay, confirm: a real sequence, so it is shown as one. */
function flow(step) {
  var labels = [t("Choose", "flowChoose"), t("Pay", "flowPay"), t("Confirm", "flowConfirm")];
  return '<ol class="flow" aria-label="' + esc(t("Steps", "flowLabel")) + '">' + labels.map(function (l, i) {
    return '<li' + (i + 1 === step ? ' aria-current="step"' : (i + 1 < step ? ' class="done"' : '')) + '>' + esc(l) + '</li>';
  }).join("") + '</ol>';
}
function money(n) { return Number(n || 0).toLocaleString("en"); }

// Voucher is the primary screen whenever it is enabled. Mobile payment is a
// separate, explicitly navigated screen when the owner enables both methods.
function renderPortalHome(cfg) {
  if (accessMethodEnabled(cfg, "voucher_login_enabled")) renderVoucher(cfg);
  else renderPlans(cfg);
}

function consentMarkup(cfg) {
  return (cfg.terms_text ? '<p class="terms">' + esc(cfg.terms_text) + '</p>' : '') +
    (cfg.consent_required ? '<label class="terms"><input type="checkbox" id="portal-consent"> ' + esc(cfg.consent_text || 'I agree to the hotspot terms.') + '</label>' : '') +
    '<p id="consent-message" class="msg" role="status"></p>';
}
function portalConsent(cfg) {
  var checkbox = document.getElementById('portal-consent');
  var accepted = !!(checkbox && checkbox.checked);
  if (cfg.consent_required && !accepted) {
    var message = document.getElementById('consent-message');
    if (message) message.textContent = 'Please agree to the hotspot terms before continuing.';
    if (checkbox) checkbox.focus();
    return null;
  }
  return accepted;
}
function renderVoucher(cfg) {
  applyBranding(cfg);
  RERENDER = function () { renderVoucher(cfg); };
  var paymentEnabled = accessMethodEnabled(cfg, "mobile_payment_enabled");
  document.getElementById("root").innerHTML =
    portalHeading(cfg, "voucherWelcome", "Connect to WiFi") +
    '<section class="voucher-login" aria-labelledby="voucher-title">' +
      '<h3 id="voucher-title">' + t("Use a voucher code", "voucherTitle") + '</h3>' +
      '<p class="voucher-help">' + t("Enter the code printed on your WiFi voucher.", "voucherHelp") + '</p>' +
      '<div class="voucher-row"><input id="voucher-code" class="code-input" inputmode="text" autocomplete="one-time-code" autocapitalize="characters" autocorrect="off" spellcheck="false" maxlength="64" placeholder="••••••••" aria-label="Voucher code">' +
      '<button class="btn" id="voucher-submit" type="button">' + t("Connect", "voucherConnect") + '</button></div>' +
      '<p class="voucher-msg" id="voucher-msg" role="status" aria-live="polite"></p>' +
    '</section>' +
    (paymentEnabled ? '<div class="portal-nav"><button class="btn btn-secondary" id="portal-go-payment" type="button">' + t("Buy WiFi by bank transfer →", "goPayment") + '</button><p class="portal-nav-note">' + t("Continue to plans and MMQR payment", "goPaymentNote") + '</p></div>' : '') +
    consentMarkup(cfg);
  document.getElementById("voucher-submit").addEventListener("click", function () { redeemVoucher(cfg); });
  document.getElementById("voucher-code").addEventListener("keydown", function (e) {
    if (e.key === "Enter") { e.preventDefault(); redeemVoucher(cfg); }
  });
  var goPayment = document.getElementById("portal-go-payment");
  if (goPayment) goPayment.addEventListener("click", function () { renderPlans(cfg); });
}

function renderPlans(cfg) {
  if (!accessMethodEnabled(cfg, "mobile_payment_enabled")) {
    if (accessMethodEnabled(cfg, "voucher_login_enabled")) { renderVoucher(cfg); return; }
    applyBranding(cfg);
    RERENDER = function () { renderPlans(cfg); };
    document.getElementById("root").innerHTML = portalHeading(cfg, "voucherWelcome", "Connect to WiFi") +
      '<p class="msg" role="status">' + t("Voucher login and bank transfer are currently unavailable. Ask the hotspot owner for help.", "accessUnavailable") + '</p>';
    return;
  }
  applyBranding(cfg);
  RERENDER = function () { renderPlans(cfg); };
  var plans = cfg.guest_plans || [];
  var rows = plans.map(function (p) {
    var dm = p.data_mb ? " · " + p.data_mb + " MB" : "";
    var mins = p.minutes ? (p.minutes >= 60 ? (p.minutes/60) + " hr" : p.minutes + " min") : "";
    var spd = p.speed_down_mbps ? " · " + p.speed_down_mbps + " Mbps" : "";
    var detail = (mins + dm + spd).replace(/^ · /, "");
    return '<button class="plan" type="button" data-key="' + esc(p.key) + '">' +
      '<span><span class="nm">' + esc(p.label) + '</span>' + (detail ? '<span class="sub">' + esc(detail) + '</span>' : '') + '</span>' +
      (p.amount ? '<span class="amt">' + esc(money(p.amount)) + ' <small>MMK</small></span>' : '') + '</button>';
  }).join("");
  document.getElementById("root").innerHTML =
    portalHeading(cfg, "buy", "Buy WiFi") +
    flow(1) +
    (PREVIEW && cfg.preview_sample ? '<p class="pv-sample-note">Sample preview data — add a plan to replace it.</p>' : '') +
    (rows ? '<div class="plan-list" role="group" aria-label="' + esc(t("Choose a plan", "choose")) + '">' + rows + '</div>' : '<p class="msg">' + t("No plans are available yet.", "noPlans") + '</p>') +
    (accessMethodEnabled(cfg, "voucher_login_enabled") ? '<button class="btn btn-secondary" id="portal-back-voucher" type="button">' + t("← Use a voucher code", "backVoucher") + '</button>' : '') +
    consentMarkup(cfg);
  document.querySelectorAll(".plan").forEach(function (b) {
    b.addEventListener("click", function () { buy(cfg, b.dataset.key); });
  });
  var backVoucher = document.getElementById("portal-back-voucher");
  if (backVoucher) backVoucher.addEventListener("click", function () { renderVoucher(cfg); });
}


// ---------------------------------------------------------------------------
// Media: after a voucher is accepted the guest can browse the owner's library
// and open it on their own device. The catalogue is served by Magic Magic, but
// the FILES are not: the guest is sent to the owner's own media server on the
// business network. Nothing streams through our servers.
//
// Only rendered once access is verified, and only when the owner has set a media
// host. When no media server is configured this returns an empty string, so the
// portal behaves exactly as before.
function mediaPanel(cfg) {
  var host = cfg && cfg.media_host ? String(cfg.media_host).trim() : "";
  if (!host) return "";
  var port = cfg.media_port ? String(cfg.media_port).trim() : "8096";
  var base = "http://" + host + ":" + port;
  // Only ever render what the owner configured. The host is validated as a
  // private IPv4 address server-side, so it cannot point anywhere public.
  var poster = cfg.media_background ? ' style="background-image:url(' + esc(cfg.media_background) + ')"' : "";
  return '<section class="mm-media" id="mm-media">' +
    '<h2 class="mm-media-title">' + esc(t("Your movies", "mediaTitle")) + '</h2>' +
    '<p class="mm-media-sub">' + esc(t("Playing happens on this network. Nothing is sent to the internet.", "mediaSub")) + '</p>' +
    '<div class="mm-media-grid" id="mm-media-grid"></div>' +
    '<a class="btn mm-media-open" id="mm-media-open" href="' + esc(base) + '" target="_blank" rel="noopener">' +
      esc(t("Open the full library", "mediaOpenAll")) + '</a>' +
    '<span class="mm-media-note" id="mm-media-note"></span>' +
  '</section>';
}

// Populate the poster grid from the owner's published design. A failure here
// must never block access: the guest keeps their internet either way.
async function loadMediaTiles() {
  var grid = document.getElementById("mm-media-grid");
  if (!grid) return;
  var note = document.getElementById("mm-media-note");
  if (!DEVICE) { grid.innerHTML = ""; return; }
  try {
    var res = await fetch(API + "/portal/" + encodeURIComponent(DEVICE) + "/media", {
      headers: { "Accept": "application/json" }
    });
    // 404 simply means this business has not published a library. The guest
    // keeps their internet either way, so this is not an error worth showing.
    if (!res.ok) { grid.innerHTML = ""; return; }
    var data = await res.json();
    var design = (data && data.design) || {};
    var tiles = design.tiles || {};
    var keys = Object.keys(tiles);
    if (!keys.length) { grid.innerHTML = ""; return; }

    // Posters can be absent; the owner may publish titles with no artwork.
    grid.innerHTML = keys.slice(0, 18).map(function (key) {
      var tile = tiles[key] || {};
      var img = tile.image
        ? '<img class="mm-media-img" src="' + esc(tile.image) + '" alt="" loading="lazy" decoding="async">'
        : '<span class="mm-media-img mm-media-img-empty" aria-hidden="true"></span>';
      var cap = tile.caption ? '<span class="mm-media-cap">' + esc(tile.caption) + '</span>' : "";
      return '<figure class="mm-media-card">' + img +
        '<figcaption>' + esc(tile.title || "") + cap + '</figcaption></figure>';
    }).join("");

    // The poster images are catalogue artwork only. Playback always happens on
    // the owner's own server, reached by the button above, which already carries
    // the configured host and port.
    if (note) note.textContent = t("Choose a film on the next screen.", "mediaOpenHint");
  } catch (e) {
    grid.innerHTML = "";
    if (note) note.textContent = t("The library could not be loaded. The link above still works.", "mediaLoadFailed");
  }
}

function setVoucherPending(pending) {
  VOUCHER_PENDING = pending;
  var button = document.getElementById("voucher-submit");
  var input = document.getElementById("voucher-code");
  var payment = document.getElementById("portal-go-payment");
  var language = document.getElementById("lang");
  if (button) {
    button.disabled = pending;
    button.textContent = pending ? t("Activating…", "activating") : t("Connect", "voucherConnect");
  }
  if (input) input.readOnly = pending;
  if (payment) payment.disabled = pending;
  if (language) language.setAttribute("aria-disabled", String(pending));
}

async function redeemVoucher(cfg) {
  var input = document.getElementById("voucher-code");
  var button = document.getElementById("voucher-submit");
  var msg = document.getElementById("voucher-msg");
  if (!input || !button || !msg || VOUCHER_PENDING || button.disabled) return;
  var code = input.value.trim().replace(/\s+/g, "").toUpperCase();
  input.value = code;
  msg.className = "voucher-msg msg";
  if (!code) { msg.textContent = t("Enter your voucher code.", "voucherRequired"); input.focus(); return; }
  var consent = portalConsent(cfg);
  if (consent === null) return;
  if (PREVIEW) {
    msg.className = "voucher-msg ok";
    msg.textContent = t("Preview only — a valid voucher connects this device.", "voucherPreview");
    return;
  }
  var mac = clientMac();
  if (!DEVICE || !mac) {
    msg.textContent = t("Open this page from the hotspot login screen, then try again.", "hotspotContext");
    return;
  }
  setVoucherPending(true);
  msg.textContent = t("Keep this page open while your voucher is checked.", "voucherChecking");
  try {
    var payload = { client_mac: mac, code: code, consent: consent };
    if (CLIENT_IP) payload.client_ip = CLIENT_IP;
    var redeemPath = cfg.guest_voucher_access === true ? "/guest-voucher" : "/redeem";
    var res = await fetch(API + "/portal/" + encodeURIComponent(DEVICE) + redeemPath, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-MM-CSRF": "1" },
      body: JSON.stringify(payload)
    });
    var data = await res.json().catch(function () { return {}; });
    if (!button.isConnected) return;
    if (!res.ok) throw new Error(data.error || ("http-" + res.status));
    var verifiedGuestAccess = cfg.guest_voucher_access === true && data.product === "guest-internet" && data.state === "active" &&
      new Date(data.expires_at).getTime() > Date.now() && Number.isInteger(data.data_mb) && data.data_mb >= 1 && data.data_mb <= 1048576;
    if (cfg.guest_voucher_access === true && !verifiedGuestAccess) throw new Error("Gateway access verification incomplete. Contact the hotspot owner.");
    applyBranding(cfg);
    RERENDER = null;
    document.getElementById("root").innerHTML =
      '<div class="pass">' +
      (verifiedGuestAccess ? '<p class="pass-state"><i></i>' + t("Pass active", "passActive") + '</p>' : '') +
      '<div class="head"><h1 class="ok">' + (verifiedGuestAccess ? t("Voucher accepted", "voucherAccepted") : esc("Voucher recorded")) + '</h1>' +
      '<p class="welcome">' + (verifiedGuestAccess ? t("Voucher activated for this device. Open a website to check your internet access. If you are asked to sign in again, use the same code.", "voucherActivated") : esc("Gateway internet access has not been verified. Contact the hotspot owner before using another code.")) + '</p></div>' +
      (verifiedGuestAccess ? '<div class="pass-clock"><span>' + esc(t("Time left", "timeLeft")) + '</span><b id="pass-left"></b></div>' +
        '<dl class="pass-rows"><div><dt>' + esc(t("Valid until: ", "voucherExpiry")) + '</dt><dd>' + esc(new Date(data.expires_at).toLocaleString()) + '</dd></div>' +
        '<div><dt>' + esc(t("Total data allowance: ", "voucherAllowance")) + '</dt><dd>' + esc(data.data_mb) + ' MB</dd></div></dl>' : '') +
      mediaPanel(cfg) +
      (cfg.terms_text ? '<p class="terms">' + esc(cfg.terms_text) + '</p>' : '') + '</div>';
    if (verifiedGuestAccess) { loadMediaTiles(); tickPass(new Date(data.expires_at).getTime()); }
  } catch (e) {
    if (msg.isConnected) msg.textContent = (e.message || t("Could not redeem this voucher.", "voucherFailed")) + ' ' +
      t("Retry the same code. If access is still unavailable, ask the hotspot owner before using another voucher.", "voucherRetry");
  } finally {
    setVoucherPending(false);
  }
}

async function buy(cfg, key) {
  var root = document.getElementById("root");
  var consent = portalConsent(cfg);
  if (consent === null) return;
  // Preview must not create real purchase requests on the owner's queue.
  var plan = (cfg.guest_plans || []).filter(function (p) { return p.key === key; })[0] || null;
  if (PREVIEW) { renderPay(cfg, "preview-request", new Date(Date.now() + (cfg.guest_grace_minutes || 10) * 60000).toISOString(), plan); return; }
  if (!DEVICE || !clientMac()) {
    root.innerHTML = '<p class="msg">' + t("Open this page from the hotspot login screen, then try again.", "hotspotContext") + '</p>';
    return;
  }
  var fd = new FormData();
  fd.append("plan_key", key);
  fd.append("client_mac", clientMac());
  fd.append("consent", String(consent));
  try {
    var res = await fetch(API + "/portal/" + DEVICE + "/guest-purchase", { method: "POST", headers: { "X-MM-CSRF": "1" }, body: fd });
    var d = await res.json();
    if (!res.ok) throw new Error(d.error || ("http-" + res.status));
    renderPay(Object.assign({}, cfg, { guest_grace_data_mb: d.grace_data_mb, guest_grace_minutes: d.grace_minutes }), d.request_id, d.grace_expires_at, plan);
  } catch (e) {
    root.innerHTML = '<p class="msg">' + esc(e.message) + '</p>';
  }
}

function renderPay(cfg, reqId, graceExp, plan) {
  applyBranding(cfg);
  RERENDER = function () { renderPay(cfg, reqId, graceExp, plan); };
  var end = new Date(graceExp).getTime();
  var canCopy = !!(navigator.clipboard && navigator.clipboard.writeText);
  function row(labelEn, labelKey, value, copyable) {
    return '<div><dt>' + t(labelEn, labelKey) + '</dt><dd class="mono">' + esc(value) + '</dd>' +
      (copyable && canCopy ? '<button class="btn btn-quiet" type="button" data-copy="' + esc(value) + '">' + t("Copy", "copy") + '</button>' : '<span></span>') + '</div>';
  }
  var hasBank = cfg.bank_name || cfg.bank_holder || cfg.bank_account;
  document.getElementById("root").innerHTML =
    '<div class="head"><h1>' + t("Pay by bank transfer", "pay") + '</h1>' +
    '<p class="welcome">' + t("Scan the QR and complete the bank transfer before the trial ends.", "trial") + '</p></div>' +
    flow(2) +
    (plan && plan.amount ? '<div class="due"><span>' + t("Amount to pay", "amountDue") + '</span><b>' + esc(money(plan.amount)) + ' <small>MMK</small></b></div>' : '') +
    (cfg.mmqr_url ? '<div class="qr"><img src="' + esc(cfg.mmqr_url) + '" alt="MMQR"></div>' : '') +
    (hasBank ? '<dl class="bank-rows">' +
      (cfg.bank_name ? row("Bank", "bankName", cfg.bank_name, false) : '') +
      (cfg.bank_holder ? row("Name", "holderName", cfg.bank_holder, false) : '') +
      (cfg.bank_account ? row("Account number", "accountNo", cfg.bank_account, true) : '') + '</dl>' : '') +
    (cfg.payment_instructions ? '<p class="pay-note">' + esc(cfg.payment_instructions) + '</p>' : '') +
    '<div class="grace"><span class="mute">' + t("Temporary internet ends in", "trialEndsIn") + '</span><p class="timer" id="timer"></p>' +
    '<p class="grace-note" id="grace-allowance">' + (LANG === "my" ? 'ယာယီအင်တာနက်: ' : 'Temporary internet: ') + esc(cfg.guest_grace_data_mb || 50) + ' MB · ' + esc(cfg.guest_grace_minutes || 10) + (LANG === "my" ? ' မိနစ်။ အချိန် သို့မဟုတ် ဒေတာ ကန့်သတ်ချက် ပြည့်လျှင် ရပ်ပါမည်။' : ' minutes. Access ends when either limit is reached. Uploads and downloads count.') + '</p></div>' +
    '<h3>' + t("After paying, confirm here", "afterPay") + '</h3>' +
    '<div class="field"><label for="phone">' + t("Phone number you paid from", "phone") + '</label><input id="phone" type="tel" inputmode="tel" autocomplete="tel"></div>' +
    '<div class="field"><label for="last6">' + t("Last 6 digits of the bank / transaction number", "last6") + '</label><input id="last6" class="mono" inputmode="numeric" maxlength="6" autocomplete="off"></div>' +
    '<div class="field"><span class="lbl">' + t("Receipt photo (optional)", "receipt") + '</span>' +
      '<label class="file" for="receipt"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 8a2 2 0 0 1 2-2h2l1.2-1.6h5.6L16 6h2a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2zM12 16a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7z"/></svg><span id="receipt-name">' + t("Choose photo", "pickPhoto") + '</span><input id="receipt" type="file" accept="image/*"></label></div>' +
    '<button class="btn" id="confirm" type="button">' + t("I have paid — verify", "verConfirm") + '</button>' +
    '<p class="msg" id="msg" role="status"></p>' +
    (cfg.terms_text ? '<p class="terms">' + esc(cfg.terms_text) + '</p>' : '');
  tickTimer(end);
  document.getElementById("confirm").addEventListener("click", function () { confirm(reqId); });
  var receipt = document.getElementById("receipt");
  receipt.addEventListener("change", function () {
    var f = receipt.files && receipt.files[0];
    document.getElementById("receipt-name").textContent = f ? f.name : t("Choose photo", "pickPhoto");
  });
  document.querySelectorAll("[data-copy]").forEach(function (b) {
    b.addEventListener("click", function () {
      navigator.clipboard.writeText(b.getAttribute("data-copy")).then(function () {
        b.textContent = t("Copied", "copied");
        setTimeout(function () { if (b.isConnected) b.textContent = t("Copy", "copy"); }, 1600);
      }, function () {});
    });
  });
}

function fmtClock(ms) {
  var total = Math.max(0, Math.floor(ms / 1000)), h = Math.floor(total / 3600), m = Math.floor(total % 3600 / 60), sec = total % 60;
  return (h > 0 ? h + ":" + (m < 10 ? "0" : "") : "") + m + ":" + (sec < 10 ? "0" : "") + sec;
}
var PASS_TIMER = null;
/** Counts the active pass down to its expiry; stops when the screen is replaced. */
function tickPass(end) {
  clearInterval(PASS_TIMER);
  var el = document.getElementById("pass-left");
  if (!el) return;
  var fn = function () {
    if (!el.isConnected) { clearInterval(PASS_TIMER); return; }
    var diff = end - Date.now();
    if (diff <= 0) { el.textContent = t("Pass ended", "passEnded"); clearInterval(PASS_TIMER); return; }
    el.textContent = fmtClock(diff);
  };
  PASS_TIMER = setInterval(fn, 1000); fn();
}

function tickTimer(end) {
  clearInterval(GRACE_TIMER);
  var timerEl = document.getElementById("timer");
  if (!timerEl) return;
  var fn = function () {
    var diff = end - Date.now();
    if (!timerEl.isConnected) { clearInterval(GRACE_TIMER); return; }
    if (diff <= 0) { timerEl.textContent = t("Trial expired", "expired"); clearInterval(GRACE_TIMER); return; }
    var m = Math.floor(diff/60000), s = Math.floor(diff%60000/1000);
    timerEl.textContent = (m<10?"0":"")+m+":"+(s<10?"0":"")+s;
  };
  GRACE_TIMER = setInterval(fn, 1000); fn();
}

async function confirm(reqId) {
  var msg = document.getElementById("msg");
  var phone = document.getElementById("phone").value.trim();
  var last6 = document.getElementById("last6").value.trim();
  if (!phone || last6.length !== 6) {
    msg.textContent = t("Enter the phone and the 6-digit transaction number.", "inputErr");
    return;
  }
  if (PREVIEW) { msg.textContent = t("Preview only — nothing was submitted.", "pvInert"); return; }
  var fd = new FormData();
  fd.append("paid_phone", phone);
  fd.append("tx_last6", last6);
  var rf = document.getElementById("receipt").files[0];
  if (rf) fd.append("receipt", rf);
  msg.textContent = "…";
  try {
    var res = await fetch(API + "/portal/guest-purchase/" + reqId + "/receipt", { method: "POST", headers: { "X-MM-CSRF": "1" }, body: fd });
    var d = await res.json();
    if (!res.ok) throw new Error(d.error || ("http-" + res.status));
    document.getElementById("root").innerHTML = flow(4) +
      '<div class="head"><h1 class="ok">' + t("Request received", "requestRec") + '</h1>' +
      '<p class="ok">' + t("The owner has been notified. Temporary access ends when its time or data allowance runs out; paid access starts after approval.", "waiting") + '</p></div>';
  } catch (e) {
    msg.textContent = t("Could not submit", "submitErr") + ": " + e.message;
  }
}

/* --- preview bridge ------------------------------------------------------ */

// Same-origin only: a preview frame must not accept a config pushed by any
// other page. The parent answers our "ready" ping so the first paint is never
// lost to a race with its own load.
function wirePreview() {
  var badge = document.createElement("div");
  badge.className = "pv-badge";
  badge.textContent = "PREVIEW";
  document.body.appendChild(badge);
  document.getElementById("root").innerHTML =
    '<p class="msg" id="load">' + t("Waiting for the console…", "pvWaiting") + '</p>';

  window.addEventListener("message", function (ev) {
    if (ev.origin !== location.origin) return;
    var d = ev.data;
    if (!d || d.type !== "mm-portal-preview" || !d.config) return;
    if (d.lang === "my" || d.lang === "en") { LANG = d.lang; applyLang(); }
    // The console's "Bank transfer page" tab asks for the plan picker, as the router's pay link does.
    if (d.start === "pay" && accessMethodEnabled(d.config, "mobile_payment_enabled")) renderPlans(d.config); else renderPortalHome(d.config);
  });
  if (window.parent && window.parent !== window) {
    window.parent.postMessage({ type: "mm-portal-preview-ready" }, location.origin);
  }
}

if (PREVIEW) {
  applyLang();
  var langElPv = document.getElementById("lang");
  langElPv.addEventListener("click", toggleLang);
  langElPv.style.display = "none";   // language is driven by the console in preview
  wirePreview();
} else if (!DEVICE) {
  document.getElementById("root").innerHTML = '<p class="msg">' + t("Missing deviceId in the URL.", "missingDevice") + '</p>';
} else {
  applyLang();
  var langEl = document.getElementById("lang");
  langEl.addEventListener("click", toggleLang);
  langEl.addEventListener("keydown", function (e) { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); toggleLang(); } });
  loadConfig();
}
