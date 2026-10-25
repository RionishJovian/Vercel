import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {JSDOM} from 'jsdom';
const code = fs.readFileSync('web/assets/js/workspaces.js','utf8');
const source = fs.readFileSync('web/app.html','utf8');
function setup(html='') {
 const dom = new JSDOM(`<main>${html}</main>`,{runScripts:'outside-only',url:'https://example.test'});
 dom.window.eval(code); return {window:dom.window,host:dom.window.document.querySelector('main'),workspace:dom.window.MMWorkspace};
}
test('related routes stay reachable with one active parent and filtered child navigation', () => {
 const {workspace,host} = setup();
 for (const route of ['sites','routers','topology','fleet','internet','network','wan','pppoe','wifi','existing','backups','incidents','security','scans','events','managed-devices']) {
  assert.equal(workspace.group(route),'gateways');
  host.innerHTML = workspace.navigation(route);
  assert.equal(host.querySelectorAll('nav:first-child a[aria-current="page"]').length,1);
  assert.ok(host.querySelector(`a[href="#${route}"]`),`${route} is reachable`);
 }
 host.innerHTML = workspace.navigation('jellyfin-portal', key => key !== 'jellyfin-portal');
 assert.equal(host.querySelector('[href="#jellyfin-portal"]'),null);
 assert.equal(workspace.group('portal'),'vouchers');
 assert.equal(workspace.group('jellyfin-portal'),'media');
});
test('sidebar has seven customer destinations; legacy and platform access checks remain', () => {
 const dom = new JSDOM(source); const rail = dom.window.document.querySelector('.rail-nav-scroll');
 assert.equal(rail.querySelectorAll(':scope > button:not([hidden])').length,7);
 assert.equal(rail.querySelector('[data-view="team"]').closest('details').id,'rail-advanced');
 assert.ok(source.includes('button.hidden = !canOpenView(button.dataset.view, me)'));
 assert.ok(source.includes('if (!canOpenView(name, me))'));
});
test('voucher tasks keep unsaved fields, hidden forms and live status; switching sends no requests', () => {
 const {window,host,workspace} = setup(`<p role="alert">Installation unconfirmed</p><div data-voucher-panel="stock">Stock</div><div data-voucher-panel="create"><input id="draft" value="Daily"></div><section data-voucher-panel="shops"><form hidden id="new-shop"></form></section><div data-voucher-panel="print"><input id="pl-bname"></div>`);
 window.fetch = () => { throw Error('Task switches must not fetch or write'); };
 workspace.mountVouchers(host);
 assert.equal(host.querySelector('[data-voucher-panel="stock"]').hidden,false);
 host.querySelector('[data-voucher-tab="create"]').click();
 host.querySelector('#draft').value = 'Unsaved plan';
 host.querySelector('[data-voucher-tab="shops"]').click();
 assert.equal(host.querySelector('#new-shop').hidden,true);
 host.querySelector('[data-voucher-tab="create"]').click();
 assert.equal(host.querySelector('#draft').value,'Unsaved plan');
 assert.equal(host.querySelector('[role="alert"]').hidden,false);
 let redraw = 0;host.querySelector('#pl-bname').addEventListener('input', () => redraw++);
 host.querySelector('[data-voucher-tab="print"]').click();
 assert.equal(redraw,1);assert.equal(host.querySelector('[data-voucher-panel="print"]').hidden,false);
 workspace.mountVouchers(host,'print');
 assert.equal(host.querySelector('[data-voucher-panel="print"]').hidden,false);
});
test('rendered voucher page exposes all tasks and print shortcut opens its actual panel', async () => {
 const {window:w,host} = setup('<div id="content"></div>');
 Object.assign(w, {ME:{business:{business_id:'fixture'},user:{user_role:'OWNER'}},SITE_EDITOR_ROLES:['OWNER','ADMIN','DEVELOPER'],asArray:x=>Array.isArray(x)?x:[],esc:x=>String(x??''),businessStatusBadge:x=>x,guestVoucherCapabilityMessage:()=>'',ts:x=>String(x),MM:{get:async path=>path==='/guest-vouchers/config'?{enabled:true,batch_recovery_enabled:true}:path==='/guest-vouchers/batches'?{reseller_assignment_enabled:true,stock_batches:[{batch_id:'fixture-batch',plan_label:'Daily',total_count:3,installed_count:0,activated_count:0}]}:path==='/routers'?{routers:[]}:{}}});
 w.HTMLElement.prototype.scrollIntoView = () => {};
 w.eval(source.slice(source.indexOf('const SHEET_MM ='),source.indexOf('/* ---------- Magic Coins')));
 await w.renderVouchers(host);
 assert.equal(host.querySelectorAll('[data-voucher-tab]').length,4);
 host.querySelector('[data-shop-print]').click();
 assert.equal(host.querySelector('[data-voucher-panel="print"]').hidden,false);
 assert.equal(host.querySelector('#pl-issue-batch').value,'fixture-batch');
 assert.equal(host.querySelectorAll('form#guest-voucher-form').length,1);
});
