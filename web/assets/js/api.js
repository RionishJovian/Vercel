/* Mega Magic API client — minimal, no deps. Talks to /api/v1 on the same origin
   (nginx proxies /api/v1 -> Go API on :8080). Stores the JWT in localStorage. */

const MM = (() => {
  const API = "/api/v1";
  const KEY = "mm_token";
  const KEY_REFRESH = "mm_refresh";

  function token() { return localStorage.getItem(KEY); }
  function signedIn() { return !!token(); }
  function clear() { localStorage.removeItem(KEY); localStorage.removeItem(KEY_REFRESH); }

  let refreshInFlight = null;

  async function renew() {
    const refreshToken = localStorage.getItem(KEY_REFRESH);
    if (!refreshToken) return false;
    if (!refreshInFlight) {
      refreshInFlight = (async () => {
        try {
          const res = await fetch(API + "/auth/refresh", {
            method: "POST",
            headers: { "Accept": "application/json", "Content-Type": "application/json" },
            body: JSON.stringify({ refresh_token: refreshToken }),
          });
          if (!res.ok) return false;
          const data = await res.json();
          if (!data || !data.token) return false;
          localStorage.setItem(KEY, data.token);
          if (data.refresh_token) localStorage.setItem(KEY_REFRESH, data.refresh_token);
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

  async function req(method, path, body, retried = false) {
    const headers = { "Accept": "application/json" };
    if (body !== undefined) headers["Content-Type"] = "application/json";
    if (token()) headers["Authorization"] = "Bearer " + token();

    let res;
    try {
      res = await fetch(API + path, {
        method,
        headers,
        body: body !== undefined ? JSON.stringify(body) : undefined,
      });
    } catch (e) {
      throw new Error("network-unreachable");
    }
    if (res.status === 401) {
      if (!retried && path !== "/auth/refresh" && await renew()) {
        return req(method, path, body, true);
      }
      clear();
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
    return data;
  }

  const get  = (p) => req("GET", p);
  const post = (p, b) => req("POST", p, b);
  const put  = (p, b) => req("PUT", p, b);
  const del  = (p, b) => req("DELETE", p, b);

  return { token, signedIn, clear, get, post, put, del };
})();
