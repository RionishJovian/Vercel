/* Mega Magic API client — minimal, no deps. Talks to /api/v1 on the same origin
   (nginx proxies /api/v1 -> Go API on :8080). Stores the JWT in localStorage. */

const MM = (() => {
  const API = "/api/v1";
  const KEY = "mm_token";
  const KEY_REFRESH = "mm_refresh";

  function token() { return localStorage.getItem(KEY); }
  function signedIn() { return !!token(); }
  function clear() { localStorage.removeItem(KEY); localStorage.removeItem(KEY_REFRESH); }

  async function req(method, path, body) {
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
    if (res.status === 401) { clear(); throw new Error("unauthorized"); }

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