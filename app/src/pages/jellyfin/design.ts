/* eslint-disable @typescript-eslint/no-explicit-any */
// Guest-page design model and preview document for the Jellyfin guest page.
// The pure functions below are ported verbatim from the previous app so the preview matches what guests get.
// Hosted design publishing. Playback/paid access remains disabled until enforced.
export const defaults: Record<string, any> = {
  name: "Magic Media", headline: "A little movie magic.", subtitle: "Your next favourite story is waiting.",
  accent: "#eeeeee", theme: "charcoal", price: 1500, duration: 24,
  movies: true, series: true, music: true, featured: true,
  posterDensity: "comfortable",
  layout: "shelves", background: "", tiles: {}, posters: {},
};
export const tileDefaults: Record<string, {title: string; caption: string; image: string}> = {
  movies: {title:"Movies", caption:"Stories worth staying in for", image:""},
  series: {title:"TV Shows", caption:"One more episode", image:""},
  music: {title:"Music", caption:"Find your rhythm", image:""},
};
export const posterNames: string[] = ["The quiet coast","Beyond the stars","A city after rain","Golden afternoons"];
export const templates: Record<string, {label: string; description: string; theme: string; accent: string; layout: string; posterDensity: string}> = {
  cinema: {label:"Cinema", description:"Charcoal, wide library covers and compact landscape shelves.", theme:"charcoal", accent:"#eeeeee", layout:"shelves", posterDensity:"compact"},
  editorial: {label:"Editorial", description:"Warm white, quiet typography and large portrait covers.", theme:"paper", accent:"#202020", layout:"lounge", posterDensity:"comfortable"},
  screening: {label:"Screening room", description:"Solid slate, generous artwork and wide media shelves.", theme:"slate", accent:"#d8dde2", layout:"shelves", posterDensity:"comfortable"},
};
const imageValue = (value: unknown): string => typeof value === "string" && value.length <= 300000 && /^data:image\/(?:jpeg|png|webp);base64,[A-Za-z0-9+/]+={0,2}$/.test(value) ? value : "";
export function applyTemplate(value: any, name: string) {
  return normalize({...value, ...(templates[name] || templates.cinema)});
}
const escape = (value: unknown) => String(value).replace(/[&<>"']/g, c => (({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"} as Record<string,string>)[c]));
export function normalize(value: any = {}): any {
  const data = {...defaults, ...value};
  for (const key of ["name","headline","subtitle"]) data[key] = String(data[key] ?? defaults[key]).slice(0,160);
  data.accent = /^#[0-9a-f]{6}$/i.test(data.accent) ? data.accent : defaults.accent;
  const legacyThemes: Record<string, string> = {forest:"charcoal", midnight:"slate", plum:"charcoal"};
  data.theme = ["charcoal","paper","slate"].includes(data.theme) ? data.theme : legacyThemes[data.theme] || defaults.theme;
  data.posterDensity = ["comfortable","compact"].includes(data.posterDensity) ? data.posterDensity : "comfortable";
  data.layout = ["shelves","lounge"].includes(data.layout) ? data.layout : defaults.layout;
  data.background = imageValue(data.background);
  data.libraries=Array.isArray(value.libraries)?[...new Set(value.libraries.filter((id: string)=>typeof id==='string' && /^(?:[0-9a-f]{32}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$/.test(id)))].slice(0,16):[];
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

function guestBase(input: any): string {
  const d = normalize(input);
  const palette = ({charcoal:{page:"#111111",panel:"#181818",ink:"#f1f1f1",muted:"#b2b2b2",line:"#343434",art:"#242424"},paper:{page:"#f4f2ed",panel:"#fffefa",ink:"#202020",muted:"#60605a",line:"#d9d6cf",art:"#e8e5de"},slate:{page:"#191e24",panel:"#232930",ink:"#f2f3f4",muted:"#b5bdc5",line:"#424951",art:"#30373f"}} as Record<string, any>)[d.theme];
  const categories = Object.keys(tileDefaults).filter(key => d[key]);
  const categoryHTML = categories.map(key => {const tile=d.tiles[key]; return `<article class="category"><div class="category-cover">${tile.image ? `<img src="${tile.image}" alt="">` : `<span class="cover-label">${escape(tile.title)}</span>`}</div><div class="tile-text"><strong>${escape(tile.title)}</strong><small>${escape(tile.caption)}</small></div></article>`;}).join("");
  const sample = posterNames.map((_,i) => {const poster=d.posters[i]; return `<article class="poster"><div class="art">${poster.image ? `<img src="${poster.image}" alt="">` : `<div class="art-placeholder"><span class="cover-index">${String(i+1).padStart(2,"0")}</span><span class="cover-label">Add cover artwork</span></div>`}</div><h3>${escape(poster.title)}</h3><p>Sample title · Not playable</p></article>`;}).join("");
  const rgb=d.accent.slice(1).match(/../g).map((n: string)=>parseInt(n,16));
  const buttonInk=(rgb[0]*299+rgb[1]*587+rgb[2]*114)/1000>145 ? "#111111" : "#ffffff";
  return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'"><style>
  *{box-sizing:border-box}body{margin:0;font:15px/1.5 Arial,Helvetica,sans-serif;color:${palette.ink};background:${palette.page};--accent:${d.accent};--panel:${palette.panel};--muted:${palette.muted};--line:${palette.line};--art:${palette.art}}.demo{padding:8px 20px;background:var(--panel);color:var(--muted);border-bottom:1px solid var(--line);font-size:11px;text-align:center}header{padding:22px 5%;display:flex;gap:16px;justify-content:space-between;align-items:center;background:var(--panel);border-bottom:1px solid var(--line)}.brand{font-size:19px;font-weight:700;letter-spacing:-.4px}.guest{font-size:12px;color:var(--muted)}main{padding:32px 5% 40px}.hero{padding:30px;background:var(--panel);border:1px solid var(--line);border-radius:4px}.kicker{color:var(--muted);font-size:11px;letter-spacing:1.5px;font-weight:700}.hero h1{font-size:clamp(28px,5vw,46px);line-height:1.15;max-width:650px;margin:14px 0;letter-spacing:-1px;overflow-wrap:anywhere}.hero p{max-width:600px;color:var(--muted)}.pass{margin-top:24px;display:flex;gap:18px;align-items:center;flex-wrap:wrap}.pass strong{font-size:14px}button{border:0;border-radius:4px;background:var(--accent);color:${buttonInk};padding:12px 18px;font-weight:700}button:disabled{cursor:not-allowed}.note{font-size:11px;color:var(--muted);margin:12px 0 0}.categories{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:20px;margin:20px 0 32px}.category{min-width:0}.category-cover,.art{position:relative;overflow:hidden;background:var(--art);border:1px solid var(--line);border-radius:4px}.category-cover{aspect-ratio:16/9;display:grid;place-items:center;padding:20px}.category-cover .cover-label{font-size:clamp(18px,3vw,30px);font-weight:700;letter-spacing:-.6px}.category img,.art img{position:absolute;inset:0;width:100%;height:100%;object-fit:cover;filter:none;opacity:1}.tile-text{padding:10px 0}.tile-text strong,.tile-text small{display:block;overflow-wrap:anywhere}.tile-text strong{font-size:14px;font-weight:600}.tile-text small{font-size:12px;color:var(--muted);margin-top:3px}h2{font-size:20px;font-weight:600;margin:30px 0 16px;letter-spacing:-.3px}.row{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:20px}.art{aspect-ratio:3/4}.art-placeholder{position:absolute;inset:0;display:flex;flex-direction:column;justify-content:space-between;padding:18px;color:var(--muted)}.cover-index{font-size:12px;letter-spacing:2px}.art-placeholder .cover-label{font-size:13px;line-height:1.5;max-width:90px}.poster{min-width:0}.poster h3{font-size:13px;font-weight:600;margin:10px 0 0;overflow-wrap:anywhere}.poster p{font-size:11px;color:var(--muted);margin:4px 0}.empty{padding:22px;border:1px dashed var(--line);border-radius:4px;color:var(--muted)}footer{margin-top:32px;padding:18px 0;border-top:1px solid var(--line);font-size:11px;color:var(--muted);background:var(--panel)}.shelf-heading{display:flex;justify-content:space-between;align-items:baseline;gap:12px;background:var(--panel)}@media(max-width:520px){header{padding:18px 6%}.guest{display:none}main{padding:20px 6%}.hero{padding:22px}.categories{grid-template-columns:1fr;gap:14px}.row{grid-template-columns:repeat(2,minmax(0,1fr));gap:14px}}
  </style></head><body><div class="demo">DESIGN PREVIEW · sample content · purchases and playback unavailable</div><header><div class="brand">${escape(d.name)}</div><span class="guest">Media library</span></header><main><section class="hero"><span class="kicker">WELCOME TO YOUR MEDIA LIBRARY</span><h1>${escape(d.headline)}</h1><p>${escape(d.subtitle)}</p><div class="pass"><button disabled>Buy media pass · Preview only</button><strong>${d.price.toLocaleString("en-US")} MMK / ${d.duration} hours</strong></div><p class="note">Media access is purchased separately from internet access.</p></section><div class="shelf-heading"><h2>Your libraries</h2><span class="guest">Example categories</span></div><section class="categories">${categoryHTML || '<p class="empty">No categories selected.</p>'}</section>${d.featured ? `<div class="shelf-heading"><h2>Featured stories</h2><span class="guest">Preview artwork</span></div><section class="row">${sample}</section>` : ""}<footer>Powered by Mega Magic · This preview is not connected to a Jellyfin server.</footer></main></body></html>`;
}

export function guestDocument(input: any): string {
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
export function publishedDocument(input: any, purchasesEnabled = false): string {
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
export async function prepareImage(file: File): Promise<string> {
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

