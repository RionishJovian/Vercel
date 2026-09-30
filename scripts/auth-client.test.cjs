const { test } = require('node:test');
const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const vm = require('node:vm');
const source = readFileSync('web/assets/js/api.js', 'utf8') + '\n globalThis.client = MM;';
function setup(responses) {
  const calls = [], removed = [];
  const context = { localStorage: { removeItem: k => removed.push(k) }, fetch: async (url, options) => {
    calls.push({ url, options }); const status = responses.shift();
    if (!status) throw new Error('Unexpected request');
    return { status, ok: status < 400, headers: { get: () => 'application/json' }, json: async () => ({ token: 'test', error: 'invalid credentials' }) };
  }};
  vm.runInNewContext(source, context);
  return { client: context.client, calls, removed };
}
test('cookie client refreshes once and retries without exposing bearer tokens', async () => {
  const { client, calls, removed } = setup([401, 200, 200]);
  await client.get('/me');
  assert.deepEqual(removed, ['mm_token', 'mm_refresh']);
  assert.deepEqual(calls.map(c => c.url), ['/api/v1/me', '/api/v1/auth/refresh', '/api/v1/me']);
  for (const call of calls) { assert.equal(call.options.headers.Authorization, undefined); assert.equal(call.options.headers['X-MM-CSRF'], '1'); }
  assert.equal(calls[1].options.body, '{}');
  assert.equal(client.signedIn(), true);
});
test('bad login does not refresh an unrelated session', async () => {
  const { client, calls } = setup([401]);
  await assert.rejects(client.post('/auth/login', {}), /unauthorized/);
  assert.equal(calls.length, 1);
});
test('logout calls the backend and clears authenticated state', async () => {
  const { client, calls } = setup([200, 204]);
  await client.get('/me'); await client.clear();
  assert.equal(calls[1].url, '/api/v1/auth/logout');
  assert.equal(client.signedIn(), false);
});
test('simultaneous unauthorized requests share one refresh operation', async () => {
  const { client, calls } = setup([401, 401, 200, 200, 200]);
  await Promise.all([client.get('/me'), client.get('/routers')]);
  assert.equal(calls.filter(c => c.url === '/api/v1/auth/refresh').length, 1);
});
