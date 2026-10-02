export function mountHubEnrollment(root, {api, escape:esc}) {
  root.querySelectorAll('[data-hub-enrollment]').forEach(container => {
    const form=container.querySelector('form'), status=container.querySelector('[role="status"]');
    const submit=form.querySelector('button[type="submit"]');
    const gateway=container.dataset.hubEnrollment;
    let busy=false;
    const management=container.querySelector('[data-hub-management]');
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
      const publicKey=String(values.get('router_public_key')||'').trim();
      if(!/^[A-Za-z0-9+/]{43}=$/.test(publicKey)){status.textContent='Paste the WireGuard public key, not a password or private key.';return;}
      busy=true;submit.disabled=true;status.textContent='Creating or verifying the Hub allocation…';
      try {
        const result=await api.post(`/vps/peers/${encodeURIComponent(gateway)}/enroll`,{
          router_public_key:publicKey,management_rest_port:Number(values.get('management_rest_port')),
          confirm_enrollment:values.get('confirm_enrollment')==='on'
        });
        if(!container.isConnected)return;
        if(result.state!=='hub_peer_enrolled'||result.router_configuration_required!==true)throw Error('unverified');
        const panels=scriptPanel('setup','Connection — paste in WinBox New Terminal',result.router_script)+
          `<details><summary>Disconnect this tunnel</summary><p>Disconnects this enrollment locally, preserving the key for reconnection. This does not delete the Hub allocation.</p>${scriptPanel('disconnect','Disconnect script',result.disconnect_script)}</details>`;
        status.innerHTML=`<strong>Hub allocation confirmed — paste the connection script next</strong>
          <p>Review the script, then paste the entire block into WinBox New Terminal. It does not enable management services or change existing WAN/LAN/firewall settings.</p>${panels}
          <dl><dt>Assigned address</dt><dd><code>${esc(result.tunnel_address)}</code></dd>
          <dt>Hub public key</dt><dd><code>${esc(result.hub_public_key||'Not reported')}</code></dd>
          <dt>Endpoint</dt><dd><code>${esc(result.endpoint||'Not reported')}</code></dd>
          <dt>Allowed addresses</dt><dd><code>${esc(Array.isArray(result.allowed_ips)?result.allowed_ips.join(', '):'Not reported')}</code></dd></dl>
          <h4>3. Verify the tunnel</h4><p>Use “Check Magic Hub” to check the handshake. LAN access is not yet verified. Router management also requires configured credentials, a pinned SSH host key and a tunnel-scoped firewall rule; enrollment alone does not enable configuration tools.</p>`;
      } catch(error) {
        if(!container.isConnected)return;
        status.textContent=error.status===503?'Enrollment is not enabled on this deployment yet. No connection was confirmed.':
          error.status===403?'Your account needs gateway-management permission and an active plan.':
          'Enrollment was not confirmed. Retry with the same public key; do not create a replacement key.';
      } finally {
        busy=false;if(submit.isConnected)submit.disabled=false;
      }
    });
  });
}
