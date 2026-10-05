export function mountHubEnrollment(root, {api, escape:esc}) {
  root.querySelectorAll('[data-hub-enrollment]').forEach(container => {
    const form=container.querySelector('form'), status=container.querySelector('[role="status"]');
    const submit=form.querySelector('button[type="submit"]');
    const gateway=container.dataset.hubEnrollment;
    let busy=false;
    const finish=container.querySelector('[data-hub-finish]');
    finish?.addEventListener('click',async()=>{
      if(finish.disabled)return;
      const message=container.querySelector('[data-hub-finish-status]');
      finish.disabled=true;message.textContent='Checking the private connection and verifying router management…';
      try {
        const result=await api.post(`/vps/peers/${encodeURIComponent(gateway)}/finish`,{});
        if(!container.isConnected)return;
        if(result.state!=='management_authenticated')throw Error('unverified');
        message.textContent='Connected. Router management is ready. LAN devices require their own verification.';
      } catch(error) {
        if(container.isConnected)message.textContent=error.data?.code==='hub_handshake_unverified'
          ?'No recent tunnel connection is verified. The router may be offline or the network path may be blocked. Your gateway record is kept; this check does not reset it or require a new script.'
          :error.data?.code==='hub_management_unverified'
          ?'The tunnel has a recent handshake, but router management is not verified. This check does not reset the connection. If setup expired or the app restarted, its management script may need renewal.'
          :error.status===503?'Connection checking is temporarily unavailable. Your gateway record is kept; no connection was confirmed.'
          :'Connection not verified yet. Your gateway record is kept. No reset was performed.';
      }
      finally {if(finish.isConnected)finish.disabled=false;}
    });
    const scriptPanel=(name,title,script)=>{
      if(typeof script!=='string'||!script.trim()||script.length>32768)throw Error('missing script');
      return `<h4>${esc(title)}</h4><textarea class="input mono" rows="9" readonly spellcheck="false" aria-label="${esc(title)}" data-hub-script="${name}">${esc(script)}</textarea><button class="btn btn-sm" type="button" data-hub-copy="${name}">Copy script</button>`;
    };
    container.addEventListener?.('click',async event=>{
      const button=event.target.closest?.('[data-hub-copy]');
      if(!button||!container.contains(button))return;
      const name=button.dataset.hubCopy;
      if(!['bootstrap','setup','disconnect'].includes(name))return;
      const text=container.querySelector(`[data-hub-script="${name}"]`);
      if(!text)return;
      try {await navigator.clipboard.writeText(text.value);button.textContent='Copied';}
      catch {text.focus();text.select();button.textContent='Selected — copy manually';}
    });
    const prepare=container.querySelector('[data-hub-bootstrap]');
    prepare?.addEventListener?.('click',async()=>{
      if(prepare.disabled)return;
      prepare.disabled=true;
      const output=container.querySelector('[data-hub-bootstrap-output]');
      output.textContent='Preparing router-local key instructions…';
      try {
        const result=await api.get(`/vps/peers/${encodeURIComponent(gateway)}/bootstrap`);
        if(!container.isConnected)return;
        if(result.state!=='router_key_required')throw Error('unverified');
        output.innerHTML=scriptPanel('bootstrap','Preparation — paste in WinBox New Terminal',result.bootstrap_script);
      } catch {if(container.isConnected)output.textContent='Preparation is unavailable. Check enrollment availability and gateway-management permission. No router changes were made.';}
      finally {if(prepare.isConnected)prepare.disabled=false;}
    });
    form.addEventListener('submit',async event=>{
      event.preventDefault();
      if(busy || submit.disabled || !form.reportValidity())return;
      const values=new FormData(form);
      if(values.get('confirm_enrollment')!=='on'){status.textContent='Confirm that you authorize enrolling this gateway.';return;}
      busy=true;submit.disabled=true;status.textContent='Creating or verifying the Hub allocation…';
      try {
        const result=await api.post(`/vps/peers/${encodeURIComponent(gateway)}/quick-enroll`,{
          management_rest_port:Number(values.get('management_rest_port')),
          confirm_enrollment:values.get('confirm_enrollment')==='on'
        });
        if(!container.isConnected)return;
        if(result.state!=='hub_peer_enrolled'||result.router_configuration_required!==true||result.one_paste!==true||result.automatic_management!==true)throw Error('unverified');
        status.innerHTML=`<strong>One-paste script ready</strong>
          <p>Copy the entire block into WinBox → New Terminal and press Enter within 30 minutes. It installs the private tunnel and a dedicated management account. Then select Finish connection below.</p>${scriptPanel('setup','Connection — paste in WinBox New Terminal',result.router_script)}
          <dl><dt>Assigned address</dt><dd><code>${esc(result.tunnel_address)}</code></dd>
          <dt>Hub public key</dt><dd><code>${esc(result.hub_public_key||'Not reported')}</code></dd>
          <dt>Endpoint</dt><dd><code>${esc(result.endpoint||'Not reported')}</code></dd>
          <dt>Allowed addresses</dt><dd><code>${esc(Array.isArray(result.allowed_ips)?result.allowed_ips.join(', '):'Not reported')}</code></dd></dl>
          <p>Finish connection verifies the tunnel and router before enabling management. Keep this script private.</p>`;
      } catch(error) {
        if(!container.isConnected)return;
        status.textContent=error.data?.code==='hub_router_key_unavailable'?'This gateway already has a reserved tunnel key, but its original installation script is no longer available. The existing connection was preserved; no replacement key was installed.':
          error.status===503?'Enrollment preparation is unavailable. No connection was confirmed; your gateway record is preserved.':
          error.status===403?'Your account needs gateway-management permission and an active plan.':
          'One-paste enrollment was not confirmed. Retry once; no router changes were made.';
      } finally {
        busy=false;if(submit.isConnected)submit.disabled=false;
      }
    });
  });
}
