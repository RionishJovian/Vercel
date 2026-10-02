import assert from 'node:assert/strict';
import {enrollCreateOnly} from './hub-enrollment.mjs';
const key=Buffer.alloc(32,1).toString('base64'), otherKey=Buffer.alloc(32,2).toString('base64');
function fixture() {
  let persisted=[], writes=0, syncs=0, failSync=false, verified=true;
  const auth={tenantId:'tenant',routerId:'router',requestedBy:'owner',scope:'peers:create'};
  const body={tenantId:'tenant',routerId:'router',routerPublicKey:key,managementRestPort:443};
  const deps={auth,routerId:'router',body,load:()=>structuredClone(persisted),
    save:peers=>{writes++;persisted=structuredClone(peers)},allocate:()=> '10.77.0.2/32',
    sync:()=>{syncs++;if(failSync)throw Error('offline')},verify:()=>verified,uuid:()=> 'peer',now:()=> '2026-10-02T00:00:00Z'};
  return {deps,get peers(){return persisted},get writes(){return writes},get syncs(){return syncs},set failSync(v){failSync=v},set verified(v){verified=v}};
}
const f=fixture();
assert.equal(enrollCreateOnly(f.deps).status,200);assert.equal(f.peers[0].state,'active');assert.equal(f.writes,2);
assert.equal(enrollCreateOnly(f.deps).status,200);assert.equal(f.writes,2);assert.equal(f.syncs,1);
f.deps.body.routerPublicKey=otherKey;
assert.equal(enrollCreateOnly(f.deps).status,409);assert.equal(f.peers[0].routerPublicKey,key);assert.equal(f.syncs,1);
f.deps.body.routerPublicKey=key;f.deps.body.managementRestPort=8443;
assert.equal(enrollCreateOnly(f.deps).status,409);assert.equal(f.writes,2);
const crash=fixture();crash.failSync=true;
assert.equal(enrollCreateOnly(crash.deps).status,503);assert.equal(crash.peers[0].state,'enrolling');
crash.failSync=false;assert.equal(enrollCreateOnly(crash.deps).status,200);assert.equal(crash.peers.length,1);assert.equal(crash.peers[0].tunnelAddress,'10.77.0.2/32');
const failedRead=fixture();failedRead.deps.load=()=>{throw Error('corrupt')};
assert.equal(enrollCreateOnly(failedRead.deps).status,503);assert.equal(failedRead.syncs,0);assert.equal(failedRead.writes,0);
const foreign=fixture();foreign.deps.load=()=>[{tenantId:'another',routerId:'another',routerPublicKey:key,state:'active'}];
assert.equal(enrollCreateOnly(foreign.deps).status,409);assert.equal(foreign.syncs,0);
const disabled=fixture();disabled.deps.load=()=>[{tenantId:'tenant',routerId:'router',routerPublicKey:key,managementRestPort:443,state:'disabled'}];
assert.equal(enrollCreateOnly(disabled.deps).status,409);assert.equal(disabled.syncs,0);
const denied=fixture();denied.deps.auth.scope='peers:inspect';
assert.equal(enrollCreateOnly(denied.deps).status,401);assert.equal(denied.writes,0);
for(const bad of ['',Buffer.alloc(32).toString('base64'),key+'\n']) {
  const invalid=fixture();invalid.deps.body.routerPublicKey=bad;
  assert.equal(enrollCreateOnly(invalid.deps).status,400);assert.equal(invalid.writes,0);
}
const unverified=fixture();unverified.verified=false;
assert.equal(enrollCreateOnly(unverified.deps).status,503);assert.equal(unverified.peers[0].state,'enrolling');
const restarted=fixture();assert.equal(enrollCreateOnly(restarted.deps).status,200);
restarted.verified=false;restarted.deps.sync=()=>{restarted.verified=true};
assert.equal(enrollCreateOnly(restarted.deps).status,200);assert.equal(restarted.writes,2);assert.equal(restarted.peers.length,1);
console.log('Create-only Hub enrollment passed: retries, crash recovery, key/port conflicts, cross-tenant key reuse, disabled peers, corrupt store, signature scope and read-back failure. No live network or peers used.');
