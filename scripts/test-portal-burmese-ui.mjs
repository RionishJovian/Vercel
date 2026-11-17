import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';
const src=readFileSync(new URL('../web/assets/js/portal.js',import.meta.url),'utf8');
const block=src.match(/var MM = (\{[\s\S]*?\n\});/);
assert.ok(block,'portal.js must define the MM dictionary');
const MM=vm.runInNewContext('('+block[1]+')');
const keys=[...src.matchAll(/\bt\(\s*(?:"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')\s*,\s*"(\w+)"\s*\)/g)].map(m=>m[1]);
test('every hosted-portal t() key has a Burmese entry',()=>{
 assert.ok(keys.length>=60,'expected to find the portal t() calls, found '+keys.length);
 const missing=[...new Set(keys)].filter(k=>!(typeof MM[k]==='string'&&MM[k].trim()));
 assert.deepEqual(missing,[],'portal keys with no Burmese text: '+missing.join(', '));
});
test('portal has no raw English left in the voucher result',()=>{
 assert.doesNotMatch(src,/esc\("(Voucher recorded|Gateway internet access)/);
});
