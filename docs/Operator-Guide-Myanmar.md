# Mega Magic — အော်ပရေတာ လမ်းညွှန် (Myanmar Operator Guide)

**ဘာသာစကား:** မြန်မာ (my-MM) · **ဗားရှင်း:** လက်ရှိ build (C1+C2+C3 + E2 ပြီးစီး)
**ပရိသတ်:** Wi-Fi / guest-portal လုပ်ငန်းရှင် (ဥပမာ — Alpha WISP test tenant, `nish@magic.com`; device `CCR2004`)

> ⚠️ **Native-review လိုအပ်သည်။** ဤစာတမ်းရှိ မြန်မာစာသားများကို နည်းပညာဝေါဟာရများအတွက် English အတိအကျ ထားရှိပြီး ရေးသားထားသည်။ မြန်မာစာ သဘာဝကျမှုကို native speaker တစ်ဦးမှ အတည်ပြုပေးပါရန်။ အောက်ဆုံးတွင် **flagged strings** စာရင်းထားသည် — မှားနိုင်သည်ဟု သံသယရှိသော စာကြောင်းများ။

**ဝေါဟာရ သတ်မှတ်ချက် (D2):** MMQR, RouterOS, backup, restore, plan, OTP, Hotspot, owner, dashboard → English အတိုင်း ထားသည်။

---

## ၁။ ပထမဆုံး Login ဝင်ခြင်း နှင့် Dashboard

1. Browser တွင် `https://mikromagic.com` ကို ဖွင့်ပါ။
2. `Sign in` ကို နှိပ်ပြီး သင့် email (`nish@magic.com`) နှင့် password ထည့်ပါ။
3. Login ဝင်ပြီးပါက ဘေးဘက် (left rail) menu တွင် အောက်ပါအရာများ ပေါ်လာမည်။

| Menu | ဘာလုပ်ပေးသည် |
|---|---|
| **Overview** | fleet အနေအထား အနှစ်ချုပ် — peer အရေအတွက်၊ up / stale / offline |
| **Fleet health** | device (peer) တစ်ခုချင်းစီ၏ state |
| **Sites** | router များ (device) စာရင်း |
| **Topology** | network map (router ဆက်စပ်ပုံ) |
| **Incidents** | ပြဿနာများ (AI နှင့် policy alert) |
| **Security** | RouterOS version နှင့် security posture (affected / safe / unknown) |
| **Backups** | config backup, change history နှင့် restore |
| **Vouchers** | portal အတွက် voucher code များ |
| **Captive portal** | guest Wi-Fi configuration နှင့် purchase request များ |
| **Team** | ဝန်ထမ်းများနှင့် role စီမံခန့်ခွဲမှု |
| **Sign out** | ထွက်ရန် |

> မှတ်ချက် — `Platform` menu ကို **DEVELOPER** (ပလက်ဖောင်း owner) သာ တွေ့မြင်နိုင်သည်။

---

## ၂။ နေ့စဉ် လုပ်ငန်းတာဝန်များ (Guest Purchase / Sales / Monitoring)

### ၂.၁ Guest ဝယ်ယူမှု အတည်ပြုခြင်း (MMQR)

1. Guest တစ်ဦးသည် captive portal တွင် plan ရွေးပြီး **MMQR** ဖြင့် ငွေလွှဲမည်။
2. ငွေလွှဲပြီးနောက် guest သည် ဖုန်းနံပါတ်နှင့် transaction နောက်ဆုံး ၆ လုံး ထည့်သွင်းမည်။
3. သင့် telegram bot တွင် **Approve / Reject** button ဖြင့် အကြောင်းကြားလာပါမည်။
4. **Approve** နှိပ်လိုက်ပါက guest ၏ pass ကို router တွင် အလိုအလျောက် တိုးပေးမည် (data + time limit)။
5. Web dashboard မှလည်း `Captive portal` → **Guest purchase requests** တွင် အတည်ပြု / ငြင်းနိုင်သည်။

### ၂.၂ Plan ရောင်းအား ကြည့်ခြင်း

- `Captive portal` တွင် guest request များ (MAC, plan, status, အချိန်) ကို ကြည့်နိုင်သည်။
- Voucher များကို `Vouchers` menu တွင် ကြည့်နိုင်သည် (code, plan, ပမာဏ, status)။

