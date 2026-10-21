/* Mega Magic API client — minimal, no deps. Talks to /api/v1 on the same origin
   (nginx proxies /api/v1 -> Go API on :8080). Uses Secure HttpOnly session cookies. */

const MM = (() => {
  const API = "/api/v1";
  const KEY = "mm_token";
  const KEY_REFRESH = "mm_refresh";

  localStorage.removeItem(KEY); localStorage.removeItem(KEY_REFRESH);
  let authenticated = false;
  function signedIn() { return authenticated; }
  async function clear() { await req("POST", "/auth/logout", {}); authenticated = false; }

  let refreshInFlight = null;

  async function renew() {

    if (!refreshInFlight) {
      refreshInFlight = (async () => {
        try {
          const res = await fetch(API + "/auth/refresh", {
            method: "POST",
            headers: { "Accept": "application/json", "Content-Type": "application/json", "X-MM-CSRF": "1" },
            body: "{}",
          });
          if (!res.ok) return false;
          return true;
        } catch (_) {
          return false;
        } finally {
          refreshInFlight = null;
        }
      })();
    }
    return refreshInFlight;
  }

  // signal: optional AbortSignal for bounded, cancellable requests (live
  // router probes). Absent opts keep every existing call unchanged.
  async function req(method, path, body, retried = false, signal) {
    const headers = { "Accept": "application/json", "X-MM-CSRF": "1" };
    if (body !== undefined) headers["Content-Type"] = "application/json";

    let res;
    try {
      res = await fetch(API + path, {
        method,
        headers,
        body: body !== undefined ? JSON.stringify(body) : undefined,
        signal,
      });
    } catch (e) {
      throw new Error("network-unreachable");
    }
    if (res.status === 401) {
      if (!retried && !path.startsWith("/auth/") && await renew()) {
        return req(method, path, body, true, signal);
      }

      authenticated = false;
      throw new Error("unauthorized");
    }

    let data = null;
    const ct = res.headers.get("content-type") || "";
    if (ct.includes("application/json")) {
      try { data = await res.json(); } catch (_) { data = null; }
    }
    if (!res.ok) {
      const detail = (data && data.error) ? data.error : ("http-" + res.status);
      const err = new Error(detail);
      err.status = res.status;
      err.data = data;
      throw err;
    }
    if (!path.startsWith("/auth/") || path === "/auth/login") authenticated = true;
    return data;
  }

  const get  = (p, opts) => req("GET", p, undefined, false, opts && opts.signal);
  const post = (p, b) => req("POST", p, b);
  const put  = (p, b) => req("PUT", p, b);
  const del  = (p, b) => req("DELETE", p, b);

  return { signedIn, clear, get, post, put, del };
})();
