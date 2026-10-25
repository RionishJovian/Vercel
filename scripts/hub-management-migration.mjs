// Signed Hub control-plane extension. Dependencies are synchronous and the Hub
// must remain a single writer. No public keys, interfaces or shell args come
// from the browser. Candidate addresses are chosen/reserved by the allocator.
const operation=/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const terminal=m=>!m||['completed','rolled_back'].includes(m.phase);
const validAddress=s=>{const p=/^(\d+)\.(\d+)\.(\d+)\.(\d+)\/32$/.exec(s||'');if(!p||p.slice(1).some(n=>Number(n)>255))return false;return Number(p[1])===10||(Number(p[1])===172&&Number(p[2])>=16&&Number(p[2])<=31)||(Number(p[1])===192&&Number(p[2])===168)};

export function migrationAddresses(peer){
 const m=peer.managementMigration;

 if(!validAddress(peer.tunnelAddress))throw Error('invalid peer management allocation');
 if(m&&(!operation.test(m.id)||!['staged','committed','completed','rolled_back'].includes(m.phase)||!validAddress(m.oldAddress)||!validAddress(m.newAddress)||m.oldAddress===m.newAddress||(!terminal(m)&&!Number.isFinite(Date.parse(m.expiresAt)))))throw Error('invalid migration journal');
 if(m&&peer.tunnelAddress!==(['committed','completed'].includes(m.phase)?m.newAddress:m.oldAddress))throw Error('migration canonical allocation mismatch');
 return !terminal(m)?[m.oldAddress,m.newAddress]:[peer.tunnelAddress];
}
export function migrationReservations(peers){
 return peers.flatMap(p=>p.state==='removed'?[]:migrationAddresses(p).map(tunnelAddress=>({state:'active',tunnelAddress})));
}
function response(peer){const m=peer.managementMigration;return {peerId:peer.peerId,tenantId:peer.tenantId,routerId:peer.routerId,routerPublicKey:peer.routerPublicKey,operationId:m?.id||'',oldAddress:m?.oldAddress||peer.tunnelAddress,newAddress:m?.newAddress||'',phase:m?.phase||'none',expiresAt:m?.expiresAt||''};}
export function managementMigration({auth,routerId,action,body={},load,save,allocate,sync,verify,now}){
 const fail=(status,error)=>({status,body:{error}});
 if(auth.scope!=='peers:migrate'||routerId!==auth.routerId)return fail(401,'unauthorized');
 try{
  const peers=load(),matches=peers.filter(p=>p.state!=='removed'&&p.tenantId===auth.tenantId&&p.routerId===routerId);
  if(matches.length!==1)return fail(matches.length?409:404,'peer_conflict');const p=matches[0];
  if(p.state!=='active')return fail(409,'peer_not_active');if(peers.filter(other=>other.state!=='removed'&&other.routerPublicKey===p.routerPublicKey).length!==1)return fail(409,'peer_key_conflict');let m=p.managementMigration;
  if(action==='inspect')return {status:200,body:response(p)};
  if(action==='candidate'){
   if(!terminal(m))return fail(409,'migration_pending');if(!verify(p))return fail(503,'existing_route_unverified');
   return {status:200,body:{...response(p),phase:'candidate',oldAddress:p.tunnelAddress,newAddress:allocate([...peers,...migrationReservations(peers)])}};
  }
  if(!operation.test(body.operationId||'')||body.routerPublicKey!==p.routerPublicKey)return fail(400,'invalid_operation');
  if(action==='stage'){
   if(m?.id===body.operationId){if(m.oldAddress!==body.oldAddress||m.newAddress!==body.newAddress)return fail(409,'migration_conflict');if(m.phase==='rolled_back'||m.phase==='completed')return fail(409,'operation_terminal');sync(p);if(!verify(p))return fail(503,'verification_failed');return {status:200,body:response(p)}}
   if(!terminal(m)||p.tunnelAddress!==body.oldAddress)return fail(409,'migration_conflict');if(!verify(p))return fail(503,'existing_route_unverified');
   const candidate=allocate([...peers,...migrationReservations(peers)]);
   if(!validAddress(candidate)||candidate===body.oldAddress)return fail(503,'invalid_allocation');if(candidate!==body.newAddress)return fail(409,'allocation_changed');
   m={id:body.operationId,oldAddress:p.tunnelAddress,newAddress:candidate,phase:'staged',expiresAt:new Date(now()+5*60000).toISOString(),requestedBy:auth.requestedBy};
   p.managementMigration=m;m.updatedAt=new Date(now()).toISOString();p.updatedAt=m.updatedAt;save(peers); // intent durable BEFORE wg changes
   sync(p);if(!verify(p))return fail(503,'verification_failed');return {status:200,body:response(p)};
  }
  if(action==='abort'&&terminal(m)&&m?.id!==body.operationId&&body.oldAddress===p.tunnelAddress){
   if(!validAddress(body.newAddress)||body.newAddress===body.oldAddress)return fail(400,'invalid_address');
   p.managementMigration={id:body.operationId,oldAddress:body.oldAddress,newAddress:body.newAddress,phase:'rolled_back',expiresAt:''};save(peers);sync(p);if(!verify(p))return fail(503,'verification_failed');return {status:200,body:response(p)};
  }
  if(!m||m.id!==body.operationId||m.oldAddress!==body.oldAddress||m.newAddress!==body.newAddress)return fail(409,'migration_conflict');
  if(action==='commit'){
   if(m.phase==='committed'){sync(p);if(!verify(p))return fail(503,'verification_failed');return {status:200,body:response(p)}}
   if(m.phase!=='staged'||Date.parse(m.expiresAt)<=now())return fail(409,'migration_expired');
   // Both addresses stay reserved/routed indefinitely across restarts until
   // the app journal confirms its canonical target and finishes cleanup.
   m.phase='committed';p.tunnelAddress=m.newAddress;m.updatedAt=new Date(now()).toISOString();m.updatedBy=auth.requestedBy;p.updatedAt=m.updatedAt;save(peers);sync(p);
  }else if(action==='finalize'){
   if(!['committed','completed'].includes(m.phase))return fail(409,'migration_not_committed');
   m.phase='completed';p.tunnelAddress=m.newAddress;m.updatedAt=new Date(now()).toISOString();m.updatedBy=auth.requestedBy;p.updatedAt=m.updatedAt;save(peers);sync(p);
  }else if(action==='abort'){
   if(m.phase==='completed')return fail(409,'migration_finalized');
   p.tunnelAddress=m.oldAddress;m.phase='rolled_back';m.updatedAt=new Date(now()).toISOString();m.updatedBy=auth.requestedBy;p.updatedAt=m.updatedAt;save(peers);sync(p);
  }else return fail(400,'unknown_action');
  if(!verify(p))return fail(503,'verification_failed');return {status:200,body:response(p)};
 }catch(_){return fail(503,'migration_unavailable')}
}
export function recoverExpiredMigrations({load,save,sync,verify,now}){
 const peers=load();
 for(const p of peers){const m=p.managementMigration;if(!m||p.state!=='active')continue;
  if(m.phase==='staged'&&Date.parse(m.expiresAt)<=now()){p.tunnelAddress=m.oldAddress;m.phase='rolled_back';save(peers)}
  // Reconcile committed and terminal records too: a previous sync or its
  // acknowledgment may have failed after the durable transition.
  sync(p);if(!verify(p))throw Error('migration recovery unverified');
 }
}
