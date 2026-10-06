import {csvCell} from './revenue-csv.mjs';
export function paymentState(v){return !v.sale?'unpaid':v.sale.refunded_at?'refunded':'paid'}
export function filterStock(vouchers,{site='',status='',payment='',search='',expiring=false},now=Date.now()/1000){
 const needle=search.trim().toLowerCase();return vouchers.filter(v=>(!site||(site==='business'?!v.site_id:v.site_id===site))&&(!status||v.status===status)&&(!payment||paymentState(v)===payment)&&(!needle||[v.id,v.code_hint,v.policy.name].some(x=>String(x||'').toLowerCase().includes(needle)))&&(!expiring||(v.status==='unused'&&v.expires>now&&v.expires<=now+7*86400)));
}
export function stockCsv(vouchers,sites){
 const fields=['voucher_id','code_ending','package','site','status','payment','created_utc','redeem_deadline_utc'];
 const rows=vouchers.map(v=>[v.id,v.code_hint,v.policy.name,sites.find(s=>s.id===v.site_id)?.name||v.site_id||'Business-wide',v.status,paymentState(v),new Date(v.created*1000).toISOString(),new Date(v.expires*1000).toISOString()]);
 return '\ufeff'+[fields,...rows].map(row=>row.map(csvCell).join(',')).join('\r\n')+'\r\n';
}
const htmlEscape=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
export function voucherSheet(items,business,sites=[]){
 const cards=items.filter(v=>v.code).map(v=>`<article><h2>${htmlEscape(business)}</h2><p>${htmlEscape(sites.find(s=>s.id===v.site_id)?.name||v.site_id||'Business-wide')}</p><h3>${htmlEscape(v.policy.name)}</h3><strong>${htmlEscape(v.code)}</strong><p>Redeem before ${htmlEscape(new Date(v.expires*1000).toISOString())} (UTC)</p><p>${htmlEscape(v.policy.service)} · ${htmlEscape(v.policy.mode)} · ${htmlEscape(v.policy.price)} MMK</p><small>Payment must be confirmed separately. Live access awaits the site adapter.</small><p class="id">${htmlEscape(v.id)}</p></article>`).join('');
 return '<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Private voucher sheet</title><style>body{font:14px system-ui;color:#16382b;background:#fff;margin:12mm}header{margin-bottom:8mm}.sheet{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:6mm}article{border:1px dashed #2d6554;padding:6mm;break-inside:avoid}h2{margin:0;font-size:20px}strong{font:600 17px monospace;overflow-wrap:anywhere}.id{font-size:9px} @media print{header{display:none}body{margin:0}@page{size:A4;margin:10mm}}@media(max-width:500px){.sheet{grid-template-columns:1fr}}</style><body><header><h1>Private voucher sheet</h1><p>Contains full redeemable codes. Keep private. Use your browser’s Print command; header is omitted from printed cards.</p></header><main class="sheet">'+cards+'</main></body></html>';
}
