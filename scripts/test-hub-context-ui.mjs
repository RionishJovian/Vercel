import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {JSDOM} from 'jsdom';

function setup(t,{multiple=false,saved='',canEdit=true}={}){
 const dom=new JSDOM('<main id="host"></main>',{runScripts:'outside-only',url:'https://example.test'}),w=dom.window;
 t.after(()=>w.close());w.AbortSignal=AbortSignal;
 for(const file of ['network-control','management-migration','hub-management'])w.eval(fs.readFileSync(`web/assets/js/${file}.js`,'utf8'));
 const routers=[{device_id:'one',hardware_name:'Router <One>'},...(multiple?[{device_id:'two',hardware_name:'Router Two'}]:[])];
 if(saved)w.sessionStorage.setItem('mm_selected_router',saved);
 const inventory={write_enabled:true,wifi_write_enabled:false,interfaces:[{name:'ether2',type:'ether',running:true}],health:{uptime:'1d'},available:{filters:true,queues:true,addresses:true,routes:true},objects:{filters:[],queues:[],addresses:[],routes:[]}};
 const requests=[],writes=[];
 const api={async get(path){requests.push(path);return path.includes('/management-migration')?{enabled:true,job:null}:path.endsWith('/gateway/defaults')?{defaults:{}}:{inventory}},async post(path,body){writes.push({path,body});return {review:{confirmation:'fixture',steps:['Reviewed operation'],warnings:[]}}}};
 const host=w.document.querySelector('#host');w.MMHubManagement.mount(host,routers,canEdit,api);
 const choose=key=>host.querySelector(`[data-hub-section="${key}"]`).onclick();
 const visibleDeviceSelectors=()=>[...host.querySelectorAll('[data-router],[data-move-router],[data-health-router],[data-hub-router]')].filter(el=>!el.closest('[hidden]'));
 return {w,host,api,requests,writes,inventory,routers,choose,visibleDeviceSelectors};
}

test('Every Hub action uses the only router immediately without device selection',async t=>{
 const f=setup(t);assert.equal(f.requests.length,0);assert.equal(f.visibleDeviceSelectors().length,0);
 assert.equal(f.host.querySelector('[data-hub-target]').textContent,'Router <One>');assert.equal(f.host.querySelector('img'),null);
 for(const section of ['overview','wan','ip','existing','protection','qos','wifi','pppoe','migration','maintenance']){
  await f.choose(section);
  assert.equal(f.visibleDeviceSelectors().length,0,section+' must not ask for a device');
 }
 assert.ok(f.requests.some(path=>path==='/routers/one/network/overview'));
 assert.ok(f.requests.some(path=>path==='/routers/one/management-migration'));
 assert.ok(f.requests.every(path=>path.startsWith('/routers/one/')));
 assert.equal(f.writes.length,0,'Opening any category must not write');
 assert.equal(f.w.sessionStorage.getItem('mm_selected_router'),'one');
});

test('Multiple routers share one remembered context across all editors',async t=>{
 const f=setup(t,{multiple:true,saved:'two'}),picker=f.host.querySelector('[data-hub-router]');
 assert.equal(picker.value,'two');await f.choose('protection');
 assert.equal(f.visibleDeviceSelectors().length,1);assert.equal(f.host.querySelector('[data-router]').options.length,2);
 assert.equal(f.requests.at(-1),'/routers/two/network/inventory');
 picker.value='one';await picker.onchange();assert.equal(f.requests.at(-1),'/routers/one/network/inventory');
 await f.choose('migration');assert.equal(f.requests.at(-1),'/routers/one/management-migration');
 assert.equal(f.host.querySelector('[data-move-router]').value,'one');assert.equal(f.visibleDeviceSelectors().length,1);
 await f.choose('overview');assert.equal(f.requests.at(-1),'/routers/one/network/overview');
 assert.equal(f.host.querySelector('[data-health-router]'),null);assert.equal(f.w.sessionStorage.getItem('mm_selected_router'),'one');
});

test('Stale or foreign remembered IDs never select an arbitrary router',async t=>{
 const f=setup(t,{multiple:true,saved:'foreign'});await f.choose('protection');
 assert.equal(f.requests.length,0);assert.match(f.host.textContent,/Choose your router once above/);
 const picker=f.host.querySelector('[data-hub-router]');picker.value='two';await picker.onchange();
 assert.deepEqual(f.requests,['/routers/two/network/inventory']);
 assert.equal(f.writes.length,0);
});

test('A late read from the previous router cannot replace the new context',async t=>{
 const f=setup(t,{multiple:true,saved:'one'});let release;
 f.api.get=path=>path.includes('/one/')?new Promise(resolve=>release=resolve):Promise.resolve({inventory:{...f.inventory,health:{uptime:'NEW'}}});
 const old=f.choose('overview'),picker=f.host.querySelector('[data-hub-router]');picker.value='two';await picker.onchange();
 release({inventory:{...f.inventory,health:{uptime:'STALE'}}});await old;
 assert.match(f.host.querySelector('[data-health-status]').textContent,/NEW/);assert.ok(!f.host.textContent.includes('STALE'));
});

test('Shared context and categories stay locked throughout a confirmed network write',async t=>{
 const f=setup(t,{multiple:true,saved:'one'});await f.choose('protection');
 const form=f.host.querySelector('form');form.reportValidity=()=>true;form.elements.interface.value='ether2';await form.onsubmit({preventDefault(){}});
 assert.equal(f.host.querySelector('[data-apply]').disabled,true);
 assert.match(f.host.querySelector('[data-review]').textContent,/Router: Router <One>/);assert.equal(f.host.querySelector('[data-review] img'),null);
 const consent=f.host.querySelector('[data-consent]');consent.checked=true;consent.onchange({target:consent});
 let release;f.api.post=(path,body)=>{f.writes.push({path,body});return new Promise(resolve=>release=resolve)};
 const writing=f.host.querySelector('[data-apply]').onclick(),picker=f.host.querySelector('[data-hub-router]');
 assert.equal(picker.disabled,true);picker.value='two';picker.onchange();assert.equal(picker.value,'one');
 await f.choose('migration');assert.ok(f.host.querySelector('[data-task="firewall"]'));assert.equal(f.host.querySelector('[data-move-router]'),null);assert.match(f.host.querySelector('[data-review]').textContent,/Backing up, applying/);
 assert.equal(f.writes.at(-1).path,'/routers/one/network/apply');
 release({result:{verified:true,backup:'fixture'}});await writing;assert.equal(picker.disabled,false);
});

test('Migration preview locks shared context and still requires separate apply consent',async t=>{
 const f=setup(t,{multiple:true,saved:'one'});await f.choose('migration');let release;
 f.api.post=(path,body)=>{f.writes.push({path,body});return new Promise(resolve=>release=resolve)};
 const preview=f.host.querySelector('[data-move-preview]').onclick(),picker=f.host.querySelector('[data-hub-router]');
 assert.equal(picker.disabled,true);picker.value='two';picker.onchange();assert.equal(picker.value,'one');
 await f.choose('protection');assert.ok(f.host.querySelector('[data-move-router]'));
 release({});await preview;assert.equal(picker.disabled,false);assert.equal(f.writes.length,1);
 assert.equal(f.writes[0].path,'/routers/one/management-migration/preview');
});

test('Removing selection steps preserves read-only role and server write holds',async t=>{
 const f=setup(t,{canEdit:false});await f.choose('protection');assert.equal(f.host.querySelector('form'),null);assert.equal(f.visibleDeviceSelectors().length,0);assert.equal(f.writes.length,0);
});