### ၂.၃ Device အနေအထား စောင့်ကြည့်ခြင်း

- `Fleet health` တွင် peer တစ်ခုချင်းစီ၏ state (up / stale / offline) ကို ကြည့်ပါ။
- `Overview` တွင် စုစုပေါင်း peer၊ up၊ stale၊ offline အရေအတွက် ကို တစ်ချက်တည်း ကြည့်နိုင်သည်။

---

## ၃။ Device များ စီမံခန့်ခွဲခြင်း

### ၃.၁ Security posture စစ်ဆေးခြင်း

1. `Security` menu ကို ဖွင့်ပါ။
2. ကွက်လပ် (stat) များတွင် **Affected / Safe / Unknown** အရေအတွက် ပေါ်မည်။
3. တစ်ခုချင်းစီ၏ RouterOS version နှင့် posture ကို ဇယားတွင် ကြည့်ပါ။
4. **Affected** ဖြစ်နေပါက RouterOS ကို update လုပ်ရန် စီစဉ်ပါ။

> "Affected" ဆိုသည်မှာ ကျွန်ုပ်တို့ ခြေရာခံထားသော security advisory တစ်ခုခုနှင့် ကိုက်ညီခြင်း ဖြစ်သည်။ "Unknown" ဆိုသည်မှာ version ကို မဖတ်နိုင်သေးခြင်း ဖြစ်သည် (safe ဟု မမှတ်ယူရ)။

### ၃.၂ Config Backup ယူခြင်း (C2)

1. `Backups` menu ကို ဖွင့်ပါ။
2. Device ကို ရွေးပါ။
3. **Capture backup** ကို နှိပ်ပါ။
4. Backup တစ်ခု ဖန်တီးပြီးပါက ဇယားတွင် (label, kind, size, အချိန်) ပေါ်လာမည်။

> Backup ယူခြင်းသည် device ကို SSH ဖြင့် ချိတ်ဆက်ရသည်။ Live applier ပိတ်ထားပါက (fail-closed) ၎င်းသည် 503 ပြမည်။

### ၃.၃ Change History ကြည့်ခြင်း

- `Backups` menu အောက်ရှိ **Change history** ဇယားတွင် (အချိန်, role, action, reason) ကို ကြည့်နိုင်သည်။

### ၃.၄ One-click Restore (C2)

1. `Backups` ဇယားတွင် backup တစ်ခု၏ **Restore** ကို နှိပ်ပါ။
2. **OTP** ထည့်ရန် တောင်းဆိုမည် (high-risk action)။
3. OTP မှန်ပါက device ကို ထို backup အနေအထားသို့ ပြန်လည် ရောက်စေမည်။

> Restore သည် OTP-gated ဖြစ်သည်။ OTP မရှိပါက ငြင်းပယ်မည် (fail-closed)။

---

## ၄။ အဖွဲ့နှင့် Role များ (C3)

| Role | အခွင့်အရေး (အနှစ်ချုပ်) |
|---|---|
| **OWNER** | အမြင့်ဆုံး — team အဖွဲ့ဝင် ထည့် / role ပြောင်း / ပယ်ဖျက် နိုင်သည် |
| **ADMIN** | webhook, backup capture/restore အပါအဝင် စီမံခန့်ခွဲမှု |
| **OPERATOR** | နေ့စဉ် လုပ်ငန်း — guest purchase အတည်ပြု/ငြင်း |
| **TECHNICIAN** | နည်းပညာ လုပ်ငန်းများ |
| **SUPPORT** | guest အကူအညီ |
| **VIEWER** | ကြည့်ရုံသာ |
| **DEVELOPER** | ပလက်ဖောင်း owner သာ (cross-tenant `Platform` console) — tenant အဖွဲ့ဝင်အဖြစ် ပေးအပ်ခွင့် မရှိ |

**Team စီမံခန့်ခွဲခြင်း (OWNER သာ):** `Team` menu တွင် —
- **Add member** — name, email, password (၈ လုံးအထက်), role ထည့်ပါ။
- Role ပြောင်းလိုပါက ဇယားထဲတွင် ရွေး၍ **Save** နှိပ်ပါ။
- **Disable** ဖြင့် ဝန်ထမ်းကို ပိတ်နိုင်သည်။

