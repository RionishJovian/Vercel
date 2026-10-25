import fs from 'node:fs';
import test from 'node:test';
import assert from 'node:assert/strict';
import {JSDOM} from 'jsdom';
const app=fs.readFileSync(new URL('../web/app.html',import.meta.url),'utf8');
const code=app.slice(app.indexOf('function storePrice('),app.indexOf('async function renderLegacyStoreCheckout('));
function fixture(fail=[]){
 const dom=new JSDOM('<main><div id="content"></div></main>',{runScripts:'outside-only'}),w=dom.window,reads=[];
 const data={'/subscription':{mode:'legacy'},'/me/app-pass':{scope:'account',authority:'app_account_passes',kind:'none',state:'no_pass'},'/store/purchases':{purchases:[{service_key:'annual',price_mmk:10000,status:'approved',created_at:'2025-01-01'}]}};
 Object.assign(w,{asArray:v=>Array.isArray(v)?v:[],esc:v=>String(v??'').replaceAll('<','&lt;'),ts:v=>v,requestStatusBadge:v=>v,MM:{get:async path=>{reads.push(path);if(fail.includes(path))throw Error('unavailable');return data[path];},post:()=>{throw Error('unexpected purchase');}}});w.eval(code);
 return {dom,w,reads,data,main:w.document.querySelector('main')};
}
test('current pass and legacy history remain separate and offer no old checkout',async t=>{
 const f=fixture();t.after(()=>f.dom.window.close());await f.w.renderStore(f.main);
 assert.match(f.main.textContent,/No current pass recorded/);assert.match(f.main.textContent,/Legacy Store history/);assert.match(f.main.textContent,/annual/);
 assert.deepEqual(f.reads,['/subscription','/me/app-pass','/store/purchases']);assert.equal(f.main.querySelector('[data-store-buy]'),null);assert.equal(f.main.querySelector('form'),null);
});
test('missing pass or malformed history never fabricates a free pass or empty history',async t=>{
 const f=fixture(['/me/app-pass']);t.after(()=>f.dom.window.close());f.data['/store/purchases']={};await f.w.renderStore(f.main);
 assert.match(f.main.textContent,/pass could not be loaded/);assert.match(f.main.textContent,/history could not be loaded/);assert.doesNotMatch(f.main.textContent,/No current pass recorded|No legacy Store purchases/);
});
test('legacy enforced recovery remains explicitly compatible',async t=>{
 const f=fixture();t.after(()=>f.dom.window.close());f.data['/subscription']={mode:'subscription_only'};let called=0;f.w.renderLegacyStoreCheckout=()=>{called++};await f.w.renderStore(f.main);assert.equal(called,1);assert.deepEqual(f.reads,['/subscription']);
});
