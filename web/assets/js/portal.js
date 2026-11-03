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
  requestRec:"လက်ခံရရှိပါပြီ — owner က မကြာမီ အတည်ပြုပါလိမ့်မည်"  // [typo fixed; native review]
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

/** Pick a readable foreground for a brand background, so a light owner colour
 *  does not end up as white-on-yellow. */
function readableOn(hex) {
  var h = hex.replace("#", "");
  if (h.length === 3 || h.length === 4) { h = h[0]+h[0]+h[1]+h[1]+h[2]+h[2]; }
  var r = parseInt(h.substring(0,2),16), g = parseInt(h.substring(2,4),16), b = parseInt(h.substring(4,6),16);
  var lum = (0.299*r + 0.587*g + 0.114*b) / 255;
  return lum > 0.62 ? "#111" : "#fff";
}

function applyBranding(cfg) {
  var root = document.documentElement, s = root.style;
  var brand = safeHex(cfg.primary_hex, "#0e5a5a");
  var tint  = safeHex(cfg.glass_tint_hex, "#7ad0ff");
  s.setProperty("--brand", brand);
  s.setProperty("--brand-ink", readableOn(brand));
  s.setProperty("--tint", tint);
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
  return '<h1>' + t(titleEnglish, titleKey) + '</h1>' +
    (cfg.business_name ? '<p class="biz">' + esc(cfg.business_name) + '</p>' : '') +
    (cfg.welcome_text ? '<p class="welcome">' + esc(cfg.welcome_text) + '</p>' : '');
}

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
      '<div class="voucher-row"><input id="voucher-code" inputmode="text" autocomplete="one-time-code" autocapitalize="characters" spellcheck="false" maxlength="64" placeholder="Voucher code" aria-label="Voucher code">' +
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
    var amt = p.amount ? " · " + p.amount.toLocaleString("en") + " MMK" : "";
    var spd = p.speed_down_mbps ? " · " + p.speed_down_mbps + " Mbps" : "";
    return '<button class="plan" data-key="' + esc(p.key) + '">' +
      '<div class="nm">' + esc(p.label) + '</div>' +
      '<div class="sub">' + esc(mins) + esc(dm) + esc(spd) + amt + '</div></button>';
  }).join("");
  document.getElementById("root").innerHTML =
    portalHeading(cfg, "buy", "Buy WiFi") +
    '<p class="steps">' + t("1) Choose a plan · 2) Pay by bank transfer (free trial) · 3) Enter phone + last 6 digits", "steps") + '</p>' +
    '<h3>' + t("Choose a plan", "choose") + '</h3>' +
    (PREVIEW && cfg.preview_sample ? '<p class="pv-sample-note">Sample preview data — add a plan to replace it.</p>' : '') +
    (rows || '<p class="msg">' + t("No plans are available yet.", "noPlans") + '</p>') +
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
      '<h1 class="ok">' + (verifiedGuestAccess ? t("Voucher accepted", "voucherAccepted") : esc("Voucher recorded")) + '</h1>' +
      '<p class="welcome">' + (verifiedGuestAccess ? t("Voucher activated for this device. Open a website to check your internet access. If you are asked to sign in again, use the same code.", "voucherActivated") : esc("Gateway internet access has not been verified. Contact the hotspot owner before using another code.")) + '</p>' +
      (verifiedGuestAccess ? '<p class="steps">' + esc(t("Valid until: ", "voucherExpiry")) + esc(new Date(data.expires_at).toLocaleString()) + '<br>' + esc(t("Total data allowance: ", "voucherAllowance")) + esc(data.data_mb) + ' MB</p>' : '') +
      mediaPanel(cfg) +
      (cfg.terms_text ? '<p class="terms">' + esc(cfg.terms_text) + '</p>' : '');
      if (verifiedGuestAccess) loadMediaTiles();
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
  if (PREVIEW) { renderPay(cfg, "preview-request", new Date(Date.now() + (cfg.guest_grace_minutes || 10) * 60000).toISOString()); return; }
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
    renderPay(Object.assign({}, cfg, { guest_grace_data_mb: d.grace_data_mb, guest_grace_minutes: d.grace_minutes }), d.request_id, d.grace_expires_at);
  } catch (e) {
    root.innerHTML = '<p class="msg">' + esc(e.message) + '</p>';
  }
}

function renderPay(cfg, reqId, graceExp) {
  applyBranding(cfg);
  RERENDER = function () { renderPay(cfg, reqId, graceExp); };
  var end = new Date(graceExp).getTime();
  document.getElementById("root").innerHTML =
    '<h1>' + t("Pay by bank transfer", "pay") + '</h1>' +
    '<p class="steps">' + t("Scan the QR and complete the bank transfer before the trial ends.", "trial") + '</p>' +
    '<div class="qr">' + (cfg.mmqr_url ? '<img src="' + esc(cfg.mmqr_url) + '">' : '') +
      (cfg.bank_name || cfg.bank_holder || cfg.bank_account
        ? '<div class="bank">' + esc(cfg.bank_name||"") + ' · ' + esc(cfg.bank_holder||"") + ' · ' + esc(cfg.bank_account||"") + '</div>' : '') +
    '</div>' +
    (cfg.payment_instructions ? '<p class="steps pay-note">' + esc(cfg.payment_instructions) + '</p>' : '') +
    '<p class="timer" id="timer"></p>' +
    '<p class="steps" id="grace-allowance">' + (LANG === "my" ? 'ယာယီအင်တာနက်: ' : 'Temporary internet: ') + esc(cfg.guest_grace_data_mb || 50) + ' MB · ' + esc(cfg.guest_grace_minutes || 10) + (LANG === "my" ? ' မိနစ်။ အချိန် သို့မဟုတ် ဒေတာ ကန့်သတ်ချက် ပြည့်လျှင် ရပ်ပါမည်။' : ' minutes. Access ends when either limit is reached. Uploads and downloads count.') + '</p>' +
    '<h3>' + t("After paying, confirm here", "afterPay") + '</h3>' +
    '<div class="form-row"><label>' + t("Phone number you paid from", "phone") + '</label><input id="phone" inputmode="tel"></div>' +
    '<div class="form-row"><label>' + t("Last 6 digits of the bank / transaction number", "last6") + '</label><input id="last6" inputmode="numeric" maxlength="6"></div>' +
    '<div class="form-row"><label>' + t("Receipt photo (optional)", "receipt") + '</label><input id="receipt" type="file" accept="image/*"></div>' +
    '<button class="btn" id="confirm">' + t("I have paid — verify", "verConfirm") + '</button>' +
    '<p class="msg" id="msg"></p>' +
    (cfg.terms_text ? '<p class="terms">' + esc(cfg.terms_text) + '</p>' : '');
  tickTimer(end);
  document.getElementById("confirm").addEventListener("click", function () { confirm(reqId); });
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
    document.getElementById("root").innerHTML =
      '<h1 class="ok">' + t("Request received", "requestRec") + '</h1>' +
      '<p class="ok">' + t("The owner has been notified. Temporary access ends when its time or data allowance runs out; paid access starts after approval.", "waiting") + '</p>';
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
    renderPortalHome(d.config);
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
