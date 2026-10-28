import assert from 'node:assert/strict';
import fs from 'node:fs';
import {createRequire} from 'node:module';
const {JSDOM}=createRequire(import.meta.url)('jsdom');
const source=fs.readFileSync('web/assets/js/device-workspace.js','utf8');
const networkSource=fs.readFileSync('web/assets/js/network-control.js','utf8');
const tick=()=>new Promise(resolve=>setImmediate(resolve));
const sites=[{site_id:'a',name:'First site'},{site_id:'b',name:'Other site'}];
const routers=[{device_id:'alpha',site_id:'a',hardware_name:'Gateway <Alpha>',vendor:'mikrotik'}, {device_id:'beta',site_id:'b',hardware_name:'Other gateway',vendor:'mikrotik'}];
const inventory=(name='ether1',write=false)=>({write_enabled:write,wifi_write_enabled:false,read_at:'2026-10-26T00:00:00Z',interfaces:[{name,type:'ether',running:true}],available:{dhcp_clients:true,pppoe_clients:true,routes:true,dhcp:true,pools:true,wifi:true,hotspot_sessions:false},objects:{dhcp_clients:[{id:'*1',values:{interface:name,gateway:'192.0.2.1',disabled:'no'}}],routes:[],pppoe_clients:[],dhcp:[],pools:[],wifi:[{id:'*2',values:{name:'wifi1','configuration.ssid':'<img src=x onerror=alert(1)>','security.passphrase':'PRIVATE_TEST_VALUE',disabled:'no'}}]}});
function setup({initialRouter='alpha',canEdit=true,write=false,getOverride}={}) {
  const dom=new JSDOM('<main id="host"></main>',{runScripts:'outside-only',url:'http://workspace.test'});
  const w=dom.window;w.eval(networkSource);w.eval(source);
  if(initialRouter)w.sessionStorage.setItem('mm_selected_router',initialRouter);
  const calls=[];let posts=[];const inv=inventory('ether1',write);
  const api={
    async get(path){
      calls.push(path);
      const custom=getOverride?.(path);if(custom!==undefined)return custom;
      if(path==='/sites')return {sites};
      if(path==='/routers')return {routers};
      if(path==='/fleet/health')return {peers:[{device_id:'alpha',state:'up'}]};
      if(path.endsWith('/capabilities'))return {capabilities:{detected:true,board_name:'hAP ax²',routeros_version:'fixture'}};
      if(path.endsWith('/network/inventory'))return {inventory:inv};
      if(path.endsWith('/gateway/defaults'))return {defaults:{}};
      throw Error('Unexpected read: '+path);
    },
    async post(path,body){posts.push({path,body});return {review:{steps:['Change fixture routing'],warnings:[],confirmation:'fixture'}};}
  };
  const host=w.document.querySelector('#host');
  const mount=()=>w.MMDeviceWorkspace.mount(host,{api,canEdit,icon:()=>''});
  return {w,host,api,calls,posts,inv,mount};
}

// A site and device persist; routine monitoring never writes or invents counts.
const first=setup();await first.mount();
assert.equal(first.host.querySelector('[data-context-site]').value,'a');
assert.equal(first.host.querySelector('[data-context-device]').value,'alpha');
assert.equal(first.host.querySelectorAll('[data-context-device] option').length,2,'Other-site routers must not be offered');
assert.match(first.host.textContent,/Unavailable/,'An unread session source cannot be shown as zero guests');
assert.equal(first.host.querySelector('[data-workspace-tool="hotspot"]').disabled,true);
assert.equal(first.posts.length,0);
assert.ok(first.host.textContent.includes('Gateway <Alpha>'));
assert.equal(first.host.querySelector('.device-workspace-identity img'),null,'Router identity must be escaped');

// Menu selection mounts the requested existing control, sharing the initial read.
first.host.querySelector('[data-workspace-tool="wifi"]').click();await tick();
assert.match(first.host.querySelector('[data-settings]').textContent,/Router changes are not enabled/);
assert.ok(first.host.querySelector('[data-current-settings]').textContent.includes('<img src=x onerror=alert(1)>'));
assert.equal(first.host.querySelector('[data-current-settings] img'),null);
assert.ok(!first.host.textContent.includes('PRIVATE_TEST_VALUE'));
assert.equal(first.calls.filter(path=>path.endsWith('/network/inventory')).length,1,'Opening settings reuses the current read');
assert.equal(first.host.querySelector('[data-router]').value,'alpha');
assert.equal(first.host.querySelector('[data-router]').closest('label').hidden,true,'Do not ask for the device twice');
first.host.querySelector('[data-workspace-tool="wan"]').click();await tick();
assert.match(first.host.querySelector('[data-settings]').textContent,/Router changes are not enabled/);
assert.equal(first.host.querySelector('[data-settings] form'),null);

