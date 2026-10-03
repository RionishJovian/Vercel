export function mountHubEnrollment(root, {api, escape:esc}) {
  root.querySelectorAll('[data-hub-enrollment]').forEach(container => {
    const form=container.querySelector('form'), status=container.querySelector('[role="status"]');
    const submit=form.querySelector('button[type="submit"]');
    const gateway=container.dataset.hubEnrollment;
    let busy=false;
    const management=container.querySelector('[data-hub-management]');
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
      } catch(error) {if(container.isConnected)message.textContent=error.status===503?'Automatic setup is unavailable on this deployment.':'Connection not verified yet. Run the entire script and retry. Setup expires after 30 minutes or an app restart; generate a new script if needed.';}
      finally {if(finish.isConnected)finish.disabled=false;}
    });
    management?.addEventListener?.('submit',async event=>{
      event.preventDefault();
      const button=management.querySelector('button[type="submit"]'),message=management.querySelector('[role="status"]');
      if(button.disabled||!management.reportValidity())return;
      const fields=new FormData(management);
      if(fields.get('confirm')!=='on')return;
      button.disabled=true;message.textContent='Verifying private, pinned SSH access…';
      try {
        const result=await api.post(`/vps/peers/${encodeURIComponent(gateway)}/authenticate`,{username:fields.get('username'),password:fields.get('password'),port:Number(fields.get('port')),fingerprint:String(fields.get('fingerprint')||'').trim(),confirm:true});
        if(!container.isConnected)return;
        if(result.state!=='management_authenticated')throw Error('unverified');
        message.textContent='Management authenticated and securely saved. LAN devices still need separate verification.';
      } catch {if(container.isConnected)message.textContent='Management was not confirmed. Check the recent Hub handshake, trusted SSH fingerprint, credentials and private firewall rule.';}
      finally {management.querySelector('[name="password"]').value='';if(button.isConnected)button.disabled=false;}
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
        status.textContent=error.status===503?'Enrollment is not enabled on this deployment yet. No connection was confirmed.':
          error.status===403?'Your account needs gateway-management permission and an active plan.':
          'One-paste enrollment was not confirmed. Retry once; no router changes were made.';
      } finally {
        busy=false;if(submit.isConnected)submit.disabled=false;
      }
    });
  });
}
