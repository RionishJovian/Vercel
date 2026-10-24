// Generate a candidate only. Exact baseline identity is mandatory; this script
// never edits the running service, contacts WireGuard or restarts anything.
import fs from 'node:fs';import crypto from 'node:crypto';import assert from 'node:assert/strict';
const [input,output]=process.argv.slice(2);assert(input&&output,'input/output paths required');
let s=fs.readFileSync(input,'utf8');assert.equal(crypto.createHash('sha256').update(s).digest('hex'),'760beadcd1cba2ceb95741aa8b60ec5ed82ee8adb9059332121a53dc06adb246','Hub source drift; review a new baseline');
function replace(before,after){assert.equal(s.split(before).length,2,'non-unique integration anchor');s=s.replace(before,after)}
replace('import { enrollCreateOnly, reserveTunnelAddresses } from "./hub-enrollment.mjs";', 'import { enrollCreateOnly, reserveTunnelAddresses } from "./hub-enrollment.mjs";\nimport { managementMigration, migrationAddresses, migrationReservations, recoverExpiredMigrations } from "./hub-management-migration.mjs";');
replace('.map((p) => String(p.tunnelAddress || p.address || "").split("/")[0]),','.flatMap(p => migrationAddresses(p).map(raw => String(raw).split("/")[0])),');
replace('const ip = String(peer.tunnelAddress || "").split("/")[0];','const ips = migrationAddresses(peer).join(",");');
replace('"allowed-ips", `${ip}/32`','"allowed-ips", ips');
replace('match[0][3] === p.tunnelAddress','match[0][3].split(",").sort().join(",") === migrationAddresses(p).sort().join(",")');
replace('  const byRouter = /^\\/v1\\/peers\\/by-router\\/([^/]+)$/.exec(pathName);','  const migrationRoute = /^\\/v1\\/peers\\/by-router\\/([^/]+)\\/management-migration\\/(candidate|inspect|stage|commit|finalize|abort)$/.exec(pathName);\n  const byRouter = /^\\/v1\\/peers\\/by-router\\/([^/]+)$/.exec(pathName);');
replace('if (!(byRouter || peerDisable || peerOne))','if (!(byRouter || peerDisable || peerOne || migrationRoute))');
const marker='  if (req.method === "GET" && byRouter) {';
replace(marker,`  if (migrationRoute) {
    if (cleanEnvValue(env.MM_HUB_MANAGEMENT_MIGRATION_ENABLED || "") !== "true") return json(res,503,{error:"migration_disabled"});
    const action=migrationRoute[2];
    if (req.method !== (["candidate","inspect"].includes(action)?"GET":"POST")) return json(res,405,{error:"method_not_allowed"});
    const result=managementMigration({auth,routerId:decodeURIComponent(migrationRoute[1]),action,body,load:()=>peers,save:savePeers,allocate:migrationAllocate,sync:migrationSafeSync,verify:migrationVerify,now:()=>Date.now()});
    return json(res,result.status,result.body);
  }
  if(req.method!=="GET") {
    const matching=peers.filter(p=>p.state!=="removed"&&p.tenantId===auth.tenantId&&p.routerId===auth.routerId);
    if(matching.some(p=>p.managementMigration&&!['completed','rolled_back'].includes(p.managementMigration.phase))) return json(res,409,{error:"management_migration_pending"});
  }
`+marker);
const helpers=`
function migrationAllocate(peers) {
 const live=execFileSync("wg",["show",WG_IFACE,"allowed-ips"],{encoding:"utf8"});
 return allocateAddress(reserveTunnelAddresses([...peers,...migrationReservations(peers)],live,SUBNET,cleanEnvValue(env.MM_HUB_APP_ROUTE||"")));
}
function migrationSafeSync(peer) {
 const live=execFileSync("wg",["show",WG_IFACE,"allowed-ips"],{encoding:"utf8"}).trim().split("\\n").filter(Boolean);
 const others=live.filter(row=>row.trim().split(/\\s+/)[0]!==peer.routerPublicKey).join("\\n");
 if(migrationAddresses(peer).some(address=>reserveTunnelAddresses([],others,SUBNET,cleanEnvValue(env.MM_HUB_APP_ROUTE||"")).some(r=>r.tunnelAddress===address)))throw Error("migration address conflicts with another peer");
 wgSyncPeer(peer);
}
function migrationVerify(peer) {
 const live=execFileSync("wg",["show",WG_IFACE,"allowed-ips"],{encoding:"utf8"}).trim().split("\\n").map(row=>row.trim().split(/\\s+/));
 const own=live.filter(row=>row[0]===peer.routerPublicKey);
 if(own.length!==1||own[0].slice(1).join("").split(",").sort().join(",")!==migrationAddresses(peer).sort().join(","))return false;
 const others=live.filter(row=>row[0]!==peer.routerPublicKey).map(row=>row.join(" ")).join("\\n");
 return !migrationAddresses(peer).some(address=>reserveTunnelAddresses([],others,SUBNET,cleanEnvValue(env.MM_HUB_APP_ROUTE||"")).some(r=>r.tunnelAddress===address));
}
`;
replace('const [host, portStr] =',helpers+'\nconst [host, portStr] =');
replace('  const server = http.createServer((req, res) => {',`  const recover=()=>recoverExpiredMigrations({load:loadPeers,save:savePeers,sync:migrationSafeSync,verify:migrationVerify,now:()=>Date.now()});
  recover(); // reconciliation failure prevents accepting new writes
  setInterval(()=>{try{recover()}catch(_){console.error("Management migration reconciliation unverified")}},30000).unref();
  const server = http.createServer((req, res) => {`);
fs.writeFileSync(output,s,{flag:'wx'});console.log('Candidate staged; existing service and peers unchanged.');
