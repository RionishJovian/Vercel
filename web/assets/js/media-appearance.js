// Hosted design publishing. Playback/paid access remains disabled until enforced.
export const defaults = {
  name: "Magic Media", headline: "A little movie magic.", subtitle: "Your next favourite story is waiting.",
  accent: "#eeeeee", theme: "charcoal", price: 1500, duration: 24,
  movies: true, series: true, music: true, featured: true,
  posterDensity: "comfortable",
  layout: "shelves", background: "", tiles: {}, posters: {},
};
const tileDefaults = {
  movies: {title:"Movies", caption:"Stories worth staying in for", image:""},
  series: {title:"TV Shows", caption:"One more episode", image:""},
  music: {title:"Music", caption:"Find your rhythm", image:""},
};
const posterNames = ["The quiet coast","Beyond the stars","A city after rain","Golden afternoons"];
export const templates = {
  cinema: {label:"Cinema", description:"Charcoal, wide library covers and compact landscape shelves.", theme:"charcoal", accent:"#eeeeee", layout:"shelves", posterDensity:"compact"},
  editorial: {label:"Editorial", description:"Warm white, quiet typography and large portrait covers.", theme:"paper", accent:"#202020", layout:"lounge", posterDensity:"comfortable"},
  screening: {label:"Screening room", description:"Solid slate, generous artwork and wide media shelves.", theme:"slate", accent:"#d8dde2", layout:"shelves", posterDensity:"comfortable"},
};
const imageValue = value => typeof value === "string" && value.length <= 300000 && /^data:image\/(?:jpeg|png|webp);base64,[A-Za-z0-9+/]+={0,2}$/.test(value) ? value : "";
export function applyTemplate(value, name) {
  return normalize({...value, ...(templates[name] || templates.cinema)});
}
const escape = value => String(value).replace(/[&<>"']/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
export function normalize(value = {}) {
  const data = {...defaults, ...value};
  for (const key of ["name","headline","subtitle"]) data[key] = String(data[key] ?? defaults[key]).slice(0,160);
  data.accent = /^#[0-9a-f]{6}$/i.test(data.accent) ? data.accent : defaults.accent;
  const legacyThemes = {forest:"charcoal", midnight:"slate", plum:"charcoal"};
  data.theme = ["charcoal","paper","slate"].includes(data.theme) ? data.theme : legacyThemes[data.theme] || defaults.theme;
  data.posterDensity = ["comfortable","compact"].includes(data.posterDensity) ? data.posterDensity : "comfortable";
  data.layout = ["shelves","lounge"].includes(data.layout) ? data.layout : defaults.layout;
  data.background = imageValue(data.background);
  data.libraries=Array.isArray(value.libraries)?[...new Set(value.libraries.filter(id=>typeof id==='string' && /^(?:[0-9a-f]{32}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$/.test(id)))].slice(0,16):[];
  const payment=value?.payment;
  data.payment=payment && typeof payment==='object' ? {bank_name:String(payment.bank_name||'').slice(0,100),bank_account:String(payment.bank_account||'').slice(0,100),bank_holder:String(payment.bank_holder||'').slice(0,160),mmqr:imageValue(payment.mmqr)} : null;
  data.tiles = Object.fromEntries(Object.entries(tileDefaults).map(([key, fallback]) => {
    const tile = value?.tiles?.[key] || {};
    return [key, {title:String(tile.title ?? fallback.title).slice(0,80), caption:String(tile.caption ?? fallback.caption).slice(0,160), image:imageValue(tile.image)}];
  }));
  data.posters = Object.fromEntries(posterNames.map((title,i) => [i,{title:String(value?.posters?.[i]?.title ?? title).slice(0,80), image:imageValue(value?.posters?.[i]?.image)}]));
  for (const key of ["price","duration"]) data[key] = Number.isFinite(Number(data[key])) && Number(data[key]) > 0 ? Math.min(Number(data[key]),1000000) : defaults[key];
  for (const key of ["movies","series","music","featured"]) data[key] = data[key] === true;
  return data;
}

function guestBase(input) {
  const d = normalize(input);
  const palette = {charcoal:{page:"#111111",panel:"#181818",ink:"#f1f1f1",muted:"#b2b2b2",line:"#343434",art:"#242424"},paper:{page:"#f4f2ed",panel:"#fffefa",ink:"#202020",muted:"#60605a",line:"#d9d6cf",art:"#e8e5de"},slate:{page:"#191e24",panel:"#232930",ink:"#f2f3f4",muted:"#b5bdc5",line:"#424951",art:"#30373f"}}[d.theme];
  const categories = Object.keys(tileDefaults).filter(key => d[key]);
  const categoryHTML = categories.map(key => {const tile=d.tiles[key]; return `<article class="category"><div class="category-cover">${tile.image ? `<img src="${tile.image}" alt="">` : `<span class="cover-label">${escape(tile.title)}</span>`}</div><div class="tile-text"><strong>${escape(tile.title)}</strong><small>${escape(tile.caption)}</small></div></article>`;}).join("");
  const sample = posterNames.map((_,i) => {const poster=d.posters[i]; return `<article class="poster"><div class="art">${poster.image ? `<img src="${poster.image}" alt="">` : `<div class="art-placeholder"><span class="cover-index">${String(i+1).padStart(2,"0")}</span><span class="cover-label">Add cover artwork</span></div>`}</div><h3>${escape(poster.title)}</h3><p>Sample title · Not playable</p></article>`;}).join("");
  const rgb=d.accent.slice(1).match(/../g).map(n=>parseInt(n,16));
  const buttonInk=(rgb[0]*299+rgb[1]*587+rgb[2]*114)/1000>145 ? "#111111" : "#ffffff";
  return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'"><style>
  *{box-sizing:border-box}body{margin:0;font:15px/1.5 Arial,Helvetica,sans-serif;color:${palette.ink};background:${palette.page};--accent:${d.accent};--panel:${palette.panel};--muted:${palette.muted};--line:${palette.line};--art:${palette.art}}.demo{padding:8px 20px;background:var(--panel);color:var(--muted);border-bottom:1px solid var(--line);font-size:11px;text-align:center}header{padding:22px 5%;display:flex;gap:16px;justify-content:space-between;align-items:center;background:var(--panel);border-bottom:1px solid var(--line)}.brand{font-size:19px;font-weight:700;letter-spacing:-.4px}.guest{font-size:12px;color:var(--muted)}main{padding:32px 5% 40px}.hero{padding:30px;background:var(--panel);border:1px solid var(--line);border-radius:4px}.kicker{color:var(--muted);font-size:11px;letter-spacing:1.5px;font-weight:700}.hero h1{font-size:clamp(28px,5vw,46px);line-height:1.15;max-width:650px;margin:14px 0;letter-spacing:-1px;overflow-wrap:anywhere}.hero p{max-width:600px;color:var(--muted)}.pass{margin-top:24px;display:flex;gap:18px;align-items:center;flex-wrap:wrap}.pass strong{font-size:14px}button{border:0;border-radius:4px;background:var(--accent);color:${buttonInk};padding:12px 18px;font-weight:700}button:disabled{cursor:not-allowed}.note{font-size:11px;color:var(--muted);margin:12px 0 0}.categories{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:20px;margin:20px 0 32px}.category{min-width:0}.category-cover,.art{position:relative;overflow:hidden;background:var(--art);border:1px solid var(--line);border-radius:4px}.category-cover{aspect-ratio:16/9;display:grid;place-items:center;padding:20px}.category-cover .cover-label{font-size:clamp(18px,3vw,30px);font-weight:700;letter-spacing:-.6px}.category img,.art img{position:absolute;inset:0;width:100%;height:100%;object-fit:cover;filter:none;opacity:1}.tile-text{padding:10px 0}.tile-text strong,.tile-text small{display:block;overflow-wrap:anywhere}.tile-text strong{font-size:14px;font-weight:600}.tile-text small{font-size:12px;color:var(--muted);margin-top:3px}h2{font-size:20px;font-weight:600;margin:30px 0 16px;letter-spacing:-.3px}.row{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:20px}.art{aspect-ratio:3/4}.art-placeholder{position:absolute;inset:0;display:flex;flex-direction:column;justify-content:space-between;padding:18px;color:var(--muted)}.cover-index{font-size:12px;letter-spacing:2px}.art-placeholder .cover-label{font-size:13px;line-height:1.5;max-width:90px}.poster{min-width:0}.poster h3{font-size:13px;font-weight:600;margin:10px 0 0;overflow-wrap:anywhere}.poster p{font-size:11px;color:var(--muted);margin:4px 0}.empty{padding:22px;border:1px dashed var(--line);border-radius:4px;color:var(--muted)}footer{margin-top:32px;padding:18px 0;border-top:1px solid var(--line);font-size:11px;color:var(--muted);background:var(--panel)}.shelf-heading{display:flex;justify-content:space-between;align-items:baseline;gap:12px;background:var(--panel)}@media(max-width:520px){header{padding:18px 6%}.guest{display:none}main{padding:20px 6%}.hero{padding:22px}.categories{grid-template-columns:1fr;gap:14px}.row{grid-template-columns:repeat(2,minmax(0,1fr));gap:14px}}
  </style></head><body><div class="demo">DESIGN PREVIEW · sample content · purchases and playback unavailable</div><header><div class="brand">${escape(d.name)}</div><span class="guest">Media library</span></header><main><section class="hero"><span class="kicker">WELCOME TO YOUR MEDIA LIBRARY</span><h1>${escape(d.headline)}</h1><p>${escape(d.subtitle)}</p><div class="pass"><button disabled>Buy media pass · Preview only</button><strong>${d.price.toLocaleString("en-US")} MMK / ${d.duration} hours</strong></div><p class="note">Media access is purchased separately from internet access.</p></section><div class="shelf-heading"><h2>Your libraries</h2><span class="guest">Example categories</span></div><section class="categories">${categoryHTML || '<p class="empty">No categories selected.</p>'}</section>${d.featured ? `<div class="shelf-heading"><h2>Featured stories</h2><span class="guest">Preview artwork</span></div><section class="row">${sample}</section>` : ""}<footer>Powered by Mega Magic · This preview is not connected to a Jellyfin server.</footer></main></body></html>`;
}

export function guestDocument(input) {
  const d=normalize(input),columns=d.posterDensity==="compact"?6:4;
  // Shared by preview and the future real-catalog page; no guest-script access.
  return guestBase(d).replace("default-src 'none';", "default-src 'none'; img-src data:;").replace('</head>',`<style>
    .row{grid-template-columns:repeat(${columns},minmax(0,1fr))}
    ${d.background ? `body{background-image:url("${d.background}");background-size:cover;background-attachment:fixed}main{background:var(--panel);margin:24px 3%;padding:24px 2%}` : ""}
    ${d.layout==="shelves" ? `.hero{padding:24px}.hero h1{font-size:30px}.categories{display:flex;overflow-x:auto;padding-bottom:8px}.category{flex:0 0 240px}.art{aspect-ratio:16/9}.art-placeholder{padding:12px}.art-placeholder .cover-label{max-width:100%;font-size:11px}` : ""}
    @media(max-width:520px){.row{grid-template-columns:repeat(2,minmax(0,1fr))}.category{flex-basis:200px}}
    </style></head>`);
}

// Public pages never pass off sample posters as a real Jellyfin catalog.
export function publishedDocument(input,purchasesEnabled=false) {
  const document=guestDocument({...input,featured:false})
    .replace('DESIGN PREVIEW · sample content · purchases and playback unavailable','GUEST PAGE · purchases and playback are not available yet')
    .replace('Buy media pass · Preview only','Media access not available yet')
    .replace('WELCOME TO YOUR MEDIA LIBRARY','WELCOME')
    .replace('Your libraries','Categories')
    .replace('Example categories','Planned media categories')
    .replace('Media access is purchased separately from internet access.','Planned pass pricing only. No payment is collected and no playback access is granted.')
    .replace('This preview is not connected to a Jellyfin server.','Your provider has published this design. Media access is not available yet.');
  return purchasesEnabled ? document.replace('GUEST PAGE · purchases and playback are not available yet','GUEST PAGE · bank/MMQR payments require owner approval').replace('Media access not available yet','Use the payment form above').replace('Planned pass pricing only. No payment is collected and no playback access is granted.','Pay only to the receiving details on your order. Receipt upload is not approval.') : document;
}

// Decode and re-encode uploads: never retain SVG, remote URLs or file metadata.
async function prepareImage(file) {
  if (!file || !["image/jpeg","image/png","image/webp"].includes(file.type) || file.size > 5 * 1024 * 1024) throw new Error("Choose a JPG, PNG or WebP picture up to 5 MB.");
  const url = URL.createObjectURL(file);
  try {
    const img = new Image(); img.src = url; await img.decode();
    if (!img.naturalWidth || !img.naturalHeight || img.naturalWidth * img.naturalHeight > 40000000) throw new Error("Choose a smaller picture (under 40 megapixels).");
    const canvas = document.createElement("canvas");
    const scale = Math.min(1,1200 / Math.max(img.naturalWidth,img.naturalHeight));
    canvas.width = Math.max(1,Math.round(img.naturalWidth*scale)); canvas.height = Math.max(1,Math.round(img.naturalHeight*scale));
    const context = canvas.getContext("2d");
    if (!context) throw new Error("This browser cannot prepare pictures.");
    context.fillStyle = "#17242b"; context.fillRect(0,0,canvas.width,canvas.height); context.drawImage(img,0,0,canvas.width,canvas.height);
    for (const quality of [.85,.7,.5,.3]) {const result=canvas.toDataURL("image/jpeg",quality); if (imageValue(result)) return result;}
    throw new Error("Picture is too detailed to save as a browser draft. Choose a smaller picture.");
  } finally {URL.revokeObjectURL(url);}
}

export function mountAppearance(content, business, user, api, isActive = () => true) {
  const initial = normalize({name:business?.company_name || defaults.name});
  const businessID = business?.business_id || user?.business_id;
  const key = businessID && user?.email ? `mm_media_design:${businessID}:${user.email}` : null;
  let draft = initial, revision = 0, loaded = false, busy = false, edited = false, paidAccessReady=false;
  if (key) { try { const raw = localStorage.getItem(key); if (raw) draft = normalize(JSON.parse(raw)); } catch (_) {} }
  content.innerHTML = `<div class="panel"><div class="panel-body"><span class="fleet-kicker">MAGIC MEDIA · GUEST APPEARANCE</span><h2>Make your media lounge feel like yours.</h2><p>Publish your branded guest page to a shareable link. Publishing saves this design on the server; it does not enable Jellyfin playback or paid access yet.</p><p id="media-publication" role="status">Loading published design…</p><button type="button" class="btn btn-sm" data-media-reload>Reload published design</button><button type="button" class="btn btn-sm" data-media-unpublish disabled>Unpublish guest page</button></div></div>
    <div class="media-design-workspace"><form class="panel media-design-form" id="media-design-form"><div class="panel-body">
      <h2>Ready-made templates</h2><p class="muted small">Choose a starting design, then make it yours. Your uploaded pictures and wording are kept.</p>
      <div class="media-template-list">${Object.entries(templates).map(([key,t])=>`<button type="button" class="media-template-card" data-media-template="${key}"><span class="media-template-swatch ${key}" aria-hidden="true"><i></i><span><b></b><b></b><b></b></span></span><strong>${t.label}</strong><small>${t.description}</small></button>`).join("")}</div>
      <h2>Brand & welcome</h2>
      <label>Business name<input class="input" name="name" maxlength="160" required></label>
      <label>Welcome headline<input class="input" name="headline" maxlength="160"></label>
      <label>Welcome message<textarea class="input" name="subtitle" maxlength="160" rows="3"></textarea></label>
      <label>Guest palette<select class="input" name="theme"><option value="charcoal">Charcoal</option><option value="paper">Warm white</option><option value="slate">Slate</option></select></label>
      <label>Accent colour<input name="accent" type="color"></label>
      <label>Page layout<select class="input" name="layout"><option value="lounge">Welcome lounge · portrait posters</option><option value="shelves">Cinema shelves · wide tiles and artwork</option></select></label>
      <label>Background picture<input type="file" accept="image/jpeg,image/png,image/webp" data-media-image="background"></label><button type="button" class="btn btn-sm" data-media-clear="background">Remove background picture</button>
      <h2>Guest homepage</h2>
      <label>Movie poster layout<select class="input" name="posterDensity"><option value="comfortable">Comfortable — larger posters</option><option value="compact">Compact — more titles per row</option></select></label>
      ${["movies","series","music","featured"].map(name => `<label class="media-check"><input name="${name}" type="checkbox">Show ${name === "featured" ? "featured stories" : name}</label>`).join("")}
      <h2>Category tiles</h2><p class="muted small">Use your own professionally designed covers. Pictures keep their colours, with captions underneath—not tinted overlays. JPG, PNG or WebP, up to 5 MB; pictures are resized for the preview.</p>
      ${Object.entries(tileDefaults).map(([key,t])=>`<details class="media-tile-editor" open><summary>${t.title}</summary><label>Tile title<input class="input" name="tile-${key}-title" maxlength="80"></label><label>Caption<input class="input" name="tile-${key}-caption" maxlength="160"></label><label>Tile picture<input type="file" accept="image/jpeg,image/png,image/webp" data-media-image="tile-${key}"></label><button type="button" class="btn btn-sm" data-media-clear="tile-${key}">Remove picture</button></details>`).join("")}
      <h2>Featured preview artwork</h2><p class="muted small">These four samples show your design, not playable movies. This does not change movie metadata or artwork in Jellyfin.</p>
      ${posterNames.map((_,i)=>`<details class="media-tile-editor"><summary>Preview card ${i+1}</summary><label>Sample title<input class="input" name="poster-${i}-title" maxlength="80"></label><label>Preview picture<input type="file" accept="image/jpeg,image/png,image/webp" data-media-image="poster-${i}"></label><button type="button" class="btn btn-sm" data-media-clear="poster-${i}">Remove picture</button></details>`).join("")}
      <h2>Guest libraries</h2><p class="muted small">Choose only libraries you are authorized to share. Files stay on your PC. Keep Jellyfin and the connector running for guest playback.</p><button type="button" class="btn" data-media-libraries>Load libraries from my PC</button><div data-media-library-options></div><p role="status" data-media-library-status></p>
      <details data-media-sales><summary>Media sales design settings · not active</summary><p class="muted small">These are future display settings. Paid access and playback remain unavailable until local delivery passes acceptance. Publishing a design does not collect payments or grant playback.</p>
      <label>Price (MMK)<input class="input" name="price" type="number" min="1" max="1000000" step="1" required></label>
      <label>Duration (hours)<input class="input" name="duration" type="number" min="1" max="720" step="1" required></label>
      <h2>Bank / MMQR payment details</h2><p class="muted small">Guests transfer to your account and attach a receipt. Your own Telegram bot delivers it for your approval or rejection. Saving receiving details does not activate checkout.</p>
      <label>Bank name<input class="input" name="bank_name" maxlength="100"></label>
      <label>Bank account number<input class="input" name="bank_account" maxlength="100"></label>
      <label>Account holder name<input class="input" name="bank_holder" maxlength="160"></label>
      <label>Your MMQR image<input type="file" accept="image/jpeg,image/png" data-media-image="mmqr"></label><button type="button" class="btn btn-sm" data-media-clear="mmqr">Remove MMQR image</button>
      </details><div class="media-design-actions"><button type="submit" class="btn btn-primary" disabled>Publish guest page</button><button type="button" class="btn" data-media-reset>Reset design</button></div>
      <p class="muted small">Publishing replaces the hosted design for your business. The preview uses sample artwork; the public page does not show sample movies.</p><p role="status" id="media-design-status"></p>
    </div></form><section class="panel media-preview-panel"><div class="panel-head"><h2 class="panel-title">Guest preview</h2><div class="media-design-actions"><button class="btn btn-sm" type="button" aria-pressed="true" data-media-width="desktop">Desktop</button><button class="btn btn-sm" type="button" aria-pressed="false" data-media-width="mobile">Mobile</button></div></div><div class="media-preview-wrap"><iframe title="Magic Media guest appearance preview" sandbox="" id="media-guest-preview"></iframe></div></section></div>`;
  const form = content.querySelector("#media-design-form"), frame = content.querySelector("iframe"), status = content.querySelector("#media-design-status"), publication = content.querySelector('#media-publication');
  const active = () => isActive() && content.contains(form);
  const controls = () => {form.querySelector('[type=submit]').disabled = busy || !loaded; content.querySelector('[data-media-unpublish]').disabled = busy || !loaded || !revision; content.querySelector('[data-media-reload]').disabled = busy;};
  const showPublication = p => {
    publication.replaceChildren();
    if (!p) {publication.textContent='No guest page has been published.';return;}
    publication.append(document.createTextNode('Published guest page: '));
    const link=document.createElement('a');link.href='/media.html?id='+encodeURIComponent(p.public_id);link.textContent='Open guest page';link.target='_blank';link.rel='noopener';publication.append(link);
    publication.append(document.createTextNode(' · revision '+p.revision+(paidAccessReady?'. Paid playback requires selected libraries, an online PC and your Telegram review bot.':'. Paid playback has not been enabled on this server.')));
  };
  let images = {}, uploadGeneration = 0, selectedLibraries=[];
  const libraryOptions=form.querySelector('[data-media-library-options]'),libraryStatus=form.querySelector('[data-media-library-status]');
  let connectorReady=false;
  const libraryButton=form.querySelector('[data-media-libraries]');libraryButton.disabled=true;
  api?.get('/media/capabilities').then(value=>{if(!active())return;connectorReady=value.connector?.available===true;libraryButton.disabled=!connectorReady;if(!connectorReady)libraryStatus.textContent='The local computer connector is not enabled. Guest-page design can be published separately.';}).catch(()=>{if(active())libraryStatus.textContent='Connector availability could not be confirmed. Library reads are unavailable.';});
  form.querySelector('[data-media-libraries]').onclick=async event=>{
    if(!connectorReady)return;
    const button=event.currentTarget;button.disabled=true;libraryStatus.textContent='Reading libraries from your media PC…';
    try{const id=crypto.randomUUID();await api.post('/media/jobs',{id,operation:'libraries',payload:{}});
      const deadline=Date.now()+125000;let libraries=null;
      while(active() && Date.now()<deadline){const job=await api.get('/media/jobs/'+id);if(job.state==='succeeded'){libraries=job.result?.libraries;break;}if(['failed','cancelled','expired'].includes(job.state))throw new Error('Library request '+job.state+'. Check the connector.');await new Promise(resolve=>setTimeout(resolve,2000));}
      if(!active())return;if(!Array.isArray(libraries))throw new Error('Libraries could not be confirmed. Keep the connector running and retry.');
      libraryOptions.replaceChildren();
      for(const library of libraries){if(!/^(?:[0-9a-f]{32}|[0-9a-f-]{36})$/.test(library.ItemId||'')||['photos','books'].includes(library.CollectionType))continue;
        const label=document.createElement('label'),input=document.createElement('input');input.type='checkbox';input.value=library.ItemId;input.checked=selectedLibraries.includes(input.value);label.className='media-check';label.append(input,document.createTextNode(' '+library.Name));libraryOptions.append(label);
        input.onchange=()=>{if(input.checked&&selectedLibraries.length>=16){input.checked=false;libraryStatus.textContent='Select at most 16 libraries.';return;}selectedLibraries=input.checked?[...new Set([...selectedLibraries,input.value])]:selectedLibraries.filter(id=>id!==input.value);edited=true;};
      }
      libraryStatus.textContent=libraryOptions.childElementCount?'Select the libraries guests may play, then publish.':'No playable libraries found. Add videos or music in Jellyfin first.';
    }catch(e){if(active())libraryStatus.textContent=e.message||'Libraries unavailable.';}finally{button.disabled=!connectorReady;}
  };
  const pendingUploads = new Map();
  const fill = data => {
    selectedLibraries=[...(data.libraries||[])];libraryOptions.querySelectorAll('input').forEach(input=>input.checked=selectedLibraries.includes(input.value));
    images = {background:data.background,mmqr:data.payment?.mmqr||''}; uploadGeneration++; pendingUploads.clear();
    for(const name of ['bank_name','bank_account','bank_holder'])form.elements.namedItem(name).value=data.payment?.[name]||'';
    for (const [name,value] of Object.entries(data)) { const field = form.elements.namedItem(name); if (field) {if (field.type === "checkbox") field.checked = value; else field.value = value;} }
    for (const [key,tile] of Object.entries(data.tiles)) {for (const prop of ["title","caption"]) form.elements.namedItem(`tile-${key}-${prop}`).value=tile[prop]; images[`tile-${key}`]=tile.image;}
    for (const [key,poster] of Object.entries(data.posters)) {form.elements.namedItem(`poster-${key}-title`).value=poster.title; images[`poster-${key}`]=poster.image;}
    form.querySelectorAll('input[type=file]').forEach(field=>{field.value="";});
  };
  const read = () => {
    const data=Object.fromEntries(Object.keys(defaults).filter(name=>!["tiles","posters","background"].includes(name)).map(name => {
      const field = form.elements.namedItem(name); return [name, field.type === "checkbox" ? field.checked : field.value];
    }));
    data.background=images.background;
    data.libraries=selectedLibraries;
    const payment=Object.fromEntries(['bank_name','bank_account','bank_holder'].map(name=>[name,form.elements.namedItem(name).value.trim()]));payment.mmqr=images.mmqr||'';
    data.payment=Object.values(payment).some(Boolean)?payment:null;
    data.tiles=Object.fromEntries(Object.keys(tileDefaults).map(key=>[key,{title:form.elements.namedItem(`tile-${key}-title`).value,caption:form.elements.namedItem(`tile-${key}-caption`).value,image:images[`tile-${key}`]}]));
    data.posters=Object.fromEntries(posterNames.map((_,i)=>[i,{title:form.elements.namedItem(`poster-${i}-title`).value,image:images[`poster-${i}`]}]));
    return normalize(data);
  };
  const preview = () => {frame.srcdoc = guestDocument(read());};
  fill(draft); preview();
  form.querySelectorAll('[data-media-template]').forEach(button=>button.addEventListener('click',()=>{edited=true;fill(applyTemplate(read(),button.dataset.mediaTemplate)); preview(); status.textContent="Template applied to preview. Publish to update the guest page.";}));
  form.querySelectorAll('[data-media-image]').forEach(field=>field.addEventListener('change',async()=>{
    const target=field.dataset.mediaImage, generation=uploadGeneration, token=Symbol(); pendingUploads.set(target,token);
    if (!field.files[0]) {pendingUploads.delete(target); return;}
    status.textContent="Preparing picture…";
    try {
      const result=await prepareImage(field.files[0]);
      if (generation!==uploadGeneration || pendingUploads.get(target)!==token || !content.contains(form)) return;
      edited=true;images[target]=result; preview(); status.textContent="Picture updated. Publish to update the guest page.";
    } catch(error) {if (generation===uploadGeneration && pendingUploads.get(target)===token) status.textContent=error.message || "Picture could not be read.";}
    finally {if (pendingUploads.get(target)===token) pendingUploads.delete(target);}
  }));
  form.querySelectorAll('[data-media-clear]').forEach(button=>button.addEventListener('click',()=>{edited=true;const target=button.dataset.mediaClear; pendingUploads.delete(target); images[target]=""; form.querySelector(`[data-media-image="${target}"]`).value=""; preview(); status.textContent="Picture removed from preview. Publish to update the guest page.";}));
  form.addEventListener("input", () => {edited=true;preview(); status.textContent = "Preview updated. Publish to update the guest page.";});
  const load = async (explicit=false) => {
    if(busy || !api)return;
    if(explicit && edited && !confirm('Discard your unsaved edits and reload the published design?'))return;
    busy=true;controls();
    try {const response=await api.get('/media/publication');if(!active())return;paidAccessReady=response.paid_access_ready===true;const p=response.publication;revision=p?.revision||0;loaded=true;showPublication(p);if(explicit||!edited){fill(p?normalize(p.design):draft);preview();edited=false;}}
    catch(e){if(active()){loaded=false;publication.textContent=e.status===404||e.status===503?'Publishing is unavailable on this server. Your local preview is still editable; no design has been published.':(e.message||'Published design could not be loaded.');}}
    finally{busy=false;if(active())controls();}
  };
  form.addEventListener("submit", async event => {
    event.preventDefault();
    if (pendingUploads.size) {status.textContent="Wait for picture preparation to finish before saving."; return;}
    if (!loaded || busy || !api) return;
    busy=true;controls();status.textContent='Publishing guest page…';
    const design=read();
    try {const response=await api.put('/media/publication?revision='+revision,design);if(!active())return;paidAccessReady=response.paid_access_ready===true;const p=response.publication;revision=p.revision;showPublication(p);edited=JSON.stringify(read())!==JSON.stringify(design);status.textContent='Guest page published. '+(paidAccessReady?'Paid playback checks your library and computer before checkout.':'Paid playback has not been enabled on this server.')+(edited?' You have newer unpublished edits.':'');if(key){try{localStorage.removeItem(key);}catch(_){}}}
    catch(e){if(active()){status.textContent=e.message||'Publication not confirmed.';loaded=false;}}
    finally{busy=false;if(active())controls();}
  });
  content.querySelector('[data-media-reload]').onclick=()=>load(true);
  content.querySelector('[data-media-unpublish]').onclick=async()=>{if(busy||!revision||!confirm('Remove the hosted guest page? Existing media access is not changed.'))return;busy=true;controls();try{await api.del('/media/publication?revision='+revision);if(!active())return;revision=0;showPublication(null);status.textContent='Guest page unpublished.';}catch(e){if(active()){status.textContent=e.message||'Unpublishing not confirmed.';loaded=false;}}finally{busy=false;if(active())controls();}};
  content.querySelector("[data-media-reset]").addEventListener("click", () => {edited=true;fill(initial); preview(); status.textContent = "Design reset in preview. Publish to replace the hosted design.";});
  content.querySelectorAll("[data-media-width]").forEach(button => button.addEventListener("click", () => {
    frame.classList.toggle("is-mobile", button.dataset.mediaWidth === "mobile");
    content.querySelectorAll("[data-media-width]").forEach(b => b.setAttribute("aria-pressed", String(b === button)));
  }));
  load();
}