> **အရေးကြီး:** နောက်ဆုံး OWNER ကို role ပြောင်းခြင်း သို့မဟုတ် ပိတ်ခြင်း မပြုနိုင်ပါ (စနစ်က ကာကွယ်ထားသည်)။

---

## ၅။ Billing / Treasury အခြေခံ

- သင့် plan / quota နှင့် ပတ်သက်သော အချက်အလက်များကို `Captive portal` နှင့် `Vouchers` တွင် ကြည့်နိုင်သည်။
- ငွေလွှဲစစ်ဆေးမှုနှင့် အတည်ပြုမှုများကို **MMQR** flow ဖြင့် လုပ်ဆောင်သည်။
- Voucher တစ်ခုချင်းစီတွင် price ပါဝင်သဖြင့် ရောင်းအားကို တွက်ချက်နိုင်သည်။

---

## ၆။ ပြဿနာများ ဖြေရှင်းခြင်း (Troubleshooting)

### ၆.၁ Device offline ဖြစ်နေသည်
- `Fleet health` တွင် state ကို စစ်ပါ။ device မှာ power / internet ရှိမရှိ စစ်ပါ။
- ပြန်လည် ချိတ်ဆက်မိပါက state သည် `up` သို့ ပြန်ဖြစ်မည်။

### ၆.၂ Guest ချိတ်ဆက်နိုင်ခြင်း မရှိသည်
- `Captive portal` တွင် guest ၏ request status ကို စစ်ပါ (pending / approved / rejected)။
- pending ဖြစ်နေပါက **Approve** လုပ်ပေးပါ။

### ၆.၃ ငွေလွှဲမှု (Payment) pending ဖြစ်နေသည်
- guest ထည့်သော ဖုန်းနံပါတ်နှင့် transaction နောက်ဆုံး ၆ လုံး မှန်မမှန် စစ်ပါ။
- မှန်ပါက Approve လုပ်ပါ။ မှားနေပါက Reject လုပ်ပြီး guest ကို ပြန်ထည့်ခိုင်းပါ။

### ၆.၄ Restore မအောင်မြင်ခြင်း
- OTP မှန်မမှန် စစ်ပါ။
- Live applier ပိတ်ထားပါက restore 503 ပြမည် — ဆက်သွယ်ရန် လိုအပ်နိုင်သည်။

---

## Flagged strings for NATIVE review

အောက်ပါ မြန်မာစာသားများကို native speaker ဖြင့် အတည်ပြုပေးပါ။ အကြောင်းအရာ မှန်ပါသည်၊ စကားအသုံးအနှုန်း သဘာဝကျမှုသာ စစ်ရန်:

1. "ငွေလွှဲပြီးနောက် guest သည် ဖုန်းနံပါတ်နှင့် transaction နောက်ဆုံး ၆ လုံး ထည့်သွင်းမည်။"
2. "သင့် telegram bot တွင် ... အကြောင်းကြားလာပါမည်။" ✅ NATIVE-CORRECTED (owner Telegram bot → your Telegram bot)
3. "Approve နှိပ်လိုက်ပါက guest ၏ pass ကို router တွင် အလိုအလျောက် တိုးပေးမည် (data + time limit)။"
4. "'Affected' ဆိုသည်မှာ ကျွန်ုပ်တို့ ခြေရာခံထားသော security advisory တစ်ခုခုနှင့် ကိုက်ညီခြင်း ဖြစ်သည်။"
5. "Backup ယူခြင်းသည် device ကို SSH ဖြင့် ချိတ်ဆက်ရသည်။"
6. "OTP မှန်ပါက device ကို ထို backup အနေအထားသို့ ပြန်လည် ရောက်စေမည်။"
7. "နောက်ဆုံး OWNER ကို role ပြောင်းခြင်း သို့မဟုတ် ပိတ်ခြင်း မပြုနိုင်ပါ။"
8. "Live applier ပိတ်ထားပါက (fail-closed) ၎င်းသည် 503 ပြမည်။"

> မှတ်ချက် — ဤစာတမ်းကို non-native ရေးသားထားသည်။ Technical token များကို English အတိုင်း ထားရှိပြီး၊ သံသယရှိသော စာကြောင်းများကို အထက်တွင် စာရင်းပြုထားသည်။ မည်သည့် စကားလုံးကိုမျှ ခန့်မှန်း transliteration မပြုလုပ်ထားပါ။
