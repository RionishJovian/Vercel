import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';
import {JSDOM} from 'jsdom';

const source = fs.readFileSync(new URL('../web/app.html', import.meta.url), 'utf8');
const functions = source.slice(source.indexOf('async function renderNetworkSetup('), source.indexOf('// Inline edit form for one router'));
const routers = [
  {device_id:'one', hardware_name:'Cafe gateway', deployment_layer:'MAIN_GATEWAY'},
  {device_id:'two', hardware_name:'Branch gateway', deployment_layer:'MAIN_GATEWAY'},
];
const interfaces = [{name:'ether1',type:'ether'},{name:'ether2',type:'ether'}];
const defaults = {name:'guest', gateway_cidr:'192.168.50.1/24', pool_start:'192.168.50.10', pool_end:'192.168.50.250'};
const flush = () => new Promise(resolve => setTimeout(resolve, 0));

function fixture() {
  const dom = new JSDOM('<main><div id="content"></div></main>', {runScripts:'outside-only',url:'https://example.test'});
  const {window} = dom;
  Object.assign(window, {
    ME:{user:{user_role:'OWNER'}}, SITE_EDITOR_ROLES:['OWNER','ADMIN','DEVELOPER'],
    asArray:value=>Array.isArray(value)?value:[],
    esc:value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])),
  });
  window.confirm = () => true;
  window.eval(functions);
  const calls = [], reads = [];
  window.MM = {
    async get(path) {
      reads.push(path);
      if (path === '/routers') return {routers};
      if (path.endsWith('/interfaces')) return {interfaces};
      if (path.endsWith('/defaults')) return {defaults};
      throw Error('Unexpected read '+path);
    },
    post(path, body) { return new Promise((resolve,reject)=>calls.push({path,body,resolve,reject})); },
  };
  const main = window.document.querySelector('main');
  return {dom, window, main, calls, reads};
}

function setupForm(f) {
  const {window,main} = f;
  main.innerHTML = '<select id="network-router"><option value="one">Cafe gateway</option><option value="two">Branch gateway</option></select><div id="editor"></div>';
  const editor = main.querySelector('#editor'), target = main.querySelector('select');
  editor.innerHTML = window.gatewaySetupForm(interfaces, defaults);
  const form = editor.querySelector('form');
  form.elements.wan_interface.value='ether1'; form.elements.lan_interface.value='ether2';
  window.wireGatewaySetup(editor,routers[0],target);
  return {form,target,status:form.querySelector('#gateway-status'),apply:form.querySelector('#gateway-apply')};
}

async function review(f, form) {
  form.dispatchEvent(new f.window.Event('submit',{cancelable:true}));
  f.calls.at(-1).resolve({plan:{confirmation:'fixture-review',steps:['Create guest LAN and Hotspot']}});
  await flush();
}

test('Basic setup locks the router and fields and blocks duplicate writes until verification', async t => {
  const f=fixture(); t.after(()=>f.dom.window.close());
  const {form,target,status,apply}=setupForm(f);
  await review(f,form);
  apply.click();
  assert.equal(f.calls.length,2);
  assert.equal(target.disabled,true,'Target must remain locked while RouterOS is being changed');
  for(const control of form.querySelectorAll('input,select,button')) assert.equal(control.disabled,true);
  const pendingText=status.textContent;
  form.elements.name.dispatchEvent(new f.window.Event('change',{bubbles:true}));
  form.dispatchEvent(new f.window.Event('submit',{cancelable:true}));
  apply.dispatchEvent(new f.window.Event('click'));
  assert.equal(f.calls.length,2,'Exactly one reviewed apply is allowed');
  assert.equal(status.textContent,pendingText,'Pending apply status must survive stray events');
  assert.equal(f.calls[1].path,'/routers/one/gateway/apply');
  assert.equal(f.calls[1].body.lan_interface,'ether2');
  f.calls[1].resolve({result:{verified:true,backup:'fixture-backup'}}); await flush();
  assert.match(status.textContent,/applied and verified/);
  assert.equal(target.disabled,false);
  assert.equal(form.elements.name.disabled,false);
  assert.equal(apply.hidden,true);
  assert.equal(form.elements.radius_secret.value,'');
});

test('Failed or unverified apply unlocks controls and requires a fresh review', async t => {
  for(const outcome of ['failed','unverified','truthy-unverified']) {
    const f=fixture(); t.after(()=>f.dom.window.close());
    const {form,target,status,apply}=setupForm(f);
    await review(f,form); apply.click();
    if(outcome==='failed') f.calls[1].reject({data:{error:'Write failed',result:{rolled_back:true}}});
    else f.calls[1].resolve({result:{verified:outcome==='unverified'?false:'true'}});
    await flush();
    assert.equal(target.disabled,false);
    assert.equal(form.elements.wan_interface.disabled,false);
    assert.equal(apply.hidden,true);
    assert.match(status.textContent,outcome==='failed'?/rollback verified/:/No successful verification/);
    apply.dispatchEvent(new f.window.Event('click'));
    assert.equal(f.calls.length,2,'Failed review token cannot be reused');
  }
});

test('Changing a selection invalidates review and cancelled confirmation sends no write', async t => {
  const f=fixture(); t.after(()=>f.dom.window.close());
  const {form,apply}=setupForm(f);
  await review(f,form);
  form.elements.hotspot.checked=false;
  form.elements.hotspot.dispatchEvent(new f.window.Event('change',{bubbles:true}));
  assert.equal(apply.hidden,true,'Change events must invalidate the exact reviewed inputs');
  apply.dispatchEvent(new f.window.Event('click')); assert.equal(f.calls.length,1);
  await review(f,form);
  f.window.confirm=()=>false; apply.click(); assert.equal(f.calls.length,2);
});

