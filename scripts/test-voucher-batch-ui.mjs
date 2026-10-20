import assert from 'node:assert/strict';
import fs from 'node:fs';
import test from 'node:test';
import {JSDOM} from 'jsdom';

const source=fs.readFileSync(new URL('../web/app.html',import.meta.url),'utf8');
const functions=source.slice(source.indexOf('const SHEET_MM ='),source.indexOf('/* ---------- Magic Coins'));
const issued=Array.from({length:105},(_,i)=>({id:`new-${i}`,code:`NEW-${i+1}`,plan_key:'guest-internet',plan_label:'Daily <Wi-Fi>',price_mmk:1500,duration_minutes:1440,device_id:'gateway',status:'unused'}));
const flush=()=>new Promise(resolve=>setTimeout(resolve,0));

function fixture({listFailure=false}={}) {
  const dom=new JSDOM('<main><div id="content"></div></main>',{runScripts:'outside-only',url:'https://example.test'});
  const {window}=dom;
  Object.assign(window,{
    ME:{user:{user_role:'OWNER'}},SITE_EDITOR_ROLES:['OWNER','ADMIN','DEVELOPER'],
    asArray:v=>Array.isArray(v)?v:[],ts:v=>String(v??''),businessStatusBadge:v=>String(v),
    guestVoucherCapabilityMessage:()=> 'Creation unavailable',
    esc:v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])),
  });
  window.setTimeout=()=>0;
  const old={id:'old',code:'OLD-INVENTORY',status:'unused',price_mmk:500,plan_key:'business-pass'};
  const writes=[];
  window.MM={
    async get(path) {
      if(path==='/vouchers') {if(listFailure)throw Error('Inventory unavailable');return {vouchers:[old]};}
      if(path==='/guest-vouchers/config')return {enabled:true};
      if(path==='/routers')return {routers:[{device_id:'gateway',hardware_name:'Cafe',deployment_layer:'MAIN_GATEWAY'}]};
      if(path==='/voucher-print-layout')return {layout:{show_price:true,show_expiry:true}};
      return {};
    },
    async post(path,body) {writes.push({path,body});return {vouchers:issued,product:'guest-internet',data_mb:1024,validity_starts:'first_use'};},
  };
  window.eval(functions);
  return {dom,window,writes,main:window.document.querySelector('main')};
}

async function createBatch(f) {
  await f.window.renderVouchers(f.main);
  const d=f.window.document;
  d.querySelector('#gv-gateway').value='gateway'; d.querySelector('#gv-label').value='Daily Wi-Fi';
  d.querySelector('#gv-count').value='105';
  d.querySelector('#guest-voucher-form').dispatchEvent(new f.window.Event('submit',{cancelable:true}));
  await flush();
}

test('New guest batch is selected for printing without older inventory or sample codes',async t=>{
  const f=fixture();t.after(()=>f.dom.window.close());await createBatch(f);
  const d=f.window.document;
  assert.equal(f.writes.length,1);
  assert.equal(f.writes[0].path,'/guest-vouchers');
  assert.equal(d.querySelector('#pl-batch-source')?.value,'issued','Printing must start from the exact returned batch');
  assert.match(d.querySelector('#pl-batch-range').textContent,/1–100 of 105/);
  assert.match(d.querySelector('#gv-batch-result').textContent,/105/);
  assert.ok(!d.querySelector('#pl-stage').textContent.includes('OLD-INVENTORY'));
  d.querySelector('#pl-print').click();
  let printed=d.querySelector('iframe').srcdoc;
  assert.equal((printed.match(/class="sheet-code"/g)||[]).length,100);
  assert.ok(!printed.includes('OLD-INVENTORY')&&!printed.includes('SAMPLE-'));
  assert.match(printed,/1500 MMK/);
  assert.match(printed,/Daily &lt;Wi-Fi&gt;/);
  assert.match(printed,/1440 min/);assert.match(printed,/first use/);assert.match(printed,/1024 MB/);
  d.querySelector('#pl-batch-next').click(); d.querySelector('#pl-print').click();
  printed=d.querySelectorAll('iframe')[1].srcdoc;
  assert.equal((printed.match(/class="sheet-code"/g)||[]).length,5);
  assert.ok(printed.includes('NEW-101')&&printed.includes('NEW-105')&&!printed.includes('NEW-100<'));
  const choice=d.querySelector('#pl-batch-source');choice.value='inventory';choice.dispatchEvent(new f.window.Event('change'));
  assert.match(d.querySelector('#pl-stage').textContent,/OLD-INVENTORY/);
});

