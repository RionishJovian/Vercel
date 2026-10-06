import {build} from 'esbuild';
import {mkdir,readFile,writeFile,rm} from 'node:fs/promises';
import assert from 'node:assert/strict';
const folder='.ui-check';await mkdir(folder,{recursive:true});
try{
 const bundle=await build({entryPoints:['src/PortalSettings.jsx'],bundle:true,platform:'node',format:'esm',external:['react','react-dom/server'],write:false});
 await writeFile(folder+'/components.mjs',bundle.outputFiles[0].text);
 const {PortalEditor,GuestPurchase}=await import('../'+folder+'/components.mjs');
 const React=await import('react');const {renderToStaticMarkup}=await import('react-dom/server');
 const cfg={business_name:'Golden Cafe · test preview',welcome:'Choose internet or local cinema access.',language:'both',terms:'Respect other guests.',contact:'Counter staff',direct_purchase:true,voucher_login:true,payment_destination:'Cafe counter · sample only',payment_instructions:'Submit a test receipt and transaction reference. The owner reviews your submission.',payment_methods:['Cash at counter','Mobile wallet'],hostname:'portal.example.com',guest_interface:'bridge-guest',guest_subnet:'192.168.50.0/24',tls_required:true,isolate_guests:true,allowed_hosts:['portal.example.com','192.168.50.10']};
 const portal={revision:4,draft:cfg,published_revision:3};
 const editor=renderToStaticMarkup(React.createElement(PortalEditor,{portal,busy:true,act:()=>{throw Error('Static preview cannot submit')}}));
 assert.match(editor,/Guest interface/);assert.match(editor,/Publish saved guest content/);assert.match(editor,/Router settings remain unapplied/);
 const guest=renderToStaticMarkup(React.createElement(GuestPurchase,{packages:[],portal:{revision:3,settings:{...cfg,payment_destination:'<script>untrusted</script>'}},busy:true,act:()=>{},go:()=>{}}));
 assert.match(guest,/&lt;script&gt;untrusted&lt;\/script&gt;/);assert.doesNotMatch(guest,/<script>untrusted/);assert.doesNotMatch(guest,/bridge-guest/);assert.match(guest,/Transaction reference/);
 const closed=renderToStaticMarkup(React.createElement(GuestPurchase,{packages:[],portal:{revision:3,settings:{...cfg,direct_purchase:false}},busy:true}));
 assert.doesNotMatch(closed,/Submit for owner review/);assert.match(closed,/currently disabled/);
 assert.match(guest,/Redeem voucher/);assert.doesNotMatch(guest,/still being built/);
 const disabledVouchers=renderToStaticMarkup(React.createElement(GuestPurchase,{packages:[],portal:{revision:3,settings:{...cfg,voucher_login:false}},busy:true}));assert.doesNotMatch(disabledVouchers,/Redeem voucher/);
 const css=await readFile('src/styles.css','utf8');await mkdir('_qa',{recursive:true});
 await writeFile('_qa/portal-preview.html',`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Portal editor · static test preview</title><style>${css}</style></head><body style="--background:#f0ece2;--surface:#fffaf0;--text:#16382b;--primary:#2d6554;--accent:#91b29d"><div class="app"><main><div class="eyebrow">MIKROMAGIC · STATIC COMPONENT CHECK · SAMPLE DATA</div><div class="title"><div><h1>Captive portal</h1><p>SME configuration editor. This view uses sample data; it cannot save or publish.</p></div></div><fieldset disabled style="border:0;margin:0;padding:0">${editor}</fieldset></main></div></body></html>`);
 console.log('Portal UI checks passed: editor fields, closed purchases, escaped content, private network fields absent from guest view.');
}finally{await rm(folder,{recursive:true,force:true})}
