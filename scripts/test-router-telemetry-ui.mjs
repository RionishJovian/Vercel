import fs from 'node:fs';import assert from 'node:assert/strict';import {JSDOM} from 'jsdom';
const source=fs.readFileSync('web/assets/js/router-telemetry.js','utf8');const tick=()=>new Promise(resolve=>setImmediate(resolve));
const at=new Date().toISOString();
const payload=(device='router')=>({read_only:true,router_changed:false,telemetry:{device_id:device,source:'routeros-ssh',state:'live',checked_at:at,last_success_at:at,retry_after_seconds:15,history:[{read_at:at,cpu_percent:0,memory_percent:50}],snapshot:{device_id:device,read_at:at,observed_at:{cpu:at,memory:at,uptime:at,interfaces:at,hotspot:at,dhcp:at},resources:{cpu_percent:0,memory_total_bytes:'1073741824',memory_used_bytes:'536870912',uptime_seconds:90061},hotspot_sessions:0,dhcp_bound_leases:5,interfaces:[{id:'*1',name:'<img src=x onerror=bad()>',type:'ether',running:true,disabled:false,rx_bytes:'1000',tx_bytes:'2000',rx_bps:0,tx_bps:1000000}],issues:[]}}});
function setup(get){const dom=new JSDOM('<main><div id="host"></div></main>',{url:'http://localhost/app.html',runScripts:'outside-only',pretendToBeVisual:true}),w=dom.window;w.eval(source);const host=w.document.querySelector('#host');let calls=[];const api={get:async path=>{calls.push(path);return get?get(path):payload()}};const dispose=w.MMRouterTelemetry.mount(host,{api,deviceID:'router',deviceName:'Cafe <router>'});return {dom,w,host,calls,dispose}}
const first=setup();await tick();assert.match(first.host.textContent,/0.0%/);assert.match(first.host.textContent,/50.0%/);assert.match(first.host.textContent,/0 bps/);assert.match(first.host.textContent,/1.00 Mbps/);assert.match(first.host.textContent,/1d 1h 1m/);assert.match(first.host.textContent,/Hotspot sessions0/);assert.equal(first.host.querySelector('img'),null);assert.equal(first.calls[0],'/routers/router/telemetry');assert.equal(first.host.querySelector('[data-telemetry-state]').dataset.state,'live');
first.host.querySelector('[data-telemetry-pause]').click();assert.match(first.host.querySelector('[data-telemetry-state]').textContent,/paused/);assert.equal(first.host.querySelector('[data-telemetry-pause]').getAttribute('aria-pressed'),'true');first.dispose();first.dom.window.close();
let calls=0;const retained=setup(()=>{if(++calls===1)return payload();throw Error('network failure with private details')});await tick();retained.host.querySelector('[data-telemetry-refresh]').click();await tick();assert.match(retained.host.textContent,/Reconnecting/);assert.match(retained.host.textContent,/50.0%/);assert(!retained.host.textContent.includes('private details'));assert(!retained.host.textContent.toLowerCase().includes('unavailable'));assert.equal(retained.host.querySelector('[data-telemetry-state]').dataset.state,'retrying');retained.dispose();retained.dom.window.close();
const foreign=setup(()=>payload('different-router'));await tick();assert.equal(foreign.host.querySelector('[data-telemetry-state]').dataset.state,'retrying');assert(!foreign.host.textContent.includes('50.0%'));assert.equal(foreign.host.querySelector('img'),null);foreign.dispose();foreign.dom.window.close();
let resolve;const departed=setup(()=>new Promise(done=>resolve=done));departed.host.remove();await tick();resolve(payload());await tick();assert.equal(departed.w.document.querySelector('main').textContent,'');departed.dispose();departed.dom.window.close();
const helpers=new JSDOM('',{runScripts:'outside-only'});helpers.window.eval(source);const chart=helpers.window.MMRouterTelemetry.chart([{read_at:'2026-10-28T00:00:00Z',cpu_percent:0},{read_at:'2026-10-28T00:01:00Z',cpu_percent:100}],'cpu_percent','CPU');assert.equal((chart.match(/class="telemetry-chart-line"/g)||[]).length,2,'A sampling gap must not be drawn as continuous traffic');helpers.window.close();
console.log('Router telemetry UI passed: actual zeroes, units, escaped labels, retained readings, honest reconnects, source identity, pause, route departure and gaps.');

// Slow or failed settings inventory cannot remount a paused telemetry widget.
const parentDOM=new JSDOM('<main id="workspace"></main>',{url:'http://localhost/app.html',runScripts:'outside-only',pretendToBeVisual:true});
const parent=parentDOM.window;parent.eval(fs.readFileSync('web/assets/js/device-workspace.js','utf8'));
let mounted=0,disposed=0,releaseInventory;
parent.MMRouterTelemetry={mount(host){mounted++;host.textContent='Retained widget';return ()=>disposed++}};
const parentAPI={get:async path=>{
  if(path==='/sites')return {sites:[{site_id:'site',name:'Cafe'}]};
  if(path==='/routers')return {routers:[{device_id:'router',site_id:'site',hardware_name:'RouterBOARD'}]};
  if(path.endsWith('/network/inventory'))return new Promise((resolve,reject)=>releaseInventory=()=>reject(Error('Settings read failed')));
  if(path.endsWith('/capabilities'))return {capabilities:{}};
  if(path==='/fleet/health')return {peers:[]};
  throw Error('Unexpected read '+path);
}};
const workspace=parent.document.querySelector('#workspace'),mounting=parent.MMDeviceWorkspace.mount(workspace,{api:parentAPI,network:{},icon:()=>''});
await tick();assert.equal(mounted,1);const widget=workspace.querySelector('[data-router-telemetry]');
releaseInventory();await mounting;assert.equal(mounted,1);assert.equal(disposed,0);assert.equal(workspace.querySelector('[data-router-telemetry]'),widget);
assert.equal(workspace.querySelector('.device-workspace').dataset.mode,'monitor');
workspace.querySelector('[data-workspace-tab="config"]').click();assert.equal(disposed,1);parentDOM.window.close();
console.log('Telemetry workspace lifecycle passed: slow/failed inventory preserves the monitor, and Config disposes its subscription.');
