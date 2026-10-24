import { test } from 'node:test';
import assert from 'node:assert/strict';
import { prepareUpstreamHeaders, UPSTREAM_ORIGIN } from './proxy-security.js';

const ORIGIN = 'https://mega-magic-example.vercel.app';
function request(method = 'GET', headers = {}, path = '/api/v1/me') {
  return new Request(ORIGIN + path, { method, headers });
}
async function rejected(req, status) {
  const res = prepareUpstreamHeaders(req);
  assert.ok(res instanceof Response);
  assert.equal(res.status, status);
  assert.equal(res.headers.get('cache-control'), 'private, no-store');
  assert.equal(res.headers.get('vercel-cdn-cache-control'), 'no-store');
  assert.ok((await res.json()).error);
}

test('normal same-origin GET preserves host-only session cookie and does not invent Origin', () => {
  const headers = prepareUpstreamHeaders(request('GET', { cookie: 'mm_access=test-only' }));
  assert.ok(headers instanceof Headers);
  assert.equal(headers.get('cookie'), 'mm_access=test-only');
  assert.equal(headers.get('origin'), null);
  assert.equal(headers.get('host'), 'megamagic.tech');
  assert.equal(headers.get('cache-control'), 'no-store');
});

test('valid browser POST translates Origin only after exact validation', () => {
  const headers = prepareUpstreamHeaders(request('POST', {
    origin: ORIGIN, 'x-mm-csrf': '1', 'content-type': 'application/json',
    cookie: 'mm_access=test-only',
  }));
  assert.ok(headers instanceof Headers);
  assert.equal(headers.get('origin'), UPSTREAM_ORIGIN);
  assert.equal(headers.get('x-mm-csrf'), '1');
  assert.equal(headers.get('content-type'), 'application/json');
  assert.equal(headers.get('cookie'), 'mm_access=test-only');
});

test('foreign, opaque, path-bearing and credential-bearing origins are refused', async () => {
  for (const origin of ['https://evil.example', 'null', ORIGIN + '/',
    'https://user@mega-magic-example.vercel.app', ORIGIN + '.evil.example']) {
    await rejected(request('POST', { origin, 'x-mm-csrf': '1' }), 403);
  }
});

test('authentication cannot omit CSRF header or explicit Origin', async () => {
  await rejected(request('POST', { origin: ORIGIN }, '/api/v1/auth/login'), 403);
  await rejected(request('POST', { 'x-mm-csrf': '1' }, '/api/v1/auth/refresh'), 403);
  await rejected(request('POST', { origin: ORIGIN, 'x-mm-csrf': '0' }), 403);
});

test('cookie writes cannot bypass intent validation with Authorization', async () => {
  await rejected(request('DELETE', { cookie: 'mm_access=test-only', authorization: 'Bearer test-only' }), 403);
});

test('malicious forwarded host never establishes same-origin access', async () => {
  await rejected(request('PUT', { origin: 'https://evil.example',
    'x-forwarded-host': 'evil.example', host: 'evil.example', 'x-mm-csrf': '1' }), 403);
  const headers = prepareUpstreamHeaders(request('PUT', { origin: ORIGIN, 'x-mm-csrf': '1',
    'x-forwarded-host': 'evil.example', 'x-forwarded-proto': 'http',
    forwarded: 'host=evil.example', 'x-original-url': 'https://evil.example',
    'x-middleware-next': '1', 'x-vercel-protection-bypass':'test-only' }));
  assert.equal(headers.get('host'), 'megamagic.tech');
  assert.equal(headers.get('forwarded'), null);
  assert.equal(headers.get('x-forwarded-host'), null);
  assert.equal(headers.get('x-middleware-next'), null);
  assert.equal(headers.get('x-vercel-protection-bypass'), null);
});

test('cross-origin GET and preflight fail before credentials can reach upstream', async () => {
  await rejected(request('GET', { origin: 'https://evil.example' }), 403);
  await rejected(request('OPTIONS', { origin: 'https://evil.example' }), 403);
});

test('non-API path never becomes an arbitrary upstream proxy', async () => {
  await rejected(request('GET', {}, '/api/v10/me'), 404);
  await rejected(request('GET', {}, '/https://evil.example'), 404);
  await rejected(new Request('http://mega-magic-example.vercel.app/api/v1/me'), 404);
});
