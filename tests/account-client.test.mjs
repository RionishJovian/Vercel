import {test} from 'node:test';
import assert from 'node:assert/strict';
import {accountClient} from '../src/account-api.mjs';
test('private portal export uses authenticated POST and returns binary without token URLs',async()=>{
 let call;const archive=new Blob(['test-archive']);const client=accountClient(()=>{},async(url,options)=>{call={url,options};return {ok:true,blob:async()=>archive}});client.setToken('private-export-token');assert.equal(await client.download('portal/export',{expected_revision:3,public_origin:'https://app.example.com'}),archive);assert.equal(call.options.method,'POST');assert.equal(call.options.headers.Authorization,'Bearer private-export-token');assert.equal(JSON.parse(call.options.body).expected_revision,3);assert.equal(call.url,'/v1/portal/export');assert.ok(!call.url.includes('private-export-token'));
});
test('account client never targets demo and clears expired bearer',async()=>{
 const calls=[];let expired=0;
 const client=accountClient(()=>expired++,async(url,options)=>{calls.push({url,options});return {ok:calls.length!==2,status:calls.length===2?401:200,json:async()=>calls.length===2?{detail:'Session expired'}:{}}});
 client.setToken('private-test-session');await client.request('orders');await assert.rejects(client.request('me'),/Session expired/);await client.request('packages');
 assert.equal(expired,1);assert.ok(calls.every(c=>c.url.startsWith('/v1/')));assert.equal(calls[0].options.headers.Authorization,'Bearer private-test-session');assert.equal(calls[2].options.headers.Authorization,undefined);
});
test('validation errors reach the UI and login errors do not end another session',async()=>{
 let expired=0;const client=accountClient(()=>expired++,async()=>({ok:false,status:422,json:async()=>({detail:[{msg:'Positive quota required'}]})}));
 await assert.rejects(client.request('packages',{}),/Positive quota required/);assert.equal(expired,0);
});
test('receipt bytes use private bearer headers and upload keys, without JSON encoding',async()=>{
 const calls=[];const file=new Blob(['test-image'],{type:'image/png'});const client=accountClient(()=>{},async(url,options)=>{calls.push({url,options});return {ok:true,json:async()=>({id:'image-id'}),blob:async()=>file}});
 client.setToken('test-private-token');await client.upload('orders/order-id/receipts',file,'upload-key-123456');assert.equal(calls[0].options.body,file);assert.equal(calls[0].options.headers['Content-Type'],'application/octet-stream');assert.equal(calls[0].options.headers['X-Upload-Key'],'upload-key-123456');assert.equal(calls[0].options.headers.Authorization,'Bearer test-private-token');assert.equal(await client.blob('orders/order-id/receipts/image-id'),file);assert.ok(calls.every(c=>!c.url.includes('test-private-token')));
});
