// Synchronous create-only control-plane operation. The managed Hub must have a
// single peer-store writer. All dependencies are injected for isolated tests.
export function enrollCreateOnly({auth, routerId, body, load, save, allocate, sync, verify, uuid, now}) {
  const fail = (status, error) => ({status, body:{error}});
  const identifier = /^[A-Za-z0-9_-]{1,128}$/;
  if (auth.scope !== 'peers:create' || routerId !== auth.routerId ||
      ![auth.tenantId,auth.routerId,auth.requestedBy].every(v => typeof v === 'string' && identifier.test(v)))
    return fail(401,'unauthorized');
  if (!body || typeof body !== 'object' || Array.isArray(body)) return fail(400,'invalid_body');
  const key = body.routerPublicKey;
  const port = body.managementRestPort;
  if (typeof key !== 'string' || !/^[A-Za-z0-9+/]{43}=$/.test(key) ||
      Buffer.from(key,'base64').length !== 32 || Buffer.from(key,'base64').toString('base64') !== key ||
      Buffer.from(key,'base64').every(b => b === 0) || !Number.isInteger(port) || port < 1 || port > 65535 ||
      body.tenantId !== auth.tenantId || body.routerId !== auth.routerId)
    return fail(400,'invalid_body');
  try {
    // The load dependency must throw on missing/corrupt/unreadable storage.
    // Silently treating a failed read as an empty store can overwrite peers.
    const peers = load();
    if (!Array.isArray(peers)) return fail(503,'peer_store_unavailable');
    const matches = peers.filter(p => p.tenantId === auth.tenantId && p.routerId === routerId && p.state !== 'removed');
    if (matches.length > 1) return fail(409,'peer_conflict');
    let peer = matches[0];
    if (peers.some(p => p.state !== 'removed' && p.routerPublicKey === key && p !== peer))
      return fail(409,'peer_conflict'); // WireGuard public keys are global identities.
    if (peer) {
      if (peer.routerPublicKey !== key || peer.managementRestPort !== port ||
          !['active','enrolling'].includes(peer.state)) return fail(409,'peer_conflict');
      if (peer.state === 'active') {
        if (!verify(peer)) return fail(503,'peer_verification_failed');
        return {status:200,peer}; // No writes, key rotation, reactivation or allocation.
      }
    } else {
      peer = {peerId:uuid(),tenantId:auth.tenantId,routerId,routerPublicKey:key,
        routerName:routerId,managementRestPort:port,tunnelAddress:allocate(peers),state:'enrolling',
        createdBy:auth.requestedBy,createdAt:now(),updatedAt:now(),lastHandshakeAt:null};
      peers.push(peer);
      save(peers); // Atomic, fsynced intent BEFORE any WireGuard mutation.
    }
    sync(peer);
    if (!verify(peer)) return fail(503,'peer_verification_failed');
    peer.state='active';peer.updatedAt=now();save(peers);
    return {status:200,peer};
  } catch {
    // A durable enrolling record preserves key/address for retry after a crash
    // or an uncertain sync. Never claim connection success or allocate again.
    return fail(503,'enrollment_unavailable');
  }
}
