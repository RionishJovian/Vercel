import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';
import {JSDOM} from 'jsdom';
const html=fs.readFileSync(new URL('../web/app.html',import.meta.url),'utf8');
const code=html.slice(html.indexOf('const SHEET_MM ='),html.indexOf('/* ---------- Magic Coins'));
const flush=()=>new Promise(r=>setTimeout(r,0));
function fixture({failure=false}={}){
 const dom=new JSDOM('<main><div id="content"></div></main>',{runScripts:'outside-only',url:'https://app.test'}),w=dom.window;
 const writes=[],batch='11111111-1111-4111-8111-111111111111',shop='22222222-2222-4222-8222-222222222222';let assigned='';
 Object.assign(w,{ME:{user:{user_role:'OWNER'}},SITE_EDITOR_ROLES:['OWNER'],asArray:v=>Array.isArray(v)?v:[],ts:v=>String(v??''),businessStatusBadge:v=>v,guestVoucherCapabilityMessage:()=>'',esc:v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))});
 w.HTMLElement.prototype.scrollIntoView=()=>{};w.setTimeout=()=>0;
 const stock=[{batch_id:batch,plan_label:'Daily',total_count:2,installed_count:1,activated_count:0,created_at:'today'}];
 w.MM={async get(path){
 if(path==='/vouchers')return{vouchers:[{id:'one',code:'RIGHT-CODE',issue_batch_id:batch,install_state:'verified',status:'unused',reseller_shop:assigned?'Shop <premium>':''},{id:'two',code:'UNVERIFIED-CODE',issue_batch_id:batch,install_state:'pending',status:'unused'},{id:'other',code:'OTHER-BATCH',issue_batch_id:'other',install_state:'verified',status:'unused'}]};
 if(path==='/guest-vouchers/batches')return {batches:[],stock_batches:stock.map(b=>({...b,reseller_id:assigned,shop_name:assigned?'Shop <premium>':''})),reseller_assignment_enabled:true};
 if(path==='/voucher-resellers')return {resellers:[{id:shop,shop_name:'Shop <premium>',code_prefix:'SHP',active:true}]};
 if(path==='/guest-vouchers/config')return {enabled:true,batch_recovery_enabled:true};
 if(path==='/routers')return{routers:[{device_id:'gateway',deployment_layer:'MAIN_GATEWAY',hardware_name:'Lab'}]};return{};},
 async put(path,body){writes.push({path,body});await flush();if(failure)throw Error('batch shop assignment changed; refresh before retrying');assigned=body.reseller_id;return {assignment:{batch_id:batch,reseller_id:assigned},codes_changed:false};},
 async post(path,body){writes.push({path,body});return{batch_id:body.batch_id,state:'ready',vouchers:[]};}};
 w.eval(fs.readFileSync(new URL('../web/assets/js/workflow-revamp.js',import.meta.url),'utf8'));
 w.eval(code);return{dom,w,batch,shop,writes,main:w.document.querySelector('main')};
}
test('Shop assignment uses current revision, prevents duplicate submits and persists readback',async t=>{
 const f=fixture();t.after(()=>f.dom.window.close());await f.w.renderVouchers(f.main);
 let form=f.main.querySelector('[data-batch-shop]');form.querySelector('select').value=f.shop;
 form.dispatchEvent(new f.w.Event('submit',{cancelable:true}));form.dispatchEvent(new f.w.Event('submit',{cancelable:true}));await flush();await flush();
 assert.equal(f.writes.length,1);assert.deepEqual(JSON.parse(JSON.stringify(f.writes[0].body)),{reseller_id:f.shop,expected_reseller_id:'',confirmed:true});
 assert.equal(f.main.querySelector('[data-batch-shop]').dataset.expectedShop,f.shop);
 assert.equal(f.main.querySelector('[data-batch-shop] select').value,f.shop);assert.equal(f.main.querySelector('premium'),null);
});
test('Stale assignment is visibly retryable without claiming success',async t=>{
 const f=fixture({failure:true});t.after(()=>f.dom.window.close());await f.w.renderVouchers(f.main);let form=f.main.querySelector('[data-batch-shop]');form.querySelector('select').value=f.shop;form.dispatchEvent(new f.w.Event('submit',{cancelable:true}));await flush();await flush();
 assert.match(form.querySelector('[role=status]').textContent,/changed/);assert.equal(form.querySelector('button').disabled,false);assert.equal(form.dataset.expectedShop,'');
});
test('Selecting shop stock prints only verified codes from that exact batch',async t=>{
 const f=fixture();t.after(()=>f.dom.window.close());await f.w.renderVouchers(f.main);f.main.querySelector('[data-shop-print]').click();f.main.querySelector('#pl-print').click();const printed=f.w.document.querySelector('iframe').srcdoc;
 assert.match(printed,/RIGHT-CODE/);assert.doesNotMatch(printed,/OTHER-BATCH|UNVERIFIED-CODE/);
});
test('Issuance includes the selected shop in the saved idempotent request',async t=>{
 const f=fixture();t.after(()=>f.dom.window.close());await f.w.renderVouchers(f.main);const d=f.w.document;d.querySelector('#gv-gateway').value='gateway';d.querySelector('#gv-label').value='Daily';d.querySelector('#gv-count').value='2';d.querySelector('#gv-reseller').value=f.shop;d.querySelector('#guest-voucher-form').dispatchEvent(new f.w.Event('submit',{cancelable:true}));await flush();
 assert.equal(f.writes[0].body.reseller_id,f.shop);assert.match(f.writes[0].body.batch_id,/^[0-9a-f-]{36}$/);
});
