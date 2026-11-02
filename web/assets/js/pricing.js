/* Prices come from GET /api/v1/pricing (the platform price list), never from this
   file. If the API cannot be reached we say so and leave the figures blank:
   a pricing page must never show a number the product did not supply. */
(function () {
  var dash = "—", last = null, failed = false;
  function mmk(n) { return typeof n === "number" ? n.toLocaleString("en-US") : dash; }
  function setText(id, v) { var el = document.getElementById(id); if (el) { el.textContent = v; el.classList.toggle("dash", v === dash); } }
  function row(a, b) { var r = document.createElement("div"), x = document.createElement("span"), y = document.createElement("span"); x.textContent = a; y.textContent = b; r.appendChild(x); r.appendChild(y); return r; }
  function notice(text, warn) { var d = document.createElement("div"); d.className = "notice" + (warn ? " warn" : ""); d.textContent = text; document.getElementById("notice-slot").appendChild(d); }
  function render() {
    var slotN = document.getElementById("notice-slot"), slotP = document.getElementById("promo-slot"), rows = document.getElementById("tier-rows"), save = document.getElementById("annual-save"), src = document.getElementById("source-line");
    slotN.textContent = ""; slotP.textContent = ""; save.hidden = true; src.textContent = "";
    if (failed) {
      notice(mmT("Prices could not be loaded. Refresh this page, or sign in to see your plans.", "စျေးနှုန်းများ မရယူနိုင်ပါ။ စာမျက်နှာကို ပြန်လည်ဖွင့်ပါ သို့မဟုတ် ဝင်ပြီး သင့်အစီအစဉ်များကို ကြည့်ပါ။"), true);
      src.textContent = mmT("Live price list unavailable.", "တိုက်ရိုက် စျေးနှုန်းစာရင်း မရနိုင်ပါ။");
      return;
    }
    var d = last; if (!d) return;
    var monthly = (d.plans && d.plans.monthly) || {}, annual = (d.plans && d.plans.annual) || {};
    setText("monthly-price", mmk(monthly.standard_mmk));
    setText("annual-price", mmk(annual.standard_mmk));
    var saving = annual.saving_vs_monthly_mmk;
    if (typeof saving === "number" && saving > 0) {
      save.textContent = mmT("Saves " + mmk(saving) + " MMK against twelve monthly payments.", "လစဉ် ၁၂ ကြိမ်ပေးသည်ထက် " + mmk(saving) + " MMK သက်သာသည်။");
      save.hidden = false;
    }
    rows.textContent = "";
    if (d.account_policy && d.account_policy.mode === "individual_pass") {
      rows.appendChild(row(mmT("Authorized login", "ခွင့်ပြုထားသော login"), "1"));
      rows.appendChild(row(mmT("Site and RouterBOARD", "ဆိုက်နှင့် RouterBOARD"), "1 / 1"));
      rows.appendChild(row(mmT("Vouchers, AI and Jellyfin", "ဘောက်ချာ၊ AI နှင့် Jellyfin"), mmT("No app usage quota", "အသုံးပြုမှု ကန့်သတ်ချက် မရှိ")));
      notice(mmT("The pass starts when the platform approves it.", "Pass သည် platform အတည်ပြုချိန်မှ စတင်သည်။"), false);
    } else if (Array.isArray(d.tiers) && d.tiers.length) {
      d.tiers.forEach(function (t) { var q = t.device_quota; rows.appendChild(row(t.label || t.key || dash, q === null || q === undefined ? dash : q.toLocaleString("en-US"))); });
    } else { rows.appendChild(row(dash, dash)); }
    var p = d.promo || {};
    if (p.live) {
      var box = document.createElement("div"); box.className = "promo";
      box.textContent = (p.label || mmT("Current offer", "လက်ရှိ ကမ်းလှမ်းချက်")) + (p.ends_on ? " · " + mmT("until ", "") + p.ends_on + mmT("", " အထိ") : "");
      slotP.appendChild(box);
    }
    if (d.currency && d.currency !== "MMK") notice(mmT("Prices are quoted in ", "စျေးနှုန်းများကို ") + d.currency + mmT(".", " ဖြင့် ဖော်ပြထားသည်။"), false);
    if (d.generated_at) src.textContent = mmT("Prices updated ", "စျေးနှုန်း ပြင်ဆင်ချိန် ") + d.generated_at;
  }
  document.addEventListener("mm-lang", render);
  fetch("/api/v1/pricing", { headers: { "Accept": "application/json" } })
    .then(function (r) { if (!r.ok) throw new Error("HTTP " + r.status); return r.json(); })
    .then(function (d) { last = d; render(); })
    .catch(function () { failed = true; render(); });
})();
