import {publishedDocument} from './media-appearance.js?v=7';
const status=document.querySelector('#media-guest-status'),frame=document.querySelector('#media-page');
const id=new URLSearchParams(location.search).get('id');
const payment=document.querySelector('#media-payment'),checkout=document.querySelector('#media-checkout'),form=document.querySelector('#media-receipt-form'),orderStatus=document.querySelector('#media-order-status'),refresh=document.querySelector('#media-order-refresh');
const bank=document.querySelector('#media-bank-details'),bankText=document.querySelector('#media-bank-text'),qr=document.querySelector('#media-mmqr'),terms=document.querySelector('#media-pass-terms');
const valid=value=>typeof value==='string' && /^[0-9a-f]{64}$/.test(value);
const random=()=>Array.from(crypto.getRandomValues(new Uint8Array(32)),b=>b.toString(16).padStart(2,'0')).join('');
let order=null,busy=false;
const storageKey='mm_media_guest_order:'+id;
try{const stored=JSON.parse(sessionStorage.getItem(storageKey)||'null');if(stored && valid(stored.secret) && valid(stored.key) && (!stored.passId||valid(stored.passId)))order=stored;}catch(_){}
function save(){sessionStorage.setItem(storageKey,JSON.stringify(order));}
function controls(){checkout.disabled=busy;refresh.disabled=busy;form.querySelector('button').disabled=busy;}
async function request(path,options={}){
  const headers={Accept:'application/json',Authorization:'Bearer '+order.secret,...options.headers};
  const response=await fetch('/api/v1/media-guest/'+id+path,{...options,headers,credentials:'omit',cache:'no-store'});
  const body=await response.json().catch(()=>({}));
  if(!response.ok)throw new Error(body.error||'Request could not be completed. Check again before paying.');
  return body;
}
function showPass(pass,submitted=false){
  if(!valid(pass?.id))throw new Error('Invalid purchase response. Do not transfer payment.');
  order.passId=pass.id;save();checkout.hidden=true;refresh.hidden=false;
  terms.textContent=pass.amount+' '+pass.currency+' / '+pass.duration_seconds/3600+' hours';
  const pending=pass.state==='pending',timely=Date.parse(pass.payment_deadline)>Date.now();
  bank.hidden=!pending||submitted||!timely;form.hidden=!pending||!timely;
  const details=pass.payment;
  bankText.textContent=details?[details.bank_name,details.bank_account,details.bank_holder].filter(Boolean).join('\n'):'';
  qr.hidden=!details?.mmqr;qr.removeAttribute('src');
  if(details?.mmqr && /^data:image\/(png|jpeg);base64,/.test(details.mmqr))qr.src=details.mmqr;
  const labels={paid:'Approved. Your pass is paid; its timer starts at first authorized playback.',rejected:'The owner rejected this receipt. No access was granted.',refunded:'This pass was refunded. Access has ended.'};
  const expired=pass.state==='paid' && pass.expires_at && Date.parse(pass.expires_at)<=Date.now();
  orderStatus.textContent=expired?'Your paid pass has expired. Access has ended.':labels[pass.state]||(submitted?'Receipt submitted. Waiting for the owner to review it.':timely?'Transfer to the owner and upload your successful receipt.':'The receipt upload deadline has passed. Contact the owner; do not transfer payment.');
}
async function check(){if(busy||!order?.passId)return;busy=true;controls();try{const result=await request('/passes/'+order.passId);showPass(result.pass,result.receipt_submitted);}catch(e){orderStatus.textContent=e.message;}finally{busy=false;controls();}}
checkout.addEventListener('click',async()=>{
  if(busy)return;busy=true;controls();
  try{if(!order){order={secret:random(),key:random()};save();}const result=await request('/checkout',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({checkout_key:order.key})});showPass(result.pass);}
  catch(e){orderStatus.textContent=e.message||'Keep this page open and retry before paying.';}finally{busy=false;controls();}
});
refresh.addEventListener('click',check);
form.addEventListener('submit',async event=>{
  event.preventDefault();if(busy||!order?.passId)return;
  const file=form.elements.namedItem('receipt').files[0];
  if(!file || file.size>5*1024*1024 || !['image/png','image/jpeg','image/webp'].includes(file.type)){orderStatus.textContent='Choose a receipt photo up to 5 MiB.';return;}
  busy=true;controls();
  try{const data=new FormData();data.append('receipt',file);const result=await request('/passes/'+order.passId+'/receipt',{method:'POST',body:data});orderStatus.textContent=result.owner_notified?'Receipt sent to the owner. Waiting for approval.':'Receipt saved, but Telegram delivery was not confirmed. Retry with the same receipt or contact the owner. No access was granted.';bank.hidden=true;}
  catch(e){orderStatus.textContent=e.message;}finally{busy=false;controls();}
});
async function load(){
  try {
    if(!/^[0-9a-f]{64}$/.test(id||''))throw new Error('This guest page link is invalid.');
    const response=await fetch('/api/v1/media-guest/'+id,{credentials:'omit',cache:'no-store',headers:{Accept:'application/json'}});
    if(!response.ok)throw new Error(response.status===404?'This guest page is unavailable or has been unpublished.':'Guest page could not be loaded. Please try again.');
    const page=await response.json();frame.srcdoc=publishedDocument(page.design,page.purchases_enabled===true);frame.hidden=false;status.hidden=true;
    payment.hidden=!page.purchases_enabled;
    if(page.purchases_enabled){terms.textContent=page.design.price+' MMK / '+page.design.duration+' hours';if(order?.passId)await check();}
  }catch(e){status.textContent=e.message||'Guest page unavailable.';}
}
load();
