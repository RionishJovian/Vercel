import test from 'node:test';
import assert from 'node:assert/strict';
import { cpSync, existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { installLegacyConsole } from './legacy-console-assets.mjs';

const source = path.resolve(import.meta.dirname, '../deploy/compat/console-5931ed0');
test('legacy release assets are complete and integrity-checked in the build output', () => {
  const dir = mkdtempSync(path.join(tmpdir(), 'mm-legacy-'));
  try {
    assert.equal(installLegacyConsole(source, dir), 66);
    assert.ok(existsSync(path.join(dir, 'assets/index-BkXQbGXZ.js')));
    assert.ok(existsSync(path.join(dir, 'assets/index-htuW4A0D.css')));
    assert.ok(!existsSync(path.join(dir, 'index.html')), 'the old entry page must not become canonical');
  } finally { rmSync(dir, { recursive: true, force: true }); }
});
test('tampered legacy asset aborts before copying any files', () => {
  const dir = mkdtempSync(path.join(tmpdir(), 'mm-legacy-bad-'));
  try {
    const input = path.join(dir, 'source'), out = path.join(dir, 'out');
    cpSync(source, input, { recursive: true });
    const manifestPath = path.join(input, 'manifest.json');
    const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'));
    manifest.files['index-BkXQbGXZ.js'] = '0'.repeat(64);
    writeFileSync(manifestPath, JSON.stringify(manifest));
    assert.throws(() => installLegacyConsole(input, out), /integrity mismatch/);
    assert.ok(!existsSync(out));
  } finally { rmSync(dir, { recursive: true, force: true }); }
});
test('Vercel retains legacy asset URLs instead of redirecting them to new hash names', () => {
  const config = JSON.parse(readFileSync(new URL('../vercel.json', import.meta.url), 'utf8'));
  const route = config.redirects.find(({ source }) => source.startsWith('/v2/:path('));
  assert.ok(route, 'legacy pages need a redirect');
  const matcher = new RegExp('^' + route.source.replace(':path', '') + '$');
  assert.equal(matcher.test('/v2/assets/Guide-crNcwfXW.js'), false);
  assert.equal(matcher.test('/v2/assets/no-such.js'), false);
  assert.equal(matcher.test('/v2/network/new'), true);
  assert.ok(config.redirects.some(({ source }) => source === '/v2/'));
});
