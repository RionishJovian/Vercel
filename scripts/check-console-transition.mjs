// Opt-in browser integration check. Requires Caddy, Chromium, Playwright and the previous release checkout/build.
// Uses only a disposable HTTP API fixture: no production account, network write or saved credential.
// Usage: CADDY_BIN=/path/caddy PREVIOUS_ROOT=/path/previous node scripts/check-console-transition.mjs
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { createServer } from 'node:http';
import { createHash } from 'node:crypto';
import { spawn } from 'node:child_process';
import { cpSync, existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
const { chromium } = createRequire(import.meta.url)('playwright');
// Config swaps close listeners; do not reuse a pooled connection from the previous stage.
const fetch = (url, options = {}) => globalThis.fetch(url, { ...options, headers: { Connection: 'close', ...options.headers } });
const root = path.resolve(import.meta.dirname, '..');
const previous = path.resolve(process.env.PREVIOUS_ROOT ?? '');
const caddy = process.env.CADDY_BIN;
assert.ok(caddy && existsSync(path.join(previous, 'app/dist/index.html')), 'set CADDY_BIN and PREVIOUS_ROOT; build the previous release first');
assert.ok(existsSync(path.join(root, 'vercel-public/v2/assets/index-BkXQbGXZ.js')), 'run npm run build first');
const temp = mkdtempSync(path.join(tmpdir(), 'mm-transition-'));
const oldRoot = path.join(temp, 'old'), negative = path.join(temp, 'negative'), rollback = path.join(temp, 'rollback');
cpSync(path.join(previous, 'web'), oldRoot, { recursive: true });
cpSync(path.join(previous, 'app/dist'), path.join(oldRoot, 'v2'), { recursive: true });
cpSync(path.join(root, 'vercel-public'), negative, { recursive: true });
rmSync(path.join(negative, 'v2'), { recursive: true });
cpSync(oldRoot, rollback, { recursive: true });
cpSync(path.join(root, 'deploy/compat/rollback-route.js'), path.join(rollback, 'rollback-route.js'));
const rollbackIndex = path.join(rollback, 'v2/index.html');
writeFileSync(rollbackIndex, readFileSync(rollbackIndex, 'utf8').replace('</head>', '<script src="/rollback-route.js"></script></head>'));
let switchStage;
const fixture = createServer(async (req, res) => {
  if (req.method === 'POST' && req.url.startsWith('/__teststage?') && switchStage) {
    try { await switchStage(new URL(req.url, 'http://fixture').searchParams.get('stage')); res.writeHead(200); res.end('switched'); }
    // Keep error details on this console, not in the HTTP response.
    catch (error) { console.error(error); res.writeHead(500); res.end('stage switch failed'); }
    return;
  }
  if (req.method !== 'GET') { res.writeHead(405); res.end('No writes in this test'); return; }
  const p = new URL(req.url, 'http://fixture').pathname;
  const data = p.endsWith('/me') ? { user: { user_id: 'synthetic', email: 'qa@example.invalid', user_role: 'OWNER', full_name: 'QA' }, business: { business_id: 'synthetic', company_name: 'Transition fixture' } }
    : p.endsWith('/fleet') || p.endsWith('/fleet/health') ? { total_peers: 0, peers: [] }
    : p.endsWith('/sites') ? { sites: [] } : p.endsWith('/routers') ? { routers: [] }
    : p.endsWith('/router-connections') ? { connections: [] } : {};
  res.writeHead(200, { 'Content-Type': 'application/json' }); res.end(JSON.stringify(data));
});
const reserve = async () => { const s = createServer(); await new Promise(r => s.listen(0, '127.0.0.1', r)); const p = s.address().port; await new Promise(r => s.close(r)); return p; };
let browser, processHandle;
const checks = [];
try {
  await new Promise(r => fixture.listen(0, '127.0.0.1', r));
  const apiPort = fixture.address().port, port = Number(process.env.TRANSITION_PORT) || await reserve(), admin = await reserve();
  const origin = 'http://127.0.0.1:' + port;
  const configure = async (source, directory, first = false) => {
    let text = readFileSync(source, 'utf8').replace('import security-headers.caddy', 'import ' + path.join(root, 'deploy/security-headers.caddy'));
    text = `{ admin 127.0.0.1:${admin}\n auto_https off\n}\n` + text.replace(':80 {', `:${port} {`).replaceAll('root * /srv/v2', 'root * ' + path.join(directory, 'v2')).replaceAll('root * /srv', 'root * ' + directory).replaceAll('api:8080', '127.0.0.1:' + apiPort);
    const config = path.join(temp, 'Caddyfile'); writeFileSync(config, text);
    if (first) {
      processHandle = spawn(caddy, ['run', '--config', config, '--adapter', 'caddyfile'], { stdio: ['ignore', 'ignore', 'pipe'] });
      processHandle.stderr.on('data', () => {});
      for (let i = 0; i < 80; i++) { try { if ((await fetch(origin + '/v2/')).ok) return; } catch {} await new Promise(r => setTimeout(r, 50)); }
      throw Error('Caddy did not start');
    }
    const adapted = await new Promise((resolve, reject) => {
      const child = spawn(caddy, ['adapt', '--config', config, '--adapter', 'caddyfile'], { stdio: ['ignore', 'pipe', 'pipe'] });
      let out = '', error = ''; child.stdout.on('data', c => out += c); child.stderr.on('data', c => error += c);
      child.on('exit', code => code ? reject(Error(error)) : resolve(out)); child.on('error', reject);
    });
    const response = await fetch('http://127.0.0.1:' + admin + '/load', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: adapted });
    assert.ok(response.ok, await response.text());
  };
  const oldConfig = path.join(previous, 'deploy/Caddyfile.go-staging'), nextConfig = path.join(root, 'deploy/Caddyfile.go-staging');
  await configure(oldConfig, oldRoot, true);
  // The shared cloud browser can exercise the same fixture when process sandboxing blocks a new Chromium.
  switchStage = async (stage) => {
    if (stage === 'old') return configure(oldConfig, oldRoot);
    if (stage === 'candidate') return configure(nextConfig, path.join(root, 'vercel-public'));
    if (stage === 'negative') return configure(nextConfig, negative);
    if (stage === 'rollback') return configure(path.join(root, 'deploy/compat/Caddyfile.rollback'), rollback);
    if (stage === 'missing-map') {
      const brokenConfig = path.join(temp, 'broken.Caddyfile');
      writeFileSync(brokenConfig, readFileSync(nextConfig, 'utf8').replace('    encode gzip', '    @missingMap path /app/assets/SiteMap-*.js\n    respond @missingMap "simulated missing map" 404\n    encode gzip'));
      return configure(brokenConfig, path.join(root, 'vercel-public'));
    }
    throw Error('Unknown local test stage');
  };
  if (process.env.TRANSITION_MANUAL === '1') {
    console.log(JSON.stringify({ origin, control: 'http://127.0.0.1:' + apiPort + '/__teststage', temp }));
    await new Promise(() => {});
  }
  if (process.env.TRANSITION_HTTP_ONLY === '1') {
    await switchStage('candidate');
    const manifest = JSON.parse(readFileSync(path.join(root, 'deploy/compat/console-5931ed0/manifest.json'), 'utf8'));
    for (const [name, hash] of Object.entries(manifest.files)) {
      const response = await fetch(origin + '/v2/assets/' + name, { redirect: 'manual' });
      assert.equal(response.status, 200, name);
      assert.equal(createHash('sha256').update(Buffer.from(await response.arrayBuffer())).digest('hex'), hash, name);
    }
    assert.equal((await fetch(origin + '/v2/assets/absent.js')).status, 404);
    for (const route of ['/app/sites', '/signin', '/pricing']) assert.equal((await fetch(origin + route)).status, 200, route);
    assert.equal((await fetch(origin + '/v2/sites?keep=1', { redirect: 'manual' })).headers.get('location'), '/app/sites?keep=1');
    await switchStage('negative');
    assert.equal((await fetch(origin + '/v2/assets/index-BkXQbGXZ.js')).status, 404);
    await switchStage('rollback');
    for (const route of ['/app/activity?source=rollback', '/signin', '/pricing']) assert.equal((await fetch(origin + route, { redirect: 'manual' })).status, 200, route);
    assert.match(await (await fetch(origin + '/app/activity')).text(), /rollback-route.js/);
    assert.equal((await fetch(origin + '/v2/assets/index-BkXQbGXZ.js')).status, 200);
    console.log('HTTP transition checks passed: 66 exact assets, missing assets 404, negative control, clean routes, query redirect and rollback HTML. Browser behavior is not covered by HTTP-only mode.');
  } else {
  browser = await chromium.launch({ executablePath: process.env.CHROME_BIN || '/usr/bin/chromium', headless: true, args: ['--no-sandbox'] });
  const context = await browser.newContext({ viewport: { width: 390, height: 844 } });
  await context.addInitScript(() => { localStorage.setItem('mm2_lang', 'en'); });
  const page = await context.newPage();
  await page.goto(origin + '/v2/#/sites');
  await page.getByRole('button', { name: 'Add site', exact: true }).waitFor();
  // Keep an old entry/module graph in memory; do not reload it during the release switch.
  const documentId = await page.evaluate(() => { window.transitionDocument = Math.random(); return window.transitionDocument; });
  await configure(nextConfig, path.join(root, 'vercel-public'));
  const errors = []; page.on('pageerror', e => errors.push(e.message));
  await page.getByRole('button', { name: 'Add site', exact: true }).click();
  await page.locator('#s-name').fill('Keep my unsaved site');
  await page.locator('.site-map').waitFor();
  assert.equal(await page.locator('#s-name').inputValue(), 'Keep my unsaved site');
  assert.equal(await page.evaluate(() => window.transitionDocument), documentId);
  assert.deepEqual(errors, []); checks.push('old tab loads previously unopened map chunk after upgrade; input and document preserved');
  const oldAsset = '/v2/assets/Guide-crNcwfXW.js';
  assert.equal((await fetch(origin + oldAsset)).status, 200);
  assert.equal((await fetch(origin + '/v2/assets/no-such.js')).status, 404);
  checks.push('preserved old assets are 200; missing old assets are genuine 404');
  await configure(nextConfig, negative);
  assert.equal((await fetch(origin + oldAsset)).status, 404); checks.push('negative control fails when compatibility assets are removed');
  await configure(nextConfig, path.join(root, 'vercel-public'));
  // Future chunk failure after typing: local map boundary must leave the adjacent form alive.
  const nextContext = await browser.newContext({ viewport: { width: 390, height: 844 } });
  await nextContext.addInitScript(() => { localStorage.setItem('mm2_lang', 'en'); });
  const next = await nextContext.newPage();
  let releaseMissingMap;
  const mapReady = new Promise(resolve => { releaseMissingMap = resolve; });
  await next.route('**/assets/SiteMap-*.js', async route => {
    await mapReady;
    await route.fulfill({ status: 404, contentType: 'text/plain', body: 'simulated missing chunk' });
  });
  await next.goto(origin + '/app/sites');
  await next.getByRole('button', { name: 'Add site', exact: true }).click();
  await next.locator('#s-name').fill('Do not lose this');
  releaseMissingMap();
  await next.getByRole('heading', { name: 'Part of the app could not load' }).waitFor();
  let confirms = 0; next.on('dialog', async dialog => { confirms++; await dialog.dismiss(); });
  await next.getByRole('button', { name: 'Reload', exact: true }).click();
  assert.equal(confirms, 1); assert.equal(await next.locator('#s-name').inputValue(), 'Do not lose this');
  checks.push('failed new map stays inline; cancelling reload retains typed input');
  // A rollback serves clean paths in place, so previously cached /v2/ -> /app/ redirects cannot loop.
  await configure(path.join(root, 'deploy/compat/Caddyfile.rollback'), rollback);
  await page.goto(origin + '/app/activity?source=rollback');
  await page.getByRole('heading', { name: 'Activity', exact: true }).waitFor();
  assert.equal(new URL(page.url()).pathname, '/app/activity');
  assert.match(new URL(page.url()).hash, /^#\/activity\?source=rollback$/);
  assert.equal((await fetch(origin + '/signin')).status, 200);
  assert.equal((await fetch(origin + '/pricing')).status, 200);
  checks.push('rollback opens shared /app/activity link and clean public URLs without a redirect loop');
  console.log(JSON.stringify({ passed: checks.length, checks }, null, 2));
  }
} finally {
  await browser?.close();
  if (processHandle) { const ended = new Promise(r => processHandle.once('exit', r)); processHandle.kill('SIGTERM'); await ended; }
  await new Promise(r => fixture.close(r));
  rmSync(temp, { recursive: true, force: true });
}
