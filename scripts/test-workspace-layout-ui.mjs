// Real Chromium geometry checks of the shared console layout, using source styles.
// Route/data integration is separately covered by console unit tests and the browser audit.
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {createServer} from 'node:http';
import {mkdtemp, readFile, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
const styles = await Promise.all(['tokens','base','shell','pages','telemetry','workspace'].map(n => readFile(new URL(`../app/src/styles/${n}.css`, import.meta.url), 'utf8')));
styles.push(await readFile(new URL('../app/src/components/tables.css', import.meta.url), 'utf8')); // Lazy table CSS can arrive after workspace.css.
const fixture = `<!doctype html><html data-theme="night" lang="en"><head><meta name="viewport" content="width=device-width,initial-scale=1"><style>${styles.join('\n')}</style></head><body><div class="shell"><main class="view"><div class="workspace-grid route-workspace"><section class="glass pad" id="summary">Status summary</section><section class="workspace-main glass pad" id="primary"><h1>Workspace</h1><p>${'Readable operational notes. '.repeat(20)}</p></section><aside class="workspace-rail glass pad" id="rail"><div class="dt-tools three" id="filters"><input class="input" value="ABC12345"><select class="input"><option>All batches</option></select><button class="dt-filter">Refresh</button></div></aside><section class="workspace-grid" id="solo"><div class="workspace-main glass pad">No side rail</div></section><section class="nt-layout"><div class="glass nt-stage" id="canvas">Topology canvas</div><aside class="glass pad nt-inspector" id="inspector">Selected node</aside></section></div></main></div></body></html>`;
const server=createServer((_req,res)=>{res.writeHead(200,{'Content-Type':'text/html'});res.end(fixture);});
const wait=async(fn,label,ms=12000)=>{const end=Date.now()+ms;while(Date.now()<end){if(await fn())return;await new Promise(r=>setTimeout(r,50));}throw new Error('timeout: '+label);};
let browser,profile,ws;
try {
 await new Promise(r=>server.listen(0,'127.0.0.1',r));
 profile=await mkdtemp(join(tmpdir(),'mm-workspace-layout-'));
 browser=spawn(process.env.CHROME_BIN||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',['--headless=new','--no-first-run','--disable-background-networking','--remote-debugging-port=0',`--user-data-dir=${profile}`,'about:blank'],{stdio:'ignore'});
 let debugPort;
 await wait(async()=>{try{debugPort=Number((await readFile(join(profile,'DevToolsActivePort'),'utf8')).split('\n')[0]);return !!debugPort;}catch{return false;}},'Chrome',45000);
 const tab=await fetch(`http://127.0.0.1:${debugPort}/json/new?about:blank`,{method:'PUT'}).then(r=>r.json());
 ws=new WebSocket(tab.webSocketDebuggerUrl);
 await new Promise((ok,bad)=>{ws.addEventListener('open',ok,{once:true});ws.addEventListener('error',bad,{once:true});});
 let serial=0;const pending=new Map();
 ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(m.id){const p=pending.get(m.id);pending.delete(m.id);m.error?p.reject(Error(m.error.message)):p.resolve(m.result);}});
 const send=(method,params={})=>new Promise((resolve,reject)=>{const id=++serial;pending.set(id,{resolve,reject});ws.send(JSON.stringify({id,method,params}));});
 const evaluate=async expression=>{const r=await send('Runtime.evaluate',{expression,returnByValue:true});if(r.exceptionDetails)throw Error(r.exceptionDetails.text);return r.result.value;};
 await send('Page.enable');await send('Page.navigate',{url:`http://127.0.0.1:${server.address().port}`});await wait(()=>evaluate(`!!document.querySelector('#rail')`),'fixture');
 let checks=0;
 for(const width of [375,1023,1024,1440,1920])for(const theme of ['night','day'])for(const lang of ['en','my']){
  await send('Emulation.setDeviceMetricsOverride',{width,height:1000,deviceScaleFactor:1,mobile:false});
  await evaluate(`document.documentElement.dataset.theme=${JSON.stringify(theme)};document.documentElement.lang=${JSON.stringify(lang)}`);
  const m=await evaluate(`(()=>{const box=id=>{const r=document.querySelector(id).getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,right:r.right,bottom:r.bottom}};return {summary:box('#summary'),primary:box('#primary'),rail:box('#rail'),solo:box('#solo'),soloMain:box('#solo > div'),canvas:box('#canvas'),inspector:box('#inspector'),view:box('.view'),scroll:document.documentElement.scrollWidth,tracks:getComputedStyle(document.querySelector('.route-workspace')).gridTemplateColumns.split(' ').length,filterTracks:getComputedStyle(document.querySelector('#filters')).gridTemplateColumns.split(' ').length,measure:parseFloat(getComputedStyle(document.querySelector('#primary p')).maxWidth)}})()`);
  const label=`${width}px ${theme} ${lang}`;
  assert.ok(m.scroll<=width,`${label}: document overflow`);
  if(width>=1024){
   assert.equal(m.filterTracks,1,`${label}: lazy table styles do not squeeze rail filters into three columns`);
   assert.equal(m.tracks,12,`${label}: twelve tracks`);
   assert.ok(Math.abs(m.primary.y-m.rail.y)<1,`${label}: primary and rail share a row`);
   assert.ok(m.primary.right<m.rail.x,`${label}: primary is left of rail`);
   assert.ok(Math.abs(m.primary.width-(m.rail.width*2+24))<2,`${label}: eight/four split`);
   assert.ok(Math.abs(m.summary.width-m.view.width)<1,`${label}: full-width summary`);
   assert.ok(Math.abs(m.solo.width-m.soloMain.width)<1,`${label}: absent rail releases its tracks`);
   assert.ok(Math.abs(m.canvas.y-m.inspector.y)<1,`${label}: topology split`);
   assert.ok(Math.abs(m.canvas.width-(m.inspector.width*3+48))<2,`${label}: nine/three canvas split`);
   assert.ok(m.view.width<=1360,`${label}: 1360px cap`);
   assert.ok(m.measure>0&&m.measure<700,`${label}: readable paragraph measure`);
   if(width===1920)assert.equal(m.view.width,1360,`${label}: workspace expands to cap`);
  }else{
   assert.ok(m.rail.y>=m.primary.bottom,`${label}: mobile/tablet panels stack`);
   assert.ok(Math.abs(m.primary.width-m.rail.width)<1,`${label}: mobile panels fill one column`);
  }
  checks++;
 }
 console.log(`Workspace layout: ${checks} viewport/theme/language combinations passed (375, 1023, 1024, 1440, 1920px).`);
} finally {
 ws?.close();
 if(browser&&!browser.killed){browser.kill('SIGTERM');await new Promise(r=>browser.once('exit',r));}
 await new Promise(r=>server.close(r));
 if(profile)await rm(profile,{recursive:true,force:true,maxRetries:5,retryDelay:100});
}