const search=first.host.querySelector('[data-settings-search]');search.value='guest login';search.dispatchEvent(new first.w.Event('input'));
assert.equal(first.host.querySelector('[data-workspace-tool="login"]').hidden,false);
assert.equal(first.host.querySelector('[data-workspace-tool="wan"]').hidden,true);
search.value='';search.dispatchEvent(new first.w.Event('input'));
const siteSelect=first.host.querySelector('[data-context-site]');siteSelect.value='b';siteSelect.dispatchEvent(new first.w.Event('change'));await tick();
assert.equal(first.host.querySelector('[data-context-device]').value,'beta');
assert.equal(first.w.sessionStorage.getItem('mm_selected_router'),'beta');
assert.equal(first.w.sessionStorage.getItem('mm_selected_site'),'b');
assert.equal(first.host.querySelector('[data-workspace-tab="config"]').getAttribute('aria-selected'),'true');

// No role/write gate is bypassed by the new workspace.
const viewer=setup({canEdit:false,write:true});await viewer.mount();viewer.host.querySelector('[data-workspace-tool="wan"]').click();await tick();
assert.match(viewer.host.querySelector('[data-settings]').textContent,/owner or administrator/);
assert.equal(viewer.host.querySelector('form'),null);

// An in-flight device read must not overwrite a newly selected site/device.
let releaseOld;const stale=setup({getOverride:path=>path==='/routers/alpha/network/inventory'?new Promise(resolve=>releaseOld=resolve):path==='/routers/beta/network/inventory'?{inventory:inventory('beta-port')}:undefined});
const oldMount=stale.mount();await tick();
const changeSite=stale.host.querySelector('[data-context-site]');changeSite.value='b';changeSite.dispatchEvent(new stale.w.Event('change'));await tick();
releaseOld({inventory:inventory('OLD-PORT')});await oldMount;await tick();
assert.equal(stale.host.querySelector('[data-context-device]').value,'beta');
assert.match(stale.host.querySelector('#workspace-pane').textContent,/beta-port/);
assert.ok(!stale.host.querySelector('#workspace-pane').textContent.includes('OLD-PORT'));

// A remembered device outside the authorized response cannot trigger a read.
const foreign=setup({initialRouter:'foreign-device'});await foreign.mount();
assert.equal(foreign.host.querySelector('[data-context-device]').value,'');
assert.equal(foreign.calls.some(path=>path.includes('/routers/foreign')),false);
assert.equal(foreign.w.sessionStorage.getItem('mm_selected_router'),null);

// Vendor labels are metadata. A Ruijie record cannot enter RouterOS controls.
const otherVendor=setup({getOverride:path=>path==='/routers'?{routers:[{...routers[0],vendor:'ruijie'}]}:undefined});await otherVendor.mount();
assert.match(otherVendor.host.textContent,/RouterOS controls require a MikroTik device/);
assert.equal(otherVendor.calls.some(path=>path.includes('/routers/alpha/')),false);

// Parent context and settings navigation lock while a reviewed write is pending.
const writer=setup({write:true});await writer.mount();writer.host.querySelector('[data-workspace-tool="wifi"]').click();await tick();
assert.match(writer.host.querySelector('[data-settings]').textContent,/Wi-Fi editing is not enabled/);
writer.host.querySelector('[data-workspace-tool="wan"]').click();await tick();
const form=writer.host.querySelector('[data-form]');form.reportValidity=()=>true;await form.onsubmit({preventDefault(){}});
const consent=writer.host.querySelector('[data-consent]');consent.checked=true;consent.onchange({target:consent});
let finishWrite;writer.api.post=async()=>new Promise(resolve=>finishWrite=resolve);
const writing=writer.host.querySelector('[data-apply]').onclick();
assert.equal(writer.host.querySelector('[data-context-site]').disabled,true);
assert.equal(writer.host.querySelector('[data-context-device]').disabled,true);
assert.equal(writer.host.querySelector('[data-workspace-tab="monitor"]').disabled,true);
assert.equal(writer.host.querySelector('[data-workspace-tool="wifi"]').disabled,true);
finishWrite({result:{verified:true,backup:'fixture-backup'}});await writing;
assert.equal(writer.host.querySelector('[data-context-site]').disabled,false);
assert.equal(writer.host.querySelector('[data-workspace-tool="hotspot"]').disabled,true,'Unsupported tools remain disabled after the write');
assert.match(writer.host.querySelector('[data-review]').textContent,/Router settings verified/);

