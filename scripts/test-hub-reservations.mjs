import assert from 'node:assert/strict';
import {reserveTunnelAddresses} from './hub-enrollment.mjs';
const result=reserveTunnelAddresses([], 'key\t10.77.0.9/32\nkey2\t10.99.0.0/24','10.77.0.0/24','10.77.0.10/32');
assert.deepEqual(result.map(p=>p.tunnelAddress),['10.77.0.9/32','10.77.0.10/32']);
assert.equal(reserveTunnelAddresses([],'key\t10.77.0.0/24','10.77.0.0/24').length,252);
for(const value of ['bad','key\tbad','key\t10.77.0.9/33'])assert.throws(()=>reserveTunnelAddresses([],value,'10.77.0.0/24'));
console.log('Live tunnel and app address reservations passed.');
