import fs from 'node:fs';
import {randomUUID} from 'node:crypto';
import vm from 'node:vm';
import test from 'node:test';
import assert from 'node:assert/strict';
import {JSDOM} from 'jsdom';
const source=fs.readFileSync(new URL('../web/app.html',import.meta.url),'utf8');
const escape=x=>String(x??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
function fixture(fn,get,post){const dom=new JSDOM('<main><div id="content"></div></main>',{url:'https://checkout.fixture/'});const context=vm.createContext({document:dom.window.document,MM:{get,post},ME:{user:{user_id:'fixture-user'},business:{business_id:'fixture-business'}},sessionStorage:dom.window.sessionStorage,crypto:{randomUUID},storePrice:x=>`${x} MMK`,esc:escape,ts:x=>x,apiErrorText:(e,fallback)=>e?.data?.error||fallback});const end=fn==='renderIndividualPass'?'async function renderLegacyStoreCheckout':'async function renderPlatform(main)';vm.runInContext(source.slice(source.indexOf('async function '+fn+'('),source.indexOf(end)),context);return {dom,context,main:dom.window.document.querySelector('main'),panel:dom.window.document.querySelector('#content')};}
const tick=()=>new Promise(r=>setTimeout(r,0));
const options={available:true,plans:{monthly:{amount_mmk:29000},annual:{amount_mmk:285000}},bank_accounts:[{id:'bank',bank_name:'Fixture Bank',holder_name:'Disposable Payee',account_number:'TEST-ONLY'}]};
const expired={state:'expired',authorized_login:true};
function paymentFixture({history=()=>[],post=async()=>({}),pass=expired,checkout=options}={}){return fixture('renderIndividualPass',async path=>path.endsWith('checkout-options')?checkout:path==='/me/app-pass'?pass:{requests:await history()},post);}
function preparePayment(f,plan='monthly'){f.panel.querySelector(`[data-pass-plan="${plan}"]`).click();const form=f.panel.querySelector('form');form.elements.bank_reference.value='TEST-REFERENCE';form.elements.confirmed.checked=true;return form;}
test('expired client submits quoted monthly or annual payment once and sees the pending request',async()=>{
 for(const plan of ['monthly','annual']){
  let pending=false,release;const writes=[];
  const f=paymentFixture({history:()=>pending?[{id:'r',plan,status:'pending',created_at:'now',amount_mmk:options.plans[plan].amount_mmk,bank_reference:'TEST-REFERENCE'}]:[],post:async(path,body)=>{writes.push({path,body:JSON.parse(JSON.stringify(body))});await new Promise(r=>release=r);pending=true;}});
  await f.context.renderIndividualPass(f.main,expired);const form=preparePayment(f,plan);
  form.requestSubmit();form.dispatchEvent(new f.dom.window.Event('submit',{cancelable:true}));assert.equal(writes.length,1);
  assert.equal(writes[0].path,'/me/app-pass/renewals');assert.equal(writes[0].body.plan,plan);assert.equal(writes[0].body.expected_amount_mmk,options.plans[plan].amount_mmk);assert.equal(writes[0].body.bank_reference,'TEST-REFERENCE');assert.match(writes[0].body.idempotency_key,/^[a-f0-9-]{36}$/);
  release();await tick();await tick();assert.match(f.panel.textContent,/awaiting review|Awaiting review/);assert.equal(f.panel.querySelector('[data-pass-plan]').disabled,true);assert.match(f.panel.textContent,/Existing guest internet sessions continue/);assert.equal(f.context.sessionStorage.length,0);
 }
});
test('uncertain submission retries the same persisted payment details and request ID',async()=>{
 const writes=[];let saved=false;
 const f=paymentFixture({history:()=>saved?[{id:'r',plan:'monthly',status:'pending',created_at:'now'}]:[],post:async(path,body)=>{writes.push(JSON.parse(JSON.stringify(body)));if(writes.length===1)throw {status:503};saved=true;}});
 await f.context.renderIndividualPass(f.main,expired);let form=preparePayment(f);form.requestSubmit();await tick();
 assert.equal(f.context.sessionStorage.length,1);assert.match(form.textContent,/not confirmed|same details/);assert.equal(form.elements.bank_reference.disabled,true);
 // Reloading the renderer recovers the saved request rather than minting a new key.
 await f.context.renderIndividualPass(f.main,expired);form=f.panel.querySelector('form');form.elements.bank_reference.value='TAMPERED-DOM';form.requestSubmit();await tick();await tick();
 assert.equal(writes.length,2);assert.deepEqual(writes[0],writes[1]);assert.equal(f.context.sessionStorage.length,0);assert.match(f.panel.textContent,/awaiting review|Awaiting review/);
});
test('unavailable history, payment options, or unauthorized login cannot offer a purchase',async()=>{
 const f=fixture('renderIndividualPass',async()=>{throw Error('private database detail')},async()=>assert.fail('unexpected payment'));
 await f.context.renderIndividualPass(f.main,expired);assert.equal(f.panel.querySelector('[data-pass-plan]').disabled,true);assert.match(f.panel.textContent,/unavailable/);assert.doesNotMatch(f.panel.textContent,/No renewal requests|private database/);
 const unavailable=paymentFixture({checkout:{...options,available:false,bank_accounts:[]}});await unavailable.context.renderIndividualPass(unavailable.main,expired);assert.equal(unavailable.panel.querySelector('[data-pass-plan]').disabled,true);assert.match(unavailable.panel.textContent,/complete payment account/);
 const foreign=paymentFixture();await foreign.context.renderIndividualPass(foreign.main,{state:'authorized_login_required',authorized_login:false});assert.equal(foreign.panel.querySelector('[data-pass-plan]').disabled,true);assert.equal(foreign.panel.querySelector('form'),null);
});
test('platform administrator sees the real prices and exemption without a payment form',async()=>{
 const f=paymentFixture();await f.context.renderIndividualPass(f.main,{state:'platform_exempt',capacity_exempt:true});assert.match(f.panel.textContent,/29000 MMK/);assert.match(f.panel.textContent,/285000 MMK/);assert.equal(f.panel.querySelector('form'),null);assert([...f.panel.querySelectorAll('[data-pass-plan]')].every(b=>b.disabled));assert.match(f.panel.textContent,/No pass purchase is needed/);
});
test('refresh fetches the new account pass after administrator approval',async()=>{
 const pass={state:'expired',authorized_login:true};const f=paymentFixture({pass});await f.context.renderIndividualPass(f.main,pass);pass.state='active';pass.expires_at='new-expiry';f.panel.querySelector('[data-pass-refresh]').click();await tick();assert.match(f.panel.textContent,/Active/);assert.match(f.panel.textContent,/new-expiry/);
});
test('operator renewal decision requires deliberate review and stays blocked while uncertain',async()=>{
 const writes=[];let release;const f=fixture('loadPlatformRenewals',async()=>({requests:[{id:'req',email:'<script>client</script>',plan:'annual',created_at:'now'}]}),async(path,body)=>{writes.push({path,body:JSON.parse(JSON.stringify(body))});await new Promise(r=>release=r);throw {data:{error:'Uncertain decision; refresh'}};});await f.context.loadPlatformRenewals(f.panel);assert.equal(f.panel.querySelector('script'),null);const form=f.panel.querySelector('form'),button=form.querySelector('button');form.requestSubmit(button);assert.equal(writes.length,0);form.elements.confirmed.checked=true;form.requestSubmit(button);form.dispatchEvent(new f.dom.window.SubmitEvent('submit',{cancelable:true,submitter:button}));assert.equal(writes.length,1);assert.deepEqual(writes[0],{path:'/platform/account-renewals/req/decision',body:{approve:true}});release();await tick();assert.match(f.panel.textContent,/Uncertain decision/);assert.equal(button.disabled,true);
});
test('unavailable operator inbox never reports no pending requests',async()=>{
 const f=fixture('loadPlatformRenewals',async()=>{throw Error('private database detail')});await f.context.loadPlatformRenewals(f.panel);assert.match(f.panel.textContent,/unavailable/);assert.doesNotMatch(f.panel.textContent,/No pending|private database/);assert(f.panel.querySelector('button'));
});
test('paid approval shows escaped bank evidence and requires an explicit payment verification flag',async()=>{
 const writes=[];const f=fixture('loadPlatformRenewals',async()=>({requests:[{id:'paid',email:'client@fixture',plan:'annual',created_at:'now',amount_mmk:285000,bank_reference:'<img src=x onerror=alert(1)>',bank_snapshot:{bank_name:'Fixture Bank',account_number:'TEST-ONLY',holder_name:'Disposable Payee'}}]}),async(path,body)=>{writes.push(JSON.parse(JSON.stringify(body)));throw {status:503};});
 await f.context.loadPlatformRenewals(f.panel);assert.equal(f.panel.querySelector('img'),null);assert.match(f.panel.textContent,/285000 MMK/);assert.match(f.panel.textContent,/checked this payment against the bank record/);const form=f.panel.querySelector('form');form.elements.confirmed.checked=true;form.requestSubmit(form.querySelector('[data-decision="approve"]'));await tick();assert.deepEqual(writes,[{approve:true,payment_verified:true}]);
});
