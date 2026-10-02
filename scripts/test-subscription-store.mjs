import fs from 'node:fs';
import vm from 'node:vm';
import assert from 'node:assert/strict';

const source=fs.readFileSync(new URL('../web/app.html',import.meta.url),'utf8');
const start=source.indexOf('async function renderStore(');
const end=source.indexOf('/* ---------- Portal branding editor',start);
let status={mode:'subscription_only',state:'trial',trial_expires_at:'2026-10-15'};
let fail=false;
const content={innerHTML:'',querySelectorAll:()=>[]};
const context=vm.createContext({
  MM:{get:async p=>{
    if(p==='/subscription'){if(fail)throw new Error('Unavailable');return status;}
    if(p==='/pricing')return {plans:{monthly:{current_mmk:1000},annual:{current_mmk:10000}},tiers:[{label:'Old tier',device_quota:50}]};
    if(p==='/store/checkout-options')return {bank_accounts:[{id:'bank',bank_name:'Test',account_number:'TEST'}]};
    if(p==='/store/purchases')return {purchases:[{service_label:'Past purchase',price_mmk:1000,status:'approved'}]};
    return {};
  }},
  asArray:x=>Array.isArray(x)?x:[],esc:x=>String(x??''),ts:x=>String(x),storePrice:x=>String(x),coinAmt:x=>String(x??0),requestStatusBadge:x=>x,
  document:{getElementById:()=>({addEventListener(){}})},
});
vm.runInContext(source.slice(start,end),context);
const render=()=>context.renderStore({querySelector:()=>content});
await render();
assert(content.innerHTML.includes('14-day trial ends 2026-10-15'));
assert(!content.innerHTML.includes('CAPABILITY KEYS')&&!content.innerHTML.includes('Published capacity tiers'));
assert(content.innerHTML.includes('Past purchase'),'history preserved');
assert(content.innerHTML.includes('data-store-buy="monthly"')&&content.innerHTML.includes('data-store-buy="annual"'));
assert(!content.innerHTML.includes('Checkout unavailable'));
status={mode:'subscription_only',state:'payment_required'};await render();
assert(content.innerHTML.includes('restore access'));
assert(!content.innerHTML.includes('Checkout unavailable'),'expired accounts can renew');
status={mode:'subscription_only',state:'paid',paid_expires_at:'2026-12-01'};await render();
assert(content.innerHTML.includes('Subscription active until 2026-12-01'));
fail=true;await render();
assert(content.innerHTML.includes('Subscription status is unavailable'));
assert.equal((content.innerHTML.match(/Checkout unavailable/g)||[]).length,2);
fail=false;status={mode:'legacy'};await render();
assert(content.innerHTML.includes('CAPABILITY KEYS')&&content.innerHTML.includes('Published capacity tiers'));
console.log('Subscription Store checks passed: trial/paid/expiry notices, renewal availability, hidden legacy offers, history and fail-closed checkout.');
