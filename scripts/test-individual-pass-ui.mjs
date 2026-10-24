import fs from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';
import assert from 'node:assert/strict';
import {JSDOM} from 'jsdom';
const source=fs.readFileSync(new URL('../web/app.html',import.meta.url),'utf8');
const escape=x=>String(x??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
function fixture(fn,get,post){const dom=new JSDOM('<main><div id="content"></div></main>');const context=vm.createContext({document:dom.window.document,MM:{get,post},esc:escape,ts:x=>x,apiErrorText:(e,fallback)=>e?.data?.error||fallback});const end=fn==='renderIndividualPass'?'async function renderLegacyStoreCheckout':'async function renderPlatform(main)';vm.runInContext(source.slice(source.indexOf('async function '+fn+'('),source.indexOf(end)),context);return {dom,context,main:dom.window.document.querySelector('main'),panel:dom.window.document.querySelector('#content')};}
const tick=()=>new Promise(r=>setTimeout(r,0));
test('expired client can request renewal, with server-managed activation and no duplicate submission',async()=>{
 let pending=false,release;const writes=[];const f=fixture('renderIndividualPass',async()=>({requests:pending?[{id:'r',plan:'monthly',status:'pending',created_at:'now'}]:[]}),async(path,body)=>{writes.push({path,body:JSON.parse(JSON.stringify(body))});await new Promise(r=>release=r);pending=true;});
 const pass={state:'expired',authorized_login:true};await f.context.renderIndividualPass(f.main,pass);const form=f.panel.querySelector('form');form.requestSubmit();form.dispatchEvent(new f.dom.window.Event('submit',{cancelable:true}));assert.equal(writes.length,1);assert.equal(writes[0].path,'/me/app-pass/renewals');assert.deepEqual(writes[0].body,{plan:'monthly'});release();await tick();assert.match(f.panel.textContent,/awaiting review/);assert.equal(f.panel.querySelector('button').disabled,true);assert.match(f.panel.textContent,/Existing guest internet sessions continue/);
});
test('missing history or unauthorized legacy login cannot silently request a pass',async()=>{
 let writes=0;const f=fixture('renderIndividualPass',async()=>{throw Error('unavailable')},async()=>writes++);await f.context.renderIndividualPass(f.main,{state:'expired',authorized_login:true});assert.equal(f.panel.querySelector('button').disabled,true);assert.match(f.panel.textContent,/unavailable/);assert.equal(writes,0);
 f.context.MM.get=async()=>({requests:[]});await f.context.renderIndividualPass(f.main,{state:'authorized_login_required',authorized_login:false});assert.equal(f.panel.querySelector('button').disabled,true);assert.match(f.panel.textContent,/review the authorized login/);
});
test('operator renewal decision requires deliberate review and stays blocked while uncertain',async()=>{
 const writes=[];let release;const f=fixture('loadPlatformRenewals',async()=>({requests:[{id:'req',email:'<script>client</script>',plan:'annual',created_at:'now'}]}),async(path,body)=>{writes.push({path,body:JSON.parse(JSON.stringify(body))});await new Promise(r=>release=r);throw {data:{error:'Uncertain decision; refresh'}};});await f.context.loadPlatformRenewals(f.panel);assert.equal(f.panel.querySelector('script'),null);const form=f.panel.querySelector('form'),button=form.querySelector('button');form.requestSubmit(button);assert.equal(writes.length,0);form.elements.confirmed.checked=true;form.requestSubmit(button);form.dispatchEvent(new f.dom.window.SubmitEvent('submit',{cancelable:true,submitter:button}));assert.equal(writes.length,1);assert.deepEqual(writes[0],{path:'/platform/account-renewals/req/decision',body:{approve:true}});release();await tick();assert.match(f.panel.textContent,/Uncertain decision/);assert.equal(button.disabled,true);
});
test('unavailable operator inbox never reports no pending requests',async()=>{
 const f=fixture('loadPlatformRenewals',async()=>{throw Error('private database detail')});await f.context.loadPlatformRenewals(f.panel);assert.match(f.panel.textContent,/unavailable/);assert.doesNotMatch(f.panel.textContent,/No pending|private database/);assert(f.panel.querySelector('button'));
});
