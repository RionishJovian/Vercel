// Mechanical, fail-closed staging only. Does not contact WireGuard or listen.
import fs from 'node:fs';
import assert from 'node:assert/strict';
const target=process.argv[2];
assert.ok(target,'staged service path required');
let source=fs.readFileSync(target,'utf8');
const patch=fs.readFileSync(new URL('./hub-create-only-enrollment.patch',import.meta.url),'utf8');
for(const hunk of patch.split(/^@@.*$/m).slice(1)) {
 const lines=hunk.trimEnd().split('\n').filter(Boolean);
 const before=lines.filter(l=>l[0]===' '||l[0]==='-').map(l=>l.slice(1)).join('\n');
 const after=lines.filter(l=>l[0]===' '||l[0]==='+').map(l=>l.slice(1)).join('\n');
 assert.equal(source.split(before).length,2,'source drift or already applied patch');
 source=source.replace(before,after);
}
fs.writeFileSync(target,source);
console.log('Hub enrollment staged; no service or peer changes.');