test('Preview races cannot revive an obsolete review or start parallel requests', async t => {
  const f=fixture(); t.after(()=>f.dom.window.close());
  const {form,apply}=setupForm(f);
  form.dispatchEvent(new f.window.Event('submit',{cancelable:true}));
  form.dispatchEvent(new f.window.Event('submit',{cancelable:true}));
  assert.equal(f.calls.length,1,'An outstanding preview cannot be submitted again');
  form.elements.name.value='changed';
  form.elements.name.dispatchEvent(new f.window.Event('input',{bubbles:true}));
  f.calls[0].resolve({plan:{confirmation:'obsolete',steps:[]}}); await flush();
  assert.equal(apply.hidden,true);
  apply.dispatchEvent(new f.window.Event('click')); assert.equal(f.calls.length,1);
});

test('Saved authorized gateway is read automatically without any configuration writes', async t => {
  const f=fixture(); t.after(()=>f.dom.window.close());
  f.window.sessionStorage.setItem('mm_selected_router','two');
  await f.window.renderNetworkSetup(f.main);
  assert.equal(f.main.querySelector('#network-router').value,'two');
  assert.deepEqual(f.reads,['/routers','/routers/two/gateway/interfaces','/routers/two/gateway/defaults']);
  assert.equal(f.calls.length,0);
  assert.equal(f.main.querySelector('[name=wan_interface]').value,'','Port roles must never be guessed');
  assert.equal(f.main.querySelector('[name=lan_interface]').value,'');
});

test('A single gateway opens automatically; foreign or removed selections are not used', async t => {
  const f=fixture(); t.after(()=>f.dom.window.close());
  f.window.sessionStorage.setItem('mm_selected_router','foreign');
  await f.window.renderNetworkSetup(f.main);
  assert.deepEqual(f.reads,['/routers']);
  assert.equal(f.main.querySelector('#network-router').value,'');
  const get=f.window.MM.get;
  f.window.MM.get=path=>path==='/routers'?Promise.resolve({routers:[routers[0]]}):get(path);
  await f.window.renderNetworkSetup(f.main);
  assert.equal(f.main.querySelector('#network-router').value,'one');
  assert.equal(f.window.sessionStorage.getItem('mm_selected_router'),'one');
  assert.ok(f.main.querySelector('form'));
});

test('The page keeps its gateway locked during apply and remembers an intentional change afterwards', async t => {
  const f=fixture(); t.after(()=>f.dom.window.close());
  f.window.sessionStorage.setItem('mm_selected_router','one');
  await f.window.renderNetworkSetup(f.main);
  const form=f.main.querySelector('form'), select=f.main.querySelector('#network-router');
  form.elements.wan_interface.value='ether1'; form.elements.lan_interface.value='ether2';
  await review(f,form); form.querySelector('#gateway-apply').click();
  assert.equal(select.disabled,true);
  select.dispatchEvent(new f.window.Event('change')); await flush();
  assert.equal(f.reads.length,3,'Target events during apply cannot replace the pending form');
  assert.equal(f.main.querySelector('form'),form);
  f.calls[1].resolve({result:{verified:true,backup:'fixture'}}); await flush();
  select.value='two'; select.dispatchEvent(new f.window.Event('change')); await flush();
  assert.equal(f.window.sessionStorage.getItem('mm_selected_router'),'two');
  assert.equal(f.reads.at(-1),'/routers/two/gateway/defaults');
  select.value=''; select.dispatchEvent(new f.window.Event('change')); await flush();
  assert.equal(f.window.sessionStorage.getItem('mm_selected_router'),null);
});

test('Late inventory never replaces a newer gateway or a page navigated away from', async t => {
  const f=fixture(); t.after(()=>f.dom.window.close());
  const pending=[];
  f.window.MM.get=path=>path==='/routers'?Promise.resolve({routers}):new Promise(resolve=>pending.push({path,resolve}));
  await f.window.renderNetworkSetup(f.main);
  const select=f.main.querySelector('#network-router');
  select.value='one'; select.dispatchEvent(new f.window.Event('change'));
  select.value='two'; select.dispatchEvent(new f.window.Event('change'));
  for(const read of pending.slice(2)) read.resolve(read.path.endsWith('interfaces')?{interfaces}:{defaults:{...defaults,name:'branch'}});
  await flush();
  for(const read of pending.slice(0,2)) read.resolve(read.path.endsWith('interfaces')?{interfaces}:{defaults:{...defaults,name:'obsolete'}});
  await flush();
  assert.equal(f.main.querySelector('[name=name]').value,'branch');
  select.value='one'; select.dispatchEvent(new f.window.Event('change'));
  f.main.querySelector('#content').innerHTML='<h2>Another task</h2>';
  for(const read of pending.slice(4)) read.resolve(read.path.endsWith('interfaces')?{interfaces}:{defaults});
  await flush();
  assert.equal(f.main.querySelector('#content').textContent,'Another task');
});

test('Unavailable browser storage does not block read-only router access', async t => {
  const f=fixture(); t.after(()=>f.dom.window.close());
  f.window.ME.user.user_role='VIEWER';
  Object.defineProperty(f.window,'sessionStorage',{get(){throw Error('Storage disabled');}});
  const get=f.window.MM.get;
  f.window.MM.get=path=>path==='/routers'?Promise.resolve({routers:[routers[0]]}):get(path);
  await f.window.renderNetworkSetup(f.main);
  assert.equal(f.main.querySelector('#network-router').value,'one');
  assert.match(f.main.textContent,/owner or administrator/);
  assert.equal(f.main.querySelector('form'),null);
  assert.equal(f.calls.length,0);
});
