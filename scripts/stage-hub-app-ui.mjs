// Apply only Gateway onboarding hunks to the production HTML snapshot.
import fs from 'node:fs';
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';
const target=process.argv[2];assert.ok(target);
let diff;
try { diff=execFileSync('diff',['-u',target,fileURLToPath(new URL('../web/app.html',import.meta.url))],{encoding:'utf8',stdio:['ignore','pipe','pipe']}); }
catch(error){if(error.status!==1)throw error;diff=error.stdout;}
let source=fs.readFileSync(target,'utf8'),count=0;
for(const hunk of diff.split(/(?=^@@ )/m).slice(1)){
 const header=/^@@ -(\d+)/.exec(hunk);assert.ok(header);
 if(Number(header[1])<1300||Number(header[1])>=1700)continue;
 const lines=hunk.split('\n').slice(1).filter(line=>[' ','+','-'].includes(line[0]));
 const before=lines.filter(l=>l[0]===' '||l[0]==='-').map(l=>l.slice(1)).join('\n');
 const after=lines.filter(l=>l[0]===' '||l[0]==='+').map(l=>l.slice(1)).join('\n');
 assert.equal(source.split(before).length,2,'nonunique UI patch anchor');source=source.replace(before,after);count++;
}
assert.equal(count,4,'unexpected onboarding hunk count');
fs.writeFileSync(target,source);
console.log('Only four Gateway onboarding UI hunks staged.');
