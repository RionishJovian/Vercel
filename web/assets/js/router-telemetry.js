/* Read-only router telemetry. Each number keeps its actual observation time.
   Old samples remain visible through reconnects; no readings are synthesized. */
window.MMRouterTelemetry = (() => {
  const esc=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const number=value=>value!==null&&value!==undefined&&value!==''&&Number.isFinite(Number(value))?Number(value):null;
  const age=(at,now=Date.now())=>{const time=Date.parse(at);if(!Number.isFinite(time))return 'Awaiting first reading';const seconds=Math.max(0,Math.floor((now-time)/1000));return seconds<60?`${seconds}s ago`:seconds<3600?`${Math.floor(seconds/60)}m ago`:`${Math.floor(seconds/3600)}h ago`};
  const bytes=value=>{let n=number(value);if(n===null)return 'Measuring';const units=['B','KiB','MiB','GiB','TiB'];let unit=0;while(n>=1024&&unit<units.length-1){n/=1024;unit++}return `${n.toLocaleString(undefined,{maximumFractionDigits:1})} ${units[unit]}`};
  const rate=value=>{const n=number(value);if(n===null)return 'Measuring';return n>=1e9?`${(n/1e9).toFixed(2)} Gbps`:n>=1e6?`${(n/1e6).toFixed(2)} Mbps`:n>=1e3?`${(n/1e3).toFixed(1)} Kbps`:`${Math.round(n)} bps`};
  const uptime=value=>{const n=number(value);if(n===null)return 'Reading';const days=Math.floor(n/86400),hours=Math.floor(n%86400/3600),minutes=Math.floor(n%3600/60);return [days?`${days}d`:'',hours?`${hours}h`:'',minutes?`${minutes}m`:''].filter(Boolean).join(' ')||`${Math.floor(n)}s`};
  function chart(history,field,label){
    const points=(Array.isArray(history)?history:[]).filter(p=>Number.isFinite(Date.parse(p.read_at))).slice(-60);
    const valid=points.filter(p=>number(p[field])!==null);if(!valid.length)return '<p class="telemetry-chart-empty">The graph starts with confirmed readings.</p>';
    const times=points.map(p=>Date.parse(p.read_at)),start=Math.min(...times),span=Math.max(1000,Math.max(...times)-start);
    const paths=[];let path='',previous=null;
    for(const point of points){const value=number(point[field]),time=Date.parse(point.read_at);if(value===null){if(path)paths.push(path);path='';previous=null;continue}const x=4+(time-start)/span*292,y=56-Math.max(0,Math.min(100,value))/100*48;if(previous!==null&&time-previous>45000){if(path)paths.push(path);path=''}path+=`${path?' L':'M'}${x.toFixed(2)},${y.toFixed(2)}`;previous=time}if(path)paths.push(path);
    const last=valid.at(-1),x=4+(Date.parse(last.read_at)-start)/span*292,y=56-Math.max(0,Math.min(100,number(last[field])))/100*48;
    return `<svg class="telemetry-chart" viewBox="0 0 300 64" role="img" aria-label="${esc(label)} from ${valid.length} measured samples"><path class="telemetry-chart-baseline" d="M4,56 L296,56"/>${paths.map(d=>`<path class="telemetry-chart-line" d="${d}"/>`).join('')}<circle cx="${x.toFixed(2)}" cy="${y.toFixed(2)}" r="2.5"/></svg><small>${valid.length} measured sample${valid.length===1?'':'s'} · gaps stay visible</small>`;
  }
  function mount(host,{api,deviceID,deviceName='RouterBOARD'}={}){
    if(!host||!api||!deviceID)return ()=>{};
    const doc=host.ownerDocument,win=doc.defaultView;
    let stopped=false,busy=false,paused=false,timer=null,view=null,error='',retry=15;
    host.innerHTML=`<section class="router-telemetry" aria-label="Router telemetry"><header class="telemetry-heading"><div><span class="network-eyebrow">ROUTER TELEMETRY</span><h3>${esc(deviceName)}</h3><p>Router readings through your saved management connection.</p></div><div class="telemetry-actions"><button class="btn btn-sm" type="button" data-telemetry-refresh>Refresh readings</button><button class="btn btn-sm" type="button" data-telemetry-pause aria-pressed="false">Pause live updates</button></div></header><p class="telemetry-state" role="status" aria-live="polite" data-telemetry-state>Connecting to RouterBOARD…</p><div data-telemetry-body></div><details class="telemetry-details"><summary>Reading details</summary><p data-telemetry-detail></p><p>Received and sent traffic refer to each router interface. Bridge and member counters can overlap, and hardware-offloaded traffic may be outside these counters. Rates start after two confirmed counter readings.</p><a href="#gateways">Connection details</a></details></section>`;
    const refresh=host.querySelector('[data-telemetry-refresh]'),pause=host.querySelector('[data-telemetry-pause]'),body=host.querySelector('[data-telemetry-body]'),state=host.querySelector('[data-telemetry-state]'),detail=host.querySelector('[data-telemetry-detail]');
    const alive=()=>!stopped&&host.isConnected;
    const schedule=()=>{if(timer!==null)win.clearTimeout(timer);timer=null;if(alive()&&!paused&&!doc.hidden)timer=win.setTimeout(load,retry*1000)};
    function times(){
      if(!alive()){dispose();return}
      const sample=view?.snapshot,last=view?.last_success_at;
      let label=paused?'Live updates paused':busy?'Refreshing router readings…':error?error:view?.state==='reconnecting'?'Reconnecting to RouterBOARD':view?.state==='syncing'?'Collecting remaining readings':view?.state==='live'?'Live router readings':'Connecting to RouterBOARD…';
      if(last&&Date.now()-Date.parse(last)>45000&&!paused&&!busy&&!error&&view?.state==='live')label='Retained reading · refreshing connection';
      state.textContent=label+(last?` · last confirmed ${age(last)}`:'');
      state.dataset.state=(!paused&&!error&&view?.state==='live'&&last&&Date.now()-Date.parse(last)<=45000)?'live':'retrying';
      host.querySelectorAll('[data-observed-source]').forEach(node=>{node.textContent=sample?.observed_at?.[node.dataset.observedSource]?`Measured ${age(sample.observed_at[node.dataset.observedSource])}`:'Waiting for first reading'});
    }
    function draw(){
      if(!alive())return;
      const sample=view?.snapshot,resources=sample?.resources||{},cpu=number(resources.cpu_percent),used=number(resources.memory_used_bytes),total=number(resources.memory_total_bytes),memory=used!==null&&total>0?used*100/total:null;
      const card=(name,value,source,extra='')=>`<article class="telemetry-card"><h4>${name}</h4><strong>${esc(value)}</strong><small data-observed-source="${source}"></small>${extra}</article>`;
      body.innerHTML=`<div class="telemetry-cards">${card('CPU load',cpu===null?'Connecting':`${cpu.toFixed(1)}%`,'cpu',chart(view?.history,'cpu_percent','CPU load'))}${card('Memory used',memory===null?'Connecting':`${memory.toFixed(1)}%`,'memory',`<p>${used===null?'Waiting for memory reading':`${esc(bytes(used))} / ${esc(bytes(total))}`}</p>${chart(view?.history,'memory_percent','Memory usage')}`)}${card('Router uptime',uptime(resources.uptime_seconds),'uptime')}${card('Hotspot sessions',number(sample?.hotspot_sessions)===null?'Connecting':String(sample.hotspot_sessions),'hotspot')}${card('DHCP clients',number(sample?.dhcp_bound_leases)===null?'Connecting':String(sample.dhcp_bound_leases),'dhcp')}</div>
        <section class="telemetry-interfaces"><header><h4>Interface traffic</h4><small data-observed-source="interfaces"></small></header>${Array.isArray(sample?.interfaces)&&sample.observed_at?.interfaces?sample.interfaces.length?`<div class="table-scroll"><table><thead><tr><th>Interface</th><th>Link</th><th>Received</th><th>Sent</th><th>Received / sent total</th></tr></thead><tbody>${sample.interfaces.map(row=>`<tr><td>${esc(row.name)}<small>${esc(row.type||'Router interface')}</small></td><td><span class="status ${row.disabled===true?'warn':row.running===true?'ok':'unknown'}">${row.disabled===true?'Disabled':row.running===true?'Running':row.running===false?'Link down':'Reading'}</span></td><td>${esc(rate(row.rx_bps))}</td><td>${esc(rate(row.tx_bps))}</td><td>${esc(bytes(row.rx_bytes))} / ${esc(bytes(row.tx_bytes))}</td></tr>`).join('')}</tbody></table></div>`:'<p>The router reported no interfaces in this reading.</p>':'<p>Connecting to the interface collector. The first rates appear after two readings.</p>'}</section>`;
      detail.textContent=error||view?.reason?'Automatic refresh will retry the saved router connection. Confirmed readings keep their original timestamps.':sample?.issues?.length?'Some collectors are retrying. Each metric retains the time it was actually measured.':'Read-only RouterOS sampling. Recent resource history is retained in this server process; refresh does not change router settings.';
      times();
    }
    async function load(){
      if(!alive()||busy||paused||doc.hidden)return;
      busy=true;refresh.disabled=true;times();
      try{
        const response=await api.get(`/routers/${encodeURIComponent(deviceID)}/telemetry`),next=response?.telemetry;
        if(!alive())return;
        if(response.read_only!==true||response.router_changed!==false||next?.device_id!==deviceID||next.source!=='routeros-ssh'||!['live','syncing','reconnecting','connecting'].includes(next.state)||!Number.isFinite(Date.parse(next.checked_at))||next.snapshot&&(next.snapshot.device_id!==deviceID||!Number.isFinite(Date.parse(next.snapshot.read_at))||!Number.isFinite(Date.parse(next.last_success_at))))throw Error('Telemetry identity could not be confirmed');
        view=next;error='';retry=Math.max(15,Math.min(60,Number(next.retry_after_seconds)||15));
      }catch(err){if(alive()){error=err?.status===403?'Monitoring access needs an administrator':err?.status===401?'Sign in to refresh telemetry':err?.status===404?'Connecting telemetry service':'Reconnecting to RouterBOARD';retry=[401,403,404,503].includes(err?.status)?60:15}}
      finally{busy=false;if(alive()){refresh.disabled=false;draw();schedule()}}
    }
    function visibility(){if(doc.hidden){if(timer!==null)win.clearTimeout(timer);timer=null}else if(!paused)load()}
    function dispose(){if(stopped)return;stopped=true;if(timer!==null)win.clearTimeout(timer);win.clearInterval(ageTimer);observer.disconnect();doc.removeEventListener('visibilitychange',visibility)}
    refresh.addEventListener('click',()=>{if(!busy){if(paused){paused=false;pause.textContent='Pause live updates';pause.setAttribute('aria-pressed','false')}load()}});
    pause.addEventListener('click',()=>{paused=!paused;pause.setAttribute('aria-pressed',String(paused));pause.textContent=paused?'Resume live updates':'Pause live updates';if(paused){if(timer!==null)win.clearTimeout(timer);timer=null;times()}else load()});
    doc.addEventListener('visibilitychange',visibility);
    const observer=new win.MutationObserver(()=>{if(!host.isConnected)dispose()});observer.observe(doc.documentElement,{childList:true,subtree:true});
    const ageTimer=win.setInterval(times,1000);draw();load();return dispose;
  }
  return {mount,chart,age,bytes,rate,uptime};
})();
