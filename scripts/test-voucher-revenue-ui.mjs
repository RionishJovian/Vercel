import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';
import {JSDOM} from 'jsdom';

const app=fs.readFileSync(new URL('../web/app.html',import.meta.url),'utf8');
const functions=app.slice(app.indexOf('function mmk('),app.indexOf('/* ---------- Captive portal ---------- */'));
const report={source:'guest-internet-vouchers',recorded_count:139,recorded_face_value:74500,first_use_count:6,available_count:131,reserved_count:1,activated_count:2,expired_count:3,revoked_count:1,other_count:1,activation_count:3,activation_time_unknown_count:1,activation_value:6500,activation_today:1500,activation_last7:3500,activation_last30:3500,gateways:[{device_id:'g',gateway_name:'Cafe <img src=x>',recorded_count:139,available_count:131,reserved_count:1,activated_count:2,expired_count:3,revoked_count:1,other_count:1,activation_value:6500}]};
const portal={today:3500,last7:3500,last30:3500,all_time:3500,total:2,approved:1,pending:1,rejected:0,days:[],plans:[]};
const flush=()=>new Promise(resolve=>setTimeout(resolve,0));

function fixture(failures=[]) {
  const dom=new JSDOM('<main><div id="content"></div></main>',{runScripts:'outside-only',url:'https://example.test'});
  const {window}=dom;
  Object.assign(window,{ME:{user:{user_role:'OWNER'}},asArray:v=>Array.isArray(v)?v:[],ts:v=>String(v??''),businessStatusBadge:v=>String(v),apiErrorText:e=>e.message,
    esc:v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])),Blob:globalThis.Blob});
  const bodies={'/voucher-report':structuredClone(report),'/guest-wifi/revenue':structuredClone(portal),'/guest-wifi/requests':{requests:[{id:'same-id',plan_key:'=FORMULA',status:'pending',amount:3500,bank_reference:'@bank'}]},'/billing/requests':{requests:[{id:'same-id',plan_key:'Business',status:'pending',amount:99999}]}};
  const writes=[],exports=[],reads=[];let downloaded='';
  window.MM={get:async path=>{reads.push(path);if(failures.includes(path))throw Error('private database details');return bodies[path];},post:async(path,body)=>{writes.push({path,body});return {};}};
  window.URL.createObjectURL=blob=>{exports.push(blob);return 'blob:fixture';};window.URL.revokeObjectURL=()=>{};
  window.HTMLAnchorElement.prototype.click=function(){downloaded=this.download;};
  window.eval(functions);
  return {dom,window,main:window.document.querySelector('main'),bodies,writes,exports,reads,get downloaded(){return downloaded;}};
}

test('Voucher activation value, unused stock and portal payments stay separate',async t=>{
  const f=fixture();t.after(()=>f.dom.window.close());await f.window.renderRevenue(f.main);
  assert.equal(f.writes.length,0,'Opening revenue must not activate vouchers or record sales');
  const vouchers=f.main.querySelector('#voucher-activation-report');
  assert.match(vouchers.textContent,/6,500 MMK/);assert.match(vouchers.textContent,/74,500 MMK/);
  assert.match(vouchers.textContent,/131 unused/);assert.match(vouchers.textContent,/1 activating/);
  assert.match(vouchers.textContent,/no verified activation timestamp/);
  assert.match(vouchers.textContent,/Printing leaves codes unused/);
  assert.match(vouchers.textContent,/not create another activation/);
  assert.equal(vouchers.querySelector('img'),null,'Gateway names must be escaped');
  const payments=f.main.querySelector('[data-portal-summary]');
  assert.match(payments.textContent,/3,500 MMK/);
  assert.ok(!payments.textContent.includes('6,500')&&!payments.textContent.includes('74,500')&&!payments.textContent.includes('99,999'));
});

test('Voucher reporting remains visible when portal totals or inboxes are unavailable',async t=>{
  const f=fixture(['/guest-wifi/revenue','/guest-wifi/requests','/billing/requests']);t.after(()=>f.dom.window.close());
  await f.window.renderRevenue(f.main);
  assert.match(f.main.querySelector('[data-voucher-summary]').textContent,/6,500 MMK/);
  assert.equal(f.main.querySelector('[data-portal-summary]'),null);
  assert.match(f.main.textContent,/Complete portal-payment totals are unavailable/);
  assert.match(f.main.textContent,/Payment inbox unavailable/);
  assert.ok(!f.main.textContent.includes('private database details'));
});

test('Unavailable or incomplete voucher data never becomes a fabricated zero',async t=>{
  const f=fixture(['/voucher-report']);t.after(()=>f.dom.window.close());await f.window.renderRevenue(f.main);
  assert.equal(f.main.querySelector('[data-voucher-summary]'),null);
  assert.match(f.main.textContent,/Voucher activation reporting is unavailable/);
  assert.match(f.main.querySelector('[data-portal-summary]').textContent,/3,500 MMK/);
  const other=fixture();t.after(()=>other.dom.window.close());delete other.bodies['/voucher-report'].activation_last30;
  await other.window.renderRevenue(other.main);assert.equal(other.main.querySelector('[data-voucher-summary]'),null);
  const partial=fixture();t.after(()=>partial.dom.window.close());delete partial.bodies['/voucher-report'].gateways[0].activation_value;
  await partial.window.renderRevenue(partial.main);assert.equal(partial.main.querySelector('[data-voucher-summary]'),null);
});

test('A partial portal summary is not substituted with limited-inbox totals',async t=>{
  const f=fixture();t.after(()=>f.dom.window.close());delete f.bodies['/guest-wifi/revenue'].today;
  await f.window.renderRevenue(f.main);
  assert.equal(f.main.querySelector('[data-portal-summary]'),null);
  assert.match(f.main.textContent,/limited payment inbox/);
  assert.ok(f.main.querySelector('[data-voucher-summary]'));
});

test('Inbox export identifies source and scope and neutralizes spreadsheet formulas',async t=>{
  const f=fixture();t.after(()=>f.dom.window.close());await f.window.renderRevenue(f.main);
  f.main.querySelector('#revenue-export').click();
  const csv=await f.exports[0].text();
  assert.match(csv,/"source","submitted_at"/);assert.match(csv,/"guest-wifi"/);assert.doesNotMatch(csv,/"billing"/);
  assert.match(csv,/"'=FORMULA"/);assert.match(csv,/"'@bank"/);
  assert.match(f.downloaded,/payment-inbox/);
  assert.ok(!csv.includes('74500')&&!csv.includes('6500'),'Inbox export cannot masquerade as complete voucher revenue');
});

test('Identical request IDs keep their source when approving; viewers cannot decide',async t=>{
  const f=fixture();t.after(()=>f.dom.window.close());await f.window.renderRevenue(f.main);
  f.main.querySelector('[data-purchase-source="guest-wifi"][data-purchase-action="approve"]').click();await flush();
  assert.equal(f.writes[0].path,'/guest-wifi/approve/same-id');
  assert.equal(f.main.querySelector('[data-purchase-source="billing"]'),null);
  assert.ok(!f.reads.includes('/billing/requests'),'Revenue must not load the old business-pass inbox');
  f.window.ME.user.user_role='VIEWER';await f.window.renderRevenue(f.main);
  assert.equal(f.main.querySelector('[data-purchase-action]'),null);
  assert.ok(f.main.querySelector('[data-voucher-summary]'));
});
