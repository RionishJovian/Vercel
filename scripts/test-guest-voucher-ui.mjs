import fs from 'node:fs';
import vm from 'node:vm';
import assert from 'node:assert/strict';

const source=fs.readFileSync(new URL('../web/app.html',import.meta.url),'utf8');
const start=source.indexOf('async function renderVouchers(');
const end=source.indexOf('/* ---------- Magic Coins',start);
const nodes={};
const content={innerHTML:''};
const main={querySelector:()=>content};
const getCalls=[];
let enabled=true,postCount=0,lastBody,release;
const values={'gv-gateway':'gateway','gv-label':' Daily ','gv-minutes':'1440','gv-data':'1024','gv-price':'1000','gv-count':'10'};
const form={reportValidity:()=>true,addEventListener:(_,fn)=>form.submit=fn};
const button={disabled:false};
nodes['guest-voucher-form']=form;nodes['gv-create']=button;nodes['gv-message']={textContent:''};
for(const [id,value] of Object.entries(values))nodes[id]={value};
const context=vm.createContext({
  ME:{user:{user_role:'OWNER'}},SITE_EDITOR_ROLES:['OWNER','ADMIN','DEVELOPER'],
  asArray:x=>Array.isArray(x)?x:[],esc:x=>String(x??''),badge:x=>x,ts:x=>String(x),
  wirePrintPreview:()=>{},document:{getElementById:id=>content.innerHTML.includes(`id="${id}"`)?nodes[id]??null:null,querySelectorAll:()=>[]},
  MM:{get:async path=>{getCalls.push(path);if(path==='/guest-vouchers/config')return {enabled};if(path==='/routers')return {routers:[{device_id:'gateway',hardware_name:'CCR',deployment_layer:'MAIN_GATEWAY'},{device_id:'ap',hardware_name:'Access point',deployment_layer:'ACCESS_POINT'}]};if(path==='/vouchers')return {vouchers:[{code:'CODE',price_mmk:1500}]};return {};},post:async(_,body)=>{postCount++;lastBody=body;await new Promise(r=>release=r);}},
});
vm.runInContext(source.slice(start,end),context);
await context.renderVouchers(main);
assert(content.innerHTML.includes('Create guest internet vouchers'));
assert(content.innerHTML.includes('value="gateway"')&&!content.innerHTML.includes('value="ap"'));
assert(content.innerHTML.includes('1500'));
const submit=form.submit;
const pending=submit({preventDefault(){}});
await submit({preventDefault(){}});
assert.equal(postCount,1,'double submit must not create another batch');
assert.equal(lastBody.plan_label,'Daily');assert.equal(lastBody.data_mb,1024);assert.equal(lastBody.count,10);
release();await pending;
enabled=false;await context.renderVouchers(main);
assert(!content.innerHTML.includes('id="guest-voucher-form"'),'disabled API must hide issuance');
getCalls.length=0;context.ME.user.user_role='VIEWER';enabled=true;
await context.renderVouchers(main);
assert(!content.innerHTML.includes('id="guest-voucher-form"'));
assert(!getCalls.includes('/guest-vouchers/config')&&!getCalls.includes('/routers'));
const portal=fs.readFileSync(new URL('../web/portal.html',import.meta.url),'utf8');
assert(portal.includes('cfg.guest_voucher_access === true ? "/guest-voucher" : "/redeem"'));
console.log('Guest voucher UI passed: capability/role visibility, gateway-only selection, quotas, price, duplicate-submit guard and dedicated guest endpoint.');
