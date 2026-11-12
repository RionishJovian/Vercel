import { createHash } from 'node:crypto';
import { mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { execFile, execFileSync } from 'node:child_process';
import { promisify } from 'node:util';

// Explicit regeneration tool, not part of normal builds. Read-only requests verify every byte against
// the currently deployed release before writing the public compatibility snapshot.
const [build, output] = process.argv.slice(2);
if (!build || !output) throw Error('Usage: node scripts/capture-legacy-console.mjs PREVIOUS_DIST OUTPUT');
const entries = readdirSync(path.join(build, 'assets')).sort();
if (entries.length !== 66) throw Error('Expected the 66 assets from release 5931ed0');
const files = {};
const run = promisify(execFile);
let cursor = 0, checked = 0;
async function verifyNext() {
 while (cursor < entries.length) {
  const name = entries[cursor++];
  if (!/^[A-Za-z0-9_-][A-Za-z0-9_.-]*\.(?:js|mjs|css|woff2?)$/.test(name) || name.includes('..')) throw Error('Unexpected asset filename: ' + name);
  const bytes = readFileSync(path.join(build, 'assets', name));
  const { stdout: served } = await run('curl', ['--fail', '--silent', '--show-error', '--max-time', '30', 'https://megamagic.tech/v2/assets/' + name], { encoding: 'buffer', maxBuffer: 8 * 1024 * 1024 });
  if (!bytes.equals(served)) throw Error('Build does not match production: ' + name);
  files[name] = createHash('sha256').update(bytes).digest('hex');
  checked++;
  if (checked % 12 === 0) console.log('Verified ' + checked + '/' + entries.length + ' deployed assets');
 }
}
await Promise.all(Array.from({ length: 6 }, verifyNext));
mkdirSync(output, { recursive: true });
const archive = path.join(path.resolve(output), 'assets.tar.gz');
execFileSync('tar', ['--sort=name', '--mtime=UTC 2026-11-12', '--owner=0', '--group=0', '--numeric-owner', '-czf', archive, '-C', path.resolve(build), 'assets']);
writeFileSync(path.join(output, 'manifest.json'), JSON.stringify({ revision: '5931ed011342ec1f9fa94f84024df42c371c9a0a', verifiedAt: new Date().toISOString(), source: 'All files byte-compared to https://megamagic.tech/v2/assets/ before migration', archiveSha256: createHash('sha256').update(readFileSync(archive)).digest('hex'), files: Object.fromEntries(entries.map(name => [name, files[name]])) }, null, 2) + '\n');
console.log('Captured ' + entries.length + ' assets; every file matches the deployed release');
