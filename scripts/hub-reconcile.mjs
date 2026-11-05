// Keeps WireGuard equal to the Hub's peer store, which is the source of truth.
// A crash or power loss skips the save that `wg-quick down` does, so peers added
// since the last save would otherwise be missing after the next start, and those
// routers would be cut off with nothing on their side wrong.
//
// Safe by construction: it only adds what the store marks active and removes what
// the store marks removed or disabled. A live peer the store does not know is
// reported and left alone, and a peer in the middle of a migration is not touched.

const sameAddresses = (a = [], b = []) => [...a].sort().join(",") === [...b].sort().join(",");

/** Reads `wg show <iface> allowed-ips` into public key -> addresses. */
export function parseLive(text) {
  const live = new Map();
  for (const row of String(text).split("\n")) {
    const [key, ...addresses] = row.trim().split(/\s+/);
    if (key) live.set(key, addresses.filter((a) => a !== "(none)"));
  }
  return live;
}

/** Reads the [Peer] blocks of a wg-quick file into public key -> addresses. */
export function parseSaved(text) {
  const saved = new Map();
  for (const block of String(text).split(/^\[Peer\]/m).slice(1)) {
    const key = /^\s*PublicKey\s*=\s*(\S+)/m.exec(block)?.[1];
    if (!key) continue;
    const raw = /^\s*AllowedIPs\s*=\s*(.+)$/m.exec(block)?.[1] ?? "";
    saved.set(key, raw.split(",").map((a) => a.trim()).filter(Boolean));
  }
  return saved;
}

const inMigration = (peer) => peer.managementMigration && !["completed", "rolled_back"].includes(peer.managementMigration.phase);

/** Decides what must change. Changes nothing itself. */
export function planReconcile({ peers, live, addressesOf }) {
  const active = peers.filter((p) => p.state === "active" && p.routerPublicKey);
  const activeKeys = new Set(active.map((p) => p.routerPublicKey));
  const known = new Set(peers.map((p) => p.routerPublicKey));
  const add = [];
  for (const peer of active) {
    if (inMigration(peer)) continue;
    const want = addressesOf(peer);
    if (!want.length) continue;
    if (!live.has(peer.routerPublicKey) || !sameAddresses(live.get(peer.routerPublicKey), want)) add.push({ key: peer.routerPublicKey, addresses: want });
  }
  const remove = peers
    .filter((p) => ["removed", "disabled"].includes(p.state) && p.routerPublicKey && live.has(p.routerPublicKey) && !activeKeys.has(p.routerPublicKey))
    .map((p) => p.routerPublicKey);
  return { add, remove: [...new Set(remove)], unknown: [...live.keys()].filter((k) => !known.has(k)) };
}

const sameMaps = (a, b) => a.size === b.size && [...a].every(([key, addresses]) => b.has(key) && sameAddresses(addresses, b.get(key)));

/** Applies the plan, then saves the file if it no longer matches what is live. */
export function reconcileHub({ load, readLive, readSaved, addressesOf, setPeer, removePeer, saveConfig }) {
  const peers = load();
  const plan = planReconcile({ peers, live: parseLive(readLive()), addressesOf });
  for (const { key, addresses } of plan.add) setPeer(key, addresses);
  for (const key of plan.remove) removePeer(key);
  const saved = !sameMaps(parseLive(readLive()), parseSaved(readSaved()));
  if (saved) saveConfig();
  return { added: plan.add.length, removed: plan.remove.length, unknown: plan.unknown.length, saved };
}
