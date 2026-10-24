/* Shared navigation only. Existing renderers, routes and API permissions stay in place. */
window.MMWorkspace = (() => {
  const router = ['gateways','sites','routers','managed-devices','topology','fleet','internet','network','wan','pppoe','wifi','existing','backups','incidents','security','scans','events'];
  const maintenance = ['backups','incidents','security','scans','events'];
  const network = ['internet','network','wan','pppoe','wifi','existing'];
  function group(route) {
    if (router.includes(route)) return 'gateways';
    if (route === 'portal') return 'vouchers';
    if (route === 'jellyfin-portal') return 'media';
    return route;
  }
  const titles = {gateways:'Magic Hub',vouchers:'Vouchers',media:'Jellyfin',revenue:'Revenue',store:'My pass',overview:'Overview',guests:'Guests'};
  function title(route) { return titles[group(route)] || ''; }
  function navigation(route, allowed = () => true) {
    const workspace = group(route);
    let links = [], childLinks = [];
    let selected = route;
    if (workspace === 'gateways') {
      links = [['gateways','Router'],['sites','Site'],['routers','Devices'],['fleet','Health'],['backups','Maintenance']];
      if (maintenance.includes(route)) {
        selected = 'backups';
        childLinks = [['backups','Backups'],['incidents','Alerts'],['security','Security'],['scans','Scans'],['events','History']];
      } else if (route === 'topology') selected = 'routers';
      else if (route === 'managed-devices') selected = 'routers';
      else if (network.includes(route)) {
        selected = 'gateways';
        childLinks = [['internet','Setup'],['network','New network'],['wan','Internet'],['existing','LAN & Hotspot'],['wifi','Wi-Fi'],['pppoe','Subscribers']];
      }
      if (['routers','managed-devices','topology'].includes(route)) childLinks = [['routers','Discovery'],['topology','Map'],['managed-devices','Saved devices']];
    } else if (workspace === 'vouchers') links = [['vouchers','Batches & shops'],['portal','Guest page']];
    else if (workspace === 'media') links = [['media','Dashboard'],['jellyfin-portal','Guest page']];
    if (!links.length) return '';
    const render = (items, current, label) => `<nav class="workspace-nav" aria-label="${label}">${items.filter(([key]) => allowed(key)).map(([key,label]) => `<a href="#${key}"${key === current ? ' aria-current="page"' : ''}>${label}</a>`).join('')}</nav>`;
    return render(links, selected, `${title(route)} sections`) + (childLinks.length ? render(childLinks, route, 'Section tools') : '');
  }
  function mountVouchers(host, initial) {
    if (!host?.querySelector('[data-voucher-panel]')) return;
    const panels = [...host.querySelectorAll('[data-voucher-panel]')];
    const bar = document.createElement('nav');
    bar.className = 'workspace-nav voucher-nav';
    bar.setAttribute('aria-label', 'Voucher tasks');
    const tasks = [['stock','Stock'],['create','Create'],['shops','Shops'],['print','Print']];
    bar.innerHTML = tasks.filter(([key]) => panels.some(p => p.dataset.voucherPanel === key)).map(([key,label]) => `<button class="btn" type="button" data-voucher-tab="${key}" aria-pressed="false">${label}</button>`).join('');
    host.prepend(bar);
    function choose(key) {
      if (!panels.some(p => p.dataset.voucherPanel === key)) key = 'stock';
      // Preserve the DOM: tab changes keep form edits and any pending request.
      panels.forEach(p => { p.hidden = p.dataset.voucherPanel !== key; });
      bar.querySelectorAll('button').forEach(b => b.setAttribute('aria-pressed', String(b.dataset.voucherTab === key)));
      try { sessionStorage.setItem('mm_voucher_section', key); } catch (_) {}
      if (key === 'print') host.querySelector('#pl-bname')?.dispatchEvent(new Event('input', {bubbles:true}));
    }
    bar.querySelectorAll('button').forEach(b => b.addEventListener('click', () => choose(b.dataset.voucherTab)));
    let saved = ''; try { saved = sessionStorage.getItem('mm_voucher_section'); } catch (_) {}
    choose(initial || saved || 'stock');
  }
  return {group,title,navigation,mountVouchers};
})();
