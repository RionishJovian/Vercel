/* Guided, server-allocated management address migration. No raw IP entry,
   credentials or RouterOS commands are accepted or stored by this screen. */
window.MMManagementMigration=(()=>{
 const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
 const final=phase=>['completed','rolled_back'].includes(phase);
 const phaseLabels={planned:'Ready for review',starting:'Starting',hub_staged:'New route reserved',router_staged:'Both router addresses available',new_verified:'New connection verified',hub_committed:'Hub allocation confirmed',router_retained:'New address retained',connection_saved:'Saved connection updated',router_cleaned:'Address cleanup verified',completed:'Completed',rolled_back:'Original connection retained'};
 const expiry=value=>Number.isFinite(Date.parse(value))?new Date(value).toLocaleString(undefined,{dateStyle:'medium',timeStyle:'short'}):'Not reported';
 async function mount(host,routers,canEdit,api=MM){
  let revision=0,busy=false,job=null,available=false;
  host.innerHTML=`<section class="panel network-workspace"><header><p class="network-eyebrow">GUIDED CONNECTION CHANGE</p><h3>Move management address</h3><p>Mega Magic checks the Hub allocation, keeps the old address, verifies the same router at its new address, then updates the saved connection. Guest LAN and DHCP addresses are separate.</p></header><label>Router<select class="input" data-move-router><option value="">Select a router</option>${routers.map(r=>`<option value="${esc(r.device_id)}">${esc(r.hardware_name)}</option>`).join('')}</select></label><div data-move-status role="status"></div><div data-move-editor></div></section>`;
  const select=host.querySelector('[data-move-router]'),status=host.querySelector('[data-move-status]'),editor=host.querySelector('[data-move-editor]');
  const endpoint=()=>`/routers/${encodeURIComponent(select.value)}/management-migration`;
  function draw(){
   editor.innerHTML='';
   if(!available){editor.innerHTML='<p>The server has not enabled this migration workflow. Existing management access is unchanged.</p><button class="btn" type="button" data-move-refresh>Refresh availability</button>';editor.querySelector('button').onclick=()=>load();return}
   if(job){editor.innerHTML=`<dl><dt>Current address</dt><dd>${esc(job.old_address)}</dd><dt>New Hub allocation</dt><dd>${esc(job.new_address)}</dd><dt>Migration status</dt><dd>${esc(phaseLabels[job.phase]||'Needs inspection')}</dd><dt>Preview expires</dt><dd>${esc(expiry(job.expires_at))}</dd></dl>${job.backup?`<p>Router backup: ${esc(job.backup)}</p>`:''}`}
   if(!canEdit){editor.innerHTML+='<p>An authorized administrator must perform this migration.</p>';return}
   if(!job||final(job.phase)){
    editor.innerHTML+=`${job?.phase==='completed'?'<p>The saved connection and address cleanup were verified. This does not prove guest internet access.</p>':job?.phase==='rolled_back'?'<p>The original management path was retained. You can prepare a new review.</p>':''}<button class="btn" type="button" data-move-preview>Check router and prepare migration</button>`;
    editor.querySelector('[data-move-preview]').onclick=()=>run('preview');return;
   }
   if(job.phase==='planned')editor.innerHTML+='<p>Review both addresses. The Hub chooses the new allocation; preparing this review has not changed the router.</p><label><input type="checkbox" data-move-consent> I reviewed this router and both addresses.</label><button class="btn btn-primary" type="button" data-move-apply disabled>Back up and migrate connection</button><button class="btn" type="button" data-move-recover>Cancel this review</button>';
   else editor.innerHTML+='<p>This operation needs recovery. Before the saved connection switches, recovery retains the original path. After the switch, recovery verifies the new connection and finishes cleanup.</p><button class="btn" type="button" data-move-recover>Recover this migration</button>';
   editor.innerHTML+='<button class="btn" type="button" data-move-refresh>Refresh durable status</button>';
   const expired=job.phase==='planned'&&Number.isFinite(Date.parse(job.expires_at))&&Date.parse(job.expires_at)<=Date.now();
   if(expired){editor.querySelector('[data-move-consent]').disabled=true;const note=document.createElement('p');note.textContent='This preview has expired. Cancel it, then prepare a fresh review.';editor.prepend(note)}
   editor.querySelector('[data-move-consent]')?.addEventListener('change',e=>editor.querySelector('[data-move-apply]').disabled=!e.target.checked);
   editor.querySelector('[data-move-apply]')?.addEventListener('click',()=>{if(editor.querySelector('[data-move-consent]')?.checked)run('apply')});
   editor.querySelector('[data-move-recover]').onclick=()=>run('recover');editor.querySelector('[data-move-refresh]').onclick=()=>load();
  }
  async function load(force=false){
   if(busy&&!force)return;const stamp=++revision,id=select.value;job=null;available=false;editor.innerHTML='';
   if(!id){status.textContent='Select a router.';return}status.textContent='Reading migration availability and saved status…';
   try{const data=await api.get(endpoint(),{signal:AbortSignal.timeout(30000)});if(!host.isConnected||stamp!==revision)return;available=data.enabled===true;job=data.job||null;status.textContent=available?'Saved migration status loaded.':data.reason||'Migration unavailable.';draw()}
   catch(_){if(host.isConnected&&stamp===revision){status.textContent='Migration status is unavailable. No action will be repeated automatically.';editor.innerHTML='<button class="btn" type="button" data-move-refresh>Retry status check</button>';editor.querySelector('button').onclick=()=>load()}}
  }
  async function run(action){
   if(busy||!available||!canEdit||!select.value)return;if(action!=='preview'&&!job)return;
   busy=true;const id=select.value,saved=job;select.disabled=true;editor.querySelectorAll('button,input').forEach(e=>e.disabled=true);status.textContent=action==='preview'?'Checking the existing pinned connection and Hub allocation…':'Working on the verified connection. Keep this view open until its status is returned.';
   let failed=false,message="";
   try{await api.post(endpoint()+'/'+action,action==='preview'?{}:{id:saved.id,revision:saved.revision,confirmed:true})}catch(e){failed=true;message=typeof e.data?.error==='string'?e.data.error:'The action response was not verified.'}
   if(host.isConnected&&select.value===id){await load(true);if(failed)status.textContent+=' '+message+' The saved checkpoint above determines recovery. Do not repeat Apply.'}
   busy=false;if(select.isConnected)select.disabled=false;
  }
  select.onchange=()=>{try{sessionStorage.setItem('mm_selected_router',select.value)}catch(_){}return load()};
  let saved='';try{saved=sessionStorage.getItem('mm_selected_router')||''}catch(_){}const target=routers.find(r=>r.device_id===saved)||(routers.length===1?routers[0]:null);if(target){select.value=target.device_id;await load()}
 }
 return {mount};
})();
