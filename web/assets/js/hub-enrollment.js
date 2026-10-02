export function mountHubEnrollment(root, {api, escape:esc}) {
  root.querySelectorAll('[data-hub-enrollment]').forEach(container => {
    const form=container.querySelector('form'), status=container.querySelector('[role="status"]');
    const submit=form.querySelector('button[type="submit"]');
    const gateway=container.dataset.hubEnrollment;
    let busy=false;
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
        status.innerHTML=`<strong>Hub allocation confirmed — router setup still required</strong>
          <p>In WinBox, add the assigned address to your WireGuard interface, then add the Hub as a peer using these values. Keep the private key on your router.</p>
          <dl><dt>Assigned address</dt><dd><code>${esc(result.tunnel_address)}</code></dd>
          <dt>Hub public key</dt><dd><code>${esc(result.hub_public_key||'Not reported')}</code></dd>
          <dt>Endpoint</dt><dd><code>${esc(result.endpoint||'Not reported')}</code></dd>
          <dt>Allowed addresses</dt><dd><code>${esc(Array.isArray(result.allowed_ips)?result.allowed_ips.join(', '):'Not reported')}</code></dd></dl>
          <p>Set persistent keepalive to 25 seconds. Then use “Check Magic Hub” to check the handshake. LAN access is not yet verified.</p>`;
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
