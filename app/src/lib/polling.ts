import { useEffect, useRef } from "react";
import { isUnauthorized } from "./api";

/**
 * SWR options for a reading that keeps itself fresh. SWR stops its interval while
 * the last read is an error, and the app turns automatic retries off, so one failed
 * read would freeze the page until it regained focus. This keeps trying at the same
 * pace instead, and stops when the session has ended or the page is hidden.
 */
export function keepPolling(ms: number) {
  return {
    refreshInterval: ms,
    shouldRetryOnError: true,
    onErrorRetry: (err: unknown, _key: string, _config: unknown, revalidate: (o: { retryCount: number }) => void, { retryCount }: { retryCount: number }) => {
      if (isUnauthorized(err) || document.visibilityState === "hidden") return;
      setTimeout(() => revalidate({ retryCount }), ms);
    },
  };
}

/** Calls `run` on a timer while the page is visible, and once when it comes back into view. */
export function useVisibleInterval(run: () => void, ms: number, enabled = true) {
  const latest = useRef(run);
  useEffect(() => { latest.current = run; });
  useEffect(() => {
    if (!enabled) return;
    const tick = () => { if (document.visibilityState !== "hidden") latest.current(); };
    const timer = window.setInterval(tick, ms);
    document.addEventListener("visibilitychange", tick);
    return () => { window.clearInterval(timer); document.removeEventListener("visibilitychange", tick); };
  }, [ms, enabled]);
}
