import fs from 'node:fs';
import vm from 'node:vm';
import assert from 'node:assert/strict';

const shared = fs.readFileSync(new URL('../web/assets/js/app.js', import.meta.url), 'utf8');
const app = fs.readFileSync(new URL('../web/app.html', import.meta.url), 'utf8');
const c = vm.createContext({});
vm.runInContext(shared, c);
for (const [state, label] of Object.entries({approved:'Approved', rejected:'Rejected', pending:'Pending'})) {
  assert.match(c.businessStatusBadge(state, 'payment'), new RegExp(`>${label}<`));
}
for (const state of ['unused','used','expired','revoked','active','inactive']) {
  assert.ok(!c.businessStatusBadge(state, 'voucher').includes('Unknown'));
}
assert.match(c.businessStatusBadge('<script>', 'payment'), />Unknown</);
assert.match(c.badge('up'), />Up</);
assert.match(c.guestVoucherCapabilityMessage({failed:true,status:404},true), /not enabled/);
assert.match(c.guestVoucherCapabilityMessage({failed:true,status:403},true), /permission/);
assert.match(c.guestVoucherCapabilityMessage({failed:true},true), /Reload/);
assert.match(c.guestVoucherCapabilityMessage(null,false), /requires an owner/);
assert.ok(!app.includes('TUNNEL ACTIVE'));
assert.ok(app.includes('MANAGEMENT REACHABLE'));
assert.ok(!app.includes('<span>Live paths</span>'));
assert.ok(app.includes('Guest voucher creation unavailable'));
assert.ok(app.includes('Could not load vouchers. Reload this page'));
for (const view of ['wan','pppoe','wifi','existing']) {
  assert.match(app.slice(app.indexOf('const PRODUCT_GUIDE')), new RegExp(`\\["${view}",`));
}
assert.ok(app.includes('businessStatusBadge(state, "payment")'));
assert.ok(app.includes('businessStatusBadge(v.status, "voucher")'));
console.log('Audit UI regressions passed: payment/voucher states, truthful topology, unavailable capability, guide coverage.');
