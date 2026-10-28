/* Internet inventory only. Dealer accounting never changes router activation.
   No tokens, voucher codes or customer identities are persisted in the browser. */
window.MMVoucherWorkspace = (() => {
  const esc = v => String(v ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const arr = v => Array.isArray(v) ? v : [];
  const names = {all:'All',generated:'Unused',handed_out:'Handed out',provisioning:'Provisioning',activated:'Activated',expired:'Expired',revoked:'Cancelled'};
  const money = v => `${Number(v || 0).toLocaleString()} MMK`;
  const date = v => v ? new Date(v).toLocaleString() : '—';
  // Spreadsheet formulas must not execute when a voucher/contact is exported.
  const csvCell = v => {let s=String(v ?? '');if(/^\s*[=+@-]|^[\t\r\n]/.test(s))s="'"+s;return '"'+s.replaceAll('"','""')+'"'};
  const csv = rows => [['Code','Plan','Site','Status','Dealer','Agreed MMK','Received MMK','First use','Expires'], ...rows.map(v => [v.code,v.plan_label,v.site_name,names[v.state] || v.state,v.dealer_name,v.dealer_price_mmk,v.paid_mmk,v.first_used_at,v.expires_at])].map(r => r.map(csvCell).join(',')).join('\r\n');
  function filtered(rows, filters) {
    const q = (filters.search || '').trim().toLowerCase();
    return rows.filter(v => (!filters.device || v.device_id===filters.device) && (!q || [v.id,v.code,v.plan_label,v.client_mac,v.dealer_name].some(x => String(x || '').toLowerCase().includes(q))) && (!filters.site || v.site_id === filters.site) && (!filters.dealer || v.reseller_id === filters.dealer) && (!filters.state || v.state === filters.state) && (!filters.payment || (v.stock_id && (filters.payment === 'paid' ? v.paid_mmk >= v.dealer_price_mmk : v.paid_mmk < v.dealer_price_mmk))));
  }
  function printCodes(rows, business) {
    // Printing a slip does not mark a code sold, handed out or activated.
    const frame = document.createElement('iframe');
    frame.title = 'Selected internet voucher print sheet'; frame.hidden = true;
    frame.srcdoc = `<!doctype html><html><head><title>Internet vouchers</title><style>@page{size:A4;margin:10mm}body{font:12px system-ui;color:#111}.slips{display:grid;grid-template-columns:repeat(3,1fr);gap:3mm}article{break-inside:avoid;border:1px dashed #999;padding:5mm;text-align:center}strong{display:block;font:700 20px monospace;margin:3mm 0}small{display:block;margin:2mm}h1{font-size:14px}</style></head><body><div class="slips">${rows.map(v => `<article><h1>${esc(business || 'Guest internet')}</h1><strong>${esc(v.code)}</strong><div>${esc(v.plan_label)}</div><small>${esc(v.minutes)} minutes · ${esc(v.data_mb)} MB · ${esc(money(v.price_mmk))}</small><small>Validity starts at first use. One device per code.</small></article>`).join('')}</div></body></html>`;
    frame.onload = () => {frame.contentWindow.addEventListener('afterprint', () => frame.remove(), {once:true});frame.contentWindow.focus();frame.contentWindow.print()};
    document.body.appendChild(frame);
  }
  async function mount(host, {api, canEdit, dealers = [], business = '', routers = []}) {
    if (!host) return;
    host.innerHTML = '<p role="status">Loading internet voucher stock…</p>';
    let rows = [], selected = new Set(), tab = 'inventory', state = '', page = 0, busy = false, notice = '';
    let filters = {search:'',site:'',device:'',dealer:'',payment:''};
    let response;
    try {response = await api.get('/voucher-inventory'); rows = arr(response.vouchers)}
    catch (e) {if(host.isConnected)host.innerHTML = `<section class="panel voucher-workspace"><div class="panel-body"><h2>Internet voucher inventory</h2><p role="alert">${esc(e?.data?.error || 'Inventory could not be loaded. An empty stock count is not confirmed.')}</p><button class="btn" data-retry>Retry inventory</button></div></section>`;host.querySelector('[data-retry]')?.addEventListener('click', () => mount(host,{api,canEdit,dealers,business,routers}));return}
    if (!host.isConnected) return;
    let savedDevice='';try{savedDevice=sessionStorage.getItem('mm_selected_router')||''}catch(_){}
    if(routers.some(router=>router.device_id===savedDevice))filters.device=savedDevice;
    const reload = async () => {
      const r = await api.get('/voucher-inventory'); rows = arr(r.vouchers); response = r; selected.clear(); page = 0;
    };
    const subset = () => filtered(rows,{...filters,state});
    const picked = () => rows.filter(v => selected.has(v.id));
    const download = chosen => {
      const blob = new Blob(['\ufeff',csv(chosen)],{type:'text/csv;charset=utf-8'}), url = URL.createObjectURL(blob), a = document.createElement('a');
      a.href = url; a.download = 'internet-voucher-stock.csv'; a.click(); setTimeout(() => URL.revokeObjectURL(url),1000);
    };
    function draw() {
      if (!host.isConnected) return;
      const scoped = filtered(rows,{...filters,state:''});
      const shown = subset(), pages = Math.max(1,Math.ceil(shown.length/50)); page = Math.min(page,pages-1);
      const visible = shown.slice(page*50,page*50+50), stock = rows.filter(v => v.stock_id);
      host.innerHTML = `<section class="panel voucher-workspace"><div class="panel-body">
        <header class="voucher-heading"><div><p class="network-eyebrow">BUSINESS / INTERNET VOUCHERS</p><h2>Voucher desk</h2><p class="muted small">Codes, dealer stock and money received — with router activation kept separate.</p></div><button class="btn" data-refresh ${busy?'disabled':''}>Refresh</button></header>
        <nav class="voucher-tabs" aria-label="Voucher workspace"><button class="btn ${tab==='inventory'?'btn-primary':''}" data-tab="inventory" aria-pressed="${tab==='inventory'}">Voucher inventory</button><button class="btn ${tab==='dealers'?'btn-primary':''}" data-tab="dealers" aria-pressed="${tab==='dealers'}">Dealers & balances</button><a class="btn" href="#voucher-creation">Create codes</a><a class="btn" href="#dealer-contacts" data-contacts>Dealer contacts</a></nav>
        <div role="status" class="voucher-notice">${esc(notice)}</div>
        ${tab==='inventory'?`<div class="voucher-filters"><label>Find a voucher<input class="input" type="search" data-search value="${esc(filters.search)}" placeholder="Code, serial / ID, plan, MAC or dealer"></label><label>Site<select class="input" data-filter="site"><option value="">All sites</option>${[...new Map(rows.filter(v=>v.site_id).map(v=>[v.site_id,v.site_name])).entries()].map(([id,name])=>`<option value="${esc(id)}" ${filters.site===id?'selected':''}>${esc(name || 'Unnamed site')}</option>`).join('')}</select></label><label>RouterBOARD<select class="input" data-filter="device"><option value="">All gateways</option>${routers.map(router=>`<option value="${esc(router.device_id)}" ${filters.device===router.device_id?'selected':''}>${esc(router.hardware_name || 'Unnamed gateway')}</option>`).join('')}</select></label><label>Dealer<select class="input" data-filter="dealer"><option value="">All dealers</option>${dealers.map(d=>`<option value="${esc(d.id)}" ${filters.dealer===d.id?'selected':''}>${esc(d.shop_name)}</option>`).join('')}</select></label><label>Payment<select class="input" data-filter="payment"><option value="">All payment states</option><option value="due" ${filters.payment==='due'?'selected':''}>Balance due</option><option value="paid" ${filters.payment==='paid'?'selected':''}>Paid / no balance</option></select></label></div>
        <div class="voucher-statuses" aria-label="Voucher status filters">${Object.entries(names).map(([key,title])=>`<button class="btn btn-sm" data-state="${key==='all'?'':key}" aria-pressed="${state===(key==='all'?'':key)}">${title} · ${key==='all'?scoped.length:scoped.filter(v=>v.state===key).length}</button>`).join('')}</div>
        <div class="voucher-bulk"><span>${selected.size} selected · ${shown.length} matching</span><div><button class="btn btn-sm" data-copy ${!selected.size?'disabled':''}>Copy codes</button><button class="btn btn-sm" data-print ${!selected.size?'disabled':''}>Print sheet</button><button class="btn btn-sm" data-export ${!shown.length&&!selected.size?'disabled':''}>Export ${selected.size?'selected':'filtered'} CSV</button>${canEdit?`<button class="btn btn-sm" data-assign ${!selected.size?'disabled':''}>Hand to dealer</button>`:''}<button class="btn btn-sm" data-clear>Clear selection</button><button class="btn btn-sm" data-reset>Reset filters</button></div></div>
        <div class="table-scroll"><table class="voucher-grid"><thead><tr><th><input type="checkbox" data-all aria-label="Select this page" ${visible.length&&visible.every(v=>selected.has(v.id))?'checked':''}></th><th>Code / plan</th><th>Status</th><th>Site / device</th><th>Dealer / balance</th><th>First use / expiry</th><th>Actions</th></tr></thead><tbody>${visible.map(v=>`<tr><td><input type="checkbox" data-select="${esc(v.id)}" aria-label="Select ${esc(v.code)}" ${selected.has(v.id)?'checked':''}></td><td><strong class="mono">${esc(v.code)}</strong><small>${esc(v.plan_label)} · ${esc(v.minutes)} min · ${esc(v.data_mb)} MB</small></td><td><span class="voucher-state" data-state="${esc(v.state)}">${esc(names[v.state]||v.state)}</span></td><td>${esc(v.site_name||'Unassigned site')}<small>${esc(routers.find(router=>router.device_id===v.device_id)?.hardware_name || 'Gateway not labelled')}</small><small class="mono">${esc(v.client_mac||'Not bound to a guest')}</small></td><td>${esc(v.dealer_name||'Business stock')}<small>${v.stock_id?`${esc(money(v.paid_mmk))} received · ${esc(money(v.dealer_price_mmk-v.paid_mmk))} due`:'Not handed out'}</small></td><td>${esc(date(v.first_used_at))}<small>${v.expires_at?esc(date(v.expires_at)):'Starts at first use'}</small></td><td><div class="voucher-row-actions"><button class="btn btn-sm" data-slip="${esc(v.id)}">Slip</button>${canEdit&&v.stock_id?`<button class="btn btn-sm" data-settle="${esc(v.id)}" ${v.paid_mmk>=v.dealer_price_mmk?'disabled':''}>Record payment</button><button class="btn btn-sm" data-return="${esc(v.id)}" ${v.guest_state!=='unused'||v.paid_mmk!==0||v.state==='revoked'?'disabled':''}>Return</button>`:''}</div></td></tr>`).join('')||'<tr><td colspan="7">No internet vouchers match. Business-subscription codes are not listed here.</td></tr>'}</tbody></table></div>
        <div class="voucher-pagination"><button class="btn btn-sm" data-prev ${page===0?'disabled':''}>Previous</button><span>Page ${page+1} / ${pages} · 50 rows per page</span><button class="btn btn-sm" data-next ${page+1>=pages?'disabled':''}>Next</button></div>`:
        `<div class="voucher-totals"><div><strong>${stock.length}</strong><span>Codes currently held by dealers</span></div><div><strong>${esc(money(stock.reduce((n,v)=>n+v.paid_mmk,0)))}</strong><span>Payments recorded on held stock</span></div><div><strong>${esc(money(stock.reduce((n,v)=>n+v.dealer_price_mmk-v.paid_mmk,0)))}</strong><span>Outstanding agreed balance</span></div></div><div class="table-scroll"><table class="voucher-grid"><thead><tr><th>Dealer</th><th>Held codes</th><th>Activated</th><th>Received</th><th>Balance</th><th></th></tr></thead><tbody>${dealers.map(d=>{const held=stock.filter(v=>v.reseller_id===d.id);return `<tr><td>${esc(d.shop_name)}<small>${esc(d.contact_name)}</small></td><td>${held.length}</td><td>${held.filter(v=>v.state==='activated').length}</td><td>${esc(money(held.reduce((n,v)=>n+v.paid_mmk,0)))}</td><td>${esc(money(held.reduce((n,v)=>n+v.dealer_price_mmk-v.paid_mmk,0)))}</td><td><button class="btn btn-sm" data-dealer="${esc(d.id)}">View stock</button></td></tr>`}).join('')||'<tr><td colspan="6">Add a dealer contact below, then hand out selected unused codes.</td></tr>'}</tbody></table></div><p class="muted small">Dealers are your business contacts, not app logins. Returns preserve history. These balances are not a sales or revenue report.</p>`}
        <p class="muted small">${filters.device?'Showing voucher records for the selected RouterBOARD. ':''}Activated means the voucher's activation was recorded; it is not proof of a currently connected guest. Printing, hand-out and settlement do not activate a code. Unused codes have no expiry until first use.</p>${response.may_be_truncated?'<p role="alert">Only the newest 5,000 codes are loaded. Counts and exports are incomplete.</p>':''}
        <dialog class="voucher-dialog" aria-labelledby="voucher-modal-title"><form method="dialog" data-dialog-form><header><h3 data-dialog-title id="voucher-modal-title"></h3><button class="btn btn-sm" value="cancel" formnovalidate aria-label="Close dialog">Close</button></header><div data-dialog-body></div><p data-dialog-message role="status"></p><footer><button class="btn" value="cancel" formnovalidate>Cancel</button><button class="btn btn-primary" type="submit" value="confirm">Confirm</button></footer></form></dialog>
      </div></section>`;
      const on=(s,event,fn)=>host.querySelectorAll(s).forEach(el=>el.addEventListener(event,fn));
      on('[data-tab]','click',e=>{tab=e.currentTarget.dataset.tab;draw()});
      on('[data-dealer]','click',e=>{filters.dealer=e.currentTarget.dataset.dealer;tab='inventory';selected.clear();page=0;draw()});
      on('[data-search]','input',e=>{const pos=e.target.selectionStart;filters.search=e.target.value;page=0;selected.clear();draw();const input=host.querySelector('[data-search]');input.focus();input.setSelectionRange(pos,pos)});
      on('[data-filter]','change',e=>{
        const key=e.target.dataset.filter;filters[key]=e.target.value;
        const chosen=routers.find(router=>router.device_id===filters.device);
        if(key==='site'&&filters.site&&chosen?.site_id!==filters.site)filters.device='';
        if(key==='device'&&chosen&&filters.site&&chosen.site_id!==filters.site)filters.site='';
        page=0;selected.clear();draw();
      });
      on('button[data-state]','click',e=>{state=e.currentTarget.dataset.state;page=0;selected.clear();draw()});
      on('[data-select]','change',e=>{e.target.checked?selected.add(e.target.dataset.select):selected.delete(e.target.dataset.select);draw()});
      on('[data-all]','change',e=>{visible.forEach(v=>e.target.checked?selected.add(v.id):selected.delete(v.id));draw()});
      on('[data-clear]','click',()=>{selected.clear();draw()});
      on('[data-reset]','click',()=>{filters={search:'',site:'',device:'',dealer:'',payment:''};state='';page=0;selected.clear();draw()});
      on('[data-prev]','click',()=>{page--;draw()});on('[data-next]','click',()=>{page++;draw()});
      on('[data-export]','click',()=>download(selected.size?picked():subset()));
      on('[data-copy]','click',async()=>{try{await navigator.clipboard.writeText(picked().map(v=>v.code).join('\n'));notice='Selected codes copied. Treat voucher exports as sensitive.'}catch(_){notice='Clipboard permission unavailable. Use CSV or print instead.'}draw()});
      on('[data-print]','click',()=>printCodes(picked(),business));
      on('[data-slip]','click',e=>printCodes(rows.filter(v=>v.id===e.currentTarget.dataset.slip),business));
      on('[data-refresh]','click',async()=>{if(busy)return;busy=true;draw();try{await reload();notice='Inventory refreshed.'}catch(_){notice='Refresh failed. Previous data remains; it is not a current reading.'}finally{busy=false;draw()}});
      on('[data-contacts]','click',e=>{e.preventDefault();document.getElementById('dealer-contacts')?.scrollIntoView({behavior:'smooth'})});
      host.querySelector('a[href="#voucher-creation"]')?.addEventListener('click',e=>{e.preventDefault();document.getElementById('voucher-creation')?.scrollIntoView({behavior:'smooth'})});
      on('[data-assign]','click',()=>{
        const chosen=picked();
        if(chosen.length>1000||chosen.some(v=>v.state!=='generated'||v.stock_id)){notice='Select at most 1,000 unused, unassigned internet codes.';draw();return}
        dialog('Hand unused stock to a dealer',`<p>${chosen.length} codes. No sale, payment or router activation is recorded.</p><label>Dealer<select class="input" name="dealer" required><option value="">Choose active dealer</option>${dealers.filter(d=>d.active).map(d=>`<option value="${esc(d.id)}">${esc(d.shop_name)}</option>`).join('')}</select></label><label>Agreed price per code (MMK)<input class="input" name="price" type="number" min="0" max="1000000000" step="1" required></label>`,f=>api.post('/voucher-dealer-stock/assign',{reseller_id:f.get('dealer'),voucher_ids:chosen.map(v=>v.id),unit_price_mmk:Number(f.get('price')),confirm:true}));
      });
      on('[data-return]','click',e=>{const v=rows.find(v=>v.id===e.currentTarget.dataset.return);dialog('Return unused dealer stock',`<p>Return <strong>${esc(v.code)}</strong> from ${esc(v.dealer_name)} to business stock? No guest access is changed. Paid or used codes cannot be returned here.</p>`,()=>api.post(`/voucher-dealer-stock/${encodeURIComponent(v.stock_id)}/return`,{confirm:true}))});
      on('[data-settle]','click',e=>{const v=rows.find(v=>v.id===e.currentTarget.dataset.settle),balance=v.dealer_price_mmk-v.paid_mmk;dialog('Record payment already received',`<p>${esc(v.dealer_name)} · ${esc(v.code)} · balance ${esc(money(balance))}. This records cash or a transfer you already received; it does not charge anyone or activate access.</p><label>Received amount (MMK)<input class="input" name="amount" type="number" min="1" max="${balance}" step="1" required></label><label>Unique receipt / transfer reference<input class="input" name="reference" minlength="3" maxlength="120" required autocomplete="off"></label>`,f=>api.post(`/voucher-dealer-stock/${encodeURIComponent(v.stock_id)}/settle`,{amount_mmk:Number(f.get('amount')),reference:f.get('reference').trim(),confirm:true}))});
    }
    function dialog(title,html,submit) {
      if(busy)return;
      const d=host.querySelector('dialog'),form=d.querySelector('form'),msg=d.querySelector('[data-dialog-message]');
      d.querySelector('[data-dialog-title]').textContent=title;d.querySelector('[data-dialog-body]').innerHTML=html;
      d.addEventListener('cancel',e=>{if(busy)e.preventDefault()});
      form.onsubmit=async e=>{
        if(e.submitter?.value!=='confirm')return;
        e.preventDefault();if(busy||!form.reportValidity())return;
        const data=new FormData(form);busy=true;form.querySelectorAll('button,input,select').forEach(el=>el.disabled=true);msg.textContent='Saving…';
        try{await submit(data);notice='Saved in business records. No router configuration or guest activation was changed.';try{await reload()}catch(_){notice+=' Inventory refresh failed; refresh before another action.'}d.close();busy=false;draw()}
        catch(e){msg.textContent=e?.data?.error||'Not confirmed. Refresh before retrying. Reuse the same payment reference for the same receipt.';busy=false;form.querySelectorAll('button,input,select').forEach(el=>el.disabled=false)}
      };
      d.showModal();
    }
    draw();
  }
  return {mount, filtered, csv};
})();
