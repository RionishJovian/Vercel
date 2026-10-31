import { useSyncExternalStore } from "react";

// Hash routes keep deep links like #/vouchers/guest working without server rewrites.
export interface Route { name: string; sub: string }
const read = (): string => location.hash;
const subscribe = (cb: () => void) => { window.addEventListener("hashchange", cb); return () => window.removeEventListener("hashchange", cb); };
export function parseRoute(hash: string): Route {
  const [path = ""] = hash.replace(/^#\/?/, "").split("?");
  const [name = "home", sub = ""] = path.split("/");
  return { name: name || "home", sub };
}
export function useRoute(): Route { return parseRoute(useSyncExternalStore(subscribe, read, () => "")); }
