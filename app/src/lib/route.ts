import { useSyncExternalStore } from "react";

/**
 * Console routes are real paths under /app/ (e.g. /app/vouchers/create?x=1).
 *
 * Compatibility: older links use hash routes (#/vouchers/create). A hash route always wins when present, and
 * the address bar is then rewritten to the path form without adding a history entry. This keeps working:
 * - bookmarks to /v2/#/… (the server redirects /v2/ to /app/ and the browser carries the #… along);
 * - links inside the console written as href="#/…" (clicks are turned into normal path navigation);
 * - code that sets location.hash or calls location.replace("#/…").
 */
export interface Route { name: string; sub: string; params: URLSearchParams }
export const APP_BASE = "/app/";
const EVENT = "mm-route";

/** Parse a hash route ("#/vouchers/create?x=1", "#portal"). */
export function parseRoute(hash: string): Route {
  const [path = "", qs = ""] = hash.replace(/^#\/?/, "").split("?");
  const [name = "home", sub = ""] = path.split("/");
  return { name: name || "home", sub, params: new URLSearchParams(qs) };
}

/** Parse a path route ("/app/vouchers/create", "?x=1"). Anything outside /app/ is Home. */
export function parsePath(pathname: string, search = ""): Route {
  const rest = pathname.startsWith(APP_BASE) ? pathname.slice(APP_BASE.length) : "";
  const [name = "home", sub = ""] = rest.split("/").map((s) => { try { return decodeURIComponent(s); } catch { return s; } });
  return { name: name || "home", sub, params: new URLSearchParams(search) };
}

const isHashRoute = (hash: string) => /^#\/|^#portal\b/.test(hash);
/** A same-site console path such as "/app/net?router=1" (not "//host/…", not a file under /app/assets/). */
const isAppPath = (href: string) => href.startsWith(APP_BASE) && !href.startsWith(APP_BASE + "assets/");

/** The path form of a hash route: "#/router/map?id=1" -> "/app/router/map?id=1". */
export function hashToPath(hash: string): string {
  const [path = "", qs = ""] = hash.replace(/^#\/?/, "").split("?");
  return APP_BASE + path.replace(/^\/+/, "") + (qs ? "?" + qs : "");
}

function notify() { window.dispatchEvent(new Event(EVENT)); }

/** Rewrite a hash route in the address bar to its path form (same history entry). */
export function normalizeLocation(): void {
  if (typeof window === "undefined" || !isHashRoute(location.hash)) return;
  try { history.replaceState(history.state, "", hashToPath(location.hash)); } catch { /* ignore */ }
  notify();
}

/** Navigate inside the console. `to` may be a path ("/app/net") or a legacy hash ("#/net"). */
export function navigate(to: string, replace = false): void {
  const url = to.startsWith("#") ? hashToPath(to) : to;
  if (!url.startsWith(APP_BASE)) { location.assign(url); return; }
  if (url === location.pathname + location.search && !location.hash) { if (!replace) window.scrollTo?.(0, 0); return; }
  if (replace) history.replaceState(null, "", url); else history.pushState(null, "", url);
  window.scrollTo?.(0, 0);
  notify();
}

let installed = false;
function install() {
  if (installed || typeof window === "undefined") return;
  installed = true;
  window.addEventListener("hashchange", normalizeLocation);
  // Plain left-clicks on console links (/app/… or a legacy #/…) navigate without reloading the page. Modified
  // clicks, other targets and downloads keep the browser's behaviour (a new tab simply opens that /app/ URL).
  document.addEventListener("click", (e) => {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    const a = (e.target as Element | null)?.closest?.("a");
    const href = a?.getAttribute("href") ?? "";
    if (!a || !(isHashRoute(href) || isAppPath(href)) || (a.target && a.target !== "_self") || a.hasAttribute("download")) return;
    e.preventDefault();
    navigate(href);
  });
}

const read = (): string => (isHashRoute(location.hash) ? location.hash : location.pathname + location.search);
const subscribe = (cb: () => void) => {
  install();
  window.addEventListener("popstate", cb); window.addEventListener("hashchange", cb); window.addEventListener(EVENT, cb);
  return () => { window.removeEventListener("popstate", cb); window.removeEventListener("hashchange", cb); window.removeEventListener(EVENT, cb); };
};

export function useRoute(): Route {
  const snap = useSyncExternalStore(subscribe, read, () => "");
  return snap.startsWith("#") ? parseRoute(snap) : parsePath(snap.split("?")[0], snap.includes("?") ? snap.slice(snap.indexOf("?")) : "");
}
