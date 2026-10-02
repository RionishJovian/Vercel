// Read-only PostgreSQL acceptance check; never inserts guest purchases.
import { readFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import assert from 'node:assert/strict';

const source = readFileSync(new URL('../internal/handler/guest_revenue_handler.go', import.meta.url), 'utf8');
const sql = source.match(/const guestRevenueSQL = `([\s\S]*?)`/)[1];
const ledger = 'FROM guest_purchase_requests WHERE business_id=$1';
assert.ok(sql.includes(ledger));
// 125 approved WiFi sales, 20 pending and 10 rejected; only approved money counts.
const fixture = `FROM (
 SELECT 'daily'::text AS plan_key, 100::numeric AS amount,
 CASE WHEN i<=125 THEN 'approved' WHEN i<=145 THEN 'pending' ELSE 'rejected' END AS status,
 now() AS decided_at, now() AS created_at FROM generate_series(1,155) AS i
) fixture`;
const input = 'BEGIN READ ONLY;\n' + sql.replace(ledger,fixture) + ';\nROLLBACK;\n';
const output = execFileSync('ssh',['mikromagic-app',
 'sudo docker exec -i mega_magic_db psql -U mega_admin -d mega_magic -v ON_ERROR_STOP=1 -At'],
 {input,encoding:'utf8',timeout:30000});
const result = JSON.parse(output.split('\n').find(line => line.startsWith('{')));
assert.equal(result.all_time,12500);
assert.equal(result.approved,125);
assert.equal(result.pending,20);
assert.equal(result.rejected,10);
assert.equal(result.total,155);
assert.equal(result.today,12500);
assert.equal(result.last7,12500);
assert.equal(result.last30,12500);
assert.equal(result.days.length,7);
assert.equal(result.plans[0].revenue,12500);
console.log('Read-only PostgreSQL revenue fixture passed: all 125 approved sales counted; pending/rejected excluded. No records inserted.');
