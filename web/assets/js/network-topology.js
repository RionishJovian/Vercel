/* Read-only topology projection. A forwarding entry is not proof of a direct cable. */
(function (root) {
  'use strict';
  const escape = value => String(value ?? '').replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
  const mac = value => /^([0-9a-f]{2}:){5}[0-9a-f]{2}$/i.test(String(value)) ? String(value).toUpperCase() : '';
  const yes = value => value === 'true' || value === 'yes';
  const asset = name => `/assets/images/topology/${name}`;
  const rows = (inv,key) => inv.available?.[key] === true ? (inv.objects?.[key] || []).map(o=>o.values || {}) : [];
  function project(device, inv = {}, labels = []) {
    const own = new Set(rows(inv,'interfaces').map(v=>mac(v['mac-address'])).filter(Boolean));
    const wireless = new Map(rows(inv,'wifi_interfaces').map(v=>[v.name,v]));
    const devices = new Map();
    const get = v => {
      const address = mac(v['mac-address']);
      if (!address || own.has(address) || (parseInt(address.slice(0,2),16)&1)) return null;
      if (!devices.has(address)) devices.set(address,{id:address,mac:address,name:'Unknown device',ip:'',kind:'device',sources:[],ports:[],state:'Reported',connection:'Attachment unknown',direct:false,upstream:false});
      return devices.get(address);
    };
    for (const key of ['discovery_leases','arp','neighbors','bridge_hosts','wifi_clients','hotspot_sessions']) {
      for (const v of rows(inv,key)) {
        if (key === 'bridge_hosts' && yes(v.local)) continue;
        const n = get(v); if (!n) continue;
        if (!n.sources.includes(key)) n.sources.push(key);
        if (v.address) n.ip = v.address;
        if (v['host-name'] || v.identity) n.name = v.identity || v['host-name'];
        if (v.board || v.platform) n.model = v.board || v.platform;
        if (key === 'neighbors' && v.interface) {n.ports.push(v.interface);n.connection='Neighbour reported';n.direct=true;}
        if (key === 'bridge_hosts' && v['on-interface']) {n.ports.push(v['on-interface']);n.connection='Learned on port';n.direct=true;}
        if (key === 'wifi_clients') {
          const wifi = wireless.get(v.interface) || {};
          n.ports.push(v.interface);n.signal=v.signal;n.state='Wi-Fi associated';n.connection='Wi-Fi';n.direct=true;
          if (String(wifi['configuration.mode']).startsWith('station')) {n.upstream=true;n.kind='upstream';n.name=wifi['configuration.ssid'] || n.name;n.connection='Wi-Fi uplink';}
        }
        if (key === 'hotspot_sessions') n.state='HotSpot session';
      }
    }
    for (const label of labels) {
      const n = devices.get(mac(label.mac)); if (!n || n.upstream) continue;
      if (label.name) n.name=label.name;
      n.kind=({ap:'ap',switch:'switch',computer:'computer',printer:'printer',controller:'device'})[label.kind] || n.kind;
    }
    const all=[...devices.values()].map(n=>({...n,ports:[...new Set(n.ports)]}));
    const board = inv.health?.['board-name'] || device.device_model || '';
    return {root:{id:device.device_id,name:device.hardware_name || 'RouterBOARD',model:board,kind:'router',image:/hAP\s*ax[²2]/i.test(board+' '+device.hardware_name) ? 'hap-ax2.webp':'switch.svg',state:inv.observed_at && (inv.health?.version || Object.values(inv.available || {}).some(Boolean))?'Management reachable':'Not read',sources:['Pinned management connection']},devices:all,observedAt:inv.observed_at,unavailable:Object.entries(inv.available || {}).filter(([,ok])=>!ok).map(([key])=>key)};
  }
  async function mount(host,{api,topology,isCurrent=()=>true}) {
    const devices=[];const collect=n=>{if(n.device)devices.push(n.device);(n.children||[]).forEach(collect);};
    (topology.roots||[]).forEach(collect);devices.push(...(topology.orphans||[]));
    const gateways=devices.filter(d=>d.deployment_layer==='MAIN_GATEWAY' || d.category==='gateway');
    if (!gateways.length) {host.innerHTML='<div class="panel"><div class="empty">No gateway assigned to this site. Open Magic Hub → Router to check its saved site.</div></div>';return;}
    let device=gateways.length===1?gateways[0]:gateways.find(d=>d.device_id===sessionStorage.getItem('mm_selected_router'));
    let revision=0,graph=null,selected='',query='';
    const alive=()=>host.isConnected && isCurrent();
    host.innerHTML=`<section class="nt-map panel" aria-label="Network topology"><div class="nt-toolbar"><div><h2>Site topology</h2><p class="muted small" data-nt-summary>Select a gateway to read connected devices.</p></div>${gateways.length>1?`<label>Router<select class="input" data-nt-router><option value="">Select router</option>${gateways.map(d=>`<option value="${escape(d.device_id)}">${escape(d.hardware_name)}</option>`).join('')}</select></label>`:''}<label class="nt-search">Find device<input type="search" class="input" data-nt-search placeholder="Name, IP or MAC"></label><button class="btn" data-nt-refresh>Refresh map</button></div><p class="nt-message" data-nt-status role="status"></p><div class="nt-layout"><div class="nt-stage" data-nt-stage></div><aside class="nt-inspector" data-nt-inspector aria-live="polite">Select a device to inspect its connection.</aside></div><details class="nt-help"><summary>Connection evidence</summary><p>Solid lines show a Wi-Fi association, neighbour or bridge forwarding entry reported by this router. A bridge entry may include an unseen switch. Dashed lines mean the physical attachment is unknown. DHCP leases and ARP records alone do not prove a device is online. Refresh to obtain a new reading.</p></details></section>`;
    const stage=host.querySelector('[data-nt-stage]'),status=host.querySelector('[data-nt-status]'),inspector=host.querySelector('[data-nt-inspector]'),refresh=host.querySelector('[data-nt-refresh]');
    const card=(n,isRoot=false)=>`<button class="nt-device ${isRoot?'nt-router':''} ${n.id===selected?'is-selected':''}" data-nt-node="${escape(n.id)}" aria-pressed="${n.id===selected}"><span class="nt-image"><img src="${asset(n.image || n.kind+'.svg')}" alt="${isRoot?escape(n.model || 'Router'):escape(n.kind==='device'?'Unidentified device':n.kind)}" width="96" height="96"></span><strong>${escape(n.name)}</strong><span class="nt-state ${/associated|session|reachable/.test(n.state)?'nt-active':''}">${escape(n.state)}</span><small>${escape(n.ip || n.model || n.mac || '')}</small></button>`;
    function inspect() {
      const n=[graph?.root,...(graph?.devices||[])].find(n=>n?.id===selected);if(!n)return;
      inspector.innerHTML=`<h3>${escape(n.name)}</h3><p class="nt-state">${escape(n.state)}</p><dl>${[['Model',n.model],['IP address',n.ip],['MAC address',n.mac],['Connection',n.connection],['Router interface',n.ports?.join(', ')],['Signal',n.signal],['Reading',graph.observedAt?new Date(graph.observedAt).toLocaleString():'Unavailable'],['Sources',n.sources?.map(s=>s.replaceAll('_',' ')).join(', ')]].filter(([,v])=>v).map(([k,v])=>`<dt>${escape(k)}</dt><dd>${escape(v)}</dd>`).join('')}</dl>${n.id===graph.root.id?'<a class="btn" href="#gateways">Open router settings</a>':''}`;
      stage.querySelectorAll('[data-nt-node]').forEach(el=>{const active=el.dataset.ntNode===selected;el.classList.toggle('is-selected',active);el.setAttribute('aria-pressed',String(active));});
    }
    function render() {
      if (!graph)return;
      const filtered=graph.devices.filter(n=>[n.name,n.ip,n.mac,n.model,...n.ports].join(' ').toLowerCase().includes(query));
      const visible=filtered.slice(0,120),up=visible.filter(n=>n.upstream),down=visible.filter(n=>!n.upstream);
      host.querySelector('[data-nt-summary]').textContent=`${device.hardware_name} · ${graph.devices.length} observed devices${filtered.length>120?' · Showing first 120 matches; search to narrow':''}`;
      const branch=n=>`<div class="nt-branch ${n.direct?'':'nt-unplaced'}"><span class="nt-link-label">${escape(n.ports.join(' / ') || n.connection)}</span>${card(n)}</div>`;
      stage.innerHTML=`${up.length?`<div class="nt-upstream">${up.map(branch).join('')}</div>`:''}<div class="nt-root">${card(graph.root,true)}</div><div class="nt-children">${down.map(branch).join('')}</div>${!down.length?'<p class="nt-no-devices muted">No downstream devices in this reading.</p>':''}`;
      stage.querySelectorAll('[data-nt-node]').forEach(el=>el.addEventListener('click',()=>{selected=el.dataset.ntNode;inspect();}));inspect();
    }
    async function read() {
      const current=++revision;if(!device){stage.innerHTML='';refresh.disabled=true;return;}
      refresh.disabled=true;status.textContent='Reading router connections…';
      graph=project(device);selected=device.device_id;render();
      try {
        const id=device.device_id;
        const [response,labels]=await Promise.all([api.get(`/routers/${encodeURIComponent(id)}/observations`),api.get(`/routers/${encodeURIComponent(id)}/discovery-labels`).catch(()=>({labels:[]}))]);
        if(!alive() || current!==revision)return;
        graph=project(device,response.inventory || {},labels.labels || []);render();
        status.textContent=graph.unavailable.length?`Some sources unavailable: ${graph.unavailable.map(s=>s.replaceAll('_',' ')).join(', ')}. The map shows readable sources only.`:'Reading complete · No router settings changed.';
      }catch(e){if(alive()&&current===revision)status.textContent=e?.message || 'Could not read router connections. Check Magic Hub management access.';}
      finally{if(alive()&&current===revision)refresh.disabled=false;}
    }
    host.querySelector('[data-nt-search]').addEventListener('input',e=>{query=e.target.value.trim().toLowerCase();render();});
    host.querySelector('[data-nt-router]')?.addEventListener('change',e=>{device=gateways.find(d=>d.device_id===e.target.value);if(device)sessionStorage.setItem('mm_selected_router',device.device_id);read();});
    if(device){sessionStorage.setItem('mm_selected_router',device.device_id);const picker=host.querySelector('[data-nt-router]');if(picker)picker.value=device.device_id;}
    refresh.addEventListener('click',read);await read();
  }
  root.MMNetworkTopology={project,mount};
})(window);
