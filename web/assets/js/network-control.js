/* Typed, reviewed network changes. No credentials or pending changes enter
   browser storage. Selecting a device only reads its inventory. */
window.MMNetwork = (() => {
 const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
 const schema={
  wan:{title:'Internet uplinks',menu:null,fields:[['mode','Routing mode','select',[['failover','Primary + backup'],['balance','Share connections across both uplinks']]],['wan1','Primary ISP port','interface'],['gateway1','Primary ISP gateway'],['wan2','Second ISP port','interface'],['gateway2','Second ISP gateway']]},
  pppoe_client:{title:'PPPoE internet uplink',menu:'pppoe_clients',create:true,fields:[['name','Uplink name'],['interface','ISP-facing port','interface'],['user','ISP username'],['password','ISP password','password'],['distance','Route priority (lower is preferred)','number'],['disabled','Service state','state']]},
  pppoe_server:{title:'PPPoE subscriber service',menu:null,fields:[['name','Service name'],['interface','Unused subscriber-facing port','interface'],['gateway_cidr','Subscriber gateway / prefix'],['pool_start','First subscriber address'],['pool_end','Last subscriber address'],['dns','Subscriber DNS'],['rate_limit','Upload / download speed (e.g. 10M/20M)']]},
  pppoe_service:{title:'Existing PPPoE services',menu:'pppoe_servers',fields:[['default-profile','Subscriber plan','record','ppp_profiles'],['disabled','Service state','state']]},
  subscriber:{title:'PPPoE subscribers',menu:'subscribers',create:true,fields:[['name','Subscriber login'],['password','Subscriber password','password'],['profile','Subscriber plan','record','ppp_profiles'],['disabled','Account state','state']]},
  ppp_profile:{title:'Subscriber plans',menu:'ppp_profiles',fields:[['local-address','Subscriber gateway address'],['remote-address','Address pool','record','pools'],['dns-server','Client DNS servers'],['rate-limit','Upload / download speed'],['only-one','One simultaneous session','select',[['yes','Yes'],['no','No']]]]},
  wifi:{title:'Wi-Fi SSIDs',menu:'wifi',fields:[['ssid','Wi-Fi network name'],['country','Actual installation country'],['password','New Wi-Fi password','password'],['disabled','Radio state','state']]},
  wireless:{title:'Legacy Wi-Fi SSIDs',menu:'wireless',fields:[['ssid','Wi-Fi network name'],['country','Actual installation country'],['password','New Wi-Fi password','password'],['disabled','Radio state','state']]},
  dhcp:{title:'DHCP services',menu:'dhcp',fields:[['address-pool','Client address pool','record','pools'],['lease-time','Lease duration (e.g. 1h)'],['disabled','Service state','state']]},
  pool:{title:'Address pools',menu:'pools',fields:[['ranges','First-last client address']]},
  dhcp_network:{title:'Client gateway and DNS',menu:'dhcp_networks',fields:[['gateway','Client gateway'],['dns-server','Client DNS servers']]},
  hotspot:{title:'Hotspot services',menu:'hotspots',fields:[['profile','Hotspot profile','record','hotspot_profiles'],['disabled','Service state','state']]},
  hotspot_profile:{title:'Hotspot login profiles',menu:'hotspot_profiles',fields:[['dns-name','Portal DNS name'],['login-by','Guest login','select',[['http-chap','Password (CHAP)'],['http-chap,mac','Password + MAC grace']]],['use-radius','Use RADIUS','select',[['yes','Yes'],['no','No']]]]},
  lease:{title:'Static DHCP reservations',menu:'leases',create:true,fields:[['address','Reserved client address'],['mac-address','Client MAC address'],['server','DHCP service','record','dhcp'],['disabled','Reservation state','state']]},
  queue:{title:'Customer bandwidth limits',menu:'queues',create:true,fields:[['name','Limit name'],['target','Client address / prefix'],['max-limit','Upload / download speed (e.g. 10M/20M)'],['disabled','Limit state','state']]}
 };
 const choices={wan:['wan','pppoe_client'],pppoe:['pppoe_server','pppoe_service','subscriber','ppp_profile'],wifi:['wifi','wireless'],existing:['dhcp','pool','dhcp_network','hotspot','hotspot_profile','lease','queue']};
 const objects=(inventory,key)=>(inventory.objects?.[key]||[]);
 const label=o=>o.values.name||o.values['service-name']||o.values.address||o.id;
 async function mount(host,workspace,routers,canEdit,api=MM){
  let revision=0,inventory=null,defaults={},review=null,pending=null;
  const kinds=choices[workspace];
  host.innerHTML=`<section class="panel"><h2>${esc({wan:'WAN balancing and failover',pppoe:'PPPoE business services',wifi:'Wi-Fi and access points',existing:'Manage existing networks'}[workspace])}</h2><p>Load the selected router, edit actual settings, review the change, then apply with backup and verified readback. Opening this page changes nothing.</p>
   <label>Managed device<select class="input" data-router><option value="">Choose a connected device</option>${routers.map(r=>`<option value="${esc(r.device_id)}">${esc(r.hardware_name)}</option>`).join('')}</select></label>
   <div data-editor aria-live="polite">${routers.length?'Choose a device to read its current settings.':'Connect a device from Gateways first.'}</div></section>`;
  const select=host.querySelector('[data-router]'),editor=host.querySelector('[data-editor]');
  const current=()=>host.isConnected&&select.value;
  const error=e=>(e.data&&e.data.error)||e.message||'Request failed';
  async function reload(){
   const stamp=++revision,id=select.value;review=null;pending=null;inventory=null;
   if(!id){editor.textContent='Choose a device.';return}
   editor.textContent='Reading live router inventory…';
   try{
    const data=await api.get(`/routers/${encodeURIComponent(id)}/network/inventory`);
    if(!current()||revision!==stamp)return;
    inventory=data.inventory;defaults={};
    if(workspace==='pppoe'){
     try{const data=await api.get(`/routers/${encodeURIComponent(id)}/gateway/defaults`);defaults=data.defaults||{}}catch(_){/* Never invent an unchecked address. */}
     if(!current()||revision!==stamp)return;
    }
    draw();
   }catch(e){if(current()&&revision===stamp)editor.textContent=`Could not read this device safely. Nothing applied. ${error(e)}`}
  }
  function draw(){
   editor.innerHTML=`<p><button class="btn" type="button" data-reload>Reload router</button></p><label>Task<select class="input" data-kind>${kinds.map(k=>`<option value="${k}">${esc(schema[k].title)}</option>`).join('')}</select></label><div data-settings></div>`;
   editor.querySelector('[data-reload]').onclick=reload;
   editor.querySelector('[data-kind]').onchange=drawSettings;drawSettings();
  }
  function drawSettings(){
   ++revision;review=null;pending=null;
   const kind=editor.querySelector('[data-kind]').value,s=schema[kind],pane=editor.querySelector('[data-settings]');
   if(!canEdit){pane.textContent='An owner or administrator must change network settings.';return}
   if(inventory.write_enabled!==true){pane.textContent='Router changes are not enabled in this release. Hardware and connectivity acceptance must finish before rollout; nothing was applied.';return}
   if((kind==='wifi'||kind==='wireless')&&inventory.wifi_write_enabled!==true){pane.textContent='Wi-Fi changes are awaiting physical-radio acceptance. No SSID was changed. WAN, PPPoE and existing-network controls have a separate rollout.';return}
   if(s.menu&&!inventory.available?.[s.menu]){pane.textContent='This RouterOS menu could not be read safely or is not supported on this device. No changes are offered.';return}
   let list=s.menu?objects(inventory,s.menu):[];
   if(kind==='lease')list=list.filter(o=>!['yes','true'].includes(o.values.dynamic));
   pane.innerHTML=(s.menu?`<label>${esc(s.title)}<select class="input" data-record>${s.create?'<option value="">Create new</option>':''}${list.map(o=>`<option value="${esc(o.id)}">${esc(label(o))}</option>`).join('')}</select></label>`:'')+'<div data-fields></div>';
   if(s.menu&&!s.create&&!list.length){pane.innerHTML+='<p>No existing records. Use Network setup for a customer LAN / Hotspot, or create a PPPoE subscriber service.</p>';return}
   const record=pane.querySelector('[data-record]');if(record)record.onchange=fields;fields();
   function fields(){
    ++revision;review=null;pending=null;
    const selected=list.find(o=>o.id===record?.value),v={...(selected?.values||{})};
    if(!selected){Object.assign(v,{name:kind==='pppoe_server'?'subscribers':'customer',distance:'2',disabled:'no',dns:'1.1.1.1,8.8.8.8',rate_limit:'10M/20M',mode:'failover',gateway_cidr:defaults.gateway_cidr||'',pool_start:defaults.pool_start||'',pool_end:defaults.pool_end||''})}
    if(kind==='wifi'){v.ssid=v['configuration.ssid'];v.country=v['configuration.country']}
    const eligible=(inventory.interfaces||[]).filter(i=>['ether','bridge','vlan','bond','pppoe-out'].includes(i.type));
    const input=f=>{
     const [key,text,type,arg]=f;let value=v[key]??'';
     if(key==='disabled')value=['yes','true'].includes(value)?'yes':'no';
     let options=null;
     if(type==='state')options=[['no','Enabled'],['yes','Disabled']];
     if(type==='select')options=arg;
     if(type==='interface')options=[['','Choose an actual port'],...eligible.map(i=>[i.name,`${i.name}${i.running?' · running':''}`])];
     if(type==='record')options=[['','Choose an existing record'],...objects(inventory,arg).map(o=>[o.values.name,o.values.name])];
     const control=options?`<select class="input" name="${key}" required>${options.map(([id,t])=>`<option value="${esc(id)}"${String(value)===id?' selected':''}>${esc(t)}</option>`).join('')}</select>`:`<input class="input" name="${key}" type="${type==='password'?'password':type==='number'?'number':'text'}" value="${type==='password'?'':esc(value)}" required ${type==='password'?'autocomplete="new-password"': ''} maxlength="128">`;
     return `<label>${esc(text)}${control}</label>`;
    };
    pane.querySelector('[data-fields]').innerHTML=`<form data-form>${s.fields.map(input).join('')}<p>${kind==='wan'?'ISP gateways must be in their ports’ current subnets. Detected DHCP gateways fill when you select a port. Balancing shares connections, not the bandwidth of one download.':''}${kind==='wifi'||kind==='wireless'?'Changing Wi-Fi disconnects clients. Set the real installation country. Manage each remote AP as its own device.':''}</p><button class="btn" type="submit">Review router changes</button></form><div data-review aria-live="polite"></div>`;
    const form=pane.querySelector('[data-form]'),output=pane.querySelector('[data-review]');
    if(kind==='wan')for(const n of ['1','2'])form.elements['wan'+n].onchange=()=>{const c=objects(inventory,'dhcp_clients').find(o=>o.values.interface===form.elements['wan'+n].value);form.elements['gateway'+n].value=c?.values.gateway||''};
    const invalidate=()=>{++revision;review=null;pending=null;output.textContent='Settings changed. Review again before applying.'};
    form.addEventListener('input',invalidate);form.addEventListener('change',invalidate);
    form.onsubmit=async e=>{
     e.preventDefault();if(!form.reportValidity())return;
     const stamp=++revision,id=select.value,body={kind,id:selected?.id||'',values:Object.fromEntries(new FormData(form))};
     review=null;pending=null;output.textContent='Checking the router and preparing the change review…';
     try{
      const data=await api.post(`/routers/${encodeURIComponent(id)}/network/preview`,body);
      if(!current()||stamp!==revision||!output.isConnected)return;
      review=data.review;pending=body;
      output.innerHTML=`<h3>Review before applying</h3><ul>${review.steps.map(t=>`<li>${esc(t)}</li>`).join('')}</ul>${review.warnings.map(t=>`<p>${esc(t)}</p>`).join('')}<label><input type="checkbox" data-consent> I checked this device and the proposed changes. Apply to this router.</label><button class="btn" type="button" data-apply disabled>Back up and apply to router</button>`;
      const apply=output.querySelector('[data-apply]');output.querySelector('[data-consent]').onchange=e=>apply.disabled=!e.target.checked;
      apply.onclick=async()=>{
       if(!review||!pending)return;const request={...pending,confirmation:review.confirmation};review=null;pending=null;++revision;
       const savedStamp=revision;select.disabled=true;form.querySelectorAll('input,select,button').forEach(el=>el.disabled=true);apply.disabled=true;output.textContent='Backing up, applying and verifying router settings…';
       try{
        const data=await api.post(`/routers/${encodeURIComponent(id)}/network/apply`,request);
        if(!output.isConnected||savedStamp!==revision)return;
        if(!data.result?.verified)throw Error('Router changes were not verified');
        form.querySelectorAll('input[type="password"]').forEach(el=>el.value='');
        output.textContent=`Router settings verified. Backup: ${data.result.backup}. Test a real client before calling the service operational. Reload router to edit again.`;
       }catch(e){if(output.isConnected&&savedStamp===revision)output.textContent=`Apply failed. ${error(e)} ${e.data?.result?.rolled_back?'Previous settings restored and verified.':'Rollback not confirmed; inspect the router and its backup before retrying.'}`}
       finally{select.disabled=false;editor.querySelector('[data-reload]').disabled=false;request.values={}}
      };
     }catch(e){if(output.isConnected&&stamp===revision)output.textContent=`Review failed; nothing applied. ${error(e)}`}
    };
   }
  }
  select.onchange=reload;
 }
 return {mount};
})();
