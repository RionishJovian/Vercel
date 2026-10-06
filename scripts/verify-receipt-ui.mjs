import {build} from 'esbuild';
import {mkdir,writeFile,readFile,rm} from 'node:fs/promises';
import assert from 'node:assert/strict';
await mkdir('.ui-check',{recursive:true});
try{
 const bundle=await build({entryPoints:['src/ReceiptEvidence.jsx'],bundle:true,platform:'node',format:'esm',external:['react'],write:false});await writeFile('.ui-check/receipts.mjs',bundle.outputFiles[0].text);
 const {default:ReceiptEvidence}=await import('../.ui-check/receipts.mjs');const React=await import('react');const {renderToStaticMarkup}=await import('react-dom/server');
 const order={id:'sample-order',status:'pending',receipt_revision:0};const guest=renderToStaticMarkup(React.createElement(ReceiptEvidence,{order,upload:true,disabled:true,api:{}}));assert.match(guest,/type="file"/);assert.match(guest,/Attach receipt/);assert.match(guest,/original file is not retained/);assert.match(guest,/disabled=""/);
 const owner=renderToStaticMarkup(React.createElement(ReceiptEvidence,{order,api:{}}));assert.doesNotMatch(owner,/type="file"/);
 const closed=renderToStaticMarkup(React.createElement(ReceiptEvidence,{order:{...order,status:'approved'},upload:true,api:{}}));assert.doesNotMatch(closed,/type="file"/);
 const workspace=await readFile('src/AccountWorkspace.jsx','utf8');assert.match(workspace,/expected_receipt_revision:o.receipt_revision/);assert.match(workspace,/<ReceiptEvidence key=\{o.id\}/);
 const css=await readFile('src/styles.css','utf8');await writeFile('../Payment-evidence-preview.html',`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>MikroMagic · Payment evidence</title><style>${css}\nbody{padding:24px}.preview{max-width:900px;margin:auto}.preview fieldset{border:0;padding:0}.preview h1{font-size:42px}</style><body><main class="preview"><h1>Payment evidence</h1><p>Static preview from actual receipt components; sample order and disabled controls. Images are attached after order submission in My access. The live API requires an authenticated purchaser or authorized SME owner to read evidence.</p><section class="panel"><h2>Guest · My access</h2><p>Sample internet pass · Awaiting payment review</p><fieldset disabled>${guest}</fieldset></section><section class="panel"><h2>SME · Review payment</h2><p>Actual receipt thumbnails are loaded privately after sign-in. In this static preview no receipt is attached.</p>${owner}<p>Approval must use the current receipt version. A new attachment requires the owner to refresh the review. Confirm funds independently before approving.</p></section></main></body></html>`);
 console.log('Receipt UI checks passed: guest-only uploader, closed decision, private viewer and review version.');
}finally{await rm('.ui-check/receipts.mjs',{force:true})}