// A site API error is an error, never a fabricated empty site list.
const failed=setup({getOverride:path=>path==='/sites'?Promise.reject(Error('Read denied')):undefined});await failed.mount();
assert.match(failed.host.textContent,/Sites and devices could not be loaded/);
assert.match(failed.host.textContent,/Read denied/);
assert.ok(failed.host.querySelector('[data-workspace-retry]'));
console.log('Device workspace passed: site/device isolation, current read reuse, supported controls, escaped facts, no secrets, role/write gates, stale-response protection and context locking during apply.');

// Clearing a selection during a read must not leave a permanent Reading state.
let releaseSelection;const cleared=setup({getOverride:path=>path==='/routers/alpha/network/inventory'?new Promise(resolve=>releaseSelection=resolve):undefined});
const clearingMount=cleared.mount();await tick();
const clearingSelect=cleared.host.querySelector('[data-context-device]');clearingSelect.value='';clearingSelect.dispatchEvent(new cleared.w.Event('change'));await tick();
assert.match(cleared.host.querySelector('#workspace-pane').textContent,/Choose a device/);
releaseSelection({inventory:inventory('outdated')});await clearingMount;
assert.ok(!cleared.host.textContent.includes('outdated'));

// Topology and the workspace share device context, with route/read races held.
const appSource=fs.readFileSync(new URL('../web/app.html',import.meta.url),'utf8');
const topoSource=appSource.slice(appSource.indexOf('let topologyRenderRevision ='),appSource.indexOf('function topologyState('));
const topologyDOM=new JSDOM('<main><div id="content"></div></main>',{runScripts:'outside-only',url:'http://workspace.test/#topology'});
const tw=topologyDOM.window;tw.asArray=x=>Array.isArray(x)?x:[];tw.esc=x=>String(x??'');tw.loading=()=>'';
const topology={roots:routers.map(device=>({device,children:[]})),orphans:[],summary:{}};
tw.topologyTree=value=>value.roots.map(({device},index)=>`<button data-topo-detail="detail-${index}">${device.device_id}</button><div class="topo-detail" id="detail-${index}"><a href="#internet" data-config-device="${device.device_id}">Open device workspace</a></div>`).join('');
let delayTopology=null;tw.MM={get:async path=>path==='/topology/map'?(delayTopology?await delayTopology:topology):path==='/sites'?{sites}:{peers:[]}};
tw.eval(topoSource+'\nwindow.testRenderTopology=renderTopology;');
const main=tw.document.querySelector('main');tw.sessionStorage.setItem('mm_selected_router','beta');
await tw.testRenderTopology(main);
assert.equal(main.querySelector('#topology-site').value,'b');
assert.ok(main.querySelector('[data-config-device="beta"]'));
assert.equal(main.querySelector('[data-config-device="alpha"]'),null);
const topologySelector=main.querySelector('#topology-site');topologySelector.value='a';topologySelector.dispatchEvent(new tw.Event('change'));await tick();
assert.equal(tw.sessionStorage.getItem('mm_selected_site'),'a');
main.querySelector('[data-topo-detail]').click();assert.equal(tw.sessionStorage.getItem('mm_selected_router'),'alpha');
let releaseTopology;delayTopology=new Promise(resolve=>releaseTopology=resolve);
const slowTopology=tw.testRenderTopology(main,'a');await tick();delayTopology=null;
await tw.testRenderTopology(main,'b');releaseTopology(topology);await slowTopology;
assert.equal(main.querySelector('#topology-site').value,'b','An older topology read cannot change the new site');
let releaseDeparted;delayTopology=new Promise(resolve=>releaseDeparted=resolve);
const departed=tw.testRenderTopology(main);await tick();main.innerHTML='<div id="content">New route</div>';releaseDeparted(topology);await departed;
assert.equal(main.textContent,'New route','A topology read cannot overwrite a later route');
console.log('Topology context passed: device/site continuity, selected node, overlapping reads and route departure.');
