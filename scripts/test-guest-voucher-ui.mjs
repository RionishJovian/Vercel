import fs from 'node:fs';
import assert from 'node:assert/strict';
import {JSDOM} from 'jsdom';

const source=fs.readFileSync(new URL('../web/app.html',import.meta.url),'utf8');
const start=source.indexOf('const SHEET_MM =');
const end=source.indexOf('/* ---------- Magic Coins',start);
const dom=new JSDOM('<main><div id="content"></div></main>',{runScripts:'outside-only',url:'https://example.test'});
const context=dom.window,main=context.document.querySelector('main'),getCalls=[];
let enabled=true,postCount=0,lastBody,release;
Object.assign(context,{
  ME:{business:{business_id:'fixture-business'},user:{user_role:'OWNER'}},SITE_EDITOR_ROLES:['OWNER','ADMIN','DEVELOPER'],
  asArray:x=>Array.isArray(x)?x:[],esc:x=>String(x??''),businessStatusBadge:x=>x,
  guestVoucherCapabilityMessage:()=>"Guest voucher creation unavailable",ts:x=>String(x),
  MM:{get:async path=>{
    getCalls.push(path);
    if(path==='/guest-vouchers/config')return {enabled,batch_recovery_enabled:true,max_batch_size:200};
    if(path==='/routers')return {routers:[{device_id:'gateway',hardware_name:'CCR',deployment_layer:'MAIN_GATEWAY'},{device_id:'ap',hardware_name:'Access point',deployment_layer:'ACCESS_POINT'}]};
    if(path==='/vouchers')return {vouchers:[{code:'CODE',price_mmk:1500}]};
    return {};
  },post:async(_,body)=>{postCount++;lastBody=body;await new Promise(r=>release=r);return {state:'ready',batch_id:body.batch_id,vouchers:[],data_mb:body.data_mb};}},
});
try {
  context.eval(source.slice(start,end));
  await context.renderVouchers(main);
  const content=main.querySelector('#content');
  assert(content.textContent.includes('Create guest internet vouchers'));
  assert(content.innerHTML.includes('value="gateway"')&&!content.innerHTML.includes('value="ap"'));
  assert(content.textContent.includes('1500'));
  const values={'gv-gateway':'gateway','gv-label':' Daily ','gv-minutes':'1440','gv-data':'1024','gv-price':'1000','gv-count':'10'};
  for(const [id,value] of Object.entries(values))context.document.getElementById(id).value=value;
  const form=context.document.getElementById('guest-voucher-form');
  form.dispatchEvent(new context.Event('submit',{cancelable:true}));
  form.dispatchEvent(new context.Event('submit',{cancelable:true}));
  assert.equal(postCount,1,'double submit must not create another batch');
  assert.equal(lastBody.plan_label,'Daily');assert.equal(lastBody.data_mb,1024);assert.equal(lastBody.count,10);
  assert.match(lastBody.batch_id,/^[0-9a-f-]{36}$/i,'batch retry identity must be saved before the request');
  assert.equal(context.pendingGuestVoucherRequest().batch_id,lastBody.batch_id);
  release();await new Promise(resolve=>setTimeout(resolve,0));
  enabled=false;await context.renderVouchers(main);
  assert(!content.querySelector('#guest-voucher-form'),'disabled API must hide issuance');
  getCalls.length=0;context.ME.user.user_role='VIEWER';enabled=true;
  await context.renderVouchers(main);
  assert(!content.querySelector('#guest-voucher-form'));
  assert(!getCalls.includes('/guest-vouchers/config')&&!getCalls.includes('/routers')&&!getCalls.includes('/guest-vouchers/batches'));
  const portal=fs.readFileSync(new URL('../web/portal.html',import.meta.url),'utf8');
  assert(portal.includes('cfg.guest_voucher_access === true ? "/guest-voucher" : "/redeem"'));
  console.log('Guest voucher UI passed: actual DOM capability/role visibility, gateway selection, quotas, price, durable retry identity and duplicate-submit guard.');
} finally {dom.window.close();}
