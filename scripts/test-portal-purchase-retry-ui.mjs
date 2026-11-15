import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

// Runs the hosted pay page's real buy() against a fake page and fake server answers. A router that lags while the
// free payment window is being set up must leave the guest with a clear message and a way to try again.
const source = readFileSync(new URL('../web/assets/js/portal.js', import.meta.url), 'utf8');
const buySource = source.match(/async function buy\(cfg, key\) \{[\s\S]*?\n\}\n/)?.[0];
assert.ok(buySource, 'buy() must exist in portal.js');

function page(answer) {
  const root = { innerHTML: '' };
  let retry = null, plansShown = 0, paid = null;
  const context = vm.createContext({
    API: '/api/v1', DEVICE: 'device-1', PREVIEW: false,
    document: { getElementById: id => id === 'root' ? root : id === 'buy-retry' && root.innerHTML.includes('id="buy-retry"') ? { addEventListener: (_, fn) => { retry = fn; } } : null },
    FormData: class { append() {} },
    fetch: answer,
    clientMac: () => '02:00:00:00:00:01', portalConsent: () => true, esc: s => String(s).replace(/</g, '&lt;'), t: en => en,
    renderPlans: () => { plansShown++; }, renderPay: (...args) => { paid = args; },
  });
  vm.runInContext(buySource, context);
  return { root, context, get retry() { return retry; }, get plansShown() { return plansShown; }, get paid() { return paid; } };
}
const cfg = { guest_plans: [{ key: 'hour', label: '1 hour' }] };
const reply = (status, body) => async () => ({ ok: status < 300, status, json: async () => body });

test('a router that did not answer shows a plain message and Try again, which returns to the plans', async () => {
  const p = page(reply(502, { error: 'temporary internet could not be configured', retryable: true }));
  await p.context.buy(cfg, 'hour');
  assert.match(p.root.innerHTML, /router did not respond in time/);
  assert.match(p.root.innerHTML, /you have not paid anything/);
  assert.match(p.root.innerHTML, /id="buy-retry"/);
  assert.equal(p.paid, null);
  p.retry();
  assert.equal(p.plansShown, 1);
});

test('a dropped connection is retryable too and never shows a raw browser error', async () => {
  const p = page(async () => { throw new TypeError('Failed to fetch'); });
  await p.context.buy(cfg, 'hour');
  assert.match(p.root.innerHTML, /Could not reach the server/);
  assert.doesNotMatch(p.root.innerHTML, /Failed to fetch/);
  assert.ok(p.retry, 'Try again must be offered');
});

test('a final answer from the server is shown as is, with no Try again', async () => {
  const p = page(reply(403, { error: 'mobile payment is disabled for this portal' }));
  await p.context.buy(cfg, 'hour');
  assert.match(p.root.innerHTML, /mobile payment is disabled/);
  assert.doesNotMatch(p.root.innerHTML, /buy-retry/);
  assert.equal(p.retry, null);
});

test('a successful start goes to the pay screen', async () => {
  const p = page(reply(201, { request_id: 'r1', grace_expires_at: '2026-11-15T10:00:00Z', grace_data_mb: 50, grace_minutes: 10 }));
  await p.context.buy(cfg, 'hour');
  assert.equal(p.paid[1], 'r1');
  assert.equal(p.root.innerHTML, '');
});

test('the new guest messages have Burmese text', () => {
  for (const key of ['tryAgain', 'routerBusy', 'netFail']) assert.match(source, new RegExp(`\\b${key}:\\s*"[^"]*[\\u1000-\\u109f]`), `${key} needs a Burmese string`);
});
