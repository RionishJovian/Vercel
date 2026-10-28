/* Business-first workflows over the existing authenticated API. Never fabricate
   live discovery, connection state, or a successful router action. */
window.MMRevamp = (() => {
  const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const list=v=>Array.isArray(v)?v:[];
  const failure=e=>e?.data?.error||e?.message||'The server did not confirm this operation.';

  async function bindSites(content,sites,api,refresh) {
    await Promise.all([...content.querySelectorAll('[data-gateway-id]')].map(async card=>{
      const id=card.dataset.gatewayId,section=card.querySelector('.router-hub-check');
      if(!section)return;
      const holder=document.createElement('div');holder.className='hub-site-step';
      section.prepend(holder);
      let selected='',failed=false;
      try {selected=(await api.get(`/vps/peers/${encodeURIComponent(id)}/site`)).site_id||''}catch(_){failed=true}
      if(!holder.isConnected)return;
      holder.innerHTML=`<h3>Site</h3>${sites.length?`<form><label>Business site<select class="input" name="site" required><option value="">Choose a site</option>${sites.map(s=>`<option value="${esc(s.site_id)}"${selected===s.site_id?' selected':''}>${esc(s.name)}</option>`).join('')}</select></label><button class="btn" type="submit">Save site</button></form>`:'<p>Add your business location first.</p>'}<a href="#sites">${sites.length?'Manage sites':'Add your first site'}</a><p role="status">${failed?'Site assignment could not be checked. Retry before connecting.':selected?'Site saved.':'Choose a site before connecting.'}</p>`;
      const wizard=section.querySelector('[data-hub-enrollment]');
      if(wizard) {wizard.hidden=!selected||failed;const title=wizard.querySelector('h3');if(title)title.textContent='2. Connect with one paste';const finish=wizard.querySelectorAll('h3')[1];if(finish)finish.textContent='3. Finish connection'}
      holder.querySelector('form')?.addEventListener('submit',async event=>{
        event.preventDefault();const form=event.currentTarget,button=form.querySelector('button'),status=holder.querySelector('[role="status"]');
        if(button.disabled||!form.reportValidity())return;
        button.disabled=true;status.textContent='Saving site assignment…';
        try {await api.put(`/vps/peers/${encodeURIComponent(id)}/site`,{site_id:form.elements.site.value});if(holder.isConnected)await refresh()}
        catch(e){if(holder.isConnected)status.textContent=failure(e)}finally{if(button.isConnected)button.disabled=false}
      });
    }));
  }

  function discoveryRows(inventory) {
    const rows=new Map();
    for(const source of ['discovery_leases','arp','neighbors']){
      if(inventory.available?.[source]!==true)continue;
      for(const o of list(inventory.objects?.[source])){
        const v=o.values||{},mac=String(v['mac-address']||'').toUpperCase(),ip=v.address||'';
        const key=mac||ip;if(!key)continue;
        const row=rows.get(key)||{mac,ip,name:'Unidentified device',type:'Not identified',sources:[],state:'Observed; online status unverified'};
        row.ip=ip||row.ip;row.name=v['host-name']||v.identity||row.name;
        row.type=v.platform||v.board||row.type;row.sources.push(({discovery_leases:'DHCP lease',arp:'ARP table',neighbors:'Router neighbor'})[source]);
        if(source==='discovery_leases')row.state=v.status==='bound'?'DHCP lease bound':'DHCP '+(v.status||'status unavailable');
        rows.set(key,row);
      }
    }
    return [...rows.values()];
  }

  // The saved selection is context, never a credential or a connection attempt.
  function routerContext(select,routers,reload) {
    const saved=sessionStorage.getItem('mm_selected_router');
    const selected=routers.find(r=>r.device_id===saved)||(routers.length===1?routers[0]:null);
    select.addEventListener('change',()=>{if(select.value)sessionStorage.setItem('mm_selected_router',select.value)});
    if(selected){select.value=selected.device_id;sessionStorage.setItem('mm_selected_router',selected.device_id);return reload()}
  }

  async function mountGuests(host,routers,api,canEdit,sites=[]) {
    const gateways=list(routers).filter(r=>r.deployment_layer==='MAIN_GATEWAY');
    let stopped=false,reading=false,busy=false,sources=[];
    const siteNames=new Map(list(sites).map(s=>[s.site_id,s.name]));
    host.innerHTML=`<section class="panel"><div class="panel-body"><h2>Connected Hotspot guests</h2><p>View current sessions and usage.</p><div class="revamp-task-grid"><label>Site<select class="input" data-guest-site><option value="">All sites</option>${[...new Set(gateways.map(g=>g.site_id||''))].map(id=>`<option value="${esc(id||'unassigned')}">${esc(siteNames.get(id)||'Unassigned site')}</option>`).join('')}</select></label><label>RouterBOARD<select class="input" data-guest-device><option value="">All gateways at this site</option></select></label><label>Find a guest<input class="input" type="search" data-guest-search placeholder="Name, IP or MAC address"></label></div><button class="btn" data-refresh type="button">Refresh guests</button><p data-guest-status role="status"></p><div data-guest-summary class="revamp-task-grid"></div><div data-guest-results></div><details class="workspace-inline-help"><summary>About guest sessions</summary><p>Sessions show Hotspot logins, not every Wi-Fi device. Disconnect ends the session; it does not block future access.</p></details><a href="#vouchers">Internet vouchers and plans</a></div></section>`;
    const site=host.querySelector('[data-guest-site]'),device=host.querySelector('[data-guest-device]'),search=host.querySelector('[data-guest-search]'),refresh=host.querySelector('[data-refresh]'),status=host.querySelector('[data-guest-status]'),summary=host.querySelector('[data-guest-summary]'),results=host.querySelector('[data-guest-results]');
    const remembered=gateways.find(g=>g.device_id===sessionStorage.getItem('mm_selected_router'));
    if(remembered)site.value=remembered.site_id||'unassigned';
    function deviceOptions(selected='') {
      const choices=gateways.filter(g=>!site.value||(g.site_id||'unassigned')===site.value);
      device.innerHTML='<option value="">All gateways at this site</option>'+choices.map(g=>`<option value="${esc(g.device_id)}">${esc(g.hardware_name)}</option>`).join('');
      if(choices.some(g=>g.device_id===selected))device.value=selected;
    }
    deviceOptions(remembered?.device_id);
    const back=document.createElement('a');back.href='#internet';back.className='btn';back.textContent='Back to device workspace';host.querySelector('[data-refresh]').before(back);
    const alive=()=>!stopped&&host.isConnected;
    function draw(){
      device.disabled=busy||reading;
      const scoped=sources.filter(s=>(!site.value||(s.gateway.site_id||'unassigned')===site.value)&&(!device.value||s.gateway.device_id===device.value)),available=scoped.filter(s=>s.ok);
      const sessions=available.flatMap(s=>s.sessions.map(session=>({source:s,session})));
      summary.innerHTML=`<div class="revamp-task"><strong>${available.length?sessions.length:'—'}</strong><span>${available.length<scoped.length?'Reported active sessions · incomplete coverage':'Active Hotspot sessions'}</span></div><div class="revamp-task"><strong>${available.length} / ${scoped.length}</strong><span>Gateway session sources available</span></div>`;
      const needle=search.value.trim().toLowerCase(),shown=sessions.filter(({session})=>Object.values(session.values||{}).some(v=>String(v).toLowerCase().includes(needle)));
      results.innerHTML=shown.length?`<div class="table-scroll"><table><thead><tr><th>Guest</th><th>Site / gateway</th><th>Status</th><th>Connected time / data</th><th>Action</th></tr></thead><tbody>${shown.map(({source,session},i)=>{const v=session.values||{};return `<tr><td>${esc(v.user||'Hotspot guest')}<br><small>${esc(v.address)} · ${esc(v['mac-address'])}</small></td><td>${esc(siteNames.get(source.gateway.site_id)||'Unassigned site')}<br><small>${esc(source.gateway.hardware_name)}</small></td><td>Active at last read<br><small>${esc(source.readAt)}</small></td><td>${esc(v.uptime||'Unavailable')}<br>${esc(v['bytes-in']??'—')} B in / ${esc(v['bytes-out']??'—')} B out</td><td><button class="btn btn-sm" data-disconnect="${i}" type="button"${canEdit&&source.writable&&!busy?'':' disabled'}>Disconnect</button></td></tr>`}).join('')}</tbody></table></div>`:`<p>${!available.length?'Guest data unavailable. No connected-guest count can be confirmed.':needle?'No reported sessions match your search.':available.length<scoped.length?'No active sessions reported by available sources. Other sources are unavailable.':'No active Hotspot sessions reported.'}</p>`;
      if(scoped.some(s=>!s.ok))results.insertAdjacentHTML('beforeend','<p role="note">Some gateway session data is unavailable. These sources are not counted as empty or offline.</p>');
      if(sessions.some(s=>!s.source.writable)||!canEdit)results.insertAdjacentHTML('beforeend','<p class="muted small">Disconnect is available only with administrator permission and verified router action support.</p>');
      results.querySelectorAll('[data-disconnect]').forEach(b=>b.addEventListener('click',async()=>{
        if(busy||reading||b.disabled||!alive())return;
        const {source,session}=shown[Number(b.dataset.disconnect)],v=session.values||{};
        if(!window.confirm(`Disconnect ${v.user} on ${source.gateway.hardware_name}? They may reconnect with valid access.`))return;
        busy=true;refresh.disabled=true;site.disabled=true;search.disabled=true;draw();status.textContent='Disconnecting and verifying the selected session…';
        let notice='';
        try{const r=await api.post(`/routers/${encodeURIComponent(source.gateway.device_id)}/guests/disconnect`,{id:session.id,mac:v['mac-address'],address:v.address,user:v.user,confirm:true});if(r.verified!==true)throw Error('Router verification was not returned.');notice='Selected session disconnected and router readback verified.'}
        catch(e){notice=failure(e)}
        finally{busy=false;if(alive()){site.disabled=false;search.disabled=false;await reload();if(alive())status.textContent=notice}}
      }));
    }
    async function reload(){
      if(!alive()||busy||reading)return;
      reading=true;refresh.disabled=true;results.querySelectorAll('[data-disconnect]').forEach(b=>b.disabled=true);status.textContent='Refreshing guest sessions across your sites…';
      const next=new Array(gateways.length);let cursor=0;
      async function worker(){while(cursor<gateways.length&&alive()){const index=cursor++,gateway=gateways[index];try{const {inventory}=await api.get(`/routers/${encodeURIComponent(gateway.device_id)}/observations`);if(inventory?.available?.hotspot_sessions!==true)throw Error('Unavailable');next[index]={gateway,ok:true,sessions:list(inventory.objects?.hotspot_sessions),writable:inventory.write_enabled===true,readAt:new Date().toLocaleTimeString()}}catch(_){next[index]={gateway,ok:false,sessions:[]}}}}
      try{await Promise.all(Array.from({length:Math.min(4,gateways.length)},worker));if(alive()){sources=next;draw();status.textContent=gateways.length?'Latest session read completed. Refresh to check for changes.':'No guest session sources are registered for this business.'}}
      finally{reading=false;if(alive()){refresh.disabled=false;device.disabled=busy}}
    }
    site.addEventListener('change',()=>{deviceOptions(device.value);if(!device.value)sessionStorage.removeItem('mm_selected_router');if(site.value)sessionStorage.setItem('mm_selected_site',site.value);draw()});
    device.addEventListener('change',()=>{if(device.value)sessionStorage.setItem('mm_selected_router',device.value);else sessionStorage.removeItem('mm_selected_router');draw()});search.addEventListener('input',draw);refresh.addEventListener('click',reload);
    await reload();
    return ()=>{stopped=true};
  }

  async function mountObserved(host,kind,routers,api,canEdit) {
    if(kind==='guests')return mountGuests(host,routers,api,canEdit);
    const guests=kind==='guests';let revision=0,busy=false;
    const gateways=list(routers).filter(r=>r.deployment_layer==='MAIN_GATEWAY');
    host.innerHTML=`<section class="panel"><div class="panel-body"><h2>${guests?'Current Hotspot guests':'Discover devices on your site'}</h2><p>${guests?'Sessions and usage are read from the selected RouterBOARD. Disconnecting ends a session; it does not permanently block a guest.':'Devices reported by your router. Some network segments may not appear.'}</p><label>Site RouterBOARD<select class="input" data-observed-router><option value="">${gateways.length?'Choose a RouterBOARD':'Connect a RouterBOARD in Magic Hub first'}</option>${gateways.map(r=>`<option value="${esc(r.device_id)}">${esc(r.hardware_name)}</option>`).join('')}</select></label><button class="btn" type="button" data-refresh disabled>${guests?'Refresh guests':'Discover devices'}</button><p data-observed-status role="status"></p><div data-observed-results></div><p class="muted small"><a href="#gateways">Magic Hub connection</a>${guests?' · <a href="#vouchers">Internet vouchers and plans</a>':' · <a href="#managed-devices">Advanced managed-device records</a>'}</p></div></section>`;
    const select=host.querySelector('[data-observed-router]'),button=host.querySelector('[data-refresh]'),status=host.querySelector('[data-observed-status]'),results=host.querySelector('[data-observed-results]');
    async function reload(){
      const id=select.value,stamp=++revision;results.innerHTML='';button.disabled=true;
      if(!id){status.textContent='Choose a RouterBOARD first.';return}
      status.textContent='Reading the selected RouterBOARD…';
      try{
        const {inventory}=await api.get(`/routers/${encodeURIComponent(id)}/observations`);
        if(!host.isConnected||stamp!==revision)return;
        const now=new Date().toLocaleString();
        if(guests){
          if(inventory.available?.hotspot_sessions!==true)throw Error('Hotspot session information is unavailable. No empty or online state can be confirmed.');
          const sessions=list(inventory.objects?.hotspot_sessions);
          results.innerHTML=sessions.length?`<div class="table-scroll"><table><thead><tr><th>Guest</th><th>Address</th><th>Status</th><th>Time / usage</th><th>Action</th></tr></thead><tbody>${sessions.map((s,index)=>{const v=s.values||{};return `<tr><td>${esc(v.user)}<small class="muted">${esc(v['mac-address'])}</small></td><td>${esc(v.address)}</td><td>Active at last read</td><td>${esc(v.uptime)}<br>${esc(v['bytes-in'])} B in / ${esc(v['bytes-out'])} B out</td><td><button class="btn btn-sm" type="button" data-disconnect="${index}"${canEdit&&inventory.write_enabled===true?'':' disabled'}>Disconnect</button></td></tr>`}).join('')}</tbody></table></div>`:'<p>No active Hotspot sessions were reported by this RouterBOARD.</p>';
          results.querySelectorAll('[data-disconnect]').forEach(b=>b.addEventListener('click',async()=>{
            if(busy||b.disabled||select.value!==id)return;
            const session=sessions[Number(b.dataset.disconnect)],v=session.values;
            if(!window.confirm(`Disconnect ${v.user} from this Hotspot session? They may reconnect with valid access.`))return;
            busy=true;select.disabled=true;button.disabled=true;b.disabled=true;status.textContent='Disconnecting and verifying the selected session…';
            try{const r=await api.post(`/routers/${encodeURIComponent(id)}/guests/disconnect`,{id:session.id,mac:v['mac-address'],address:v.address,user:v.user,confirm:true});if(!host.isConnected)return;if(r.verified!==true)throw Error('Router verification was not returned.');await reload();if(host.isConnected)status.textContent='Selected session disconnected and router readback verified.'}
            catch(e){if(host.isConnected)status.textContent=failure(e)}finally{busy=false;if(host.isConnected){select.disabled=false;button.disabled=!select.value;if(b.isConnected)b.disabled=false}}
          }));
          status.textContent=`Read ${now}. ${inventory.write_enabled===true?'':'Guest actions are held until router acceptance.'}`;
        }else{
          const sources=['discovery_leases','arp','neighbors'];
          if(!sources.some(k=>inventory.available?.[k]===true))throw Error('Device discovery tables are unavailable. No empty network can be confirmed.');
          const rows=discoveryRows(inventory);
          let labels=[],labelsFailed=false;
          try{labels=list((await api.get(`/routers/${encodeURIComponent(id)}/discovery-labels`)).labels)}catch(_){labelsFailed=true}
          if(!host.isConnected||stamp!==revision)return;
          const byMAC=new Map(labels.map(l=>[l.mac,l]));
          results.innerHTML=rows.length?`<div class="table-scroll"><table><thead><tr><th>Device name / type</th><th>IP / MAC (discovered)</th><th>Observation</th></tr></thead><tbody>${rows.map((r,index)=>{const label=byMAC.get(r.mac);return `<tr><td>${canEdit&&/^([0-9A-F]{2}:){5}[0-9A-F]{2}$/.test(r.mac)?`<form data-label="${index}"><label>Device name<input class="input" name="name" maxlength="120" required value="${esc(label?.name||r.name)}"></label><label>Device type<select class="input" name="kind">${[['other','Choose type'],['switch','Switch'],['ap','Access point'],['computer','Computer'],['printer','Printer'],['controller','Controller']].map(([value,text])=>`<option value="${value}"${label?.kind===value?' selected':''}>${text}</option>`).join('')}</select></label><button class="btn btn-sm" type="submit">Save name &amp; type</button><span role="status"></span></form>`:esc(label?.name||r.name)}<small class="muted">Router reports: ${esc(r.type)}</small></td><td>${esc(r.ip)}<br>${esc(r.mac)}</td><td>${esc(r.state)}<br><small>${esc(r.sources.join(' · '))}</small></td></tr>`}).join('')}</tbody></table></div>`:'<p>No devices were reported in the available discovery tables.</p>';
          results.querySelectorAll('[data-label]').forEach(form=>form.addEventListener('submit',async event=>{
            event.preventDefault();const button=form.querySelector('button'),message=form.querySelector('[role="status"]');
            if(button.disabled||!form.reportValidity()||select.value!==id)return;
            button.disabled=true;message.textContent='Saving…';
            try{
              const row=rows[Number(form.dataset.label)],name=form.elements.name.value.trim(),kind=form.elements.kind.value;
              const saved=await api.put(`/routers/${encodeURIComponent(id)}/discovery-labels`,{mac:row.mac,name,kind});
              if(!list(saved.labels).some(l=>l.mac===row.mac&&l.name===name&&l.kind===kind))throw Error('Saved name and type were not confirmed.');
              if(form.isConnected)message.textContent='Name and type saved. Router settings unchanged.';
            }catch(e){if(form.isConnected)message.textContent=failure(e)}finally{if(button.isConnected)button.disabled=false}
          }));
          status.textContent=`Read ${now}. ${sources.filter(k=>inventory.available?.[k]!==true).length?'Some discovery sources were unavailable; results may be incomplete.':''}`;
          if(labelsFailed)status.textContent+=' Saved device names could not be loaded.';
        }
      }catch(e){if(host.isConnected&&stamp===revision){results.innerHTML='';status.textContent=`${failure(e)} No router configuration was changed.`}}
      finally{if(host.isConnected&&stamp===revision&&!busy)button.disabled=!select.value}
    }
    select.addEventListener('change',()=>{if(!busy)reload()});button.addEventListener('click',()=>{if(!busy)reload()});
    await routerContext(select,gateways,reload);
  }
  return {bindSites,mountObserved,mountGuests,routerContext,discoveryRows};
})();
