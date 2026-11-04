// Generate a candidate only. Exact baseline identity is mandatory; this script
// never edits the running service, contacts WireGuard or restarts anything.
import fs from "node:fs";
import crypto from "node:crypto";
import assert from "node:assert/strict";

export const BASELINE = "512bfac04bd6ac3833894b43595f399f32d6c5b3448dda188f4d118da27b8b44";

export function stage(source) {
  assert.equal(crypto.createHash("sha256").update(source).digest("hex"), BASELINE, "Hub service is not the audited baseline");
  let s = source;
  const replace = (before, after) => {
    assert.equal(s.split(before).length, 2, "non-unique integration anchor");
    s = s.replace(before, after);
  };
  replace('import { enrollCreateOnly, reserveTunnelAddresses } from "./hub-enrollment.mjs";',
    'import { enrollCreateOnly, reserveTunnelAddresses } from "./hub-enrollment.mjs";\nimport { reconcileHub } from "./hub-reconcile.mjs";');
  const anchor = 'setInterval(()=>{try{recover()}catch(_){console.error("Management migration reconciliation unverified")}},30000).unref();';
  replace(anchor, `${anchor}
  // The store is the source of truth: restore WireGuard from it at start and keep the
  // saved file current, so a crash or power loss cannot cut routers off.
  const reconcile = () => {
    try {
      const result = reconcileHub({
        load: loadPeers,
        readLive: () => execFileSync("wg", ["show", WG_IFACE, "allowed-ips"], { encoding: "utf8" }),
        readSaved: () => fs.readFileSync(cleanEnvValue(env.MM_HUB_WG_CONF || "") || \`/etc/wireguard/\${WG_IFACE}.conf\`, "utf8"),
        addressesOf: migrationAddresses,
        setPeer: (key, addresses) => execFileSync("wg", ["set", WG_IFACE, "peer", key, "allowed-ips", addresses.join(",")], { stdio: "ignore" }),
        removePeer: (key) => execFileSync("wg", ["set", WG_IFACE, "peer", key, "remove"], { stdio: "ignore" }),
        saveConfig: () => execFileSync("wg-quick", ["save", WG_IFACE], { stdio: "ignore" }),
      });
      if (result.added || result.removed || result.saved) console.log(\`Hub peers reconciled: added=\${result.added} removed=\${result.removed} unknown=\${result.unknown} saved=\${result.saved}\`);
    } catch (_) {
      console.error("Hub peer reconciliation failed; will retry");
    }
  };
  reconcile();
  setInterval(reconcile, 30000).unref();`);
  return s;
}

if (process.argv[1]?.endsWith("stage-hub-reconcile.mjs")) {
  const [input, output] = process.argv.slice(2);
  assert(input && output, "input/output paths required");
  fs.writeFileSync(output, stage(fs.readFileSync(input, "utf8")));
}
