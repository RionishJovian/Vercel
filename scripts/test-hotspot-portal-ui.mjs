import fs from 'node:fs';
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { JSDOM } from 'jsdom';

const html = fs.readFileSync(new URL('../web/portal.html', import.meta.url), 'utf8');
// The page script is an external file (the CSP forbids inline scripts), so read it from the file the page references.
const scriptSrc = html.match(/<script src="\/assets\/js\/(portal\.js)"/)[1];
const script = fs.readFileSync(new URL(`../web/assets/js/${scriptSrc}`, import.meta.url), 'utf8');
const gateway = '11111111-1111-4111-8111-111111111111';
const config = { business_name: 'Test cafe', voucher_login_enabled: true, mobile_payment_enabled: true, consent_required: true, consent_text: 'Site terms' };
const response = (body, status = 200) => ({ ok: status >= 200 && status < 300, status, json: async () => body });
const grant = () => ({ product: 'guest-internet', state: 'active', expires_at: new Date(Date.now() + 3600000).toISOString(), data_mb: 100 });
const settle = () => new Promise(resolve => setImmediate(resolve));
function deferred() { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no; }); return { promise, resolve, reject }; }
function portal(t, fetch, query = `deviceId=${gateway}&mac=02:00:00:00:00:01`) {
  const dom = new JSDOM(html, { url: `https://portal.test/portal.html?${query}`, runScripts: 'outside-only' });
  t.after(() => dom.window.close());
  dom.window.localStorage.setItem('mm_lang', 'en');
  dom.window.fetch = fetch;
  dom.window.eval(script);
  return dom.window;
}
function fill(w) {
  w.document.getElementById('voucher-code').value = ' ab cd ';
  w.document.getElementById('portal-consent').checked = true;
}

test('failed config stays closed, retry is singular and loads a fresh published config', async t => {
  const wait = deferred(), calls = [];
  const w = portal(t, async (...args) => { calls.push(args); return calls.length === 1 ? response({ config }, 503) : wait.promise; });
  await settle();
  assert.equal(w.document.querySelector('#voucher-submit'), null);
  assert.equal(w.document.querySelector('.plan'), null);
  assert.match(w.document.getElementById('root').textContent, /Could not load/);
  w.document.getElementById('portal-retry').click();
  await w.loadConfig();
  assert.equal(calls.length, 2, 'pending retry must not start another config request');
  assert.equal(calls[1][1].cache, 'no-store');
  wait.resolve(response({ config, guest_voucher_access: true }));
  await settle();
  assert(w.document.getElementById('voucher-submit'));
});

test('network failure, invalid JSON and invalid successful config never enable access forms', async t => {
  for (const fetch of [async () => { throw new Error('offline'); }, async () => ({ ok: true, json: async () => { throw new Error('invalid JSON'); } }), ...[{}, null, [], { error: 'unavailable' }, { config: [] }].map(body => async () => response(body))]) {
    const w = portal(t, fetch);
    await settle();
    assert(w.document.getElementById('portal-retry'));
    assert.equal(w.document.getElementById('voucher-submit'), null);
    assert.equal(w.document.querySelector('.plan'), null);
  }
});

test('published access-method switches hide disabled voucher and payment actions', async t => {
  const cfg = { ...config, voucher_login_enabled: false, mobile_payment_enabled: false, guest_plans: [{ key: 'plan', label: 'Daily WiFi', amount: 1000 }] };
  const w = portal(t, async () => response({ config: cfg, guest_voucher_access: true }));
  await settle();
  assert.equal(w.document.querySelector('#voucher-submit, .plan'), null);
  assert.match(w.document.getElementById('root').textContent, /currently unavailable/);
  w.renderPortalHome({ ...cfg, mobile_payment_enabled: true });
  assert(w.document.querySelector('.plan'));
  assert.equal(w.document.getElementById('portal-back-voucher'), null);
  w.renderPortalHome({ ...cfg, voucher_login_enabled: true });
  assert(w.document.getElementById('voucher-submit'));
  assert.equal(w.document.getElementById('portal-go-payment'), null);
});

