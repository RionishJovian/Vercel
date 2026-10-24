import fs from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';
import assert from 'node:assert/strict';
import {JSDOM} from 'jsdom';
const app=fs.readFileSync(new URL('../web/app.html',import.meta.url),'utf8');
const code=app.slice(app.indexOf('function platformAccountTable('),app.indexOf('function platformBusinessTable('));
const escape=x=>String(x??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
function fixture(get){const dom=new JSDOM('<section id="accounts"><div data-account-results></div></section>');const context=vm.createContext({document:dom.window.document,esc:escape,ts:x=>x,MM:{get},Math,Object,Number,Set,setTimeout:()=>0,clearTimeout:()=>{}});vm.runInContext(code,context);return {dom,context,panel:dom.window.document.querySelector('section')};}
const tick=()=>new Promise(r=>setTimeout(r,0));
const account={email:'<script>bad</script>',company_name:'Cafe',user_role:'VIEWER',is_active:true,presence:'idle',pass:{kind:'annual',state:'active',scope:'account',expires_at:'2027-01-01'},limits:{scope:'business',device_count:2,device_quota_limit:10,ai_token_balance:0,legacy_capacity_enforced:true},permissions:{voucher_issuance:false},ai_usage_30d:{calls:0,tokens:0}};
test('account rows distinguish presence, pass, expiry, missing activation and shared limits',()=>{
 const f=fixture();const html=f.context.platformAccountTable([account]);f.panel.innerHTML=html;assert.equal(f.panel.querySelector('script'),null);assert.match(f.panel.textContent,/Idle/);assert.match(f.panel.textContent,/Annual/);assert.match(f.panel.textContent,/Old Store transactions are not imported/);assert.match(f.panel.textContent,/not per-user allowances/);assert.match(f.panel.textContent,/balance: 0/);assert.match(f.panel.textContent,/voucher issuance: Restricted/);assert.match(f.panel.textContent,/Account enabled/);
});
test('unavailable account API never fabricates offline accounts or zero balances',async()=>{
 const f=fixture(async()=>{throw Error('private internal detail');});await f.context.loadPlatformAccounts(f.panel,0);assert.match(f.panel.textContent,/unavailable/);assert.doesNotMatch(f.panel.textContent,/private internal/);assert(f.panel.querySelector('[data-accounts-retry]'));
});
test('bounded account pagination stays on the current page when refreshing',async()=>{
 const calls=[];const f=fixture(async path=>{calls.push(path);return {accounts:[account],total:120,limit:50,as_of:'now'};});await f.context.loadPlatformAccounts(f.panel,50);f.panel.querySelector('[data-accounts-refresh]').click();await tick();assert.deepEqual(calls,['/platform/accounts?limit=50&offset=50','/platform/accounts?limit=50&offset=50']);assert.equal(f.panel.querySelector('[data-accounts-next]').disabled,false);
});
test('late account replies cannot overwrite a newer page or detached screen',async()=>{
 const pending=[];const f=fixture(()=>new Promise(resolve=>pending.push(resolve)));const a=f.context.loadPlatformAccounts(f.panel,0),b=f.context.loadPlatformAccounts(f.panel,50);pending[1]({accounts:[account],total:100,limit:50,as_of:'new'});await b;pending[0]({accounts:[],total:0,limit:50,as_of:'old'});await a;assert.match(f.panel.textContent,/new/);assert.doesNotMatch(f.panel.textContent,/old/);
 const c=f.context.loadPlatformAccounts(f.panel,0);const before=f.panel.innerHTML;f.panel.remove();pending[2]({accounts:[],total:0,limit:50,as_of:'detached'});await c;assert.equal(f.panel.innerHTML,before);
});
const presence=fs.readFileSync(new URL('../web/assets/js/account-presence.js',import.meta.url),'utf8');
function presenceFixture(post){let now=1000000,visible='visible',interval;const dom=new JSDOM('<body></body>');Object.defineProperty(dom.window.document,'visibilityState',{get:()=>visible});const calls=[];dom.window.setInterval=fn=>{interval=fn;};const context=vm.createContext({window:dom.window,document:dom.window.document,navigator:{onLine:true},Date:{now:()=>now},Math,MM:{post:async(...args)=>{calls.push(args);return post?.(...args);}}});vm.runInContext(presence,context);return {dom,calls,client:dom.window.MMAccountPresence,interval:()=>interval(),advance:ms=>now+=ms,hidden:()=>{visible='hidden';dom.window.document.dispatchEvent(new dom.window.Event('visibilitychange'));}};}
test('presence starts once, observes idle and visibility, and sends no user/session identifiers',async()=>{
 const f=presenceFixture();f.client.start();f.client.start();await tick();assert.equal(f.calls.length,1);assert.equal(f.calls[0][0],'/me/presence');assert.deepEqual(Object.keys(f.calls[0][1]).sort(),['idle_seconds','visible']);f.advance(301000);f.interval();await tick();assert.equal(f.calls[1][1].idle_seconds,301);f.hidden();await tick();assert.equal(f.calls[2][1].visible,false);f.dom.window.document.dispatchEvent(new f.dom.window.Event('pointerdown'));await tick();assert.equal(f.calls.at(-1)[1].idle_seconds,0);
});
test('revoked presence stops sending rather than looping unauthorized requests',async()=>{
 const f=presenceFixture(()=>{throw {status:401};});f.client.start();await tick();f.interval();await tick();assert.equal(f.calls.length,1);
});
test('pass activation delegates the clock to the server and blocks duplicate submits',async()=>{
 const row={...account,user_id:'account-id',pass:{kind:'none',scope:'account',state:'no_pass'}};
 const f=fixture(async()=>({accounts:[row],total:1,limit:50,as_of:'now'})),writes=[];
 f.context.MM.put=async(path,body)=>{writes.push({path,body:JSON.parse(JSON.stringify(body))});return {recorded:true};};
 await f.context.loadPlatformAccounts(f.panel);
 const form=f.panel.querySelector('[data-record-pass]');form.requestSubmit();form.dispatchEvent(new f.dom.window.Event('submit',{cancelable:true}));await tick();
 assert.equal(writes.length,1);assert.equal(writes[0].path,'/platform/accounts/account-id/pass');assert.equal(writes[0].body.action,'activate');assert.equal(writes[0].body.activated_at,undefined);assert.equal(writes[0].body.expires_at,undefined);assert.equal(writes[0].body.revision,0);
});
