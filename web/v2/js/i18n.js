// Burmese / English. Burmese wording is a first draft: have a native speaker review.
// RouterOS and product terms (Router, Jellyfin, WebFig, Magic Hub) stay in English.
const dict = {
  en: {
    "tab.home": "Home", "tab.router": "Router", "tab.vouchers": "Vouchers", "tab.jellyfin": "Jellyfin", "tab.more": "More",
    "title.home": "Home", "title.router": "Router", "title.vouchers": "Internet vouchers", "title.jellyfin": "Jellyfin", "title.more": "More", "title.signin": "Sign in",
    "c.loading": "Loading", "c.retry": "Try again", "c.unavailable": "Not available", "c.signOut": "Sign out", "c.openLegacy": "Open in the full app",
    "c.loadFailed": "This page could not be loaded.", "c.sessionEnded": "Your session ended. Please sign in again.",
    "c.language": "Language", "c.online": "Online", "c.offline": "Offline", "c.unknown": "Unknown", "c.none": "None", "c.mmk": "MMK",
    "s.email": "Email", "s.password": "Password", "s.submit": "Sign in", "s.working": "Signing in…",
    "s.failed": "Sign-in failed. Check your email and password.", "s.network": "Could not reach the server. Try again.",
    "h.eyebrow": "Hotspot dashboard", "h.greet": "Hello,", "h.sub": "RouterBoard remote management · voucher income · live status",
    "h.revenue": "Voucher value used", "h.revenueHelp": "Value of vouchers guests started using. It is not cash received.",
    "h.today": "Today", "h.last7": "Last 7 days", "h.last30": "Last 30 days",
    "h.routers": "Routers online", "h.left": "Vouchers left", "h.internet": "Internet", "h.jellyfin": "Jellyfin",
    "h.jellyfinSoon": "Not set up yet", "h.noRouter": "No router added yet. Open Router to add one.", "h.fleetFailed": "Router status is unavailable.",
    "h.reportFailed": "Voucher figures are unavailable.",
    "v.ws.vouchers": "Vouchers", "v.ws.guest": "Guest page",
    "v.t.stock": "Stock", "v.t.create": "Create", "v.t.shops": "Shops", "v.t.print": "Print",
    "v.hint": "Guests buy a code at a shop and enter it on the router's login page to get internet.",
    "v.used": "Used", "v.onRouter": "On router", "v.shop": "Shop", "v.noShop": "No shop assigned", "v.empty": "No voucher batches yet.",
    "v.pendingTitle": "Batches not finished", "v.pendingBody": "These batches are still being installed on the router. Retry to finish them. No new codes are created.",
    "v.retry": "Retry batch", "v.retrying": "Retrying…",
    "v.f.gateway": "Router", "v.f.label": "Plan name", "v.f.duration": "Valid for", "v.f.data": "Data limit (MB)", "v.f.price": "Price (MMK)", "v.f.count": "Number of codes", "v.f.shop": "Shop",
    "v.d.60": "1 hour", "v.d.1440": "1 day", "v.d.10080": "7 days", "v.d.43200": "30 days",
    "v.create": "Create batch", "v.creating": "Installing codes on the router…", "v.noGateway": "Add a main gateway router first.",
    "v.pendingFirst": "Finish the saved batch first, then create another.", "v.ready": "Codes are installed and verified on the router.",
    "v.codesOnce": "Codes are shown only now. Print them before you leave this page.", "v.print": "Print codes", "v.validFirstUse": "Valid from first use",
    "v.notReady": "The batch is not ready yet. Retry from Stock.", "v.createFailed": "The batch could not be created.",
    "v.shops.empty": "No shops yet.", "v.shops.add": "Add shop", "v.shops.name": "Shop name", "v.shops.contact": "Contact name", "v.shops.prefix": "Code prefix (2–4 letters)",
    "v.shops.phone": "Phone", "v.shops.saved": "Shop added.", "v.shops.failed": "The shop could not be saved.", "v.print.help": "Printing and card layout still use the full app until it is rebuilt here.",
    "j.title": "Jellyfin", "j.ws.dash": "Dashboard", "j.ws.vouchers": "Vouchers", "j.ws.guest": "Guest page",
    "j.status": "Media server", "j.notReady": "Jellyfin vouchers are not available yet.",
    "j.notReadyBody": "A Jellyfin voucher lets a guest play movies from your Jellyfin PC only, with no internet. The router rule and server support for this are still being built.",
    "j.dashHelp": "Server connection and library controls move here next. Until then use the full app.",
    "r.empty": "No routers added yet.", "r.lastSeen": "Last seen", "r.never": "never", "r.tools": "Router tools", "r.toolsHelp": "Setup, Wi-Fi, protection and the network map still use the full app until they are rebuilt here.",
    "r.ws.list": "Routers", "r.ws.map": "Map", "map.failed": "The network map could not be loaded.", "map.pick": "Tap a device to see its details.",
    "map.props": "Device details", "map.seen": "How it was seen", "map.devices": "devices seen", "map.empty": "No devices seen on this router yet.", "map.unplaced": "Attachment unknown",
    "map.how.wifi": "Wi-Fi client", "map.how.neighbor": "Neighbour reported", "map.how.learned": "Learned behind", "map.how.unknown": "Seen, port unknown",
    "map.note": "Seen by the router. A forwarding entry does not prove a direct cable, so no switch or hop is invented.",
    "m.account": "Account", "m.pass": "App pass", "m.passHelp": "Plans and add-ons will be listed here.", "m.store": "Store", "m.storeSoon": "The add-on catalog is being built.",
    "m.profile": "Profile", "m.role": "Role",
  },
  my: {
    "tab.home": "ပင်မ", "tab.router": "Router", "tab.vouchers": "ဗောင်ချာ", "tab.jellyfin": "Jellyfin", "tab.more": "နောက်ထပ်",
    "title.home": "ပင်မ", "title.router": "Router", "title.vouchers": "အင်တာနက် ဗောင်ချာ", "title.jellyfin": "Jellyfin", "title.more": "နောက်ထပ်", "title.signin": "ဝင်ရောက်ရန်",
    "c.loading": "ဖွင့်နေသည်", "c.retry": "ထပ်ကြိုးစားမည်", "c.unavailable": "မရနိုင်ပါ", "c.signOut": "ထွက်မည်", "c.openLegacy": "အပြည့်အစုံ app တွင် ဖွင့်မည်",
    "c.loadFailed": "ဤစာမျက်နှာကို မဖွင့်နိုင်ပါ။", "c.sessionEnded": "အချိန်ကုန်သွားပါပြီ။ ထပ်မံဝင်ပါ။",
    "c.language": "ဘာသာစကား", "c.online": "အွန်လိုင်း", "c.offline": "အော့ဖ်လိုင်း", "c.unknown": "မသိရ", "c.none": "မရှိ", "c.mmk": "ကျပ်",
    "s.email": "အီးမေးလ်", "s.password": "စကားဝှက်", "s.submit": "ဝင်မည်", "s.working": "ဝင်နေသည်…",
    "s.failed": "ဝင်၍မရပါ။ အီးမေးလ်နှင့် စကားဝှက်ကို စစ်ပါ။", "s.network": "ဆာဗာသို့ မချိတ်ဆက်နိုင်ပါ။ ထပ်ကြိုးစားပါ။",
    "h.eyebrow": "ဟော့စပေါ့ ဒက်ရှ်ဘုတ်", "h.greet": "မင်္ဂလာပါ,", "h.sub": "Router ဝေးကွာထိန်းချုပ်မှု · ဗောင်ချာ ဝင်ငွေ · လက်ရှိ အခြေအနေ",
    "h.revenue": "အသုံးပြုပြီး ဗောင်ချာ တန်ဖိုး", "h.revenueHelp": "ဧည့်သည်များ စတင်သုံးလိုက်သော ဗောင်ချာတန်ဖိုး ဖြစ်သည်။ လက်ခံရရှိသော ငွေသား မဟုတ်ပါ။",
    "h.today": "ယနေ့", "h.last7": "ရက် ၇ ရက်", "h.last30": "ရက် ၃၀",
    "h.routers": "အွန်လိုင်း Router", "h.left": "ကျန်ရှိသော ဗောင်ချာ", "h.internet": "အင်တာနက်", "h.jellyfin": "Jellyfin",
    "h.jellyfinSoon": "မသတ်မှတ်ရသေးပါ", "h.noRouter": "Router မထည့်ရသေးပါ။ Router တွင် ထည့်ပါ။", "h.fleetFailed": "Router အခြေအနေ မရနိုင်ပါ။",
    "h.reportFailed": "ဗောင်ချာ ကိန်းဂဏန်းများ မရနိုင်ပါ။",
    "v.ws.vouchers": "ဗောင်ချာများ", "v.ws.guest": "ဧည့်သည် စာမျက်နှာ",
    "v.t.stock": "လက်ကျန်", "v.t.create": "ထုတ်မည်", "v.t.shops": "ဆိုင်များ", "v.t.print": "ပရင့်",
    "v.hint": "ဧည့်သည်က ဆိုင်မှာ ကုဒ်ဝယ်ပြီး Router ၏ login စာမျက်နှာတွင် ထည့်လျှင် အင်တာနက် ရသည်။",
    "v.used": "သုံးပြီး", "v.onRouter": "Router ထဲ ထည့်ပြီး", "v.shop": "ဆိုင်", "v.noShop": "ဆိုင် မသတ်မှတ်ရသေး", "v.empty": "ဗောင်ချာအစု မရှိသေးပါ။",
    "v.pendingTitle": "မပြီးသေးသော အစုများ", "v.pendingBody": "ဤအစုများကို Router ထဲ ထည့်နေဆဲ ဖြစ်သည်။ ပြီးအောင် ထပ်ကြိုးစားပါ။ ကုဒ်အသစ် မထုတ်ပါ။",
    "v.retry": "ထပ်ကြိုးစားမည်", "v.retrying": "ထပ်ကြိုးစားနေသည်…",
    "v.f.gateway": "Router", "v.f.label": "ပလန် အမည်", "v.f.duration": "သက်တမ်း", "v.f.data": "ဒေတာ ကန့်သတ်ချက် (MB)", "v.f.price": "စျေးနှုန်း (ကျပ်)", "v.f.count": "ကုဒ် အရေအတွက်", "v.f.shop": "ဆိုင်",
    "v.d.60": "၁ နာရီ", "v.d.1440": "၁ ရက်", "v.d.10080": "၇ ရက်", "v.d.43200": "၃၀ ရက်",
    "v.create": "ဗောင်ချာအစု ထုတ်မည်", "v.creating": "Router ထဲ ကုဒ်များ ထည့်နေသည်…", "v.noGateway": "အဓိက gateway Router ကို အရင်ထည့်ပါ။",
    "v.pendingFirst": "သိမ်းထားသော အစုကို အရင်ပြီးအောင်လုပ်ပါ။", "v.ready": "ကုဒ်များကို Router ထဲ ထည့်ပြီး စစ်ဆေးပြီးပါပြီ။",
    "v.codesOnce": "ကုဒ်များကို ယခုသာ ပြသည်။ ဤစာမျက်နှာမှ မထွက်မီ ပရင့်ထုတ်ပါ။", "v.print": "ကုဒ်များ ပရင့်ထုတ်မည်", "v.validFirstUse": "ပထမဆုံး သုံးသည့်နေ့မှ စတင်",
    "v.notReady": "အစု မပြီးသေးပါ။ လက်ကျန်တွင် ထပ်ကြိုးစားပါ။", "v.createFailed": "အစုကို မထုတ်နိုင်ပါ။",
    "v.shops.empty": "ဆိုင် မရှိသေးပါ။", "v.shops.add": "ဆိုင်ထည့်မည်", "v.shops.name": "ဆိုင်အမည်", "v.shops.contact": "ဆက်သွယ်ရမည့်သူ", "v.shops.prefix": "ကုဒ်ရှေ့စာလုံး (အက္ခရာ ၂–၄ လုံး)",
    "v.shops.phone": "ဖုန်း", "v.shops.saved": "ဆိုင်ထည့်ပြီးပါပြီ။", "v.shops.failed": "ဆိုင်ကို မသိမ်းနိုင်ပါ။", "v.print.help": "ပရင့်နှင့် ကတ်ပုံစံကို ဤနေရာတွင် ပြန်လုပ်ပြီးသည်အထိ အပြည့်အစုံ app တွင် သုံးပါ။",
    "j.title": "Jellyfin", "j.ws.dash": "ဒက်ရှ်ဘုတ်", "j.ws.vouchers": "ဗောင်ချာ", "j.ws.guest": "ဧည့်သည် စာမျက်နှာ",
    "j.status": "မီဒီယာ ဆာဗာ", "j.notReady": "Jellyfin ဗောင်ချာ မရသေးပါ။",
    "j.notReadyBody": "Jellyfin ဗောင်ချာဖြင့် ဧည့်သည်သည် သင့် Jellyfin PC မှ ရုပ်ရှင်ကိုသာ ကြည့်နိုင်ပြီး အင်တာနက် မရပါ။ Router စည်းမျဉ်းနှင့် ဆာဗာဘက်ကို ဆက်လုပ်နေဆဲ ဖြစ်သည်။",
    "j.dashHelp": "ဆာဗာချိတ်ဆက်မှုနှင့် library ထိန်းချုပ်မှုများကို နောက်တွင် ဤနေရာသို့ ပြောင်းမည်။ ထိုအချိန်အထိ အပြည့်အစုံ app ကို သုံးပါ။",
    "r.empty": "Router မထည့်ရသေးပါ။", "r.lastSeen": "နောက်ဆုံး တွေ့ရှိ", "r.never": "မတွေ့ရသေး", "r.tools": "Router ကိရိယာများ", "r.toolsHelp": "Setup၊ Wi-Fi၊ ကာကွယ်မှုနှင့် ကွန်ရက်မြေပုံတို့ကို ဤနေရာတွင် ပြန်လုပ်ပြီးသည်အထိ အပြည့်အစုံ app တွင် သုံးပါ။",
    "r.ws.list": "Router များ", "r.ws.map": "မြေပုံ", "map.failed": "ကွန်ရက်မြေပုံကို မဖွင့်နိုင်ပါ။", "map.pick": "ကိရိယာတစ်ခုကို နှိပ်ပြီး အသေးစိတ် ကြည့်ပါ။",
    "map.props": "ကိရိယာ အချက်အလက်", "map.seen": "တွေ့ရှိပုံ", "map.devices": "ကိရိယာ တွေ့ရှိ", "map.empty": "ဤ Router တွင် ကိရိယာ မတွေ့ရသေးပါ။", "map.unplaced": "ချိတ်ဆက်ရာ မသိရ",
    "map.how.wifi": "Wi-Fi client", "map.how.neighbor": "Neighbour အဖြစ် တွေ့ရှိ", "map.how.learned": "အောက်တွင် လေ့လာတွေ့ရှိ", "map.how.unknown": "တွေ့ရှိသော်လည်း port မသိရ",
    "map.note": "Router မှ လေ့လာတွေ့ရှိချက်သာ ဖြစ်သည်။ Forwarding entry သည် ကြိုးတိုက်ရိုက်ချိတ်ကြောင်း မသက်သေပြပါ၊ switch သို့ hop ကို ဖန်တီးမပြပါ။",
    "m.account": "အကောင့်", "m.pass": "App pass", "m.passHelp": "ပလန်များနှင့် add-on များကို ဤနေရာတွင် ပြမည်။", "m.store": "Store", "m.storeSoon": "Add-on စာရင်းကို ဆောက်နေဆဲ ဖြစ်သည်။",
    "m.profile": "ပရိုဖိုင်", "m.role": "အခန်းကဏ္ဍ",
  },
};

const KEY = "mm2_lang";
let lang = "my";
try { const s = localStorage.getItem(KEY); if (s === "en" || s === "my") lang = s; } catch (_) { /* storage blocked: keep default */ }
const listeners = new Set();

export const getLang = () => lang;
export function setLang(next) {
  if (next !== "en" && next !== "my") return;
  lang = next;
  try { localStorage.setItem(KEY, next); } catch (_) { /* ignore */ }
  document.documentElement.lang = next;
  listeners.forEach((fn) => fn(next));
}
export const onLang = (fn) => { listeners.add(fn); return () => listeners.delete(fn); };
export function t(key) { return (dict[lang] && dict[lang][key]) || dict.en[key] || key; }
document.documentElement.lang = lang;