test('Successful batch remains printable when the inventory refresh fails',async t=>{
  const f=fixture({listFailure:true});t.after(()=>f.dom.window.close());await createBatch(f);
  assert.match(f.main.textContent,/Could not load vouchers/);
  assert.equal(f.main.querySelector('#pl-batch-source')?.value,'issued');
  f.main.querySelector('#pl-print').click();
  assert.match(f.window.document.querySelector('iframe').srcdoc,/NEW-1/);
  assert.equal(f.writes.length,1,'Inventory failure must not create a replacement batch');
});

test('Current API prices, free vouchers, legacy amounts and escaped labels print accurately',t=>{
  const f=fixture();t.after(()=>f.dom.window.close());
  const layout={show_price:true,show_expiry:true},density={showMeta:true,brandFontPx:9,metaFontPx:8,codeFontPx:15,rowHeightMm:24,padMm:2};
  let tile=f.window.sheetTile(layout,density,{code:'FREE',price_mmk:0,amount:999,plan_label:'<img src=x>'});
  assert.match(tile,/0 MMK/);assert.ok(!tile.includes('999 MMK'));assert.ok(!tile.includes('<img'));
  tile=f.window.sheetTile(layout,density,{code:'LEGACY',amount:700});assert.match(tile,/700 MMK/);
  tile=f.window.sheetTile({...layout,show_price:false},density,{code:'HIDDEN',price_mmk:1500});assert.ok(!tile.includes('MMK'));
});

test('Every supported A4 row count reserves cut gaps and stays within the page',t=>{
  const f=fixture();t.after(()=>f.dom.window.close());
  for(let rows=4;rows<=50;rows++) {
    const density=f.window.sheetDensity('a4',rows);
    assert.ok(density.rowHeightMm*rows+(rows-1)+16 <= 297.001,`${rows} rows overflow A4`);
    const tile=f.window.sheetTile({business_name:'Coffee',wifi_name:'Guest Wi-Fi',show_price:true},density,issued[0]);
    assert.match(tile,/NEW-1/,'Code is retained at every supported density');
    if(tile.includes('data-details-omitted="true"')) assert.ok(!tile.includes('1500 MMK'),'Small tiles omit entire details rather than clipping them');
  }
  for(let rows=4;rows<=40;rows++) {
    const density=f.window.sheetDensity('thermal',rows);
    assert.ok(density.rowHeightMm>=4,'Continuous thermal strips must grow to preserve readable codes');
  }
});

test('A rendering failure after successful issuance cannot invite a duplicate batch',async t=>{
  const f=fixture();t.after(()=>f.dom.window.close());
  await f.window.renderVouchers(f.main);
  const d=f.window.document;
  d.querySelector('#gv-gateway').value='gateway';d.querySelector('#gv-label').value='Daily';
  const render=f.window.renderVouchers;
  f.window.renderVouchers=async(main,batch)=>{await render(main,batch);throw Error('Render failure after replacing content');};
  const form=d.querySelector('#guest-voucher-form');
  form.dispatchEvent(new f.window.Event('submit',{cancelable:true}));await flush();
  assert.match(d.querySelector('#gv-message').textContent,/Codes were created/);
  assert.equal(d.querySelector('#gv-create').disabled,true);
  d.querySelector('#guest-voucher-form').dispatchEvent(new f.window.Event('submit',{cancelable:true}));await flush();
  assert.equal(f.writes.length,1);
});

test('Retained batch respects refreshed used status without adding unrelated vouchers',async t=>{
  const f=fixture();t.after(()=>f.dom.window.close());
  const get=f.window.MM.get;
  f.window.MM.get=path=>path==='/vouchers'?Promise.resolve({vouchers:[{...issued[0],status:'used',first_seen_at:'2026-10-21T00:00:00Z'}]}):get(path);
  await f.window.renderVouchers(f.main,{vouchers:issued,data_mb:1024});
  assert.match(f.main.querySelector('#pl-batch-range').textContent,/of 104/);
  const stage=f.main.querySelector('#pl-stage');
  assert.equal(stage.querySelector('.sheet-code').textContent,'NEW-2');
  assert.ok(!stage.textContent.includes('OLD-INVENTORY'));
});
