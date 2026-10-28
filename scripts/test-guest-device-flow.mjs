import assert from 'node:assert/strict';
import fs from 'node:fs';
import {JSDOM} from 'jsdom';

const sources=Object.fromEntries(['device-workspace','voucher-workspace','workflow-revamp'].map(name=>[name,fs.readFileSync(`web/assets/js/${name}.js`,'utf8')]));
const tick=()=>new Promise(resolve=>setImmediate(resolve));
const routers=[{device_id:'a',site_id:'site',hardware_name:'Main <gateway>',deployment_layer:'MAIN_GATEWAY'}, {device_id:'b',site_id:'site',hardware_name:'Other gateway',deployment_layer:'MAIN_GATEWAY'}, {device_id:'c',site_id:'other',hardware_name:'Branch gateway',deployment_layer:'MAIN_GATEWAY'}];
const sites=[{site_id:'site',name:'Cafe'},{site_id:'other',name:'Branch'}];
const session=(user,address)=>({id:'*1',values:{user,address,'session-time-left':'4m','bytes-in':'100','bytes-out':'200'}});
const inventory={read_at:'2026-10-28T00:00:00Z',interfaces:[],available:{dhcp:true,hotspots:true,hotspot_sessions:true,wifi:false},objects:{dhcp:[{values:{interface:'guest',disabled:'no'}}],hotspots:[{values:{interface:'guest',disabled:'no'}}],hotspot_sessions:[session('<img src=x onerror=bad()>','10.5.50.2')]}};
const vouchers=[{id:'one',code:'MAIN-CODE',device_id:'a',site_id:'site',state:'generated'}, {id:'two',code:'OTHER-CODE',device_id:'b',site_id:'site',state:'activated'}, {id:'three',code:'BRANCH-CODE',device_id:'c',site_id:'other',state:'generated'}];
function setup(override) {
 const dom=new JSDOM('<main id="host"></main>',{url:'http://localhost/app.html#internet',runScripts:'outside-only'}),w=dom.window;
 Object.values(sources).forEach(source=>w.eval(source));
 w.sessionStorage.setItem('mm_selected_router','a');
 const calls=[],posts=[];
 const api={get:async path=>{calls.push(path);const custom=override?.(path);if(custom!==undefined)return custom;if(path==='/routers')return {routers};if(path==='/sites')return {sites};if(path==='/fleet/health')return {peers:[]};if(path.endsWith('/capabilities'))return {capabilities:{}};if(path.endsWith('/network/inventory'))return {inventory};if(path==='/voucher-inventory')return {vouchers};if(path==='/guests')return {records:[]};if(path.endsWith('/observations'))return {inventory:{...inventory,objects:{hotspot_sessions:[session(path.includes('/a/')?'MAIN-SESSION':'OTHER-SESSION','10.5.50.2')]}}};throw Error('Unexpected read '+path)},post:async(...args)=>{posts.push(args);throw Error('No writes allowed')}};
 const host=w.document.querySelector('#host');
 const open=async()=>{await w.MMDeviceWorkspace.mount(host,{api,canEdit:true,network:{mount:()=>{throw Error('No editor should open')}},icon:()=>''});host.querySelector('[data-workspace-tab="guest"]').click();await tick()};
 return {w,host,api,calls,posts,open};
}
const flow=setup();await flow.open();
assert.match(flow.host.textContent,/Services reported/);
assert.match(flow.host.textContent,/1 unused · 0 activated/,'Other routers must not influence this voucher count');
assert.match(flow.host.textContent,/1 active at last read/);
assert.match(flow.host.textContent,/RouterBOARD runs captive login/);
assert.equal(flow.host.querySelector('img'),null,'Session content must be escaped');
assert.equal(flow.host.querySelector('[data-guest-setting="wifi"]').disabled,true,'Unavailable radio has no control');
assert.equal(flow.posts.length,0,'Opening setup only reads');
assert.equal(flow.calls.filter(path=>path==='/voucher-inventory').length,1);
flow.host.querySelector('[data-workspace-tab="guest"]').dispatchEvent(new flow.w.KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true}));
assert.equal(flow.host.querySelector('[data-workspace-tab="config"]').getAttribute('aria-selected'),'true','Third tab participates in keyboard navigation');
flow.host.querySelector('[data-workspace-tab="guest"]').click();await tick();
assert.equal(flow.calls.filter(path=>path==='/voucher-inventory').length,1,'Changing tabs reuses its scoped snapshot');

