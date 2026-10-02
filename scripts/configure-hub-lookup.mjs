// Privileged, host-to-host credential handoff. Run emit on the Hub ONLY through
// a pipe to install on the app host. Never redirect emit into a transcript/log.
import fs from 'node:fs';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
const mode=process.argv[2];
function settings(raw) {
  const result={};
  for(const line of raw.split('\n')) {
    const match=/^([A-Z_]+)=(.*)$/.exec(line.trim());
    if(match)result[match[1]]=match[2].trim().replace(/^(["'])(.*)\1$/, '$2');
  }
  return result;
}
if(mode==='check') {
  // Fixed read-only binary; no privileged shell or caller-selected executable.
  const input=settings(fs.readFileSync('/etc/mega-magic-hub-lookup.env','utf8'));
  const result=spawnSync('/home/ubuntu/deploy-backups/hub-lookup-20261001.23o6EOT2/hub-check',['--config','config.yaml'],{cwd:'/home/ubuntu/mega-magic-project',env:{...process.env,...input},stdio:'inherit'});
  if(result.error)throw new Error('unable to start Hub diagnostic');
  process.exit(result.status??1);
} else if(mode==='target') {
  // Non-secret diagnostic identity, piped directly to hub-check on the app.
  const input=settings(fs.readFileSync('/etc/mikromagic-hub/env','utf8'));
  const store=JSON.parse(fs.readFileSync(input.MM_HUB_PEERS_PATH||'/var/lib/mikromagic-hub/peers.json','utf8'));
  const peer=(Array.isArray(store)?store:store.peers).find(p=>p.state!=='removed');
  assert(peer,'no existing peer for diagnostic');
  process.stdout.write(JSON.stringify({tenantId:peer.tenantId,routerId:peer.routerId,peerId:peer.peerId}));
} else if(mode==='emit') {
  assert(!process.stdout.isTTY,'credential output requires a protected pipe');
  const input=settings(fs.readFileSync('/etc/mikromagic-hub/env','utf8'));
  assert(/^[a-fA-F0-9]{64}$/.test(input.MM_HUB_SIGNING_KEY||''),'canonical credential unavailable');
  assert(/^[A-Za-z0-9_-]{1,128}$/.test(input.MM_HUB_SIGNING_KEY_ID||'v1'),'invalid key identifier');
  process.stdout.write(JSON.stringify({key:input.MM_HUB_SIGNING_KEY,keyId:input.MM_HUB_SIGNING_KEY_ID||'v1'}));
} else if(mode==='install') {
  let raw='';
  for await(const chunk of process.stdin) {raw+=chunk;if(raw.length>4096)throw new Error('oversized credential transfer');}
  let input;
  try { input=JSON.parse(raw); } catch { throw new Error('invalid credential transfer encoding'); }
  assert(/^[a-fA-F0-9]{64}$/.test(input.key||''),'invalid credential transfer');
  assert(/^[A-Za-z0-9_-]{1,128}$/.test(input.keyId||''),'invalid key identifier');
  const envPath='/etc/mega-magic-hub-lookup.env';
  const dropIn='/etc/systemd/system/mega-magic-api.service.d/97-magic-hub-lookup.conf';
  assert(!fs.existsSync(envPath)&&!fs.existsSync(dropIn),'existing configuration must be reviewed, not overwritten');
  fs.writeFileSync(envPath,`VPS_HUB_LOOKUP_ENABLED=true\nVPS_HUB_URL=https://hub.mikromagic.com/internal/provisioner\nVPS_HUB_KEY_ID=${input.keyId}\nVPS_HUB_SIGNING_KEY=${input.key}\n`,{mode:0o600,flag:'wx'});
  fs.writeFileSync(dropIn,`[Service]\nEnvironmentFile=${envPath}\n`,{mode:0o644,flag:'wx'});
  console.log('Protected Hub lookup configuration installed; credentials not displayed.');
} else throw new Error('use emit on the Hub through a pipe to install on the app host');
