// Same-origin API client. Session lives in Secure HttpOnly cookies; no token is
// ever stored in the page. X-MM-CSRF is required by the server on every request.
const API = "/api/v1";
let refreshing = null;

async function renew() {
  if (!refreshing) {
    refreshing = fetch(API + "/auth/refresh", {
      method: "POST",
      headers: { Accept: "application/json", "Content-Type": "application/json", "X-MM-CSRF": "1" },
      body: "{}",
    }).then((r) => r.ok, () => false).finally(() => { refreshing = null; });
  }
  return refreshing;
}

export async function req(method, path, body, retried = false) {
  const headers = { Accept: "application/json", "X-MM-CSRF": "1" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  let res;
  try {
    res = await fetch(API + path, { method, headers, body: body !== undefined ? JSON.stringify(body) : undefined });
  } catch (_) {
    throw new Error("network-unreachable");
  }
  if (res.status === 401) {
    if (!retried && !path.startsWith("/auth/") && (await renew())) return req(method, path, body, true);
    throw new Error("unauthorized");
  }
  let data = null;
  if ((res.headers.get("content-type") || "").includes("application/json")) {
    try { data = await res.json(); } catch (_) { data = null; }
  }
  if (!res.ok) {
    const err = new Error((data && data.error) || "http-" + res.status);
    err.status = res.status;
    err.data = data;
    throw err;
  }
  return data;
}

export const api = {
  get: (p) => req("GET", p),
  post: (p, b) => req("POST", p, b === undefined ? {} : b),
  put: (p, b) => req("PUT", p, b),
  del: (p) => req("DELETE", p),
};
