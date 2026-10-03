import {publishedDocument} from './media-appearance.js?v=6';
const status=document.querySelector('#media-guest-status'),frame=document.querySelector('#media-page');
const id=new URLSearchParams(location.search).get('id');
async function load(){
  try {
    if(!/^[0-9a-f]{64}$/.test(id||''))throw new Error('This guest page link is invalid.');
    const response=await fetch('/api/v1/media-guest/'+id,{credentials:'omit',cache:'no-store',headers:{Accept:'application/json'}});
    if(!response.ok)throw new Error(response.status===404?'This guest page is unavailable or has been unpublished.':'Guest page could not be loaded. Please try again.');
    const page=await response.json();frame.srcdoc=publishedDocument(page.design);frame.hidden=false;status.hidden=true;
  }catch(e){status.textContent=e.message||'Guest page unavailable.';}
}
load();
