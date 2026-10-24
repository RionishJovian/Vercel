// This adapter is a browser-facing entry to the existing VPS API, not a new
// authentication service. Authorize the browser origin before translating it
// to the fixed upstream origin expected by the unchanged VPS CSRF middleware.
export const UPSTREAM_ORIGIN = 'https://megamagic.tech';
export const NO_STORE = Object.freeze({
  'Cache-Control': 'private, no-store',
  'CDN-Cache-Control': 'no-store',
  'Vercel-CDN-Cache-Control': 'no-store',
});
const READ_METHODS = new Set(['GET', 'HEAD', 'OPTIONS']);
const WRITE_METHODS = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);

function refused(status, error) {
  return new Response(JSON.stringify({ error }), {
    status,
    headers: { ...NO_STORE, 'Content-Type': 'application/json; charset=utf-8' },
  });
}

export function prepareUpstreamHeaders(request) {
  const url = new URL(request.url);
  if (url.protocol !== 'https:' ||
      !(url.pathname === '/api/v1' || url.pathname.startsWith('/api/v1/'))) {
    return refused(404, 'API route unavailable');
  }
  const method = request.method.toUpperCase();
  if (!READ_METHODS.has(method) && !WRITE_METHODS.has(method)) {
    return refused(405, 'method not allowed');
  }
  const origin = request.headers.get('origin');
  // Compare the browser's exact serialized origin, never a client-supplied
  // Host/X-Forwarded-Host value. Path, credentials and opaque "null" fail.
  if (origin !== null && origin !== url.origin) {
    return refused(403, 'cross-origin request refused');
  }
  if (WRITE_METHODS.has(method) &&
      (origin !== url.origin || request.headers.get('x-mm-csrf') !== '1')) {
    return refused(403, 'same-origin request required');
  }
  const headers = new Headers(request.headers);
  for (const key of [...headers.keys()]) {
    if (key === 'forwarded' || key.startsWith('x-forwarded-') ||
        key.startsWith('x-original-') || key.startsWith('x-middleware-')) {
      headers.delete(key);
    }
  }
  headers.set('host', new URL(UPSTREAM_ORIGIN).host);
  if (origin !== null) headers.set('origin', UPSTREAM_ORIGIN);
  headers.set('cache-control', 'no-store');
  // The unchanged body, Cookie and X-MM-CSRF headers pass to the API. Vercel's
  // fixed external rewrite supplies TLS to this single allowed upstream.
  return headers;
}
