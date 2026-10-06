import {test} from 'node:test';
import assert from 'node:assert/strict';
import {csvCell,ledgerCsv} from '../src/revenue-csv.mjs';
test('CSV protects untrusted spreadsheet text and preserves numeric refunds',()=>{
 for(const v of ['=1+1','+SUM(A1)','-DDE','@SUM(A1)',' \t=HYPERLINK("x")'])assert.ok(csvCell(v).startsWith('"\''));
 assert.equal(csvCell(-3000),'"-3000"');assert.equal(csvCell('receipt "A", first\nline'),'"receipt ""A"", first\nline"');
});
test('ledger CSV includes event basis and stable source identifiers',()=>{
 const csv=ledgerCsv([{id:'event',local_time:'2026-10-01T00:30:00+06:30',timestamp_basis:'legacy_submission_estimate',kind:'refund',amount:3000,signed_amount:-3000,reference:'=BAD'}]);
 assert.ok(csv.startsWith('\ufeff'));assert.match(csv,/timestamp_basis/);assert.match(csv,/source_id/);assert.match(csv,/legacy_submission_estimate/);assert.match(csv,/"-3000"/);assert.match(csv,/"'=BAD"/);assert.ok(csv.endsWith('\r\n'));
});
