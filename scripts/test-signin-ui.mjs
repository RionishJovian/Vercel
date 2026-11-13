import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {JSDOM, VirtualConsole} from 'jsdom';
const html=readFileSync(new URL('../web/signin.html',import.meta.url),'utf8');
const script=readFileSync(new URL('../web/assets/js/signin.js',import.meta.url),'utf8');
const tick=()=>new Promise(resolve=>setTimeout(resolve,0));
const guardInHead=html.indexOf('<script src="/assets/js/signin.js')<html.indexOf('</head>');
function mount(query='', dependencies=true, loadDeferred=true) {
 const calls=[],errors=[];
 const console=new VirtualConsole();console.on('jsdomError',e=>errors.push(e));
 const dom=new JSDOM(html,{url:'https://signin.test/signin'+query,runScripts:'outside-only',virtualConsole:console,beforeParse(w){if(guardInHead)w.eval(script);}});
 if(dependencies){
  dom.window.mmT=en=>en;
  dom.window.MM={post:async(path,body)=>{calls.push({path,body});throw Object.assign(new Error('unauthorized'),{status:401});}};
 }
 if(!guardInHead&&loadDeferred)dom.window.eval(script);
 return {dom,w:dom.window,calls,errors};
}
function fill(w,id,value){w.document.getElementById(id).value=value;}
function submit(w,id){const e=new w.Event('submit',{bubbles:true,cancelable:true});w.document.getElementById(id).dispatchEvent(e);return e;}
function assertActive(w,register){
 for(const [id,active]of [['login-form',!register],['register-form',register]]){
  const form=w.document.getElementById(id);assert.equal(form.hidden,!active);
  for(const c of form.querySelectorAll('input,button'))assert.equal(c.disabled,!active,`${id}: ${c.id||c.className}`);
  assert.equal(new w.FormData(form).has('password'),active,`${id}: inactive credentials must not be successful controls`);
 }
}
test('both public forms use POST even with no scripts running',()=>{
 const dom=new JSDOM(html);try{for(const form of dom.window.document.forms)assert.equal(form.method,'post');
  for(const c of dom.window.document.querySelectorAll('#register-form input,#register-form button'))assert.equal(c.disabled,true);
 }finally{dom.window.close();}
});
test('the sign-in guard loads synchronously in head before forms or deferred dependencies',()=>{
 const dom=new JSDOM(html);try{const guard=dom.window.document.querySelector('script[src^="/assets/js/signin.js"]');assert.ok(guard);assert.equal(guard.parentElement.tagName,'HEAD');assert.equal(guard.hasAttribute('defer'),false);assert.equal(guard.hasAttribute('async'),false);}finally{dom.window.close();}
});
for(const query of ['?email=synthetic%40example.test&password=synthetic-only','?password=&register=1&next=%2Fapp%2Frevenue','?%65mail=synthetic&next=%2Fapp%2Fhome','?PASSWORD=synthetic#old'])test(`credential query is removed before the body is parsed (${query.split('=')[0]})`,()=>{
 const {dom,w}=mount(query);try{assert.equal(w.location.href,'https://signin.test/signin');}finally{dom.window.close();}
});
test('the early submit guard cancels native navigation before DOMContentLoaded and without dependencies',()=>{
 const {dom,w,calls}=mount('',false,false);try{assert.equal(w.document.readyState,'loading');const e=submit(w,'login-form');assert.equal(e.defaultPrevented,true);assert.deepEqual(calls,[]);assert.equal(w.location.search,'');}finally{dom.window.close();}
});
test('only the active public tab exposes enabled autofill controls',async()=>{
 const {dom,w}=mount();try{await tick();assertActive(w,false);w.document.getElementById('tab-register').click();assertActive(w,true);w.document.getElementById('tab-login').click();assertActive(w,false);}finally{dom.window.close();}
});
test('the register deep link enables only registration and preserves the safe return path',async()=>{
 const {dom,w}=mount('?register=1&next=%2Fapp%2Frevenue');try{await tick();assertActive(w,true);assert.equal(new w.URLSearchParams(w.location.search).get('next'),'/app/revenue');}finally{dom.window.close();}
});
test('login cancels navigation synchronously and sends credentials only in the API body',async()=>{
 const {dom,w,calls,errors}=mount('?next=%2Fapp%2Frevenue');try{await tick();fill(w,'email','  legacy@mikromagic ');fill(w,'password','synthetic-only');const e=submit(w,'login-form');assert.equal(e.defaultPrevented,true);await tick();assert.deepEqual(calls.map(c=>JSON.parse(JSON.stringify(c))),[{path:'/auth/login',body:{email:'legacy@mikromagic',password:'synthetic-only'}}]);assert.equal(w.location.search,'?next=%2Fapp%2Frevenue');assert.deepEqual(errors,[]);}finally{dom.window.close();}
});
test('registration cancels navigation and posts only the active form values',async()=>{
 const {dom,w,calls,errors}=mount('?register=1');try{await tick();for(const [id,v]of Object.entries({'r-company':'Preview shop','r-name':'Preview Owner','r-email':'synthetic@example.test','r-password':'synthetic-only'}))fill(w,id,v);assert.equal(submit(w,'register-form').defaultPrevented,true);await tick();assert.deepEqual(calls.map(c=>JSON.parse(JSON.stringify(c))),[{path:'/auth/register',body:{company_name:'Preview shop',full_name:'Preview Owner',email:'synthetic@example.test',password:'synthetic-only'}}]);assert.equal(w.location.search,'?register=1');assert.deepEqual(errors,[]);}finally{dom.window.close();}
});
test('switching tabs during a request keeps hidden controls disabled and prevents duplicate submits',async()=>{
 const {dom,w,calls}=mount();let finish;try{await tick();w.MM.post=(path,body)=>{calls.push({path,body});return new Promise((_,reject)=>{finish=()=>reject(Object.assign(new Error('unauthorized'),{status:401}));});};fill(w,'email','synthetic@example.test');fill(w,'password','synthetic-only');submit(w,'login-form');w.document.getElementById('tab-register').click();assert.equal(w.document.getElementById('password').disabled,true);w.document.getElementById('tab-login').click();submit(w,'login-form');assert.equal(calls.length,1);w.document.getElementById('tab-register').click();finish();await tick();assertActive(w,true);assert.equal(w.document.getElementById('submit').disabled,true);}finally{dom.window.close();}
});
