import assert from 'node:assert/strict';
import fs from 'node:fs';
import {JSDOM} from 'jsdom';
import {test} from 'node:test';

const source=fs.readFileSync('web/assets/js/network-control.js','utf8');
const hubSource=fs.readFileSync('web/assets/js/hub-management.js','utf8');
const router={device_id:'selected',hardware_name:'Selected router'};
const missing=Object.assign(new Error('HTTP 409'),{data:{code:'router_management_required',error:'Complete Connect Magic Hub, then refresh status.'}});
const tick=()=>new Promise(resolve=>setImmediate(resolve));
function setup(){
 const dom=new JSDOM('<details data-hub-enrollment="other"><summary>Other connection</summary></details><details data-hub-enrollment="selected"><summary>Connect Magic Hub</summary></details><main id="host"></main>',{runScripts:'outside-only',url:'https://example.test/app.html#gateways'});
 dom.window.eval(source);dom.window.eval(hubSource);
 return {w:dom.window,host:dom.window.document.querySelector('#host')};
}

test('Missing connection opens only the selected router setup; retry reads without writes',async(t)=>{
 const {w,host}=setup();t.after(()=>w.close());let reads=0,posts=0;
 const api={async get(){reads++;if(reads===1)throw missing;return {inventory:{interfaces:[],objects:{},available:{},write_enabled:false}}},async post(){posts++;throw Error('Unexpected write')}};
 await w.MMNetwork.mount(host,'pppoe',[router],true,api);
 assert.match(host.textContent,/Connect this router first/);
 assert.equal(host.querySelector('form'),null);
 host.querySelector('[data-connect-router]').click();
 assert.equal(w.document.querySelector('[data-hub-enrollment="selected"]').open,true);
 assert.equal(w.document.querySelector('[data-hub-enrollment="other"]').open,false);
 await host.querySelector('[data-retry-read]').onclick();
 assert.match(host.textContent,/Live device settings loaded/);
 assert.equal(posts,0);
});

test('Unreachable configured router remains a read failure, with safe text and retry',async(t)=>{
 const {w,host}=setup();t.after(()=>w.close());
 const api={async get(){throw Object.assign(new Error('HTTP 502'),{data:{error:'Connection <img src=x onerror=alert(1)> unavailable'}})}};
 await w.MMNetwork.mount(host,'wan',[router],true,api);
 assert.match(host.textContent,/Could not read this device safely/);
 assert.equal(host.querySelector('[data-connect-router]'),null);
 assert.equal(host.querySelector('img'),null);
 assert.ok(host.querySelector('[data-retry-read]'));
 assert.equal(host.querySelector('form'),null);
});

test('Router overview shares the actionable missing-connection state',async(t)=>{
 const {w,host}=setup();t.after(()=>w.close());let writes=0;
 w.MMHubManagement.mount(host,[router],true,{async get(){throw missing},async post(){writes++}});
 host.querySelector('[data-hub-section="overview"]').click();await tick();
 assert.match(host.querySelector('[data-health-status]').textContent,/Connect this router first/);
 assert.ok(host.querySelector('[data-connect-router]'));
 assert.equal(writes,0);
});
