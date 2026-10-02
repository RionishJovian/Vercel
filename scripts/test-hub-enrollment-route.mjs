// Feed the inspected service source on stdin. No source/env/peer files are written.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import {createHmac,randomUUID} from 'node:crypto';
import {enrollCreateOnly,reserveTunnelAddresses} from './hub-enrollment.mjs';
let source=fs.readFileSync(0,'utf8');
const patch=fs.readFileSync(new URL('./hub-create-only-enrollment.patch',import.meta.url),'utf8');
for(const hunk of patch.split(/^@@.*$/m).slice(1)) {
  const lines=hunk.trimEnd().split('\n').filter(Boolean);
  const before=lines.filter(l=>l[0]===' '||l[0]==='-').map(l=>l.slice(1)).join('\n');
  const after=lines.filter(l=>l[0]===' '||l[0]==='+').map(l=>l.slice(1)).join('\n');
  assert.equal(source.split(before).length,2,'source drift: nonunique patch anchor');
  source=source.replace(before,after);
}
source+='\nexport {handle};\n';
const signing='ab'.repeat(32),key=Buffer.alloc(32,1).toString('base64');
let store='{"peers":[]}',pending='',sets=0,renames=0;
const context=vm.createContext({Buffer,URL,console:{warn(){},log(){},error(){}},process:{argv:[],env:{MM_HUB_SIGNING_KEY:signing,MM_HUB_SIGNING_KEY_ID:'fixture',MM_HUB_PEERS_PATH:'/fixture/peers.json',MM_HUB_PUBLIC_KEY:key}}});
const module=new vm.SourceTextModule(source,{context,identifier:'file:///fixture/service.mjs'});
await module.link(async specifier=>{
  let values;
  if(specifier==='./hub-enrollment.mjs')values={enrollCreateOnly,reserveTunnelAddresses};
  else if(specifier==='node:fs')values={default:{
    readFileSync(p){assert.equal(p,'/fixture/peers.json');return store},openSync(){return 1},closeSync(){},fsyncSync(){},
    writeFileSync(fd,value){assert.equal(fd,1);pending=value},renameSync(){store=pending;renames++},existsSync(){return false},unlinkSync(){throw Error('unexpected unlink')},mkdirSync(){throw Error('legacy write')}
  }};
  else if(specifier==='node:child_process')values={execFileSync(command,args){assert.equal(command,'wg');if(args[0]==='set'){sets++;return ''}if(args[2]==='allowed-ips')return `${Buffer.alloc(32,9).toString('base64')}\t10.77.0.9/32\n`;assert.deepEqual(Array.from(args),['show','wg0','dump']);return `interface\n${key}\t(none)\t(none)\t10.77.0.2/32\t0\t0\t0\t0\n`}};
  else if(specifier==='node:http'||specifier==='node:https')values={default:{createServer(){throw Error('live listener forbidden')},request(){throw Error('live network forbidden')}}};
  else values=await import(specifier);
  return new vm.SyntheticModule(Object.keys(values),function(){for(const[k,v]of Object.entries(values))this.setExport(k,v)},{context});
});
await module.evaluate();
async function enroll({tenant='tenant',scope='peers:create',publicKey=key}={}) {
  const body=JSON.stringify({tenantId:tenant,routerId:'router',routerPublicKey:publicKey,managementRestPort:443});
  const path='/v1/peers/by-router/router',timestamp=new Date().toISOString(),id=randomUUID();
  const canonical=['POST',path,timestamp,id,scope,tenant,'router','owner',body].join('\n');
  const headers={'x-mm-key-id':'fixture','x-mm-timestamp':timestamp,'x-mm-request-id':id,'x-mm-scope':scope,'x-mm-tenant-id':tenant,'x-mm-router-id':'router','x-mm-requested-by':'owner','x-mm-signature':createHmac('sha256',Buffer.from(signing,'hex')).update(canonical).digest('base64url')};
  let status,result;
  const req={method:'POST',url:path,headers,on(event,cb){if(event==='data')cb(Buffer.from(body));if(event==='end')cb();return this}};
  await module.namespace.handle(req,{writeHead(code){status=code},end(raw){result=JSON.parse(raw)}});
  return {status,result};
}
assert.equal((await enroll()).status,200);assert.equal(sets,1);assert.equal(renames,2);
assert.equal((await enroll()).result.enrollmentVersion,1);assert.equal(sets,1);assert.equal(renames,2);
assert.equal((await enroll({publicKey:Buffer.alloc(32,2).toString('base64')})).status,409);
assert.equal((await enroll({tenant:'foreign'})).status,409);
assert.equal((await enroll({scope:'peers:inspect'})).status,401);
assert.equal(sets,1);assert.equal(renames,2);assert.equal(JSON.parse(store).peers.length,1);
console.log('Signed Hub route integration passed with mocked storage and WireGuard; no live writes or network.');
