// Same-origin client. The session lives in Secure HttpOnly cookies; X-MM-CSRF is required on every call.
const API = "/api/v1";
let refreshing: Promise<boolean> | null = null;

export class ApiError extends Error {
  status: number;
  data: unknown;
  constructor(message: string, status = 0, data: unknown = null) { super(message); this.status = status; this.data = data; }
}
export const isUnauthorized = (e: unknown) => e instanceof ApiError && e.status === 401;

function renew(): Promise<boolean> {
  refreshing ??= fetch(API + "/auth/refresh", {
    method: "POST", headers: { Accept: "application/json", "Content-Type": "application/json", "X-MM-CSRF": "1" }, body: "{}",
  }).then((r) => r.ok, () => false).finally(() => { refreshing = null; });
  return refreshing;
}

export async function request<T = unknown>(method: string, path: string, body?: unknown, retried = false): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json", "X-MM-CSRF": "1" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  let res: Response;
  try { res = await fetch(API + path, { method, headers, body: body !== undefined ? JSON.stringify(body) : undefined }); }
  catch { throw new ApiError("network-unreachable"); }
  if (res.status === 401) {
    if (!retried && !path.startsWith("/auth/") && (await renew())) return request<T>(method, path, body, true);
    throw new ApiError("unauthorized", 401);
  }
  let data: unknown = null;
  if ((res.headers.get("content-type") ?? "").includes("application/json")) { try { data = await res.json(); } catch { data = null; } }
  if (!res.ok) throw new ApiError((data as { error?: string } | null)?.error ?? "http-" + res.status, res.status, data);
  return data as T;
}

export const api = {
  get: <T,>(p: string) => request<T>("GET", p),
  post: <T,>(p: string, b: unknown = {}) => request<T>("POST", p, b),
  put: <T,>(p: string, b: unknown) => request<T>("PUT", p, b),
  del: <T,>(p: string) => request<T>("DELETE", p),
};
export const fetcher = <T,>(path: string) => api.get<T>(path);
