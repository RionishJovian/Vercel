/* Mega Magic's task-based RouterBOARD panel. All router requests go through
   authenticated tenant-scoped API handlers and the saved Hub connection. */
window.MMHubManagement = (() => {
 const escape=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
 const sections=[
  ['overview','Router overview','Health, interfaces and current RouterOS settings.'],
  ['wan','Internet connections','Primary ISP, backup uplink and PPPoE.'],
  ['ip','IP & DHCP','Addresses, routes, DNS and client reservations.'],
  ['existing','HotSpot & LAN','Guest service, login profiles and DHCP.'],
  ['protection','Protection','Invalid-traffic rule and portal bypass review.'],
  ['qos','Fairshare QoS','Shared capacity and client speed limits.'],
  ['wifi','Wi-Fi networks','SSID, password and country for onboard radios.'],
  ['pppoe','Subscriber service','Subscriber logins, plans and PPPoE service.'],
  ['migration','Management address','Move the Hub address with verification and recovery.'],
  ['maintenance','Maintenance','Backups, restore, events and safe reboot access.']
 ];
 function mount(host,routers,canEdit,api=MM) {
  if(!host)return;
  let revision=0,activeSection='',busy=false,saved='';
  try{saved=sessionStorage.getItem('mm_selected_router')||''}catch(_){}
  let selectedID=(routers.find(r=>r.device_id===saved)||(routers.length===1?routers[0]:null))?.device_id||'';
  host.innerHTML=`<section class="panel hub-management"><header><h2>Router settings</h2>${routers.length===1?`<p class="network-note" data-hub-target>${escape(routers[0].hardware_name)}</p>`:routers.length>1?`<label>Router<select class="input" data-hub-router><option value="">Choose your router</option>${routers.map(r=>`<option value="${escape(r.device_id)}">${escape(r.hardware_name)}</option>`).join('')}</select></label>`:''}</header><div class="network-tasks" data-hub-sections>${sections.map(([key,title,text])=>`<button type="button" class="network-task" title="${text}" data-hub-section="${key}" aria-pressed="false"><strong>${title}</strong></button>`).join('')}</div><div data-hub-workspace aria-live="polite"><p>Choose a setting to begin.</p></div></section>`;
  const pane=host.querySelector('[data-hub-workspace]');
  const buttons=[...host.querySelectorAll('[data-hub-section]')];
  const picker=host.querySelector('[data-hub-router]');if(picker)picker.value=selectedID;
  const currentRouter=()=>routers.find(r=>r.device_id===selectedID);
  const remember=r=>{try{sessionStorage.setItem('mm_selected_router',r.device_id)}catch(_){}};
  if(currentRouter())remember(currentRouter());
  const locked=()=>busy||pane.querySelector('[data-router]:disabled,[data-move-router]:disabled');
  if(picker)picker.onchange=()=>{
   if(locked()){picker.value=selectedID;return}
   selectedID=picker.value;
   if(currentRouter())remember(currentRouter());
   if(activeSection)return choose(activeSection);
  };
  const link=(hash,title)=>`<a class="btn" href="#${hash}">${title}</a>`;
  async function choose(section){
   // Existing network editor locks its controls during an apply. Do not discard
   // that editor from an outer category click while the request is in flight.
   if(locked())return;
   const stamp=++revision;
   activeSection=section;
   buttons.forEach(b=>b.setAttribute('aria-pressed',String(b.dataset.hubSection===section)));
   const r=currentRouter();if(r)remember(r);
   pane.replaceChildren();
   if(!r){pane.textContent=routers.length?'Choose your router once above to use its settings.':'Add and connect your RouterBOARD above first.';picker?.focus();return}
   const onBusy=value=>{if(revision===stamp){busy=value;if(picker)picker.disabled=value}};
   if(section==='migration'){const editor=document.createElement('div');pane.appendChild(editor);await MMManagementMigration.mount(editor,[r],canEdit,api,{compact:true,onBusy});return}
   if(section==='maintenance'){
    pane.innerHTML=`<h3>Recovery & maintenance</h3><p>Create and inspect recovery points before making major changes. Restore and reboot require explicit confirmation and the existing server checks.</p><div class="workspace-actions">${link('backups','Backups & restore')}${link('events','Events')}${link('fleet','Fleet health')}</div><p>Reboot is available on the router card above when its connection is verified. RouterOS updates are not automated in this release.</p>`;return;
   }
   if(section==='overview'){
    pane.innerHTML=`<h3>Router health</h3><button class="btn" type="button" data-health-refresh>Read current status</button><div data-health-status role="status">Live information has not been read.</div>`;
    const status=pane.querySelector('[data-health-status]'),refresh=pane.querySelector('[data-health-refresh]');
    let request=0;
    async function load(){
     const id=r.device_id,seq=++request;
     refresh.disabled=true;status.textContent='Reading the router through Magic Hub…';
     try{
      const data=await api.get(`/routers/${encodeURIComponent(id)}/network/overview`,{signal:AbortSignal.timeout(30000)});
      if(!pane.isConnected||revision!==stamp||request!==seq)return;
      const inventory=data.inventory||{},health=inventory.health||{};
      const labels={'cpu-load':'CPU load (%)','free-memory':'Free memory (bytes)','total-memory':'Total memory (bytes)','uptime':'Uptime','version':'RouterOS version','board-name':'RouterBOARD'};
      status.innerHTML=`<p>Read at ${escape(new Date().toLocaleTimeString())}. Configuration readback does not prove guest internet access.</p><dl>${Object.entries(labels).map(([key,label])=>`<dt>${label}</dt><dd>${escape(health[key]||'Not reported')}</dd>`).join('')}</dl><h4>Interfaces</h4>${(inventory.interfaces||[]).map(i=>`<p><strong>${escape(i.name)}</strong> · ${escape(i.type)} · ${i.running?'Running':'Not running'}</p>`).join('')||'<p>Not reported</p>'}<p>${inventory.write_enabled?'Reviewed network changes are enabled.':'Network writes are currently held by the server. Read-only status remains available.'}</p>`;
     }catch(e){if(pane.isConnected&&revision===stamp&&request===seq)MMNetwork.showReadFailure(status,e,id,load)}
     finally{if(refresh.isConnected&&request===seq)refresh.disabled=false}
    }
    refresh.onclick=load;await load();return;
   }
   // Isolate each mounted editor so late responses cannot replace a new tab.
   const editor=document.createElement('div');pane.appendChild(editor);
   await MMNetwork.mount(editor,section,[r],canEdit,api,{compact:true,onBusy});
   if(editor.isConnected&&revision===stamp&&(section==='ip'||section==='existing')){
    const shortcuts=document.createElement('div');shortcuts.className='workspace-actions';
    shortcuts.innerHTML=section==='ip'?`${link('network','Create customer network')}<button class="btn" type="button" data-quick-dns>Router name & DNS</button>`:`${link('network','Create customer network')}${link('portal','Portal design & deployment')}${link('vouchers','Issue & print vouchers')}${link('guests','Connected guests')}`;
    editor.prepend(shortcuts);
    shortcuts.querySelector('[data-quick-dns]')?.addEventListener('click',()=>{
     const selected=currentRouter();const card=[...host.parentElement.querySelectorAll('[data-router-capability]')].find(b=>b.dataset.routerCapabilityDevice===selected?.device_id&&b.dataset.routerCapability==='gateway');
     if(card)card.click();else {const note=document.createElement('p');note.textContent='Use Router name and DNS on the selected router card above.';shortcuts.append(note)}
    });
   }
  }
  buttons.forEach(b=>b.onclick=()=>choose(b.dataset.hubSection));
  try{const requested=sessionStorage.getItem('mm_hub_task');sessionStorage.removeItem('mm_hub_task');if(sections.some(s=>s[0]===requested))choose(requested)}catch(_){}
 }
 return {mount};
})();
