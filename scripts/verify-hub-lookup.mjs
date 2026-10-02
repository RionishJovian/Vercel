// Run on the Hub as a privileged operator. Credentials stay in this process;
// output contains only verification results, never keys or customer identities.
import fs from 'node:fs';
import http from 'node:http';
import {createHmac,randomUUID,createHash} from 'node:crypto';
import assert from 'node:assert/strict';
const settings = {};
for (const line of fs.readFileSync('/etc/mikromagic-hub/env','utf8').split('\n')) {
  const match = /^(MM_HUB_SIGNING_KEY|MM_HUB_SIGNING_KEY_ID|MM_HUB_PEERS_PATH)=(.*)$/.exec(line.trim());
  if (match) settings[match[1]]=match[2].trim().replace(/^(["'])(.*)\1$/, '$2');
}
const secret = settings.MM_HUB_SIGNING_KEY;
if (!secret || !/^[a-fA-F0-9]{64}$/.test(secret)) throw new Error('canonical signing credential unavailable');
const peerPath = settings.MM_HUB_PEERS_PATH || '/var/lib/mikromagic-hub/peers.json';
const raw = fs.readFileSync(peerPath);
const hash = value => createHash('sha256').update(value).digest('hex');
const store = JSON.parse(raw);
const peer = (Array.isArray(store)?store:store.peers).find(p=>p.state!=='removed');
if (!peer) throw new Error('no existing peer available for read-only verification');
async function get({tenant=peer.tenantId,router=peer.routerId,urlRouter=router,scope='peers:inspect',unsigned=false}={}) {
  const path='/v1/peers/by-router/'+encodeURIComponent(urlRouter), timestamp=new Date().toISOString(),requestId=randomUUID(),user='lookup-verifier';
  const canonical=['GET',path,timestamp,requestId,scope,tenant,router,user,''].join('\n');
  const headers=unsigned?{}:{'x-mm-key-id':settings.MM_HUB_SIGNING_KEY_ID||'v1','x-mm-timestamp':timestamp,'x-mm-request-id':requestId,'x-mm-scope':scope,'x-mm-tenant-id':tenant,'x-mm-router-id':router,'x-mm-requested-by':user,'x-mm-signature':createHmac('sha256',Buffer.from(secret,'hex')).update(canonical).digest('base64url')};
  return new Promise((resolve,reject)=>{
    const req=http.get({hostname:'127.0.0.1',port:8787,path,headers,timeout:10000},res=>{let data='';res.on('data',chunk=>{data+=chunk;if(data.length>65536)req.destroy(new Error('oversized response'));});res.on('end',()=>{try{resolve({status:res.statusCode,body:JSON.parse(data)});}catch{reject(new Error('invalid response'));}});});
    req.on('timeout',()=>req.destroy(new Error('lookup timeout')));req.on('error',reject);
  });
}
const valid=await get();
assert.equal(valid.status,200,'signed lookup failed');
// Boolean assertions avoid printing returned peer records in failure messages.
assert(valid.body.peerId===peer.peerId && valid.body.tenantId===peer.tenantId && valid.body.routerId===peer.routerId,'returned identity mismatch');
assert.equal((await get({tenant:'isolated-denied-tenant'})).status,404);
assert.equal((await get({urlRouter:'isolated-mismatch-router'})).status,401);
assert.equal((await get({scope:'peers:create'})).status,401);
assert.equal((await get({unsigned:true})).status,401);
assert(hash(fs.readFileSync(peerPath))===hash(raw),'peer store changed');
console.log('Live Hub lookup passed: signed existing peer, tenant denial, router mismatch, scope denial, unsigned denial; peer store unchanged.');
