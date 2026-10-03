// Real Chromium DOM and public-page CSP; synthetic HTTP/API fixtures only.
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {createServer} from 'node:http';
import {mkdtemp,readFile,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join,resolve,extname} from 'node:path';
const root=resolve(import.meta.dirname,'../web'),id='a'.repeat(64);
let publication=null,fail=false,writes=0,publicCookies=0;
const server=createServer(async(req,res)=>{
 const u=new URL(req.url,'http://localhost');
 const json=(code,data)=>{res.writeHead(code,{'Content-Type':'application/json'});res.end(JSON.stringify(data));};
 if(u.pathname==='/api/v1/media/publication'){
  if(req.method==='GET')return json(200,{publication});
  if(fail){fail=false;return json(409,{error:'Another operator changed this publication. Reload before publishing.'});}
  assert.equal(Number(u.searchParams.get('revision')),publication?.revision||0);
  if(req.method==='DELETE'){publication=null;res.writeHead(204);return res.end();}
  let raw='';for await(const c of req)raw+=c;writes++;
  publication={design:JSON.parse(raw),revision:(publication?.revision||0)+1,public_id:id,published_at:new Date().toISOString()};return json(200,{publication});
 }
 if(u.pathname==='/api/v1/media-guest/'+id){if(req.headers.cookie)publicCookies++;return publication?json(200,{...publication,purchases_enabled:false,playback_enabled:false}):json(404,{});}
 if(u.pathname==='/harness'){
  res.writeHead(200,{'Content-Type':'text/html'});return res.end(`<main id="content"></main><script type="module">
   import {mountAppearance} from '/assets/js/media-appearance.js';
   const req=async(method,path,body)=>{const r=await fetch('/api/v1'+path,{method,headers:{'Content-Type':'application/json'},body:body?JSON.stringify(body):undefined});const d=r.status===204?null:await r.json();if(!r.ok)throw new Error(d.error);return d;};
   mountAppearance(document.querySelector('main'),{business_id:'business',company_name:'QA cinema'},{email:'qa@test'},{get:p=>req('GET',p),put:(p,b)=>req('PUT',p,b),del:p=>req('DELETE',p)});
  </script>`);
 }
 try{const path=resolve(root,'.'+u.pathname);if(!path.startsWith(root+'/'))return json(404,{});const data=await readFile(path);res.writeHead(200,{'Content-Type':({'.html':'text/html','.js':'text/javascript','.css':'text/css'})[extname(path)]||'application/octet-stream'});res.end(data);}catch(_){json(404,{});}
});
const wait=async(fn,label)=>{const end=Date.now()+12000;while(Date.now()<end){if(await fn())return;await new Promise(r=>setTimeout(r,50));}throw new Error('timeout: '+label);};
let browser,profile,ws;
try{
 await new Promise(r=>server.listen(0,'127.0.0.1',r));const port=server.address().port;
 profile=await mkdtemp(join(tmpdir(),'mm-media-publish-'));const debugPort=9700+Math.floor(Math.random()*300);
 browser=spawn(process.env.CHROME_BIN||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',['--headless=new','--no-first-run','--disable-background-networking',`--remote-debugging-port=${debugPort}`,`--user-data-dir=${profile}`,'about:blank'],{stdio:'ignore'});
 await wait(async()=>{try{return(await fetch(`http://127.0.0.1:${debugPort}/json/version`)).ok;}catch(_){return false;}},'Chrome');
 const tab=await fetch(`http://127.0.0.1:${debugPort}/json/new?about:blank`,{method:'PUT'}).then(r=>r.json());ws=new WebSocket(tab.webSocketDebuggerUrl);
 await new Promise((ok,bad)=>{ws.addEventListener('open',ok,{once:true});ws.addEventListener('error',bad,{once:true});});
 let serial=0;const pending=new Map(),errors=[];
 ws.addEventListener('message',event=>{const m=JSON.parse(event.data);if(m.method==='Runtime.exceptionThrown')errors.push(m.params.exceptionDetails.text);if(m.method==='Log.entryAdded'&&m.params.entry.source==='security')errors.push(m.params.entry.text);if(m.method==='Page.javascriptDialogOpening')send('Page.handleJavaScriptDialog',{accept:true});if(m.id){const p=pending.get(m.id);pending.delete(m.id);m.error?p.reject(new Error(m.error.message)):p.resolve(m.result);}});
 function send(method,params={}){const n=++serial;return new Promise((resolve,reject)=>{pending.set(n,{resolve,reject});ws.send(JSON.stringify({id:n,method,params}));});}
 const evaluate=async expression=>{const r=await send('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(r.exceptionDetails)throw new Error(r.exceptionDetails.text);return r.result.value;};
 await send('Page.enable');await send('Runtime.enable');await send('Log.enable');
 const navigate=async path=>{await send('Page.navigate',{url:`http://127.0.0.1:${port}${path}`});};
 await navigate('/harness');await wait(()=>evaluate(`!!document.querySelector('[type=submit]')&&!document.querySelector('[type=submit]').disabled`),'loaded editor');
 assert.equal(writes,0,'loading does not publish');
 await evaluate(`document.querySelector('[name=price]').value='2500';document.querySelector('[name=price]').dispatchEvent(new Event('input'));document.querySelector('form').requestSubmit()`);
 await wait(()=>evaluate(`document.querySelector('#media-design-status').textContent.includes('Guest page published')`),'publish');assert.equal(writes,1);assert.equal(publication.design.price,2500);
 await navigate('/harness');await wait(()=>evaluate(`document.querySelector('[name=price]')?.value==='2500'`),'server persisted across reload');
 fail=true;await evaluate(`document.querySelector('form').requestSubmit()`);
 await wait(()=>evaluate(`document.querySelector('#media-design-status').textContent.includes('Another operator')`),'conflict');
 assert.equal(await evaluate(`document.querySelector('[type=submit]').disabled`),true,'conflict requires reload');
 await evaluate(`document.querySelector('[data-media-reload]').click()`);await wait(()=>evaluate(`!document.querySelector('[type=submit]').disabled`),'reload after conflict');
 await send('Network.setCookie',{name:'mm_access',value:'synthetic',url:`http://127.0.0.1:${port}`});
 await navigate('/media.html?id='+id);await wait(()=>evaluate(`document.querySelector('iframe')?.hidden===false`),'published public page');
 const doc=await evaluate(`document.querySelector('iframe').srcdoc`);assert.ok(doc.includes('GUEST PAGE'));assert.ok(!doc.includes('Sample title'));assert.ok(doc.includes('No payment is collected'));assert.equal(publicCookies,0,'public fetch omits staff cookies');
 assert.equal(await evaluate(`document.querySelector('iframe').getAttribute('sandbox')`),'');
 await navigate('/harness');await wait(()=>evaluate(`!!document.querySelector('[data-media-unpublish]')&&!document.querySelector('[data-media-unpublish]').disabled`),'unpublish ready');
 await evaluate(`document.querySelector('[data-media-unpublish]').click()`);await wait(()=>evaluate(`document.querySelector('#media-design-status').textContent.includes('unpublished')`),'unpublished');
 await navigate('/media.html?id='+id);await wait(()=>evaluate(`document.querySelector('#media-guest-status')?.textContent.includes('unpublished')`),'withdrawn link unavailable');
 assert.deepEqual(errors,[]);console.log('Media publishing browser checks passed: publish, server reload, stale-write handling, public CSP, cookie isolation, unpublish.');
}finally{
 ws?.close();if(browser&&browser.exitCode===null){const ended=new Promise(r=>browser.once('exit',r));browser.kill('SIGTERM');await ended;}
 await new Promise(r=>server.close(r));if(profile)await rm(profile,{recursive:true,force:true});
}
