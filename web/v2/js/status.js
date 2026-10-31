// One shared health poll drives every .dot in the app. The topbar is rebuilt on
// every screen change, so state lives here and each dot subscribes on creation.
import { h } from "./dom.js";
import { api } from "./api.js";

const dots = new Set();
let state = "idle";
let timer = null;
let busy = false;

function paint() {
  for (const d of dots) {
    // A dot is not attached yet when it is created; only forget ones that were attached and then removed.
    if (d.isConnected) d.dataset.seen = "1";
    else if (d.dataset.seen) { dots.delete(d); continue; }
    d.dataset.state = state;
    d.title = state;
  }
}

function map(res) {
  const s = res && res.overall_state;
  if (s === "up") return "ok";
  if (s === "stale") return "warn";
  if (s === "offline") return "bad";
  return "idle";
}

async function check() {
  if (busy || document.hidden) return;
  busy = true;
  try { state = map(await api.get("/fleet/health")); }
  catch (e) { state = e && e.message === "unauthorized" ? "idle" : "bad"; }
  finally { busy = false; paint(); }
}

export function statusDot() {
  const d = h("span", { class: "dot", "aria-hidden": "true" });
  dots.add(d);
  paint();
  return d;
}

export function startStatus() {
  if (timer) return;
  check();
  timer = setInterval(check, 30000);
  document.addEventListener("visibilitychange", () => { if (!document.hidden) check(); });
}

export function stopStatus() {
  if (timer) clearInterval(timer);
  timer = null;
  state = "idle";
  paint();
}
