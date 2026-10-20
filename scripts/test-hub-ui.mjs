import fs from 'node:fs';
import vm from 'node:vm';
import assert from 'node:assert/strict';
const source=fs.readFileSync(new URL('../web/app.html',import.meta.url),'utf8');
const start=source.indexOf('function routerCapabilityButton('),end=source.indexOf('async function renderSites(',start);
const context=vm.createContext({encodeURIComponent,Date,Map,Number,String,
  ME:{user:{user_role:'OWNER'}},SITE_EDITOR_ROLES:['OWNER','ADMIN','DEVELOPER'],
  asArray:x=>Array.isArray(x)?x:[],ts:x=>x,esc:x=>String(x??'').replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('>','&gt;').replaceAll('"','&quot;'),
  MMTheme:{icon:()=>''},topologyLayerLabel:x=>x});
vm.runInContext(source.slice(start,end),context);
const gateway={device_id:'gateway/a',hardware_name:'Gateway',deployment_layer:'MAIN_GATEWAY'};
assert(context.routerBoardPanel([gateway],{}).includes('data-hub-check='));
for (const step of ['1. Connect with one paste','2. Finish connection']) assert(context.routerBoardPanel([gateway],{}).includes(step));
assert(!context.routerBoardPanel([{...gateway,deployment_layer:'ACCESS_POINT'}],{}).includes('data-hub-check='));
context.ME.user.user_role='VIEWER';
assert(!context.routerBoardPanel([gateway],{}).includes('data-hub-check='));
context.ME.user.user_role='OWNER';
let click,resolve,calls=0;
const status={hidden:true,isConnected:true,textContent:'',innerHTML:''};
const button={isConnected:true,disabled:false,dataset:{hubCheck:'gateway/a'},closest:()=>({querySelector:()=>status}),addEventListener(_,fn){click=fn;}};
context.MM={get(path){calls++;assert.equal(path,'/vps/peers/gateway%2Fa/live');return new Promise(r=>resolve=r);}};
context.wireHubLookup({querySelectorAll:()=>[button]});
const pending=click();assert(button.disabled);await click();assert.equal(calls,1);
resolve({handshake_status:'recent_handshake',tunnel_address:'<img src=x>',last_handshake_at:null});await pending;
assert(!button.disabled);assert(status.innerHTML.includes('Recent tunnel handshake'));assert(!status.innerHTML.includes('<img'));assert(status.innerHTML.includes('does not verify LAN access'));
assert(context.hubLookupMarkup({handshake_status:'no_recent_handshake'}).includes('No recent tunnel handshake'));
for(const code of ['plan_required','subscription_required','hub_lookup_disabled','hub_lookup_failed']) {
  context.MM={get:async()=>{throw {data:{code},message:'INTERNAL_SECRET'};}};
  await click();assert(!status.textContent.includes('INTERNAL_SECRET'));assert(!button.disabled);
}
context.MM={get:async()=>{throw {status:404,data:{},message:'INTERNAL_SECRET'};}};
await click();assert(status.textContent.includes('Open Connect Magic Hub'));
context.MM={get:()=>new Promise(r=>resolve=r)};
const stale=click();const previous=status.innerHTML;status.isConnected=false;
resolve({handshake_status:'inactive'});await stale;assert.equal(status.innerHTML,previous,'detached page updated');
context.ME.user.user_role='VIEWER';context.MM={get(){throw new Error('viewer request');}};await click();
console.log('Hub UI passed: role/device visibility, encoded GET, duplicate-click guard, safe status/errors and detached-page guard.');
