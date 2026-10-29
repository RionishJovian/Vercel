/* Site/device workspace. All device facts come from authorized API reads.
   Existing typed network controls own review, backup, apply and verification. */
window.MMDeviceWorkspace = (() => {
  const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const list = value => Array.isArray(value) ? value : [];
  const catalog = [
    {id:'wan', group:'Network', label:'Internet connections', description:'Primary and backup internet providers', workspace:'wan', kind:'wan', sources:['dhcp_clients','pppoe_clients','routes']},
    {id:'pppoe-client', group:'Network', label:'ISP account', description:'PPPoE username and connection settings', workspace:'wan', kind:'pppoe_client', sources:['pppoe_clients']},
    {id:'dhcp', group:'Network', label:'LAN & DHCP', description:'Automatic client addresses and lease times', workspace:'existing', kind:'dhcp', sources:['dhcp']},
    {id:'pool', group:'Network', label:'Address pools', description:'Addresses available to your clients', workspace:'existing', kind:'pool', sources:['pools']},
    {id:'dns', group:'Network', label:'Client gateway & DNS', description:'Gateway and DNS supplied to clients', workspace:'existing', kind:'dhcp_network', sources:['dhcp_networks']},
    {id:'reservations', group:'Network', label:'Reserved addresses', description:'Keep a fixed address for a client', workspace:'existing', kind:'lease', sources:['leases']},
    {id:'wifi', group:'Wireless', label:'Wi-Fi networks', description:'Onboard SSIDs and security', workspace:'wifi', kind:'wifi', sources:['wifi','wireless']},
    {id:'hotspot', group:'Guest access', label:'Hotspot services', description:'Guest internet services on this gateway', workspace:'existing', kind:'hotspot', sources:['hotspots']},
    {id:'login', group:'Guest access', label:'Guest login', description:'Login methods and Hotspot profiles', workspace:'existing', kind:'hotspot_profile', sources:['hotspot_profiles']},
    {id:'limits', group:'Guest access', label:'Bandwidth limits', description:'Upload and download limits for clients', workspace:'existing', kind:'queue', sources:['queues']},
    {id:'subscribers', group:'Subscriber internet', label:'Subscriber accounts', description:'PPPoE customer logins and accounts', workspace:'pppoe', kind:'subscriber', sources:['subscribers']},
    {id:'plans', group:'Subscriber internet', label:'Subscriber plans', description:'Address allocation and subscriber speeds', workspace:'pppoe', kind:'ppp_profile', sources:['ppp_profiles']},
    {id:'services', group:'Subscriber internet', label:'PPPoE services', description:'Manage existing subscriber services', workspace:'pppoe', kind:'pppoe_service', sources:['pppoe_servers']},
  ];
  const getStored = key => { try { return sessionStorage.getItem(key) || ''; } catch (_) { return ''; } };
  const store = (key, value) => { try { value ? sessionStorage.setItem(key,value) : sessionStorage.removeItem(key); } catch (_) {} };
  const unassigned = '__unassigned__';
  const modes = ['monitor','guest','config'];
  const siteOf = router => router.site_id || unassigned;
  const errorText = error => error?.data?.error || error?.message || 'The device did not return settings.';

  async function resolveSites(routers, sites, api) {
    const known=new Set(list(sites).map(site=>site.site_id)), result=[...routers];let cursor=0;
    async function worker() {
      while(cursor<result.length) {
        const index=cursor++, device=result[index];
        if(known.has(device.site_id)||device.deployment_layer!=='MAIN_GATEWAY'||(device.vendor&&String(device.vendor).toLowerCase()!=='mikrotik'))continue;
        try {
          const assignment=await api.get(`/vps/peers/${encodeURIComponent(device.device_id)}/site`);
          if(known.has(assignment?.site_id))result[index]={...device,site_id:assignment.site_id};
          else result[index]={...device,site_id:null,site_assignment_unchecked:!!assignment?.site_id};
        } catch (_) { result[index]={...device,site_id:null,site_assignment_unchecked:true}; }
      }
    }
    await Promise.all(Array.from({length:Math.min(4,result.length)},worker));return result;
  }
  async function mount(host, {api, canEdit=false, voucherEndpoint='/voucher-inventory', network=window.MMNetwork, icon=window.MMTheme?.icon} = {}) {
    let version=0, busy=false, reading=false, inventory=null, capabilities=null, message='', fetched=false;
    let disposeTelemetry=null,telemetryDevice='';
    let routers=[], sites=[], peers=[], voucherRead={state:'idle',rows:[]};
    let selectedSite='', selectedDevice='', tab=modes.includes(getStored('mm_device_workspace_mode'))?getStored('mm_device_workspace_mode'):'monitor', taskID='wan';
    const alive = () => host.isConnected;
    const glyph = (name,size=18) => icon ? icon(name,size) : '';
    const scopedRouters = () => routers.filter(router => siteOf(router)===selectedSite);
    const router = () => scopedRouters().find(item => item.device_id===selectedDevice);
    const supported = task => !!inventory && task.sources.some(key => inventory.available?.[key]===true);
    const isMikrotik = item => !item.vendor || String(item.vendor).toLowerCase()==='mikrotik';
    host.innerHTML='<p role="status">Loading sites and devices…</p>';
    try {
      // A failed site/device read must not be presented as an empty account.
      const [siteResponse, deviceResponse] = await Promise.all([
        api.get('/sites'), api.get('/routers'),
      ]);
      if (!alive()) return;
      sites=list(siteResponse.sites || siteResponse); routers=list(deviceResponse.routers || deviceResponse);
      peers=[];
    } catch (error) {
      if (alive()) host.innerHTML=`<section class="panel"><div class="panel-body" role="alert"><h2>Sites and devices could not be loaded</h2><p>${esc(errorText(error))}</p><button class="btn" data-workspace-retry type="button">Try again</button></div></section>`;
      host.querySelector('[data-workspace-retry]')?.addEventListener('click',()=>mount(host,{api,canEdit,network,icon}));
      return;
    }
    routers=await resolveSites(routers,sites,api);
    if(!alive())return;
    const knownSites = new Set(sites.map(site => site.site_id));
    // Retain records with no site without making them an invented location.
    const hasUnassigned = routers.some(item => !item.site_id || !knownSites.has(item.site_id));
    routers=routers.map(item => item.site_id && !knownSites.has(item.site_id) ? {...item,site_id:null} : item);
    const validSites = new Set([...knownSites,...(hasUnassigned?[unassigned]:[])]);
    const remembered = routers.find(item => item.device_id===getStored('mm_selected_router'));
    const rememberedSite=getStored('mm_selected_site');
    selectedSite=remembered ? siteOf(remembered) : validSites.has(rememberedSite) ? rememberedSite : validSites.size===1 ? [...validSites][0] : '';
    const devices=scopedRouters();
    selectedDevice=devices.find(item=>item.device_id===remembered?.device_id)?.device_id || (devices.length===1 ? devices[0].device_id : '');
    remember(); frame();
    if (selectedDevice) await readDevice();

    function remember() {
      store('mm_selected_site',selectedSite); store('mm_selected_router',selectedDevice);
      store('mm_topology_router','');
    }
    function setBusy(value) {
      busy=value;
      host.querySelectorAll('[data-context-site],[data-context-device],[data-workspace-tab],[data-workspace-tool],[data-workspace-refresh],[data-workspace-link]').forEach(control=>{
        if ('disabled' in control) control.disabled=value;
        if (control.tagName==='A') { control.setAttribute('aria-disabled',String(value)); control.tabIndex=value ? -1 : 0; }
      });
      // A write completing must not enable tools missing from this device.
      if (!value) host.querySelectorAll('[data-workspace-tool]').forEach(button=>{button.disabled=!supported(catalog.find(task=>task.id===button.dataset.workspaceTool));});
    }
    function frame() {
      const device=router();
      host.innerHTML=`<section class="device-workspace" aria-label="Site and device workspace">
        <header class="device-workspace-context">
          <div class="device-workspace-site"><span class="network-eyebrow">SITE WORKSPACE</span><h2>Manage your network</h2><p>Choose a site and device once. Monitor its state or configure its supported services.</p></div>
          <div class="device-context-fields"><label for="workspace-site">Site<select class="input" id="workspace-site" data-context-site><option value="">Choose a site</option>${sites.map(site=>`<option value="${esc(site.site_id)}"${site.site_id===selectedSite?' selected':''}>${esc(site.name)}</option>`).join('')}${hasUnassigned?`<option value="${unassigned}"${selectedSite===unassigned?' selected':''}>Unassigned or unchecked device records</option>`:''}</select></label>
          <label for="workspace-device">Device<select class="input" id="workspace-device" data-context-device${!selectedSite?' disabled':''}><option value="">Choose a device</option>${scopedRouters().map(item=>`<option value="${esc(item.device_id)}"${item.device_id===selectedDevice?' selected':''}>${esc(item.hardware_name || 'Unnamed device')}</option>`).join('')}</select></label></div>
        </header>
        <div class="device-workspace-identity" data-workspace-identity></div>
        <div class="device-workspace-tabs" role="tablist" aria-label="Device workspace mode"><button class="btn" id="workspace-monitor-tab" type="button" role="tab" data-workspace-tab="monitor" aria-controls="workspace-pane">Monitor</button><button class="btn" id="workspace-guest-tab" type="button" role="tab" data-workspace-tab="guest" aria-controls="workspace-pane">Guest access</button><button class="btn" id="workspace-config-tab" type="button" role="tab" data-workspace-tab="config" aria-controls="workspace-pane">Config</button><span>Current values first. Changes are reviewed before applying.</span></div>
        <div class="device-workspace-body"><aside class="device-settings-nav" aria-label="Device settings"><label class="device-settings-search" for="workspace-search">Find a setting<input class="input" id="workspace-search" type="search" placeholder="Search WAN, Wi-Fi, guest login…" data-settings-search></label><nav data-settings-menu></nav><div class="device-workspace-shortcuts"><span class="network-eyebrow">RELATED WORKSPACES</span><a href="#network" data-workspace-link>New guest network ${glyph('network',16)}</a><a href="#topology" data-workspace-link>Site topology ${glyph('network',16)}</a><a href="#routers" data-workspace-link>Devices & discovery ${glyph('router',16)}</a><a href="#portal" data-workspace-link>Portal & payment settings ${glyph('ticket',16)}</a></div></aside>
        <section id="workspace-pane" class="device-workspace-pane" role="tabpanel" aria-live="polite"></section></div>
      </section>`;
      host.querySelector('[data-context-site]').addEventListener('change',async event=>{
        if(busy)return;
        ++version; selectedSite=event.target.value;
        const devices=scopedRouters(); selectedDevice=devices.length===1?devices[0].device_id:'';
        reading=false;inventory=null;capabilities=null;voucherRead={state:'idle',rows:[]};message='';fetched=false;remember();frame();
        if(selectedDevice)await readDevice();
      });
      host.querySelector('[data-context-device]').addEventListener('change',async event=>{
        if(busy)return;
        ++version;selectedDevice=event.target.value;reading=false;inventory=null;capabilities=null;voucherRead={state:'idle',rows:[]};message='';fetched=false;remember();frame();
        if(selectedDevice)await readDevice();
      });
      host.querySelectorAll('[data-workspace-tab]').forEach(button=>button.addEventListener('click',()=>{
        if(busy)return;tab=button.dataset.workspaceTab;paint();
      }));
      host.querySelectorAll('[data-workspace-link]').forEach(link=>link.addEventListener('click',event=>{
        if(busy){event.preventDefault();return;}remember();
      }));
      host.querySelector('[data-settings-search]').addEventListener('input',filterMenu);
      paint();
    }
    async function readDevice() {
      const device=router(), stamp=++version;
      if(!device)return;
      if(!isMikrotik(device)){reading=false;message='This device is a saved record. RouterOS controls require a MikroTik device with verified management access.';fetched=true;paint();return;}
      reading=true;inventory=null;voucherRead={state:'idle',rows:[]};message='Reading current device settings…';paint();
      const [settings,capability,fleet] = await Promise.allSettled([
        api.get(`/routers/${encodeURIComponent(device.device_id)}/network/inventory`),
        api.get(`/routers/${encodeURIComponent(device.device_id)}/capabilities`),
        api.get('/fleet/health'),
      ]);
      if(!alive()||stamp!==version||device.device_id!==selectedDevice)return;
      reading=false;peers=fleet.status==='fulfilled'?list(fleet.value?.peers):[];
      capabilities=capability.status==='fulfilled'?(capability.value.capabilities || capability.value):null;
      inventory=settings.status==='fulfilled'?settings.value.inventory:null;
      fetched=true;
      message=inventory?'':`Settings could not be read. ${settings.status==='rejected'?errorText(settings.reason):'No inventory was returned.'}`;
      // Keep the user's chosen setting when switching devices. If it is not
      // available, show that fact rather than silently configuring another one.
      paint();
    }
    function paint() {
      if(!alive())return;
      if(!(tab==='monitor'&&telemetryDevice===selectedDevice&&host.querySelector('[data-router-telemetry]'))){disposeTelemetry?.();disposeTelemetry=null;telemetryDevice='';}
      store('mm_device_workspace_mode',tab);
      host.querySelector('.device-workspace').dataset.mode=tab;
      const device=router(), peer=peers.find(item=>item.device_id===selectedDevice);
      const state=({up:'Online',stale:'Stale',offline:'Offline',unknown:'Unknown'})[peer?.state] || 'Unknown';
      const stateClass=({up:'ok',stale:'warn',offline:'bad'})[peer?.state] || 'unknown';
      host.querySelector('[data-workspace-identity]').innerHTML=device?`<span class="device-workspace-glyph">${glyph('router',26)}</span><div><h3>${esc(device.hardware_name || 'Unnamed device')}</h3><p>${esc(capabilities?.board_name || device.device_model || 'Model not reported')}<span>RouterOS ${esc(capabilities?.routeros_version || 'not read')}</span></p></div><div class="device-workspace-read"><span class="status ${stateClass}">${state}</span><small>Management reachability</small></div><button class="btn btn-sm" data-workspace-refresh type="button"${reading?' disabled':''}>${reading?'Reading…':'Refresh device'}</button>`:'<p>Select a site and device to open its workspace.</p>';
      host.querySelector('[data-workspace-refresh]')?.addEventListener('click',()=>{if(!busy)readDevice();});
      host.querySelectorAll('[data-workspace-tab]').forEach(button=>{
        const active=button.dataset.workspaceTab===tab;
        button.setAttribute('aria-selected',String(active));button.classList.toggle('btn-primary',active);
        button.tabIndex=active?0:-1;
        button.onkeydown=event=>{
          if(busy||!['ArrowLeft','ArrowRight','Home','End'].includes(event.key))return;
          event.preventDefault();const index=modes.indexOf(tab);tab=event.key==='Home'?modes[0]:event.key==='End'?modes.at(-1):modes[(index+(event.key==='ArrowRight'?1:modes.length-1))%modes.length];paint();
          host.querySelector(`[data-workspace-tab="${tab}"]`).focus();
        };
      });
      menu();
      const pane=host.querySelector('#workspace-pane');
      pane.setAttribute('aria-labelledby',`workspace-${tab}-tab`);
      if(!selectedSite){pane.innerHTML='<div class="device-workspace-placeholder"><h3>Start with a site</h3><p>Select the location you want to manage. Device controls stay scoped to that site.</p><a class="btn" href="#sites">Manage sites</a></div>';return;}
      if(!device){pane.innerHTML=`<div class="device-workspace-placeholder"><h3>${scopedRouters().length?'Choose a device':'No device assigned'}</h3><p>${scopedRouters().length?'Select a device above to inspect its current settings.':'Connect a RouterBOARD to this site through Magic Hub.'}</p><a class="btn" href="#gateways">Open Magic Hub</a></div>`;return;}
      if(tab==='monitor'&&window.MMRouterTelemetry&&isMikrotik(device)){monitor(pane);return;}
      if(tab==='guest'){guestAccess(pane);if(voucherRead.state==='idle')readVouchers();return;}
      if(!inventory){pane.innerHTML=`<div class="device-workspace-placeholder" role="${fetched?'alert':'status'}"><h3>${fetched?'Device settings unavailable':'Reading device'}</h3><p>${esc(message)}</p><p>Use Magic Hub to check its saved management connection.</p><a class="btn" href="#gateways">Open Magic Hub</a></div>`;return;}
      if(tab==='monitor'){monitor(pane);return;}
      const task=catalog.find(item=>item.id===taskID);
      if(!supported(task)){pane.innerHTML=`<div class="device-workspace-placeholder"><h3>${esc(task.label)}</h3><p>This setting was not available in the latest read from this device. Choose an available setting or refresh the device.</p></div>`;return;}
      pane.innerHTML=`<header class="device-setting-heading"><span class="network-eyebrow">${esc(task.group)}</span><h3>${esc(task.label)}</h3><p>${esc(task.description)}</p></header><div data-device-setting></div>`;
      const kind=task.id==='wifi' && !inventory.available?.wifi ? 'wireless' : task.kind;
      network.mount(pane.querySelector('[data-device-setting]'),task.workspace,[device],canEdit,api,{
        kind,compact:true,initialInventory:inventory,onBusy:setBusy,onInventory:value=>{inventory=value;},
      });
    }
    function menu() {
      const groups=[...new Set(catalog.map(task=>task.group))];
      host.querySelector('[data-settings-menu]').innerHTML=groups.map(group=>`<section data-settings-group><h3>${esc(group)}</h3>${catalog.filter(task=>task.group===group).map(task=>`<button type="button" class="device-setting-item${tab==='config'&&task.id===taskID?' is-active':''}" data-workspace-tool="${task.id}" data-setting-search="${esc(`${group} ${task.label} ${task.description}`.toLowerCase())}"${!supported(task)?' disabled':''}${tab==='config'&&task.id===taskID?' aria-current="true"':''}><span>${esc(task.label)}</span><small>${!inventory?'Awaiting device read':!supported(task)?'Not available':'View settings'}</small></button>`).join('')}</section>`).join('');
      host.querySelectorAll('[data-workspace-tool]').forEach(button=>button.addEventListener('click',()=>{
        if(busy||button.disabled)return;taskID=button.dataset.workspaceTool;tab='config';paint();
      }));
      filterMenu();
    }
    function filterMenu() {
      const query=host.querySelector('[data-settings-search]')?.value.trim().toLowerCase() || '';
      host.querySelectorAll('[data-workspace-tool]').forEach(button=>{button.hidden=!!query&&!button.dataset.settingSearch.includes(query);});
      host.querySelectorAll('[data-settings-group]').forEach(group=>{group.hidden=![...group.querySelectorAll('[data-workspace-tool]')].some(button=>!button.hidden);});
    }
    async function readVouchers() {
      const device=router(), stamp=version;
      if(!device || voucherRead.state==='loading')return;
      voucherRead={state:'loading',rows:[]};paint();
      try {
        const response=await api.get(voucherEndpoint);
        if(!Array.isArray(response?.vouchers))throw Error('No voucher inventory returned');
        if(!alive()||stamp!==version||device.device_id!==selectedDevice)return;
        voucherRead={state:'ok',rows:response.vouchers.filter(row=>row.device_id===device.device_id&&(voucherEndpoint!=='/vouchers'||row.issue_batch_id||row.plan_key==='guest-internet')),truncated:response.may_be_truncated===true};
      } catch (_) {
        if(!alive()||stamp!==version||device.device_id!==selectedDevice)return;
        voucherRead={state:'error',rows:[]};
      }
      if(tab==='guest')paint();
    }
    function guestAccess(pane) {
      const device=router(), snapshot=inventory||{}, records=key=>list(snapshot.objects?.[key]);
      const available=key=>snapshot.available?.[key]===true;
      const enabled=record=>['no','false',false].includes(record.values?.disabled);
      const hotspots=available('hotspots')?records('hotspots').filter(enabled):[];
      const dhcp=available('dhcp')?records('dhcp').filter(enabled):[];
      const matching=hotspots.filter(h=>dhcp.some(d=>d.values?.interface && d.values.interface===h.values?.interface));
      const networkKnown=available('hotspots')&&available('dhcp');
      const sessions=available('hotspot_sessions')?records('hotspot_sessions'):null;
      const codesKnown=voucherRead.state==='ok';
      const installed=row=>!row.issue_batch_id||row.install_state==='verified';
      const unused=voucherRead.rows.filter(row=>installed(row)&&['generated','handed_out','unused'].includes(row.state||row.status)).length;
      const provisioning=voucherRead.rows.filter(row=>row.state==='provisioning'||!installed(row)).length;
      const activated=voucherRead.rows.filter(row=>installed(row)&&['activated','used'].includes(row.state||row.status)).length;
      const card=(number,title,state,detail,actions)=>`<li class="guest-setup-step"><span class="guest-setup-number" aria-hidden="true">${number}</span><div><h4>${title}</h4><span class="status unknown">${esc(state)}</span><p>${esc(detail)}</p><div class="workspace-actions">${actions}</div></div></li>`;
      const link=(href,label)=>`<a class="btn btn-sm" href="${href}" data-guest-flow-link>${label}</a>`;
      const setting=(id,label)=>`<button class="btn btn-sm" type="button" data-guest-setting="${id}">${label}</button>`;
      const timestamp=snapshot.read_at?new Date(snapshot.read_at):null;
      const readLabel=timestamp&&!Number.isNaN(timestamp.getTime())?timestamp.toLocaleString():'Time not reported';
      pane.innerHTML=`<header class="device-setting-heading"><span class="network-eyebrow">GUEST ACCESS WORKFLOW</span><h3>From router setup to connected guests</h3><p>RouterBOARD runs captive login and enforces provisioned access limits. Mega Magic manages setup, vouchers and business records.</p></header>
        <ol class="guest-setup-steps" aria-label="Guest access setup steps">
        ${card(1,'Site & RouterBOARD',selectedSite===unassigned?'Site needed':'Device selected',`${sites.find(site=>site.site_id===selectedSite)?.name || 'Unassigned site'} · ${device.hardware_name || 'Unnamed device'}. Management reachability is shown above.`,link('#gateways',selectedSite===unassigned?'Check site assignment':'Connection details'))}
        ${card(2,'Guest Wi-Fi & captive login',!networkKnown?'Not read':matching.length?'Services reported':'Review network setup',!networkKnown?'DHCP or Hotspot information is unavailable.':matching.length?`${matching.length} enabled Hotspot service(s) share an interface with an enabled DHCP service. Test a guest to confirm access.`:'No enabled DHCP and Hotspot pair was reported on the same interface. Review existing settings before creating a network.',setting('wifi','Wi-Fi settings')+setting('hotspot','Hotspot settings')+link('#network','New guest network'))}
        ${card(3,'Vouchers & access limits',codesKnown?`${unused} unused · ${activated} activated`:voucherRead.state==='loading'?'Reading vouchers…':'Voucher inventory unavailable',codesKnown?`${provisioning} awaiting installation verification. Issuance, verified router installation and first use are separate states. Check batch verification before printing. ${voucherRead.truncated?'Inventory is limited; these counts may be incomplete.':'Printing does not activate access.'}`:'Voucher counts cannot be confirmed. You can retry without changing router settings.',link('#vouchers','Open vouchers')+(!codesKnown&&voucherRead.state==='error'?'<button class="btn btn-sm" type="button" data-voucher-retry>Retry voucher read</button>':''))}
        ${card(4,'Connected guests',sessions===null?'Not read':`${sessions.length} active at last read`,sessions===null?'Hotspot sessions are unavailable. This does not confirm an empty network.':'Read the session, time left and traffic from this RouterBOARD. An activated voucher does not prove a current connection.',link('#guests','View live guests'))}
        </ol>
        <section class="guest-setup-sessions"><h4>Latest Hotspot sessions</h4><p class="muted small">Router snapshot: ${esc(readLabel)}. Refresh device to read again.</p>${sessions===null?'<p role="status">Session reporting unavailable.</p>':sessions.length?`<div class="table-scroll"><table><thead><tr><th>Login / voucher</th><th>Guest IP</th><th>Time left</th><th>Download / upload</th></tr></thead><tbody>${sessions.slice(0,10).map(session=>{const v=session.values||{};return `<tr><td>${esc(v.user || 'Not reported')}</td><td>${esc(v.address || 'Not reported')}</td><td>${esc(v['session-time-left'] || 'Not reported')}</td><td>${esc(window.MMRevamp?.bytes?.(v['bytes-out']) ?? v['bytes-out'] ?? 'Not reported')} ↓ / ${esc(window.MMRevamp?.bytes?.(v['bytes-in']) ?? v['bytes-in'] ?? 'Not reported')} ↑</td></tr>`}).join('')}</tbody></table></div>${sessions.length>10?'<p>Showing the first 10 reported sessions. Open live guests for the full list.</p>':''}`:'<p>No active Hotspot sessions reported in this snapshot.</p>'}</section>`;
      pane.querySelectorAll('[data-guest-flow-link]').forEach(anchor=>anchor.addEventListener('click',event=>{if(busy){event.preventDefault();return;}remember();}));
      pane.querySelectorAll('[data-guest-setting]').forEach(button=>{
        const task=catalog.find(item=>item.id===button.dataset.guestSetting);
        button.disabled=busy||!supported(task);
        button.addEventListener('click',()=>{if(button.disabled||busy)return;taskID=task.id;tab='config';paint();});
      });
      pane.querySelector('[data-voucher-retry]')?.addEventListener('click',()=>{voucherRead={state:'idle',rows:[]};readVouchers();});
    }
    function monitor(pane) {
      if(window.MMRouterTelemetry){
        if(telemetryDevice===selectedDevice&&pane.querySelector('[data-router-telemetry]'))return;
        telemetryDevice=selectedDevice;
        pane.innerHTML='<div data-router-telemetry></div><div class="device-monitor-foot"><p>RouterBOARD enforces your network rules. Telemetry only reads its current state.</p><button class="btn" type="button" data-open-device-config>Open configuration</button></div>';
        disposeTelemetry=window.MMRouterTelemetry.mount(pane.querySelector('[data-router-telemetry]'),{api,deviceID:selectedDevice,deviceName:router()?.hardware_name||'RouterBOARD'});
        pane.querySelector('[data-open-device-config]').addEventListener('click',()=>{tab='config';paint();});return;
      }

      const interfaces=list(inventory.interfaces), running=interfaces.filter(item=>item.running===true).length;
      const records=key=>list(inventory.objects?.[key]);
      const quantity=key=>inventory.available?.[key]===true?String(records(key).length):'Unavailable';
      const lastRead=inventory.read_at?new Date(inventory.read_at):null;
      const readLabel=lastRead&&!Number.isNaN(lastRead.getTime())?lastRead.toLocaleString():'Time not reported';
      pane.innerHTML=`<header class="device-setting-heading"><span class="network-eyebrow">DEVICE MONITOR</span><h3>Current network state</h3><p>Snapshot from this device. Last read: ${esc(readLabel)}.</p></header><dl class="device-monitor-summary"><div><dt>Interfaces running</dt><dd>${running} / ${interfaces.length}</dd></div><div><dt>DHCP services</dt><dd>${quantity('dhcp')}</dd></div><div><dt>Hotspot services</dt><dd>${quantity('hotspots')}</dd></div><div><dt>Connected Hotspot guests</dt><dd>${quantity('hotspot_sessions')}</dd></div></dl>
        <section class="device-monitor-ports"><h4>Ports and interfaces</h4>${interfaces.length?`<div class="table-scroll"><table><thead><tr><th>Interface</th><th>Type</th><th>Reported state</th></tr></thead><tbody>${interfaces.map(item=>`<tr><td>${esc(item.name)}</td><td>${esc(item.type || 'Not reported')}</td><td><span class="status ${item.running===true?'ok':'unknown'}">${item.running===true?'Running':item.running===false?'Not running':'Not reported'}</span></td></tr>`).join('')}</tbody></table></div>`:'<p>No interfaces were returned.</p>'}</section>
        <div class="device-monitor-foot"><p>${inventory.write_enabled===true&&canEdit?'Supported changes are available in Config.':'Current settings are available to read. Router changes remain disabled for this account or deployment.'}</p><div class="workspace-actions"><button class="btn" type="button" data-open-guest-access>Guest access setup</button><button class="btn" type="button" data-open-device-config>Open configuration</button></div></div>`;
      pane.querySelector('[data-open-guest-access]').addEventListener('click',()=>{tab='guest';paint();});
      pane.querySelector('[data-open-device-config]').addEventListener('click',()=>{tab='config';paint();});
    }
  }
  return {mount,resolveSites};
})();
