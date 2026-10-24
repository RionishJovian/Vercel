import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {JSDOM} from 'jsdom';
const code=fs.readFileSync('web/assets/js/hub-management.js','utf8');
const network=fs.readFileSync('web/assets/js/network-control.js','utf8');
function fixture(){
 const dom=new JSDOM('<main><div id="host"></div></main>',{runScripts:'outside-only',url:'https://example.test'}),w=dom.window;
 w.AbortSignal=AbortSignal;w.eval(network);w.eval(code);
 const host=w.document.querySelector('#host'),requests=[];
 const inventory={write_enabled:true,health:{'cpu-load':'3','uptime':'1d'},interfaces:[{name:'ether2',type:'ether',running:true}],available:{filters:true,bindings:true,queues:true,addresses:true,routes:true},objects:{filters:[],queues:[],bindings:[{id:'*A',values:{type:'bypassed',comment:'Printer',disabled:'no','mac-address':'02:11:22:33:44:55'}}],addresses:[{id:'*1',values:{address:'192.168.50.1/24',interface:'ether2'}}],routes:[]}};
 const api={get:async path=>{requests.push(['GET',path]);return {inventory}},post:async(path,body)=>{requests.push(['POST',path,body]);return {review:{confirmation:'fixture',steps:['Reviewed operation'],warnings:[]}}}};
 const routers=[{device_id:'router-one',hardware_name:'<img src=x>'}];w.MMHubManagement.mount(host,routers,true,api);
 return {w,host,requests,api,routers,inventory};
}
test('Hub categories render immediately without requesting or changing router state',()=>{
 const {host,requests}=fixture();assert.equal(host.querySelectorAll('[data-hub-section]').length,10);assert.equal(requests.length,0);
});
test('Protection is a typed reviewed operation and bypass creation is not offered',async()=>{
 const {host,requests}=fixture();await host.querySelector('[data-hub-section="protection"]').onclick();
 assert.equal(requests.length,1);assert.equal(requests[0][0],'GET');
 let form=host.querySelector('form');form.elements.interface.value='ether2';form.reportValidity=()=>true;await form.onsubmit({preventDefault(){}});
 assert.equal(requests[1][2].kind,'firewall');assert.equal(host.querySelector('[data-apply]').disabled,true);
 host.querySelector('[data-task="shield"]').click();form=host.querySelector('form');assert.equal(host.querySelector('[data-record]').value,'*A');
 assert.equal(form.elements.disabled.options.length,1);assert.equal(form.elements.disabled.value,'yes');
});
test('Fairshare and IP controls share existing backend; addresses stay read-only',async()=>{
 const {host,requests}=fixture();await host.querySelector('[data-hub-section="qos"]').onclick();
 let form=host.querySelector('form');form.elements.target.value='192.168.50.0/24';form.elements['max-limit'].value='10M/50M';form.reportValidity=()=>true;await form.onsubmit({preventDefault(){}});
 assert.equal(requests.at(-1)[2].kind,'fairshare');
 await host.querySelector('[data-hub-section="ip"]').onclick();assert.equal(host.querySelector('form'),null);assert.match(host.textContent,/192\.168\.50\.1/);assert.equal(host.querySelector('img'),null);
 assert.ok(host.querySelector('a[href="#network"]'));
});
test('Offline failure shows unavailable and a retry; no fabricated health',async()=>{
 const {host,api}=fixture();api.get=async()=>{throw Error('router offline')};
 await host.querySelector('[data-hub-section="overview"]').onclick();assert.match(host.querySelector('[data-health-status]').textContent,/unavailable.*router offline/);assert.equal(host.querySelector('[data-health-refresh]').disabled,false);
});
test('Late status cannot overwrite a newly selected workspace',async()=>{
 const {host,api}=fixture();let finish;api.get=()=>new Promise(r=>finish=r);
 const reading=host.querySelector('[data-hub-section="overview"]').onclick();
 await host.querySelector('[data-hub-section="maintenance"]').onclick();finish({inventory:{health:{uptime:'STALE_RESPONSE'}}});await reading;
 assert.match(host.textContent,/Recovery & maintenance/);assert.ok(!host.textContent.includes('STALE_RESPONSE'));
});
test('Outer categories cannot discard an in-flight apply editor',async()=>{
 const {host}=fixture();await host.querySelector('[data-hub-section="protection"]').onclick();host.querySelector('[data-router]').disabled=true;
 await host.querySelector('[data-hub-section="maintenance"]').onclick();assert.equal(host.querySelector('[data-task="firewall"]').getAttribute('aria-pressed'),'true');
});
test('Server rollout hold never offers writes',async()=>{
 const {host,inventory}=fixture();inventory.write_enabled=false;await host.querySelector('[data-hub-section="protection"]').onclick();assert.equal(host.querySelector('form'),null);assert.match(host.textContent,/not enabled/);
});
