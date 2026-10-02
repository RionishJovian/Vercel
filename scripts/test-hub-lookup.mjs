// Run: node --experimental-vm-modules scripts/test-hub-lookup.mjs SOURCE
// SOURCE is a read-only copy of service.mjs, never a secret/environment file.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import { createHmac, randomUUID } from 'node:crypto';

const sourcePath = process.argv[2];
assert(sourcePath, 'provide an inspected Hub source snapshot');
const original = fs.readFileSync(sourcePath, 'utf8');
const patch = fs.readFileSync(new URL('./hub-read-only-lookup.patch', import.meta.url), 'utf8');
const addition = patch.split('\n').filter(line => line.startsWith('+') && !line.startsWith('+++')).map(line => line.slice(1)).join('\n');
const anchor = '  if (req.method === "PUT" && byRouter) {';
assert.equal(original.split(anchor).length, 2, 'source drift: unique route anchor required');
assert(!original.includes('req.method === "GET" && byRouter'), 'lookup already present; inspect before applying');
const source = original.replace(anchor, addition + '\n' + anchor) + '\nexport { handle };\n';
const key = 'ab'.repeat(32); // isolated fixture key, not production
let writes = 0, network = 0, commands = 0;
const peers = [
  {peerId:'peer-a',tenantId:'tenant-a',routerId:'router-a',routerPublicKey:'fixture-public-key',tunnelAddress:'10.77.0.2/32',state:'active'},
  {peerId:'peer-b',tenantId:'tenant-b',routerId:'router-b',state:'active'},
  {peerId:'peer-c',tenantId:'tenant-a',routerId:'removed-router',state:'removed'},
];
const before = JSON.stringify(peers);
const forbiddenWrite = () => { writes++; throw new Error('forbidden write'); };
const forbiddenNetwork = () => { network++; throw new Error('forbidden network'); };
const context = vm.createContext({ Buffer, URL, console:{log(){},warn(){},error(){}}, process:{argv:[],env:{
  MM_HUB_SIGNING_KEY:key, MM_HUB_SIGNING_KEY_ID:'fixture', MM_HUB_PEERS_PATH:'/fixture/peers.json', MM_HUB_PUBLIC_KEY:'fixture-hub-key',
}} });
const module = new vm.SourceTextModule(source, {context, identifier:'file:///fixture/service.mjs'});
await module.link(async specifier => {
  let values;
  if (specifier === 'node:fs') values = {default:{readFileSync(path){assert.equal(path,'/fixture/peers.json');return before;},writeFileSync:forbiddenWrite,mkdirSync:forbiddenWrite}};
  else if (specifier === 'node:child_process') values = {execFileSync(command,args){ commands++; assert.equal(command,'wg');assert.deepEqual(Array.from(args),['show','wg0','dump']);return 'interface\n'; }};
  else if (specifier === 'node:http' || specifier === 'node:https') values = {default:{createServer:forbiddenNetwork,request:forbiddenNetwork}};
  else values = await import(specifier);
  return new vm.SyntheticModule(Object.keys(values),function(){for(const [name,value] of Object.entries(values))this.setExport(name,value);},{context});
});
await module.evaluate();

async function lookup({tenant='tenant-a',router='router-a',urlRouter=router,scope='peers:inspect',prefix='',tamper=false,unsigned=false}) {
  const path = '/v1/peers/by-router/' + urlRouter;
  const timestamp = new Date().toISOString(), requestId = randomUUID(), requestedBy='owner-a';
  const canonical = ['GET',path,timestamp,requestId,scope,tenant,router,requestedBy,''].join('\n');
  const headers = unsigned ? {} : {'x-mm-key-id':'fixture','x-mm-timestamp':timestamp,'x-mm-request-id':requestId,'x-mm-scope':scope,'x-mm-tenant-id':tenant,'x-mm-router-id':router,'x-mm-requested-by':requestedBy,'x-mm-signature':tamper?'invalid':createHmac('sha256',Buffer.from(key,'hex')).update(canonical).digest('base64url')};
  let status, body;
  await module.namespace.handle({method:'GET',url:prefix+path,headers},{writeHead(code){status=code;},end(raw){body=JSON.parse(raw);}});
  return {status,body};
}
for (const prefix of ['', '/internal/provisioner']) {
  const result = await lookup({prefix});assert.equal(result.status,200);assert.equal(result.body.peerId,'peer-a');assert.equal(result.body.tunnelAddress,'10.77.0.2/32');
}
for(const options of [{tenant:'tenant-b'},{router:'missing-router'},{router:'removed-router'}]) assert.equal((await lookup(options)).status,404);
for(const options of [{urlRouter:'router-b'},{scope:'peers:create'},{tamper:true},{unsigned:true}]) assert.equal((await lookup(options)).status,401);
assert.equal(writes,0);assert.equal(network,0);assert.equal(commands,2);assert.equal(JSON.stringify(peers),before);
console.log('Hub lookup contract passed: 9 cases; no file writes, peer changes, router commands or network connections.');