// Follow the same saved context into both stock and live sessions.
await flow.w.MMVoucherWorkspace.mount(flow.host,{api:flow.api,canEdit:false,routers});
assert.equal(flow.host.querySelector('[data-filter="device"]').value,'a');
assert.equal(flow.host.querySelectorAll('[data-select]').length,1);
assert.match(flow.host.querySelector('[data-state=""]').textContent,/All · 1/,'Lifecycle counts must respect the device filter');
assert.match(flow.host.textContent,/MAIN-CODE/);assert(!flow.host.textContent.includes('OTHER-CODE'));
flow.host.querySelector('[data-reset]').click();assert.equal(flow.host.querySelectorAll('[data-select]').length,3,'All-router inventory stays available explicitly');
await flow.w.MMRevamp.mountGuests(flow.host,routers,flow.api,false,sites);
assert.equal(flow.host.querySelector('[data-guest-device]').value,'a');
assert.match(flow.host.querySelector('[data-guest-results]').textContent,/MAIN-SESSION/);
assert(!flow.host.querySelector('[data-guest-results]').textContent.includes('OTHER-SESSION'));
const device=flow.host.querySelector('[data-guest-device]');device.value='b';device.dispatchEvent(new flow.w.Event('change'));assert.equal(flow.w.sessionStorage.getItem('mm_selected_router'),'b');
const site=flow.host.querySelector('[data-guest-site]');site.value='other';site.dispatchEvent(new flow.w.Event('change'));assert.equal(device.value,'');assert.equal(flow.w.sessionStorage.getItem('mm_selected_router'),null);
assert.equal(device.querySelector('option[value="a"]'),null,'Site change must discard foreign device options');

// Inventory failure and partial session reads cannot become fabricated zeroes.
const failed=setup(path=>path==='/voucher-inventory'?Promise.reject(Error('denied')):path.endsWith('/network/inventory')?{inventory:{...inventory,available:{dhcp:true,hotspots:false,hotspot_sessions:false}}}:undefined);await failed.open();
assert.match(failed.host.textContent,/Voucher inventory unavailable/);assert.match(failed.host.textContent,/Session reporting unavailable/);assert(!failed.host.textContent.includes('0 active at last read'));
failed.host.querySelector('[data-voucher-retry]').click();await tick();assert.equal(failed.calls.filter(p=>p==='/voucher-inventory').length,2);assert.equal(failed.posts.length,0);

// An old voucher response cannot overwrite a newly chosen router's facts.
let resolveOld,count=0;
const race=setup(path=>path==='/voucher-inventory'?(++count===1?new Promise(resolve=>resolveOld=resolve):{vouchers:[vouchers[1]]}):undefined);await race.open();
const select=race.host.querySelector('[data-context-device]');select.value='b';select.dispatchEvent(new race.w.Event('change'));await tick();
resolveOld({vouchers:[vouchers[0]]});await tick();
assert.equal(race.host.querySelector('[data-context-device]').value,'b');
assert.match(race.host.textContent,/0 unused · 1 activated/);assert(!race.host.textContent.includes('1 unused · 0 activated'));

// A stale remembered router is never used as a trusted request target.
const foreign=setup();foreign.w.sessionStorage.setItem('mm_selected_router','not-authorized');await foreign.w.MMVoucherWorkspace.mount(foreign.host,{api:foreign.api,canEdit:false,routers});assert.equal(foreign.host.querySelector('[data-filter="device"]').value,'');
console.log('Guest device flow passed: device-scoped evidence, stock/session continuity, site changes, escaping, keyboard tabs, read failures, retry and stale-response isolation; no writes.');
