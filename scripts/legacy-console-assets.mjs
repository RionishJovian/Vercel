import { createHash } from 'node:crypto';
import { copyFileSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

/** Verify the bounded, public asset snapshot before adding it to either release output. */
export function installLegacyConsole(source, destination) {
  const manifest = JSON.parse(readFileSync(path.join(source, 'manifest.json'), 'utf8'));
  if (manifest.revision !== '5931ed011342ec1f9fa94f84024df42c371c9a0a') throw Error('Unexpected legacy console revision');
  const entries = Object.entries(manifest.files ?? {});
  if (entries.length !== 66) throw Error('Incomplete legacy console manifest');
  const archive = path.join(source, 'assets.tar.gz');
  if (createHash('sha256').update(readFileSync(archive)).digest('hex') !== manifest.archiveSha256) throw Error('Legacy archive integrity mismatch');
  const members = execFileSync('tar', ['-tzf', archive], { encoding: 'utf8' }).trim().split('\n');
  if (members.some(name => name !== 'assets/' && !entries.some(([file]) => name === 'assets/' + file))) throw Error('Unexpected legacy archive member');
  const unpacked = mkdtempSync(path.join(tmpdir(), 'mm-legacy-assets-'));
  try {
  execFileSync('tar', ['-xzf', archive, '-C', unpacked]);
  const names = readdirSync(path.join(unpacked, 'assets')).sort();
  if (JSON.stringify(names) !== JSON.stringify(entries.map(([name]) => name).sort())) throw Error('Legacy console file set differs from manifest');
  for (const [name, expected] of entries) {
    if (!/^[A-Za-z0-9_-][A-Za-z0-9_.-]*\.(?:js|mjs|css|woff2?)$/.test(name) || name.includes('..') || !/^[a-f0-9]{64}$/.test(expected)) throw Error('Invalid legacy asset manifest entry');
    const file = path.join(unpacked, 'assets', name);
    if (!lstatSync(file).isFile() || createHash('sha256').update(readFileSync(file)).digest('hex') !== expected) throw Error('Legacy asset integrity mismatch: ' + name);
  }
  // Only assets, never the previous index.html: old tabs keep their JS; new visits get the new console.
  const out = path.join(destination, 'assets');
  mkdirSync(out, { recursive: true });
  for (const [name] of entries) copyFileSync(path.join(unpacked, 'assets', name), path.join(out, name));
  return entries.length;
  } finally { rmSync(unpacked, { recursive: true, force: true }); }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  if (process.argv.length !== 4) throw Error('Usage: node legacy-console-assets.mjs SOURCE DESTINATION');
  console.log('Verified and installed ' + installLegacyConsole(path.resolve(process.argv[2]), path.resolve(process.argv[3])) + ' legacy console assets');
}
