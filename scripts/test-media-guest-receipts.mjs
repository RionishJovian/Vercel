// DOM regression test with synthetic responses; no browser, bot or real payment.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {createRequire} from 'node:module';
const {JSDOM}=createRequire(import.meta.url)('jsdom');
const id='a'.repeat(64), passID='b'.repeat(64);
const html=fs.readFileSync('web/media.html','utf8');
const source=fs.readFileSync('web/assets/js/media-guest.js','utf8').replace(/^import .*;\n/, '');
const dom=new JSDOM(html,{url:'https://example.test/media.html?id='+id,runScripts:'outside-only'});
const w=dom.window;
w.publishedDocument=()=>'<p>Synthetic guest page</p>';
let state='pending', submitted=false, checkouts=0, failDelivery=true;
const pass=()=>({id:passID,amount:1500,currency:'MMK',duration_seconds:3600,state,payment_deadline:new Date(Date.now()+60000).toISOString(),payment:{bank_name:'Fixture bank',bank_account:'123',bank_holder:'Fixture owner'}});
w.fetch=async(url,options)=>{
 assert.equal(options.credentials,'omit','Staff cookies must never authenticate a guest purchase');
 assert.equal(options.cache,'no-store');
 const reply=body=>({ok:true,json:async()=>body});
 if(url==='/api/v1/media-guest/'+id)return reply({design:{price:1500,duration:1},purchases_enabled:true});
 assert.match(options.headers.Authorization,/^Bearer [0-9a-f]{64}$/);
 if(url.endsWith('/checkout')){
  checkouts++;
  const body=JSON.parse(options.body);
  assert.deepEqual(Object.keys(body),['checkout_key']);
  assert.match(body.checkout_key,/^[0-9a-f]{64}$/);
  return reply({pass:pass()});
 }
 if(url.endsWith('/receipt')){
  assert.ok(options.body.get('receipt'),'Actual receipt file is attached');
  submitted=true;
  return reply({receipt_submitted:true,owner_notified:!failDelivery,state:'pending'});
 }
 return reply({pass:pass(),receipt_submitted:submitted});
};
const settle=()=>new Promise(resolve=>setTimeout(resolve,10));
w.eval(source);
await settle();
assert.equal(w.document.querySelector('#media-payment').hidden,false);
w.document.querySelector('#media-checkout').click();
await settle();
assert.equal(checkouts,1);
assert.match(w.document.querySelector('#media-bank-text').textContent,/Fixture owner/);
const file=new w.File(['synthetic receipt'],'receipt.png',{type:'image/png'});
Object.defineProperty(w.document.querySelector('[name=receipt]'),'files',{value:[file]});
const submit=async()=>{w.document.querySelector('#media-receipt-form').dispatchEvent(new w.Event('submit',{bubbles:true,cancelable:true}));await settle();};
await submit();
assert.match(w.document.querySelector('#media-order-status').textContent,/Telegram delivery was not confirmed/);
assert.equal(state,'pending','Receipt upload never approves a pass');
failDelivery=false;
await submit();
assert.match(w.document.querySelector('#media-order-status').textContent,/Waiting for approval/);
state='paid';
w.document.querySelector('#media-order-refresh').click();
await settle();
assert.match(w.document.querySelector('#media-order-status').textContent,/Approved/);
assert.equal(w.document.querySelector('#media-receipt-form').hidden,true);
assert.equal(checkouts,1,'Refreshing an approved order does not create a second checkout');
state='refunded';
w.document.querySelector('#media-order-refresh').click();
await settle();
assert.match(w.document.querySelector('#media-order-status').textContent,/Access has ended/);
dom.window.close();
console.log('Guest receipt DOM checks passed: bank terms, cookie isolation, upload, delivery retry, approval refresh and refund.');
