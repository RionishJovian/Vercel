import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';
import {JSDOM} from 'jsdom';

const source=fs.readFileSync(new URL('../web/app.html',import.meta.url),'utf8');
const functions=source.slice(source.indexOf('const SHEET_MM ='),source.indexOf('/* ---------- Magic Coins'));
const flush=()=>new Promise(resolve=>setTimeout(resolve,0));
const gateway='11111111-1111-4111-8111-111111111111';
const batchID='22222222-2222-4222-8222-222222222222';
function fixture(post) {
  const dom=new JSDOM('<main><div id="content"></div></main>',{runScripts:'outside-only',url:'https://example.test'});
  const w=dom.window,writes=[];
  Object.assign(w,{ME:{business:{business_id:'business-one'},user:{user_role:'OWNER'}},SITE_EDITOR_ROLES:['OWNER','ADMIN','DEVELOPER'],
    asArray:v=>Array.isArray(v)?v:[],ts:v=>String(v??''),businessStatusBadge:v=>String(v),guestVoucherCapabilityMessage:()=>'',
    esc:v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))});
  w.setTimeout=()=>0;
  w.MM={async get(path){
    if(path==='/guest-vouchers/config')return {enabled:true,batch_recovery_enabled:true,max_batch_size:200};
    if(path==='/routers')return {routers:[{device_id:gateway,hardware_name:'Cafe',deployment_layer:'MAIN_GATEWAY'}]};
    if(path==='/voucher-print-layout')return {layout:{show_price:true,show_expiry:true}};
    if(path==='/guest-vouchers/batches')return {batches:[]};
    if(path==='/vouchers')return {vouchers:[]};return {};
  },async post(path,body){writes.push({path,body:JSON.parse(JSON.stringify(body))});return post(path,body,writes.length);}};
  w.eval(functions);return {dom,w,writes,main:w.document.querySelector('main')};
}
async function submit(f){const d=f.w.document;d.querySelector('#gv-gateway').value=gateway;d.querySelector('#gv-label').value='Daily';d.querySelector('#gv-count').value='2';d.querySelector('#guest-voucher-form').dispatchEvent(new f.w.Event('submit',{cancelable:true}));await flush();}

test('lost response retries retained batch identity and exact parameters',async t=>{
  const f=fixture(async(path,body,n)=>{if(n===1)throw Error('response lost');return {batch_id:body.batch_id,state:'ready',vouchers:[],data_mb:1024};});t.after(()=>f.dom.window.close());
  await f.w.renderVouchers(f.main);await submit(f);const first=f.writes[0].body;
  assert.match(first.batch_id,/^[a-f0-9-]{36}$/i);assert.match(f.w.document.querySelector('#gv-message').textContent,/unconfirmed/);
  await submit(f);assert.deepEqual(f.writes[1].body,first);assert.equal(f.w.pendingGuestVoucherRequest(),null);
});
test('refresh restores uncertain request and refuses changed parameters',async t=>{
  const f=fixture(async()=>{throw Error('disconnected');});t.after(()=>f.dom.window.close());
  await f.w.renderVouchers(f.main);await submit(f);await f.w.renderVouchers(f.main);
  assert.equal(f.w.document.querySelector('#gv-count').value,'2');assert.ok(f.w.document.querySelector('#gv-resume-request'));
  f.w.document.querySelector('#gv-price').value='2000';f.w.document.querySelector('#guest-voucher-form').dispatchEvent(new f.w.Event('submit',{cancelable:true}));await flush();
  assert.equal(f.writes.length,1);assert.match(f.w.document.querySelector('#gv-message').textContent,/Resume the saved batch/);
  f.w.document.querySelector('#gv-resume-request').click();await flush();assert.deepEqual(f.writes[1].body,f.writes[0].body);
});
test('pending installation remains recoverable and cannot become a print batch',async t=>{
  const f=fixture(async(path,body)=>({batch_id:body.batch_id,state:'unknown',total_count:2,verified_count:0,vouchers:[],data_mb:1024}));t.after(()=>f.dom.window.close());
  await f.w.renderVouchers(f.main);await submit(f);
  assert.ok(f.w.document.querySelector('#gv-resume-request'));assert.match(f.w.document.querySelector('#gv-recovery-message').textContent,/not ready to print/);
  assert.equal(f.w.document.querySelector('#pl-batch-source'),null);assert.ok(f.w.pendingGuestVoucherRequest());
});
test('all-code filters and direct printing exclude unverified inventory',t=>{
  const f=fixture(async()=>({}));t.after(()=>f.dom.window.close());
  const vouchers=[{code:'PENDING',status:'unused',issue_batch_id:batchID,install_state:'pending'},{code:'UNKNOWN',status:'unused',issue_batch_id:batchID,install_state:'unknown'},
    {code:'VERIFIED',status:'unused',issue_batch_id:batchID,install_state:'verified'},{code:'LEGACY',status:'unused'}];
  assert.deepEqual(Array.from(f.w.voucherPrintSelection(vouchers,100,1,'all').vouchers,v=>v.code),['VERIFIED','LEGACY']);
  f.w.printSheet({a4_columns:3,a4_rows_per_page:12,paper_width_mm:80,show_qr:false,show_price:false,show_expiry:false},'a4',vouchers);
  const printed=f.w.document.querySelector('iframe').srcdoc;assert.ok(!printed.includes('PENDING')&&!printed.includes('UNKNOWN'));
});
test('server-retained batch resumes after browser storage is cleared',async t=>{
  const f=fixture(async()=>({batch_id:batchID,state:'ready',vouchers:[],data_mb:1024}));t.after(()=>f.dom.window.close());const original=f.w.MM.get;
  f.w.MM.get=async path=>path==='/guest-vouchers/batches'?{batches:[{batch_id:batchID,state:'unknown',verified_count:0,total_count:2}]}:original(path);
  await f.w.renderVouchers(f.main);f.w.document.querySelector('[data-gv-batch-retry]').click();await flush();assert.equal(f.writes[0].path,`/guest-vouchers/batches/${batchID}/retry`);
});
test('no issuance request is sent when retry identity cannot be retained',async t=>{
  const f=fixture(async()=>({}));t.after(()=>f.dom.window.close());await f.w.renderVouchers(f.main);f.w.retainGuestVoucherRequest=()=>{throw Error('storage blocked');};
  await submit(f);assert.equal(f.writes.length,0);assert.match(f.w.document.querySelector('#gv-message').textContent,/No request was sent/);
});
test('older servers cannot expose issuance without the durable-batch capability',async t=>{
  const f=fixture(async()=>({}));t.after(()=>f.dom.window.close());const original=f.w.MM.get;
  f.w.MM.get=async path=>path==='/guest-vouchers/config'?{enabled:true}:original(path);
  await f.w.renderVouchers(f.main);
  assert.equal(f.w.document.querySelector('#guest-voucher-form'),null);
  assert.match(f.main.textContent,/complete the service update/);
});
test('a response for another batch cannot clear the original request or become printable',async t=>{
  const f=fixture(async()=>({batch_id:batchID,state:'ready',vouchers:[{code:'OTHER-BATCH',status:'unused'}]}));t.after(()=>f.dom.window.close());
  await f.w.renderVouchers(f.main);await submit(f);
  assert.equal(f.w.pendingGuestVoucherRequest().batch_id,f.writes[0].body.batch_id);
  assert.equal(f.w.document.querySelector('#pl-batch-source'),null);
  assert.match(f.w.document.querySelector('#gv-message').textContent,/identity could not be confirmed/);
});
