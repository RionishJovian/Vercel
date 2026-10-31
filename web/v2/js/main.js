import { api } from "./api.js";
import { h, icon, clear } from "./dom.js";
import { t, getLang, setLang, onLang } from "./i18n.js";
import signin from "./screens/signin.js";
import home from "./screens/home.js";
import router from "./screens/router.js";
import vouchers from "./screens/vouchers.js";
import jellyfin from "./screens/jellyfin.js";
import more from "./screens/more.js";
import revenue from "./screens/revenue.js";
import store from "./screens/store.js";
import platform from "./screens/platform.js";

const SCREENS = { home, router, vouchers, jellyfin, more, revenue, store, platform };
const TABS = [["home", "home"], ["router", "router"], ["vouchers", "ticket"], ["jellyfin", "play"], ["more", "dots"]];
const root = document.getElementById("app");
let me = null;
let notice = "";
let seq = 0;

function parse() {
  const [path, qs] = location.hash.replace(/^#\/?/, "").split("?");
  const [name = "home", sub = ""] = path.split("/");
  return { name: SCREENS[name] ? name : "home", sub, params: new URLSearchParams(qs || "") };
}

function langToggle() {
  const btn = (code, label) => h("button", { type: "button", "aria-pressed": String(getLang() === code), onclick: () => setLang(code) }, label);
  return h("div", { class: "lang", role: "group", "aria-label": t("c.language") }, btn("my", "မြန်မာ"), btn("en", "EN"));
}

async function boot() {
  try { me = await api.get("/me"); } catch (_) { me = null; }
  draw();
}

function signOutToLogin(message) {
  me = null;
  notice = message || "";
  draw();
}

function draw() {
  clear(root);
  if (!me) {
    root.append(signin({ notice, onDone: () => { notice = ""; boot(); } }));
    return;
  }
  const route = parse();
  const screen = SCREENS[route.name];
  const view = h("main", { class: "view", id: "view", "aria-live": "polite" }, h("div", { class: "spinner", role: "status", "aria-label": t("c.loading") }));
  const shell = h("div", { class: "shell" },
    h("header", { class: "topbar" },
      h("div", { class: "topbar-title" }, h("span", { class: "dot", "aria-hidden": "true" }), h("span", null, t(screen.titleKey))),
      langToggle()),
    view,
    h("nav", { class: "tabbar", "aria-label": "Main" },
      TABS.map(([key, ic]) => h("a", { class: "tab", href: "#/" + key, "aria-current": key === screen.tab ? "page" : null }, icon(ic), t("tab." + key)))));
  root.append(shell);

  const mine = ++seq;
  const ctx = { me, sub: route.sub, params: route.params, refresh: draw, signOut: signOutToLogin };
  screen.render(ctx).then((node) => {
    if (mine !== seq) return;
    clear(view);
    view.append(node);
  }).catch((err) => {
    if (mine !== seq) return;
    if (err && err.message === "unauthorized") return signOutToLogin(t("c.sessionEnded"));
    clear(view);
    view.append(h("div", { class: "glass" },
      h("p", null, t("c.loadFailed")),
      h("button", { class: "pill", type: "button", onclick: draw }, t("c.retry"))));
  });
}

window.addEventListener("hashchange", () => { if (me) { window.scrollTo(0, 0); draw(); } });
onLang(() => draw());
boot();
