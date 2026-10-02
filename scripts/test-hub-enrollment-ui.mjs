import assert from 'node:assert/strict';
import {mountHubEnrollment} from '../web/assets/js/hub-enrollment.js';
const originalFormData=globalThis.FormData;
globalThis.FormData=class{constructor(form){this.values=form.values}get(name){return this.values[name]}};
try {
  const key=Buffer.alloc(32,1).toString('base64');
  let submitHandler,resolve,calls=0;
  const button={disabled:false,isConnected:true};
  const status={textContent:'',innerHTML:''};
  const form={values:{router_public_key:key,management_rest_port:'443',confirm_enrollment:'on'},reportValidity:()=>true,
    querySelector:()=>button,addEventListener(event,handler){assert.equal(event,'submit');submitHandler=handler}};
  const container={dataset:{hubEnrollment:'gateway/a'},isConnected:true,querySelector:selector=>selector==='form'?form:status};
  const api={post(path,body){calls++;assert.equal(path,'/vps/peers/gateway%2Fa/enroll');assert.equal(body.confirm_enrollment,true);assert.equal(body.router_public_key,key);return new Promise(r=>resolve=r)}};
  const esc=value=>String(value??'').replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('>','&gt;').replaceAll('"','&quot;');
  mountHubEnrollment({querySelectorAll:()=>[container]},{api,escape:esc});
  const event={preventDefault(){}};
  const pending=submitHandler(event);assert(button.disabled);await submitHandler(event);assert.equal(calls,1);
  resolve({state:'hub_peer_enrolled',router_configuration_required:true,tunnel_address:'<img src=x>',hub_public_key:key,endpoint:'hub:51820',allowed_ips:['10.77.0.1/32']});await pending;
  assert(!button.disabled);assert(!status.innerHTML.includes('<img'));assert(status.innerHTML.includes('&lt;img'));assert(status.innerHTML.includes('LAN access is not yet verified'));
  form.values.confirm_enrollment='';await submitHandler(event);assert.equal(calls,1);assert.match(status.textContent,/Confirm/);
  form.values.confirm_enrollment='on';form.values.router_public_key='password';await submitHandler(event);assert.equal(calls,1);
  form.values.router_public_key=key;
  const stale=submitHandler(event);container.isConnected=false;const previous=status.innerHTML;
  resolve({state:'hub_peer_enrolled',router_configuration_required:true});await stale;assert.equal(status.innerHTML,previous);
  container.isConnected=true;
  api.post=async()=>{throw {status:503,message:'INTERNAL_SECRET'}};
  await submitHandler(event);assert.match(status.textContent,/not enabled/);assert(!status.textContent.includes('INTERNAL_SECRET'));assert(!button.disabled);
  console.log('Hub enrollment UI passed: explicit consent, duplicate guard, encoded gateway, escaped results, disabled deployment, detached page and honest connection state.');
} finally {globalThis.FormData=originalFormData}
