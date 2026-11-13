// Mobile and Burmese typography guards for the console, in real Chromium plus a few source checks.
// Covers: the bottom navigation grows so tall Burmese marks are not clipped, dense tables switch to stacked cards below 768px with
// 44px touch targets, tabular figures never swap the text font, numeric inputs ask for a numeric keyboard, and no legacy emerald token returns.
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {createServer} from 'node:http';
import {mkdtemp, readFile, rm, readdir} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';

const read = (p) => readFile(new URL(`../${p}`, import.meta.url), 'utf8');

// Source checks first: they need no browser.
const base = await read('app/src/styles/base.css');
assert.match(base, /\.tnum \{[^}]*font-variant-numeric:\s*tabular-nums/, '.tnum sets tabular figures');
assert.doesNotMatch(base.match(/\.tnum \{[^}]*\}/)[0], /font-family/, '.tnum must not change the font; .num is the mono data class');
const tables = await read('app/src/components/tables.css');
assert.match(tables, /@media \(width < 768px\)/, 'tables stack below 768px');
assert.doesNotMatch(tables, /max-width:\s*760px/, 'old 760px breakpoint is gone');

async function walk(dir) {
  const out = [];
  for (const e of await readdir(dir, {withFileTypes: true})) {
    const p = join(dir, e.name);
    if (e.isDirectory()) out.push(...await walk(p)); else out.push(p);
  }
  return out;
}
const appFiles = (await walk(new URL('../app/src/', import.meta.url).pathname)).filter((f) => /\.(css|tsx|ts)$/.test(f));
let numericInputs = 0;
for (const f of appFiles) {
  const text = await readFile(f, 'utf8');
  assert.doesNotMatch(text, /#053d2c/i, `${f}: legacy emerald ink #053d2c is back`);
  numericInputs += (text.match(/inputMode="(numeric|decimal)"/g) ?? []).length;
}
assert.ok(numericInputs >= 26, `expected at least 26 numeric/decimal inputs, found ${numericInputs}`);
for (const f of ['app/src/pages/Sites.tsx', 'app/src/pages/network/Broadband.tsx', 'app/src/pages/network/NetworkEditor.tsx', 'app/src/pages/devices/ManagedDevices.tsx']) {
  assert.match(await read(f), /inputMode=\{?[^>]*"(numeric|decimal)"/, `${f} lost its numeric input mode`);
}

const styles = await Promise.all(['tokens', 'base', 'shell', 'pages'].map((n) => read(`app/src/styles/${n}.css`)));
styles.push(await read('app/src/components/BottomNav.css'), await read('app/src/components/tables.css'));
// The real fonts, so glyph boxes are the ones people see (Instrument Sans for Latin, Noto Sans Myanmar for Burmese).
const fontCss = `@font-face{font-family:"Instrument Sans Variable";font-weight:400 700;src:url(/fonts/instrument-sans-latin-wght-normal-BbzFLZTg.woff2) format("woff2")}
@font-face{font-family:"JetBrains Mono Variable";font-weight:100 800;src:url(/fonts/jetbrains-mono-latin-wght-normal.woff2) format("woff2")}
@font-face{font-family:"Noto Sans Myanmar";font-weight:400;src:url(/fonts/noto-sans-myanmar-myanmar-400-normal-C_Ri2PLj.woff2) format("woff2");unicode-range:U+1000-109F,U+200C-200D,U+25CC,U+A92E,U+A9E0-A9FE,U+AA60-AA7F}
@font-face{font-family:"Noto Sans Myanmar";font-weight:600;src:url(/fonts/noto-sans-myanmar-myanmar-600-normal-Cju0GbT9.woff2) format("woff2");unicode-range:U+1000-109F,U+200C-200D,U+25CC,U+A92E,U+A9E0-A9FE,U+AA60-AA7F}`;
const burmeseLabels = ['ပင်မ', 'ကွန်ရက်', 'ဗောက်ချာ', 'လှုပ်ရှားမှု', 'နောက်ထပ်'];
const englishLabels = ['Home', 'Network', 'Vouchers', 'Activity', 'More'];
const rows = Array.from({length: 3}, (_, i) => `<tr><td class="dt-check"><input type="checkbox"></td><td data-cell="primary"><span class="dt-code num">ABCDEFGH</span></td><td data-cell="status"><span class="dt-status" data-tone="ok">အသုံးပြုပြီး</span></td><td data-cell="actions" class="dt-right"><span class="dt-actions"><button class="dt-btn">ပြင်မည်</button><button class="dt-btn danger">ဖျက်မည်</button></span></td></tr>`).join('');
const page = (lang, theme) => `<!doctype html><html data-theme="${theme}" lang="${lang}"><head><meta name="viewport" content="width=device-width,initial-scale=1"><style>${fontCss}\n${styles.join('\n')}</style></head><body>
<p id="tn" class="tnum">Mixed label 1,234</p><p id="nm" class="num">1,234</p><p id="plain">Plain</p>
<nav class="bnav" id="bnav" style="--bnav-count:5;--bnav-i:0"><span class="bnav-bubble"></span>${(lang === 'my' ? burmeseLabels : englishLabels).map((l, i) => `<a class="bnav-tab" href="#" ${i === 0 ? 'aria-current="page"' : ''}><span class="bnav-ico"></span><span class="bnav-label">${l}</span></a>`).join('')}</nav>
<div class="dt-wrap"><div class="dt-scroll"><table class="dt" id="dt"><thead><tr><th></th><th>Code</th><th>Status</th><th></th></tr></thead><tbody>${rows}</tbody></table></div>
<div class="dt-filters"><button class="dt-filter" id="flt">အားလုံး</button></div><div class="dt-pager"><div class="dt-pages"><button class="dt-page" id="pg">1</button></div></div></div>
</body></html>`;

let current = page('en', 'night');
const server = createServer(async (req, res) => { const u = new URL(req.url, 'http://x');
  if (u.pathname.startsWith('/fonts/')) { try { res.writeHead(200, {'Content-Type': 'font/woff2'}); res.end(await readFile(new URL(`../web/assets/fonts/${u.pathname.slice(7).replace(/[^a-zA-Z0-9._-]/g, '')}`, import.meta.url))); } catch { res.writeHead(404); res.end(); } return; } current = page(u.searchParams.get('lang') ?? 'en', u.searchParams.get('theme') ?? 'night'); res.writeHead(200, {'Content-Type': 'text/html'}); res.end(current); });
const wait = async (fn, label, ms = 12000) => { const end = Date.now() + ms; while (Date.now() < end) { if (await fn()) return; await new Promise((r) => setTimeout(r, 50)); } throw new Error('timeout: ' + label); };
let browser, profile, ws;
try {
  await new Promise((r) => server.listen(0, '127.0.0.1', r));
  profile = await mkdtemp(join(tmpdir(), 'mm-mobile-typography-'));
  browser = spawn(process.env.CHROME_BIN || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', ['--headless=new', '--no-first-run', '--disable-background-networking', '--remote-debugging-port=0', `--user-data-dir=${profile}`, 'about:blank'], {stdio: 'ignore'});
  let debugPort;
  await wait(async () => { try { debugPort = Number((await readFile(join(profile, 'DevToolsActivePort'), 'utf8')).split('\n')[0]); return !!debugPort; } catch { return false; } }, 'Chrome', 45000);
  const tab = await fetch(`http://127.0.0.1:${debugPort}/json/new?about:blank`, {method: 'PUT'}).then((r) => r.json());
  ws = new WebSocket(tab.webSocketDebuggerUrl);
  await new Promise((ok, bad) => { ws.addEventListener('open', ok, {once: true}); ws.addEventListener('error', bad, {once: true}); });
  let serial = 0; const pending = new Map();
  ws.addEventListener('message', (e) => { const m = JSON.parse(e.data); if (m.id) { const p = pending.get(m.id); pending.delete(m.id); m.error ? p.reject(Error(m.error.message)) : p.resolve(m.result); } });
  const send = (method, params = {}) => new Promise((resolve, reject) => { const id = ++serial; pending.set(id, {resolve, reject}); ws.send(JSON.stringify({id, method, params})); });
  const evaluate = async (expression) => { const r = await send('Runtime.evaluate', {expression, returnByValue: true}); if (r.exceptionDetails) throw Error(r.exceptionDetails.text); return r.result.value; };
  await send('Page.enable');
  let checks = 0;
  for (const theme of ['night', 'day']) for (const lang of ['en', 'my']) for (const width of [375, 767, 768]) {
    await send('Emulation.setDeviceMetricsOverride', {width, height: 800, deviceScaleFactor: 1, mobile: width < 768});
    await send('Page.navigate', {url: `http://127.0.0.1:${server.address().port}/?lang=${lang}&theme=${theme}`});
    await wait(() => evaluate(`!!document.querySelector('#bnav')`), 'fixture');
    await evaluate(`Promise.all([document.fonts.load('600 11px "Instrument Sans Variable"','Home'),document.fonts.load('600 11px "Noto Sans Myanmar"','ကွန်ရက်'),document.fonts.load('400 11px "JetBrains Mono Variable"','1')]).then(()=>document.fonts.ready).then(()=>true)`);
    const label = `${width}px ${theme} ${lang}`;
    const m = await evaluate(`(()=>{const bn=document.querySelector('#bnav').getBoundingClientRect();
      const labels=[...document.querySelectorAll('.bnav-label')].map(l=>({cs:(c=>[c.display,c.lineHeight,c.paddingTop,c.paddingBottom,c.fontSize,c.height,c.boxSizing].join('|'))(getComputedStyle(l)),sh:l.scrollHeight,ch:l.clientHeight,clipped:(()=>{const r=document.createRange();r.selectNodeContents(l);const g=r.getBoundingClientRect(),b=l.getBoundingClientRect();return (g.top<b.top-0.5||g.bottom>b.bottom+0.5)?[g.top-b.top,b.bottom-g.bottom,g.height,b.height].map(n=>n.toFixed(1)).join(','):false})(),h:l.getBoundingClientRect().height,fs:parseFloat(getComputedStyle(l).fontSize),lh:parseFloat(getComputedStyle(l).lineHeight)}));
      const tr=getComputedStyle(document.querySelector('#dt tbody tr')).display;
      const h=s=>document.querySelector(s).getBoundingClientRect().height;
      const w=s=>document.querySelector(s).getBoundingClientRect().width;
      const ff=s=>getComputedStyle(document.querySelector(s)).fontFamily;
      const nv=s=>getComputedStyle(document.querySelector(s)).fontVariantNumeric;
      return {bn:bn.height,right:bn.right,labels,tr,btn:h('.dt-btn'),btnW:w('.dt-btn'),page:h('#pg'),pageW:w('#pg'),flt:h('#flt'),scroll:document.documentElement.scrollWidth,tnFont:ff('#tn'),plainFont:ff('#plain'),nmFont:ff('#nm'),tnNum:nv('#tn')}})()`);
    assert.ok(m.scroll <= width, `${label}: document overflow`);
    assert.ok(m.bn >= 66, `${label}: bottom nav keeps at least its 66px height`);
    for (const l of m.labels) assert.ok(!l.clipped, `${label}: bottom-nav label is clipped (top,bottom,glyph h,box h = ${l.clipped}; scroll ${l.sh}px in ${l.ch}px box; ${l.cs})`);
    if (lang === 'my') for (const l of m.labels) assert.ok(l.lh >= l.fs * 1.7, `${label}: Burmese bottom-nav leading is ${(l.lh / l.fs).toFixed(2)}, needs 1.75`);
    assert.equal(m.tnFont, m.plainFont, `${label}: .tnum keeps the text font`);
    assert.notEqual(m.nmFont, m.plainFont, `${label}: .num is still the data font`);
    assert.match(m.tnNum, /tabular-nums/, `${label}: .tnum sets tabular figures`);
    if (width < 768) {
      assert.equal(m.tr, 'grid', `${label}: table rows stack as cards`);
      for (const [name, v] of [['table action', m.btn], ['page button', m.page], ['filter pill', m.flt]]) assert.ok(v >= 44, `${label}: ${name} is ${v}px, needs 44`);
      assert.ok(m.btnW >= 44 && m.pageW >= 44, `${label}: touch target width`);
    } else {
      assert.equal(m.tr, 'table-row', `${label}: table rows stay a table at 768px`);
    }
    checks++;
  }
  console.log(`Mobile typography: ${checks} viewport/theme/language combinations passed (375, 767, 768px).`);
} finally {
  ws?.close();
  if (browser && !browser.killed) { browser.kill('SIGTERM'); await new Promise((r) => browser.once('exit', r)); }
  await new Promise((r) => server.close(r));
  if (profile) await rm(profile, {recursive: true, force: true, maxRetries: 5, retryDelay: 100});
}
