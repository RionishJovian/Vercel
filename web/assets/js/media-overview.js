const esc=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const number=n=>typeof n==='number'&&Number.isFinite(n)&&n>=0?n.toLocaleString():'Unavailable';
const percent=n=>typeof n==='number'&&Number.isFinite(n)&&n>=0&&n<=100?`${n.toFixed(1)}%`:'Unavailable';
const bytes=n=>typeof n==='number'&&Number.isFinite(n)&&n>=0?`${(n/1073741824).toLocaleString(undefined,{maximumFractionDigits:1})} GiB`:'Unavailable';
const card=(title,body)=>`<section class="panel"><div class="panel-body"><h3>${title}</h3>${body}</div></section>`;
export function dashboardCards(d={}) {
 const rows=items=>items.map(([label,value])=>`<p>${label}: <strong>${value}</strong></p>`).join('');
 const meter=(label,n)=>`<p>${label}: <strong>${percent(n)}</strong></p>${typeof n==='number'&&Number.isFinite(n)&&n>=0&&n<=100?`<meter min="0" max="100" value="${n}" aria-label="${label}" style="width:100%"></meter>`:''}`;
 const counts=d.counts||{},host=d.host||{},date=new Date(d.collected_at),collected=Number.isFinite(date.getTime())?esc(date.toLocaleString()):'Unavailable';
 const playback=Array.isArray(d.playback)?d.playback.slice(0,100):[];
 const volumes=Array.isArray(host.volumes)?host.volumes.slice(0,20):[];
 return `<p class="muted">Collected ${collected}. This is a snapshot; refresh for current readings.</p><div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,280px),1fr));gap:16px">`+
 card('Server',rows([['Name',esc(d.server?.ServerName)||'Unavailable'],['Version',esc(d.server?.Version)||'Unavailable'],['Server OS',esc(d.operating_system)||'Unavailable']]))+
 card('Library catalog',rows([['Movies',number(counts.MovieCount)],['Series',number(counts.SeriesCount)],['Episodes',number(counts.EpisodeCount)],['Songs',number(counts.SongCount)],['Albums',number(counts.AlbumCount)]]))+
 card('Active sessions',rows([['Users',number(d.active_users)],['Sessions / devices',number(d.active_sessions)],['Currently playing',d.active_sessions==null?'Unavailable':number(playback.length)]])+ '<p class="muted small">Sessions active within the last two minutes, not all registered users.</p>')+
 card('Media computer',meter('Computer CPU',host.cpu_percent)+meter('Computer memory',host.memory_percent)+rows([['Memory capacity',bytes(host.memory_total_bytes)],['Computer OS',esc(host.os)||'Unavailable']])+'<p class="muted small">Whole-computer measurements, not Jellyfin process usage.</p>')+
 card('Now playing',d.active_sessions==null?'<p>Session information unavailable.</p>':playback.length?playback.map(p=>`<article><h4>${esc(p.title)||'Untitled media'}</h4><p>${esc(p.user)||'Guest'} · ${esc(p.device)||'Device'} · ${p.paused?'Paused':'Playing'}</p><p>${esc(({DirectPlay:'Direct play',DirectStream:'Direct stream',Transcode:'Transcoding'})[p.method]||'Playback method unavailable')}${p.transcoding_percent==null?'':` · Transcoding progress ${percent(p.transcoding_percent)}`}</p></article>`).join(''):'<p>No active playback in this snapshot.</p>')+
 card('Media drives',volumes.length?volumes.map(v=>`<article><h4>${esc(v.label)}</h4>${rows([['Capacity',bytes(v.total_bytes)],['Used by all files',typeof v.total_bytes==='number'&&typeof v.free_bytes==='number'&&v.free_bytes<=v.total_bytes?bytes(v.total_bytes-v.free_bytes):'Unavailable'],['Free',bytes(v.free_bytes)]])}</article>`).join(''):'<p>No supported local media drive measured yet. Add a library to measure its drive. Network shares and unreadable volumes are not reported.</p>')+'</div>';
}

export function mountOverview(workspace,api,isActive) {
 workspace.innerHTML='<section class="panel"><div class="panel-body"><h2>Media overview</h2><p>Read real figures from your paired Jellyfin computer. No media files are uploaded.</p><button class="btn btn-primary" id="media-snapshot">Refresh dashboard</button><p role="status" id="media-snapshot-status"></p></div></section><div id="media-snapshot-data"></div>';
 const button=workspace.querySelector('#media-snapshot'),status=workspace.querySelector('#media-snapshot-status'),output=workspace.querySelector('#media-snapshot-data');
 button.onclick=async()=>{
  button.disabled=true;status.textContent='Checking your media connection…';
  try {
   const connection=await api.get('/media/connection');if(!isActive())return;
   if(connection.revoked_at||Date.parse(connection.expires_at)<=Date.now()||!(Date.parse(connection.last_seen_at)>Date.now()-30000)) {status.textContent='Connect your media computer in the Connection tab first.';return;}
   output.innerHTML='';const id=crypto.randomUUID();await api.post('/media/jobs',{id,operation:'dashboard',payload:{}});
   const deadline=Date.now()+125000;
   while(isActive()&&Date.now()<deadline){
    const job=await api.get('/media/jobs/'+id);if(!isActive())return;
    if(job.state==='succeeded') {if(!job.result?.dashboard)throw new Error('The connector did not return dashboard information.');output.innerHTML=dashboardCards(job.result.dashboard);status.textContent='Dashboard snapshot loaded.';return;}
    if(['failed','expired','cancelled'].includes(job.state))throw new Error('Dashboard request '+job.state+'. Check or update the local connector, then refresh.');
    status.textContent='Collecting readings from your media computer…';await new Promise(resolve=>setTimeout(resolve,2000));
   }
   if(isActive())status.textContent='Dashboard readings were not confirmed. Refresh the connection before trying again.';
  }catch(e){if(isActive())status.textContent=e?.status===404?'This server does not support media dashboards yet.':(e?.message||'Dashboard readings unavailable.');}
  finally{if(isActive())button.disabled=false;}
 };
}
