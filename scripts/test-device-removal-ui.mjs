import fs from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';
import assert from 'node:assert/strict';
import {JSDOM} from 'jsdom';
const source=fs.readFileSync(new URL('../web/app.html',import.meta.url),'utf8');
const slice=source.slice(source.indexOf('function confirmDeviceRemoval('),source.indexOf('function hubEnrollmentUnavailableMessage('));
function fixture({role='OWNER',remove=async()=>{},refresh=async()=>{}}={}) {
 const dom=new JSDOM('<main><div id="content"><button data-router-delete="device/a">Delete</button></div></main>');
 const document=dom.window.document,content=document.querySelector('#content'),button=document.querySelector('button');
 const calls=[],errors=[],refreshes=[];
 dom.window.HTMLDialogElement.prototype.showModal=function(){this.open=true;};
 dom.window.HTMLDialogElement.prototype.close=function(){this.open=false;this.dispatchEvent(new dom.window.Event('close'));};
 const context=vm.createContext({document,ME:{user:{user_role:role}},SITE_EDITOR_ROLES:['OWNER','ADMIN','DEVELOPER'],encodeURIComponent,Error,
  confirm:()=>{throw Error('Native confirmation must not be used');},MM:{del:async path=>{calls.push(path);return remove();}},
  renderSites:async(main,workspace)=>{refreshes.push(workspace);return refresh();},showErr:error=>errors.push(error.message)});
 vm.runInContext(slice,context);
 context.wireRouterRemoval(content,document.querySelector('main'),[{device_id:'device/a',hardware_name:'Test <router>'}],'gateways');
 return {dom,button,calls,errors,refreshes,content,dialog:()=>document.querySelector('dialog'),accept:()=>document.querySelector('[data-confirm-delete]').click(),cancel:()=>document.querySelector('[data-confirm-cancel]').click()};
}
const tick=()=>new Promise(resolve=>setTimeout(resolve,0));
test('cancel, Escape and viewer produce no deletion request',async()=>{
 for(const action of ['cancel','escape','viewer']){
  const f=fixture({role:action==='viewer'?'VIEWER':'OWNER'});f.button.focus();f.button.click();
  if(action==='cancel')f.cancel();
  if(action==='escape')f.dialog().dispatchEvent(new f.dom.window.Event('cancel',{cancelable:true}));
  await tick();assert.equal(f.calls.length,0);assert.equal(f.refreshes.length,0);assert.equal(f.button.disabled,false);assert.equal(f.dialog(),null);
 }
});
test('the themed modal names the record safely and requires explicit destructive confirmation',async()=>{
 const f=fixture();f.button.focus();f.button.click();assert.equal(f.calls.length,0);
 const dialog=f.dialog();assert.equal(dialog.open,true);assert.equal(dialog.className,'app-confirm-dialog');
 assert.equal(dialog.querySelector('[data-device-name]').textContent,'Test <router>');assert.equal(dialog.querySelector('router'),null);
 assert.match(dialog.textContent,/cannot be undone/);assert.match(dialog.textContent,/does not reset.*router or revoke.*Magic Hub/);assert.match(dialog.textContent,/backups and change history/);
 assert.equal(f.dom.window.document.activeElement,dialog.querySelector('[data-confirm-cancel]'));
 f.accept();await tick();assert.deepEqual(f.calls,['/routers/device%2Fa']);assert.deepEqual(f.refreshes,['gateways']);assert.equal(f.dialog(),null);
});
test('pending deletion is sent once; failure keeps record and allows retry',async()=>{
 let reject;const f=fixture({remove:()=>new Promise((_,r)=>reject=r)});f.button.click();f.button.click();assert.equal(f.dom.window.document.querySelectorAll('dialog').length,1);f.accept();await tick();assert.equal(f.calls.length,1);assert.equal(f.button.disabled,true);
 reject(new Error('Record is referenced'));await tick();assert.deepEqual(f.errors,['Record is referenced']);assert.equal(f.refreshes.length,0);assert.equal(f.button.disabled,false);
});
test('a detached response cannot redraw a different page',async()=>{
 let resolve;const f=fixture({remove:()=>new Promise(r=>resolve=r)});f.button.click();f.accept();await tick();f.content.remove();resolve();await tick();assert.equal(f.refreshes.length,0);assert.equal(f.errors.length,0);
});
test('successful deletion with failed refresh is not offered as a retry',async()=>{
 const f=fixture({refresh:async()=>{throw Error('offline');}});f.button.click();f.accept();await tick();assert.equal(f.button.disabled,true);assert.match(f.errors[0],/record was deleted.*Reload/);
});
test('card exposes removal only to the existing device-editor roles',()=>{
 const renderSource=source.slice(source.indexOf('function routerCapabilityButton('),source.indexOf('async function renderSites('));
 const context=vm.createContext({ME:{user:{user_role:'VIEWER'}},SITE_EDITOR_ROLES:['OWNER','ADMIN','DEVELOPER'],asArray:x=>x||[],esc:x=>String(x??''),ts:x=>x,MMTheme:{icon:()=>''},topologyLayerLabel:x=>x,Date,Map});
 vm.runInContext(renderSource,context);
 const router={device_id:'device/a',deployment_layer:'ACCESS_POINT'};
 assert.doesNotMatch(context.routerBoardPanel([router],{}),/data-router-delete=/);
 for(const role of ['OWNER','ADMIN','DEVELOPER']){context.ME.user.user_role=role;assert.match(context.routerBoardPanel([router],{}),/data-router-delete=/);}
});