test('one in-flight activation survives repeated Enter, navigation and language clicks', async t => {
  const wait = deferred(), posts = [];
  const w = portal(t, async (url, options) => {
    if (!options?.method) return response({ config, guest_voucher_access: true });
    posts.push({ url, body: JSON.parse(options.body) });
    return wait.promise;
  });
  await settle(); fill(w);
  const button = w.document.getElementById('voucher-submit');
  button.click();
  w.document.getElementById('voucher-code').dispatchEvent(new w.KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
  w.document.getElementById('portal-go-payment').click();
  w.document.getElementById('lang').click();
  await w.redeemVoucher(config);
  assert.equal(posts.length, 1);
  assert.equal(posts[0].url, `/api/v1/portal/${gateway}/guest-voucher`);
  assert.deepEqual(posts[0].body, { client_mac: '02:00:00:00:00:01', code: 'ABCD', consent: true });
  assert(button.disabled && w.document.getElementById('voucher-code').readOnly);
  assert.equal(w.document.getElementById('lang').getAttribute('aria-disabled'), 'true');
  assert.equal(w.document.documentElement.lang, 'en');
  assert.equal(w.document.getElementById('voucher-submit'), button);
  wait.resolve(response(grant()));
  await settle();
  const text = w.document.getElementById('root').textContent;
  assert.match(text, /Voucher accepted/);
  assert.match(text, /Open a website to check your internet access/);
  assert.match(text, /Valid until:/);
  assert.match(text, /Total data allowance: 100 MB/);
  assert.doesNotMatch(text, /Your device is connected/);
  assert.equal(w.document.getElementById('lang').getAttribute('aria-disabled'), 'false');
});

test('uncertain redemption retains the same code and consent for retry', async t => {
  const bodies = [];
  const w = portal(t, async (_url, options) => {
    if (!options?.method) return response({ config, guest_voucher_access: true });
    bodies.push(options.body);
    if (bodies.length === 1) throw new Error('Connection lost');
    return response(grant());
  });
  await settle(); fill(w);
  w.document.getElementById('voucher-submit').click();
  await settle();
  assert.match(w.document.getElementById('voucher-msg').textContent, /Retry the same code/);
  assert.equal(w.document.getElementById('voucher-code').value, 'ABCD');
  assert(w.document.getElementById('portal-consent').checked);
  assert(!w.document.getElementById('voucher-submit').disabled);
  assert(!w.document.getElementById('portal-go-payment').disabled);
  w.document.getElementById('voucher-submit').click();
  await settle();
  assert.equal(bodies.length, 2);
  assert.equal(bodies[0], bodies[1]);
  assert.match(w.document.getElementById('root').textContent, /Voucher accepted/);
});

test('incomplete, expired or denied responses never claim activation', async t => {
  for (const result of [response({}, 403), response({ state: 'active' }), response({ ...grant(), expires_at: 'invalid' }), response({ ...grant(), expires_at: new Date(0).toISOString() }), response({ ...grant(), data_mb: 0 }), response({ ...grant(), data_mb: '100' })]) {
    const w = portal(t, async (_url, options) => !options?.method ? response({ config, guest_voucher_access: true }) : result);
    await settle(); fill(w);
    w.document.getElementById('voucher-submit').click();
    await settle();
    assert(w.document.getElementById('voucher-submit'));
    assert.doesNotMatch(w.document.getElementById('root').textContent, /Voucher accepted/);
    assert.match(w.document.getElementById('voucher-msg').textContent, /Retry the same code/);
  }
});

test('a late response cannot replace a newer portal view', async t => {
  const wait = deferred();
  const w = portal(t, async (_url, options) => !options?.method ? response({ config, guest_voucher_access: true }) : wait.promise);
  await settle(); fill(w);
  w.document.getElementById('voucher-submit').click();
  w.renderPlans(config); // Simulate the view being replaced while the request is pending.
  wait.resolve(response(grant()));
  await settle();
  assert.match(w.document.getElementById('root').textContent, /Buy WiFi/);
  assert.doesNotMatch(w.document.getElementById('root').textContent, /Voucher accepted/);
  assert.equal(w.VOUCHER_PENDING, false);
});

test('preview stays inert and legacy redemption does not claim verified internet', async t => {
  const preview = portal(t, () => { throw new Error('Preview performed network I/O'); }, 'preview=1');
  preview.renderPortalHome(config); fill(preview);
  await preview.redeemVoucher(config);
  assert.match(preview.document.getElementById('voucher-msg').textContent, /Preview only/);
  const paths = [];
  const w = portal(t, async (url, options) => {
    paths.push(url);
    return !options?.method ? response({ config, guest_voucher_access: false }) : response({ status: 'recorded' });
  });
  await settle(); fill(w);
  w.document.getElementById('voucher-submit').click();
  await settle();
  assert.equal(paths[1], `/api/v1/portal/${gateway}/redeem`);
  assert.match(w.document.getElementById('root').textContent, /Gateway internet access has not been verified/);
  assert.doesNotMatch(w.document.getElementById('root').textContent, /Voucher accepted|Your device is connected/);
});

test('plan rows show the price apart, and the pay screen leads with the amount and a copyable account number', async t => {
  const cfg = { ...config, mobile_payment_enabled: true, voucher_login_enabled: false, bank_name: 'KBZ Bank', bank_holder: 'Test Owner', bank_account: '0123456789', payment_instructions: 'Add your phone number in the note.', guest_plans: [{ key: 'hour', label: '1 Hour', minutes: 60, amount: 1000, data_mb: 1024, speed_down_mbps: 5 }] };
  const w = portal(t, async () => response({ config: cfg }));
  Object.defineProperty(w.navigator, 'clipboard', { value: { writeText: async () => {} }, configurable: true });
  await settle();
  const plan = w.document.querySelector('.plan[data-key="hour"]');
  assert.match(plan.querySelector('.amt').textContent, /^1,000\s+MMK$/);
  assert.match(plan.querySelector('.sub').textContent, /1 hr · 1024 MB · 5 Mbps/);
  assert.equal(w.document.querySelector('.flow [aria-current="step"]').textContent, 'Choose');
  w.renderPay(cfg, 'req-1', new Date(Date.now() + 600000).toISOString(), cfg.guest_plans[0]);
  const text = w.document.getElementById('root').textContent;
  assert.match(w.document.querySelector('.due b').textContent, /1,000\s+MMK/);
  assert.match(text, /KBZ Bank/); assert.match(text, /Add your phone number in the note\./);
  assert.equal(w.document.querySelector('[data-copy]').getAttribute('data-copy'), '0123456789');
  assert.equal(w.document.querySelector('.flow [aria-current="step"]').textContent, 'Pay');
  assert.match(w.document.getElementById('timer').textContent, /^\d\d:\d\d$/);
  const file = new w.File(['x'], 'slip.png', { type: 'image/png' });
  Object.defineProperty(w.document.getElementById('receipt'), 'files', { value: [file] });
  w.document.getElementById('receipt').dispatchEvent(new w.Event('change'));
  assert.equal(w.document.getElementById('receipt-name').textContent, 'slip.png');
});

test('the accepted view counts the verified pass down and shows no clock for an unverified one', async t => {
  const w = portal(t, async (_u, o) => !o?.method ? response({ config, guest_voucher_access: true }) : response(grant()));
  await settle(); fill(w);
  w.document.getElementById('voucher-submit').click();
  await settle();
  assert.match(w.document.querySelector('.pass-state').textContent, /Pass active/);
  assert.match(w.document.getElementById('pass-left').textContent, /^(\d+:)?\d{1,2}:\d\d$/);
  const legacy = portal(t, async (_u, o) => !o?.method ? response({ config, guest_voucher_access: false }) : response({ status: 'recorded' }));
  await settle(); fill(legacy);
  legacy.document.getElementById('voucher-submit').click();
  await settle();
  assert.equal(legacy.document.getElementById('pass-left'), null);
  assert.equal(legacy.document.querySelector('.pass-state'), null);
});

test('an owner colour gets readable button ink by contrast, and no colour keeps the Obsidian and Cyan defaults', async t => {
  const w = portal(t, async () => response({ config }));
  await settle();
  const ink = hex => { w.applyBranding({ primary_hex: hex }); return w.document.documentElement.style.getPropertyValue('--brand-ink'); };
  assert.equal(ink('#38bdf8'), '#06131f'); // white on sky cyan would be 2.0:1
  assert.equal(ink('#075044'), '#fff');
  assert.equal(ink('#ffb547'), '#06131f');
  w.applyBranding({});
  assert.equal(w.document.documentElement.style.getPropertyValue('--brand'), '');
});
