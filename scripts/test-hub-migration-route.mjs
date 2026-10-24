// Runs the staged real service route with in-memory fs/WireGuard. Never binds
// a listener or opens a network connection. Supply the mechanically staged file.
import fs from 'node:fs';import vm from 'node:vm';import assert from 'node:assert/strict';import {createHmac,randomUUID} from 'node:crypto';
import {enrollCreateOnly,reserveTunnelAddresses} from './hub-enrollment.mjs';import * as migration from './hub-management-migration.mjs';
const source=fs.readFileSync(process.argv[2],'utf8')+'\nexport {handle};';
const key=Buffer.alloc(32,1).toString('base64'),appKey=Buffer.alloc(32,2).toString('base64'),signing='ab'.repeat(32);
let store=JSON.stringify({peers:[{peerId:'peer',tenantId:'tenant',routerId:'router',routerPublicKey:key,tunnelAddress:'10.77.0.2/32',managementRestPort:443,state:'active'}]}),pending='',sets=0;
const live=new Map([[key,'10.77.0.2/32'],[appKey,'10.77.0.9/32']]);
const context=vm.createContext({Buffer,URL,console:{warn(){},error(){},log(){}},process:{argv:[],env:{MM_HUB_SIGNING_KEY:signing,MM_HUB_SIGNING_KEY_ID:'fixture',MM_HUB_PUBLIC_KEY:key,MM_HUB_MANAGEMENT_MIGRATION_ENABLED:'true',MM_HUB_PEERS_PATH:'/fixture/peers.json'}}});
const module=new vm.SourceTextModule(source,{context,identifier:'file:///fixture/service.mjs'});
await module.link(async spec=>{let values;
 if(spec==='./hub-enrollment.mjs')values={enrollCreateOnly,reserveTunnelAddresses};else if(spec==='./hub-management-migration.mjs')values=migration;
 else if(spec==='node:fs')values={default:{readFileSync:p=>{assert.equal(p,'/fixture/peers.json');return store},openSync:()=>1,closeSync(){},fsyncSync(){},writeFileSync:(fd,value)=>{assert.equal(fd,1);pending=value},renameSync:()=>store=pending,existsSync:()=>false}};
 else if(spec==='node:child_process')values={execFileSync(command,args){assert.equal(command,'wg');if(args[0]==='set'){assert.equal(args[4],'allowed-ips');sets++;live.set(args[3],args[5]);return ''}if(args[2]==='allowed-ips')return [...live].map(([k,v])=>`${k}\t${v}`).join('\n')+'\n';if(args[2]==='dump')return 'iface\n'+[...live].map(([k,v])=>`${k}\t(none)\t(none)\t${v}\t1700000000\t1\t1\t0`).join('\n');throw Error('unexpected wg command')}};
 else if(spec==='node:http')values={default:{createServer(){throw Error('listener forbidden')}}};else values=await import(spec);
 return new vm.SyntheticModule(Object.keys(values),function(){for(const[k,v]of Object.entries(values))this.setExport(k,v)},{context});
});await module.evaluate();
async function request(action,{tenant='tenant',scope='peers:migrate',body,method,prefix='',unsigned=false,path}={}){
 const route=path||`/v1/peers/by-router/router/management-migration/${action}`;method||=['candidate','inspect'].includes(action)?'GET':'POST';const payload=body?JSON.stringify(body):'';const stamp=new Date().toISOString(),id=randomUUID();
 const canonical=[method,route,stamp,id,scope,tenant,'router','owner',payload].join('\n');
 const headers=unsigned?{}:{'x-mm-key-id':'fixture','x-mm-timestamp':stamp,'x-mm-request-id':id,'x-mm-scope':scope,'x-mm-tenant-id':tenant,'x-mm-router-id':'router','x-mm-requested-by':'owner','x-mm-signature':createHmac('sha256',Buffer.from(signing,'hex')).update(canonical).digest('base64url')};
 let status,result;await module.namespace.handle({method,url:prefix+route,headers,on(event,cb){if(event==='data')cb(Buffer.from(payload));if(event==='end')cb();return this}},{writeHead:code=>status=code,end:raw=>result=JSON.parse(raw)});return {status,result};
}
const lookup=await request(null,{path:'/v1/peers/by-router/router',method:'GET',scope:'peers:inspect'});assert.equal(lookup.status,200);assert.equal(lookup.result.enrollmentVersion,1,'readonly peer metadata must preserve the enrollment protocol version');
const before=store;assert.equal((await request('candidate',{unsigned:true})).status,401);assert.equal((await request('candidate',{scope:'peers:create'})).status,401);assert.equal((await request('candidate',{tenant:'foreign'})).status,404);assert.equal(sets,0);assert.equal(store,before);
const candidate=await request('candidate',{prefix:'/internal/provisioner'});assert.equal(candidate.status,200);assert.equal(candidate.result.newAddress,'10.77.0.3/32');assert.equal(store,before);assert.equal(sets,0);
const body={operationId:'11111111-1111-4111-8111-111111111111',oldAddress:'10.77.0.2/32',newAddress:'10.77.0.3/32',routerPublicKey:key};
assert.equal((await request('stage',{body})).status,200);assert.equal(live.get(key),'10.77.0.2/32,10.77.0.3/32');
assert.equal((await request(null,{path:'/v1/peers/by-router/router',method:'POST',scope:'peers:create',body:{tenantId:'tenant',routerId:'router',routerPublicKey:key,managementRestPort:443}})).status,409);
assert.equal((await request('commit',{body})).result.phase,'committed');assert.equal(live.get(key),'10.77.0.2/32,10.77.0.3/32');
assert.equal((await request('finalize',{body})).result.phase,'completed');assert.equal(live.get(key),'10.77.0.3/32');assert.equal(live.get(appKey),'10.77.0.9/32');
console.log('Signed staged Hub routes passed: HMAC, prefix compatibility, tenant/scope denial, allocation, dual routing, conflicting legacy writes blocked and finalization; no live writes.');
