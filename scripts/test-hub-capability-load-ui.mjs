import fs from 'node:fs';
import vm from 'node:vm';
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { JSDOM } from 'jsdom';

const source = fs.readFileSync(new URL('../web/app.html', import.meta.url), 'utf8');
const apiSource = fs.readFileSync(new URL('../web/assets/js/api.js', import.meta.url), 'utf8');

test('source contract: the hub page paints before any live capability probe', () => {
  // renderSites no longer awaits every router probe before its first paint…
  assert(!source.includes('new Map(await Promise.all(routers.map'),
    'renderSites still blocks first paint on live capability probes');
  // …every saved card starts in a checking state instead…
  assert(source.includes('const capabilityStatusByID = new Map(routers.map(router => [router.device_id, "checking"]));'),
    'saved cards no longer render in an explicit checking state');
  // …probes restart per card after the page is wired…
  assert(/routers\.forEach\(router => \{ loadRouterCapability\(content, router, capabilityByID\); \}\)/.test(source),
    'per-card capability probes are not started after render');
  // …and the panel carries the status map (after the hub-capability slot on
  // the working tree, in its place on a clean HEAD checkout).
  assert(/routerBoardPanel\(routers, fleet, capabilityByID, (?:hubCapability|undefined), capabilityStatusByID\)/.test(source),
    'routerBoardPanel lost the capability status argument');
});

test('source contract: the API client bounds a probe request', () => {
  assert(/const get\s*=\s*\(p, opts\) => req\("GET", p, undefined, false, opts && opts\.signal\)/.test(apiSource),
    'MM.get no longer forwards an AbortSignal');
  assert(apiSource.includes('signal,'), 'fetch does not receive the signal');
  assert(apiSource.includes('return req(method, path, body, true, signal);'), 'the auth retry drops the signal');
});

const start = source.indexOf('function routerCapabilityButton(');
const end = source.indexOf('async function renderSites(', start);
assert(start >= 0 && end > start, 'capability helpers not found in app.html');
const context = vm.createContext({
  encodeURIComponent, Date, Map, Number, String, AbortController, setTimeout, clearTimeout,
  ME: { user: { user_role: 'OWNER' } }, SITE_EDITOR_ROLES: ['OWNER', 'ADMIN', 'DEVELOPER'],
  asArray: x => Array.isArray(x) ? x : [],
  ts: x => x,
  esc: x => String(x ?? '').replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;'),
  MMTheme: { icon: () => '' }, topologyLayerLabel: x => x,
});
vm.runInContext(source.slice(start, end), context);
const router = { device_id: 'gateway/a', hardware_name: 'Gateway', deployment_layer: 'MAIN_GATEWAY' };

function dom(t) {
  const instance = new JSDOM('<div id="content"><article data-gateway-id="gateway/a"><div class="routerboard-capabilities"><button class="routerboard-capability pending" data-router-capability="gateway"></button></div></article></div>', { runScripts: 'outside-only' });
  t.after(() => instance.window.close());
  return { window: instance.window, content: instance.window.document.getElementById('content') };
}

function stubApi(behavior) {
  const calls = [];
  context.MM = { get(path, opts) { calls.push({ path, signal: opts && opts.signal }); return behavior(path); } };
  return calls;
}

test('cards paint immediately with a checking slot, then the per-card probe lands them', async t => {
  const checking = context.routerCapabilitySlot(router, null, 'checking');
  assert(checking.includes('Checking this router…'));
  assert(!checking.includes('data-capability-retry='));

  const { window: w, content } = dom(t);
  const capabilityByID = new Map();
  let release;
  const calls = stubApi(path => new Promise(resolve => { release = resolve; }));
  const pending = context.loadRouterCapability(content, router, capabilityByID);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].path, '/routers/gateway%2Fa/capabilities');
  assert(calls[0].signal && typeof calls[0].signal === 'object' && 'aborted' in calls[0].signal, 'probe request is not bounded by an AbortSignal');
  assert.equal(content.querySelector('[data-router-capability="gateway"]').title, 'Checking this router…');
  assert(!content.querySelector('[data-capability-retry]'));
  release({ capabilities: { device_id: 'gateway/a', detected: true, gateway: true, hotspot: { supported: true } } });
  await pending;
  assert.equal(capabilityByID.get('gateway/a').error, undefined);
  assert(content.querySelector('[data-router-capability="gateway"]').classList.contains('available'));
  assert(!content.querySelector('[data-capability-retry]'));
});

test('a bounded timeout leaves a per-card retry that restores the card on its own', async t => {
  const { window: w, content } = dom(t);
  const capabilityByID = new Map();
  let release;
  const calls = stubApi(path => new Promise((resolve, reject) => { release = { resolve, reject }; }));
  const first = context.loadRouterCapability(content, router, capabilityByID);
  release.reject({ name: 'AbortError' });
  await first;
  assert.match(String(capabilityByID.get('gateway/a').error), /timed out/);
  const retry = content.querySelector('[data-capability-retry]');
  assert(retry, 'failed card has no retry button');
  assert(content.querySelector('[data-router-capability="gateway"]').classList.contains('pending'));
  retry.dispatchEvent(new w.Event('click', { bubbles: true }));
  assert.equal(calls.length, 2);
  release.resolve({ capabilities: { device_id: 'gateway/a', detected: true, gateway: true } });
  await new Promise(r => setTimeout(r, 0));
  assert(content.querySelector('[data-router-capability="gateway"]').classList.contains('available'));
  assert(!content.querySelector('[data-capability-retry]'));
});

test('unverified probe answers stay errors and a removed card cannot be touched', async t => {
  const { content } = dom(t);
  const capabilityByID = new Map();
  for (const caps of [{ device_id: 'foreign', detected: true }, { device_id: 'gateway/a', detected: false }]) {
    const calls = stubApi(() => Promise.resolve({ capabilities: caps }));
    await context.loadRouterCapability(content, router, capabilityByID);
    assert.match(String(capabilityByID.get('gateway/a').error), /unverified/);
    assert(content.querySelector('[data-capability-retry]'));
    assert(calls[0].signal);
  }
  content.querySelector('article').remove();
  const calls = stubApi(() => Promise.resolve({ capabilities: { device_id: 'gateway/a', detected: true } }));
  await context.loadRouterCapability(content, router, capabilityByID);
  assert.equal(calls.length, 0, 'a detached card must not start another live probe');
  assert.match(String(capabilityByID.get('gateway/a').error), /unverified/);
});