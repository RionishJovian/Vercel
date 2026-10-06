import {build} from 'esbuild';
import {mkdir,writeFile,rm} from 'node:fs/promises';
import assert from 'node:assert/strict';
const folder='.ui-check';await mkdir(folder,{recursive:true});
try{
 const bundle=await build({stdin:{contents:"export * from './src/Vouchers.jsx'; export * from './src/VoucherPayments.jsx';",resolveDir:process.cwd(),loader:'jsx'},bundle:true,platform:'node',format:'esm',external:['react'],write:false});await writeFile(folder+'/vouchers.mjs',bundle.outputFiles[0].text);
 const {VoucherInventory,RedeemVoucher,VoucherPayment}=await import('../'+folder+'/vouchers.mjs');const React=await import('react');const {renderToStaticMarkup}=await import('react-dom/server');
 const html=renderToStaticMarkup(React.createElement(VoucherInventory,{packages:[],vouchers:[{id:'test-voucher',policy:{name:'Test cinema'},status:'redeemed',code_hint:'ABCD',expires:1}],act:()=>{},busy:true}));
 assert.match(html,/Open payment/);assert.match(html,/No payment recorded/);assert.match(html,/Generate voucher/);assert.match(html,/Revoke stored pass/);assert.match(html,/Issuing or redeeming a voucher does not record revenue/);assert.match(html,/•••• ABCD/);assert.doesNotMatch(html,/code_hash/);
 const guest=renderToStaticMarkup(React.createElement(RedeemVoucher,{act:()=>{},busy:true,go:()=>{}}));assert.match(guest,/Voucher code/);assert.match(guest,/Redeem voucher/);assert.match(guest,/await the site adapters/);
 const payment=renderToStaticMarkup(React.createElement(VoucherPayment,{voucher:{id:'test',policy:{name:'Test',price:3000},code_hint:'ABCD',status:'unused',sale:null},busy:false,act:()=>{}}));assert.match(payment,/I confirmed receipt/);assert.match(payment,/<button class="button" disabled="">Record confirmed payment/);
 console.log('Voucher UI checks passed: issuance, masked inventory, revocation reason, guest redemption and pending enforcement labels.');
}finally{await rm(folder,{recursive:true,force:true})}
