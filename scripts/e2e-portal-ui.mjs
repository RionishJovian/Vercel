#!/usr/bin/env node
// Captive-portal browser acceptance test. Uses only Node + Chrome's DevTools
// protocol so this still runs while the repository's dormant Vite scaffold has
// no node_modules. It exercises real pointer events, API writes, fresh readback,
// the iframe language controls, payment preview and voucher preview.

import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createServer } from "node:http";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { extname, join, normalize, resolve } from "node:path";

const root = resolve(import.meta.dirname, "..");
const webRoot = join(root, "web");
const chrome = process.env.CHROME_BIN || (process.platform === "darwin"
  ? "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
  : "google-chrome");
const issuedVoucherBatches = [];
let config = {
  business_name: "Acceptance Coffee",
  welcome_text: "Welcome",
  terms_text: "One plan, one device.",
  primary_hex: "#065f46",
  glass_tint_hex: "#8eb69b",
  bank_name: "Test Bank",
  bank_holder: "Acceptance Owner",
  bank_account: "000111",
  mmqr_url: "",
  guest_grace_minutes: 10,
  guest_grace_data_mb: 50,
  voucher_login_enabled: true,
  mobile_payment_enabled: true,
  guest_plans: [{ key: "hour", label: "One hour", minutes: 60, amount: 1000, data_mb: 1024 }],
};
const writes = [];
const chatWrites = [];
const profileWrites = [];
const passwordWrites = [];
const portalDeployments = [];
let profileSettings = { full_name: "Portal QA", email: "qa@example.test", user_role: "DEVELOPER", language: "en", has_avatar: false, has_cover: false, created_at: "2026-09-01T00:00:00Z" };
let purchaseRequests = [
  { id: "pay-approved", plan_key: "hour", amount: 1000, bank_reference: "bank-001", status: "approved", created_at: "2026-09-30T01:00:00Z", decided_at: "2026-09-30T01:05:00Z" },
];
let guestPurchaseRequests = [
  { id: "guest-pending", plan_key: "day", amount: 3500, tx_last6: "123456", status: "pending", created_at: "2026-09-30T02:00:00Z" },
];
let failNextPut = false;

const mime = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" };
const server = createServer(async (req, res) => {
  const url = new URL(req.url, "http://localhost");
  const json = (code, body) => {
    res.writeHead(code, { "content-type": "application/json", "cache-control": "no-store" });
    res.end(JSON.stringify(body));
  };
  if (url.pathname === "/api/v1/me") return json(200, {
    user: { email: "qa@example.test", full_name: "Portal QA", user_role: "DEVELOPER" },
    business: { company_name: "Acceptance Coffee" },
  });
  if (url.pathname === "/api/v1/me/profile" && req.method === "GET") return json(200, profileSettings);
  if (url.pathname === "/api/v1/me/profile" && req.method === "PUT") {
    let raw = ""; for await (const chunk of req) raw += chunk;
    const body = JSON.parse(raw || "{}"); profileWrites.push(body); profileSettings = { ...profileSettings, ...body };
    return json(200, { status: "saved" });
  }
  if (url.pathname === "/api/v1/me/password" && req.method === "POST") {
    let raw = ""; for await (const chunk of req) raw += chunk;
    passwordWrites.push(JSON.parse(raw || "{}")); return json(200, { status: "password changed" });
  }
  if ((url.pathname === "/api/v1/me/profile/avatar" || url.pathname === "/api/v1/me/profile/cover") && req.method === "GET") return json(404, { error: "not found" });
  if (url.pathname === "/api/v1/portal/config" && req.method === "GET") return json(200, { config });
  if (url.pathname === "/api/v1/billing/requests") return json(200, { requests: purchaseRequests });
  if (url.pathname === "/api/v1/guest-wifi/requests") return json(200, { requests: guestPurchaseRequests });
  if (url.pathname === "/api/v1/guest-wifi/revenue") {
    const amount = guestPurchaseRequests.some(r=>r.status==="approved") ? 3500 : 0;
    return json(200,{all_time:amount,today:amount,last7:amount,last30:amount,total:1,approved:amount?1:0,pending:amount?0:1,rejected:0,days:[],plans:[]});
  }
  if (url.pathname === "/api/v1/voucher-report") {
    if (!process.env.E2E_REVENUE_ONLY) return json(503,{error:'Voucher report not configured in this fixture'});
    return json(200,{source:'guest-internet-vouchers',recorded_count:139,recorded_face_value:74500,first_use_count:6,available_count:131,reserved_count:1,activated_count:2,expired_count:3,revoked_count:1,other_count:1,activation_count:3,activation_time_unknown_count:3,activation_value:6500,activation_today:1500,activation_last7:3500,activation_last30:3500,gateways:[{device_id:'dev-1',gateway_name:'Cafe gateway',recorded_count:139,available_count:131,reserved_count:1,activated_count:2,expired_count:3,revoked_count:1,other_count:1,activation_value:6500}]});
  }
  if (url.pathname === "/api/v1/guest-wifi/approve/guest-pending" && req.method === "POST") {
    guestPurchaseRequests = guestPurchaseRequests.map(r => r.id === "guest-pending" ? { ...r, status: "approved", decided_at: "2026-09-30T02:05:00Z" } : r);
    return json(200, { request: guestPurchaseRequests.find(r => r.id === "guest-pending"), applied: true });
  }
  if (url.pathname === "/api/v1/routers") return json(200, { routers: [{ device_id: "dev-1", hardware_name: "CHR lab router", deployment_layer: "MAIN_GATEWAY", vendor: "mikrotik", is_online: false }] });
  if (url.pathname === "/api/v1/routers/dev-1/portal/profiles") return json(200, {profiles:["guest"]});
  if (url.pathname === "/api/v1/routers/dev-1/portal/preview" && req.method === "POST") {
    let raw=""; for await (const chunk of req) raw+=chunk;
    const body=JSON.parse(raw); assert.equal(body.profile,"guest");
    return json(200,{plan:{profile:"guest",version:"1234567890abcdef",confirmation:"portal-preview",file:"flash/mm-portal-test/login.html",steps:["Retain original HotSpot files","Install portal entrypoint","Verify readback"]}});
  }
  if (url.pathname === "/api/v1/routers/dev-1/portal/apply" && req.method === "POST") {
    let raw=""; for await (const chunk of req) raw+=chunk;
    const body=JSON.parse(raw); portalDeployments.push(body);
    if(body.confirmation!=="portal-preview"||body.version!=="1234567890abcdef") return json(400,{error:"preview required"});
    return json(200,{result:{verified:true,backup:"flash/hotspot-original"}});
  }
  if (process.env.E2E_VOUCHER_BATCH_ONLY) {
    if (url.pathname === "/api/v1/guest-vouchers/config") return json(200,{enabled:true});
    if (url.pathname === "/api/v1/vouchers") return json(200,{vouchers:[{id:'older',code:'OLDER-INVENTORY',plan_key:'business-pass',price_mmk:500,status:'unused'}]});
    if (url.pathname === "/api/v1/voucher-resellers") return json(200,{resellers:[],count:0,active:0});
    if (url.pathname === "/api/v1/voucher-print-layout") return json(200,{layout:{business_name:'Acceptance Coffee',wifi_name:'Guest Wi-Fi',show_price:true,show_expiry:true}});
    if (url.pathname === "/api/v1/guest-vouchers" && req.method === "POST") {
      let raw='';for await(const chunk of req)raw+=chunk;
      const body=JSON.parse(raw);issuedVoucherBatches.push(body);
      return json(201,{product:'guest-internet',data_mb:body.data_mb,validity_starts:'first_use',vouchers:Array.from({length:body.count},(_,i)=>({id:`new-${i}`,code:`BATCH-${i+1}`,plan_key:'guest-internet',plan_label:body.plan_label,price_mmk:body.price_mmk,duration_minutes:body.duration_minutes,device_id:body.device_id,status:'unused'}))});
    }
  }
  if (url.pathname === "/api/v1/routers/dev-1/quick-config") return json(200, {config:{identity:"lab",dns_servers:["1.1.1.1"],allow_remote_dns:false}});
  if (url.pathname === "/api/v1/routers/dev-1/network/inventory") return json(200,{inventory:{write_enabled:true,wifi_write_enabled:true,interfaces:[{name:'ether1',type:'ether',running:true},{name:'ether2',type:'ether',running:true}],available:{wifi:true,wireless:false,subscribers:true,ppp_profiles:true,dhcp:true,pools:true,pppoe_clients:true},objects:{wifi:[{id:'*1',values:{name:'wifi1','configuration.ssid':'Business','configuration.country':'myanmar',disabled:'no'}}],dhcp_clients:[{id:'*2',values:{interface:'ether1',gateway:'192.0.2.1'}},{id:'*3',values:{interface:'ether2',gateway:'198.51.100.1'}}],ppp_profiles:[{id:'*4',values:{name:'business'}}],pools:[{id:'*5',values:{name:'clients',ranges:'192.168.70.10-192.168.70.200'}}],dhcp:[{id:'*6',values:{name:'customers','address-pool':'clients','lease-time':'1h',disabled:'no'}}]}}});
  if (url.pathname === "/api/v1/routers/dev-1/network/preview") {
    let raw='';for await(const chunk of req)raw+=chunk;const body=JSON.parse(raw);writes.push({network_preview:body});
    await delay(200);return json(200,{review:{confirmation:'network-confirmation',steps:['Update selected router settings'],warnings:['Test a real client after configuration readback'],expires_at:new Date(Date.now()+300000).toISOString()}});
  }
  if (url.pathname === "/api/v1/routers/dev-1/network/apply") {
    let raw='';for await(const chunk of req)raw+=chunk;const body=JSON.parse(raw);writes.push({network_apply:body});
    if(body.confirmation!=='network-confirmation')return json(400,{error:'review required'});
    return json(200,{result:{verified:true,backup:'network-test-backup'},audit_recorded:true});
  }
  if (url.pathname === "/api/v1/routers/dev-1/gateway/interfaces") return json(200, {interfaces:[{name:"ether1",type:"ether",running:true},{name:"ether2",type:"ether",running:true}]});
  if (url.pathname === "/api/v1/routers/dev-1/gateway/defaults") return json(200, {defaults:{name:"guest-lan",gateway_cidr:"192.168.52.1/24",pool_start:"192.168.52.10",pool_end:"192.168.52.250"}});
  if (url.pathname === "/api/v1/routers/dev-1/gateway/preview" && req.method === "POST") {
    let raw=""; for await (const chunk of req) raw+=chunk; const body=JSON.parse(raw);
    if (body.wan_interface===body.lan_interface) return json(400,{error:"choose distinct uplink and LAN interfaces"});
    return json(200,{plan:{confirmation:"test-preview",steps:["Create DHCP pool","Enable Hotspot"],network:"192.168.50.0/24",interface:"ether2"}});
  }
  if (url.pathname === "/api/v1/routers/dev-1/gateway/apply" && req.method === "POST") {
    let raw=""; for await (const chunk of req) raw+=chunk; const body=JSON.parse(raw);
    if (body.confirmation!=="test-preview") return json(400,{error:"preview required"});
    return json(200,{result:{verified:true,backup:"gateway-test-backup"}});
  }
  if (url.pathname === "/api/v1/routers/dev-1/webfig/secure" && req.method === "POST") return json(200,{result:{verified:true,backup:"webfig-test-backup",url:"https://10.99.0.3:443/"}});
  if (url.pathname === "/api/v1/routers/dev-1/capabilities") return json(200, { capabilities: {
    board_name: "CHR", routeros_version: "7.15.3", gateway: true,
    dhcp_server: { supported: true, configured: false, count: 0 },
    hotspot: { supported: true, configured: false, count: 0 },
    radius: { supported: true, configured: false, count: 0 },
    modern_wifi: { supported: false, count: 0 }, legacy_wireless: { supported: false, count: 0 },
    modern_capsman: { supported: false, count: 0 }, legacy_capsman: { supported: false, count: 0 },
    external_aps_likely: true, webfig: { https_enabled: false, trusted_only: false },
  } });
  if (url.pathname === "/api/v1/fleet/health") {
    await delay(250); // Deliberately slower so the navigation race test is meaningful.
    return json(200, { total_peers: 1, up: 0, stale: 0, offline: 1, unknown: 0, overall_state: "offline", peers: [{ device_id: "dev-1", hardware_name: "CHR lab router", state: "offline" }] });
  }
  if (url.pathname === "/api/v1/fleet/pulse") return json(200, { open_incidents_head: [{ id: "inc-1", severity: "critical", subject_label: "CHR lab router", kind: "peer_offline" }] });
  if (url.pathname === "/api/v1/topology/map") return json(200, {
    roots: [{ device: { device_id: "dev-1", hardware_name: "CHR lab router", device_model: "CHR 7.15.3", deployment_layer: "MAIN_GATEWAY", vendor: "mikrotik", category: "router", transport: "wireguard" }, children: [
      { device: { device_id: "dev-2", hardware_name: "Lobby access point", device_model: "cAP ax", deployment_layer: "ACCESS_POINT", vendor: "mikrotik", category: "ap", transport: "direct" }, children: [] },
    ] }],
    orphans: [], cycles: [], summary: { total_devices: 2, in_tree: 2, online: 0 },
  });
  if (url.pathname === "/api/v1/sites") return json(200, { sites: [{ site_id: "site-1", name: "Downtown Café", location: "Yangon", timezone: "Asia/Yangon", latitude: 16.8409, longitude: 96.1735, device_count: 1 }] });
  if (url.pathname === "/api/v1/router-connections") return json(200, { connections: [{ id: "conn-1", name: "Café connector", host: "10.0.0.1", port: 443, use_tls: true, connection_mode: "hub", cloud_status: "online", site_id: "site-1", site_name: "Downtown Café", device_count: 1 }] });
  if (url.pathname === "/api/v1/device-allowances") return json(200, { allowance: { routers: 1, controllers: 0, sites: 0, used_routers: 1, used_controllers: 0, used_sites: 0, used_access_points: 0 }, kinds: ["routers", "controllers", "sites"] });
  if (url.pathname === "/api/v1/device-requests") return json(200, { requests: [] });
  if (url.pathname === "/api/v1/pricing") return json(200, { plans: { monthly: { standard_mmk: 35000 }, annual: { standard_mmk: 350000, saving_vs_monthly_mmk: 70000 } }, promo: { live: false }, tiers: [{ key: "starter", label: "Starter", device_quota: 10 }] });
  if (url.pathname === "/api/v1/magic-coins/wallet") return json(200, { wallet: { balance: 24 } });
  if (url.pathname === "/api/v1/unlock-keys") return json(200, { keys: [{ id: "key-1", status: "active", kind: "webfig" }] });
  if (url.pathname === "/api/v1/magic-dude/chat" && req.method === "POST") {
    let raw = "";
    for await (const chunk of req) raw += chunk;
    chatWrites.push(JSON.parse(raw || "{}"));
    return json(200, { answer: "Your fleet has one offline lab router.", usage: { total_tokens: 24 } });
  }
  if (url.pathname === "/api/v1/portal/config" && req.method === "PUT") {
    let raw = "";
    for await (const chunk of req) raw += chunk;
    const body = JSON.parse(raw || "{}");
    if (failNextPut) {
      failNextPut = false;
      return json(503, { error: "acceptance save failure" });
    }
    writes.push(body);
    config = { ...config, ...body };
    return json(200, { status: "ok", config });
  }
  let requested = url.pathname === "/" ? "/app.html" : url.pathname;
  const file = normalize(join(webRoot, requested));
  if (!file.startsWith(webRoot)) return json(403, { error: "forbidden" });
  try {
    const data = await readFile(file);
    res.writeHead(200, { "content-type": mime[extname(file)] || "application/octet-stream", "cache-control": "no-store" });
    res.end(data);
  } catch (_) {
    json(404, { error: "not found" });
  }
});

const listen = () => new Promise((resolveListen) => server.listen(0, "127.0.0.1", () => resolveListen(server.address().port)));
const delay = ms => new Promise(r => setTimeout(r, ms));
const waitFor = async (fn, label, timeout = 10000) => {
  const end = Date.now() + timeout;
  let last;
  while (Date.now() < end) {
    try { const value = await fn(); if (value) return value; } catch (e) { last = e; }
    await delay(50);
  }
  throw new Error(`timeout waiting for ${label}${last ? `: ${last.message}` : ""}`);
};

class CDP {
  constructor(ws) {
    this.ws = ws;
    this.id = 0;
    this.pending = new Map();
    ws.addEventListener("message", event => {
      const msg = JSON.parse(String(event.data));
      if (!msg.id || !this.pending.has(msg.id)) return;
      const { resolve: ok, reject } = this.pending.get(msg.id);
      this.pending.delete(msg.id);
      msg.error ? reject(new Error(msg.error.message)) : ok(msg.result || {});
    });
  }
  send(method, params = {}) {
    const id = ++this.id;
    return new Promise((resolveSend, reject) => {
      this.pending.set(id, { resolve: resolveSend, reject });
      this.ws.send(JSON.stringify({ id, method, params }));
    });
  }
  async eval(expression) {
    const out = await this.send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true });
    if (out.exceptionDetails) throw new Error(out.exceptionDetails.exception?.description || out.exceptionDetails.text);
    return out.result?.value;
  }
  async pointerClick(selector, inFrame = false) {
    const point = await this.eval(`(() => {
      const doc = ${inFrame ? "document.querySelector('#pb-frame').contentDocument" : "document"};
      const el = doc && doc.querySelector(${JSON.stringify(selector)});
      if (!el) return null;
      ${inFrame ? "document.querySelector('#pb-frame').scrollIntoView({block:'center'}); el.scrollIntoView({block:'center'});" : "el.scrollIntoView({block:'center'});"}
      const r = el.getBoundingClientRect();
      ${inFrame ? "const f=document.querySelector('#pb-frame').getBoundingClientRect(); return {x:f.left+r.left+r.width/2,y:f.top+r.top+r.height/2};" : "return {x:r.left+r.width/2,y:r.top+r.height/2};"}
    })()`);
    assert.ok(point, `pointer target exists: ${selector}`);
    await this.send("Input.dispatchMouseEvent", { type: "mouseMoved", x: point.x, y: point.y });
    await this.send("Input.dispatchMouseEvent", { type: "mousePressed", x: point.x, y: point.y, button: "left", clickCount: 1 });
    await this.send("Input.dispatchMouseEvent", { type: "mouseReleased", x: point.x, y: point.y, button: "left", clickCount: 1 });
  }
  async activateInFrame(selector) {
    const found = await this.eval(`(() => {
      const el=document.querySelector('#pb-frame').contentDocument?.querySelector(${JSON.stringify(selector)});
      if (!el) return false; el.click(); return true;
    })()`);
    assert.ok(found, `iframe control exists: ${selector}`);
  }
}

async function portalDeploymentAcceptance(cdp) {
  await waitFor(() => cdp.eval("document.querySelector('#pd-router option[value=\"dev-1\"]') !== null"), "portal gateway selector");
  await cdp.eval("document.querySelector('#pd-router').value='dev-1'; document.querySelector('#pd-router').dispatchEvent(new Event('change'))");
  await waitFor(() => cdp.eval("document.querySelector('#pd-profile option[value=\"guest\"]') !== null"), "live profile discovery");
  await cdp.eval("document.querySelector('#pd-profile').value='guest'; document.querySelector('#pd-profile').dispatchEvent(new Event('change'))");
  assert.equal(await cdp.eval("document.querySelector('#pd-apply').hidden"),true,"apply requires review");
  await cdp.pointerClick("#pd-preview");
  await waitFor(() => cdp.eval("document.querySelector('#pd-apply').hidden === false"), "portal deployment review");
  assert.equal(portalDeployments.length,0,"review does not apply changes");
  await cdp.pointerClick("#pd-apply");
  await waitFor(() => cdp.eval("document.querySelector('#pd-status').textContent.includes('Router files and access rule verified')"), "portal router readback result");
  assert.equal(portalDeployments.length,1,"explicit deployment writes once");
  assert.equal(await cdp.eval("document.querySelector('#pd-apply').hidden"),true,"completed preview cannot be replayed");
}

let browser;
let profile;
let ws;
try {
  const port = await listen();
  if (process.env.E2E_HOLD_MS) console.log(`Preview server: http://127.0.0.1:${port}/app.html#portal`);
  profile = await mkdtemp(join(tmpdir(), "mm-portal-ui-"));
  const debugPort = 9400 + Math.floor(Math.random() * 400);
  browser = spawn(chrome, [
    "--headless=new", `--remote-debugging-port=${debugPort}`, `--user-data-dir=${profile}`,
    "--no-first-run", "--no-default-browser-check", "--disable-background-networking", "about:blank",
  ], { stdio: "ignore" });
  const version = await waitFor(async () => {
    const r = await fetch(`http://127.0.0.1:${debugPort}/json/version`).catch(() => null);
    return r?.ok ? r.json() : null;
  }, "Chrome DevTools");
  assert.ok(version.Browser.includes("Chrome"));
  const target = await fetch(`http://127.0.0.1:${debugPort}/json/new?about:blank`, { method: "PUT" }).then(r => r.json());
  ws = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((ok, bad) => { ws.addEventListener("open", ok, { once: true }); ws.addEventListener("error", bad, { once: true }); });
  const cdp = new CDP(ws);
  await cdp.send("Runtime.enable");
  await cdp.send("Page.enable");
  await cdp.send("Page.addScriptToEvaluateOnNewDocument", { source: `localStorage.setItem('mm_token','acceptance-token')` });
  await cdp.send("Page.navigate", { url: `http://127.0.0.1:${port}/app.html#portal` });
  await waitFor(() => cdp.eval("!!document.querySelector('#plan-add')"), "portal editor");
  if (process.env.E2E_REVENUE_ONLY) {
    await cdp.eval("loadView('revenue',document.querySelector('#main'),ME)");
    await waitFor(()=>cdp.eval("!!document.querySelector('[data-voucher-summary]')"),'voucher activation summary');
    assert.equal(await cdp.eval("document.querySelector('[data-voucher-summary]').textContent.includes('6,500 MMK')"),true,'Full recorded activation value is shown');
    assert.equal(await cdp.eval("document.querySelector('#voucher-activation-report').textContent.includes('74,500 MMK')"),true,'Unused inventory face value stays separate');
    assert.equal(await cdp.eval("document.querySelector('[data-portal-summary]').textContent.includes('6,500')"),false,'Voucher activations are not added to portal payments');
    await cdp.pointerClick('[data-purchase-source="guest-wifi"][data-purchase-action="approve"]');
    await waitFor(()=>cdp.eval("document.querySelector('[data-portal-summary]')?.textContent.includes('3,500 MMK')"),'approved portal payment readback');
    assert.equal(await cdp.eval("document.querySelector('[data-voucher-summary]').textContent.includes('6,500 MMK')"),true,'Approving a payment does not activate or sell another voucher');
    if (process.env.E2E_SCREENSHOT) {
      await cdp.send('Emulation.setDeviceMetricsOverride',{width:1440,height:1080,deviceScaleFactor:1,mobile:false});
      await cdp.eval("document.querySelector('#voucher-activation-report').scrollIntoView({block:'start'})");
      const shot=await cdp.send('Page.captureScreenshot',{format:'png'});
      await writeFile(process.env.E2E_SCREENSHOT,Buffer.from(shot.data,'base64'));
    }
    await cdp.send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:2,mobile:true});
    assert.equal(await cdp.eval('document.documentElement.scrollWidth<=innerWidth+1'),true,'Voucher reporting fits a phone viewport');
    console.log('Revenue Chromium acceptance passed: voucher activation value, unused inventory, portal payment approval/readback and mobile layout remain separate.');
  } else if (process.env.E2E_VOUCHER_BATCH_ONLY) {
    await cdp.eval("loadView('vouchers',document.querySelector('#main'),ME)");
    await waitFor(()=>cdp.eval("!!document.querySelector('#guest-voucher-form')"),'guest voucher issuance form');
    await cdp.eval("document.querySelector('#gv-gateway').value='dev-1';document.querySelector('#gv-label').value='Daily Wi-Fi';document.querySelector('#gv-count').value='105';document.querySelector('#gv-price').value='1500'");
    await cdp.pointerClick('#gv-create');
    await waitFor(()=>cdp.eval("document.querySelector('#pl-batch-source')?.value==='issued'"),'new batch selected for printing');
    assert.equal(issuedVoucherBatches.length,1,'one batch is issued');
    assert.equal(issuedVoucherBatches[0].device_id,'dev-1');
    assert.equal(await cdp.eval("document.querySelector('#pl-batch-range').textContent.includes('1–100 of 105')"),true);
    assert.equal(await cdp.eval("document.querySelector('#pl-stage').textContent.includes('OLDER-INVENTORY')"),false);
    await cdp.pointerClick('#pl-print');
    await waitFor(()=>cdp.eval("document.querySelector('iframe[aria-hidden=true]')?.contentDocument?.querySelectorAll('.sheet-code').length===100"),'first selection print document');
    assert.equal(await cdp.eval("document.querySelector('iframe[aria-hidden=true]').contentDocument.body.textContent.includes('1500 MMK')"),true);
    assert.equal(await cdp.eval("document.querySelector('iframe[aria-hidden=true]').contentDocument.body.textContent.includes('1440 min from first use · 1024 MB')"),true);
    assert.equal(await cdp.eval("document.querySelector('iframe[aria-hidden=true]').contentDocument.body.textContent.includes('SAMPLE-')"),false);
    assert.equal(await cdp.eval("[...document.querySelector('#pl-stage').querySelectorAll('.sheet-code')].every(code=>{const c=code.getBoundingClientRect(),t=code.closest('.sheet-tile').getBoundingClientRect();return c.top>=t.top&&c.bottom<=t.bottom})"),true,'Voucher codes stay visible in the preview');
    assert.equal(await cdp.eval("[...document.querySelector('#pl-stage').querySelectorAll('.sheet-tile > div')].every(line=>{const r=line.getBoundingClientRect(),t=line.closest('.sheet-tile').getBoundingClientRect();return r.top>=t.top&&r.bottom<=t.bottom})"),true,'Preview branding, price and allowance lines fit inside their cut lines');
    assert.equal(await cdp.eval("(() => { const doc=document.querySelector('iframe[aria-hidden=true]').contentDocument;return [...doc.querySelectorAll('.sheet-tile > div')].every(line=>{const r=line.getBoundingClientRect(),t=line.closest('.sheet-tile').getBoundingClientRect();return r.top>=t.top&&r.bottom<=t.bottom})})()"),true,'Printed details fit the same border boxes as the preview');
    assert.equal(await cdp.eval("(() => {const doc=document.querySelector('iframe[aria-hidden=true]').contentDocument;return [...doc.querySelectorAll('.sheet')].every(sheet=>[...sheet.querySelectorAll('.sheet-tile')].every(tile=>tile.getBoundingClientRect().bottom<=sheet.getBoundingClientRect().bottom+0.1))})()"),true,'Grid gaps and padding cannot push the last row past the A4 page');
    await cdp.pointerClick('#pl-batch-next');
    assert.equal(await cdp.eval("document.querySelector('#pl-batch-range').textContent.includes('101–105 of 105')"),true);
    await cdp.pointerClick('#pl-print');
    await waitFor(()=>cdp.eval("document.querySelectorAll('iframe[aria-hidden=true]')[1]?.contentDocument?.querySelectorAll('.sheet-code').length===5"),'last selection print document');
    if (process.env.E2E_SCREENSHOT) {
      await cdp.send('Emulation.setDeviceMetricsOverride',{width:1280,height:960,deviceScaleFactor:1,mobile:false});
      await cdp.eval("document.querySelector('#pl-a4r').dispatchEvent(new Event('input'))");
      await cdp.eval("document.querySelector('#pl-batch-source').scrollIntoView({block:'start'})");
      const shot=await cdp.send('Page.captureScreenshot',{format:'png'});
      await writeFile(process.env.E2E_SCREENSHOT,Buffer.from(shot.data,'base64'));
    }
    await cdp.pointerClick('#pl-mode-thermal');
    assert.equal(await cdp.eval("[...document.querySelector('#pl-stage').querySelectorAll('.sheet-tile > div')].every(line=>{const r=line.getBoundingClientRect(),t=line.closest('.sheet-tile').getBoundingClientRect();return r.top>=t.top&&r.bottom<=t.bottom})"),true,'Default thermal details fit their tiles');
    await cdp.eval("document.querySelector('#pl-tr').value='40';document.querySelector('#pl-tr').dispatchEvent(new Event('input'))");
    assert.equal(await cdp.eval("[...document.querySelector('#pl-stage').querySelectorAll('.sheet-code')].every(code=>{const r=code.getBoundingClientRect(),t=code.closest('.sheet-tile').getBoundingClientRect();return r.top>=t.top&&r.bottom<=t.bottom})"),true,'Dense continuous strips keep readable codes inside their tiles');
    await cdp.send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:2,mobile:true});
    assert.equal(await cdp.eval('document.documentElement.scrollWidth<=innerWidth+1'),true,'Voucher batch preview fits mobile');
    console.log('Voucher batch Chromium acceptance passed: one issuance, exact returned batch, 100 + 5 print selections, price/time/data details, no samples or older codes, and mobile layout.');
  } else if (process.env.E2E_NETWORK_CONTROL_ONLY) {
    const choose=async view=>{
      await cdp.eval(`loadView(${JSON.stringify(view)},document.querySelector('#main'),ME)`);
      await waitFor(()=>cdp.eval("!!document.querySelector('[data-router]')"),'network device selector');
      await cdp.eval("document.querySelector('[data-router]').value='dev-1';document.querySelector('[data-router]').dispatchEvent(new Event('change'))");
      await waitFor(()=>cdp.eval("!!document.querySelector('[data-form]')"),'network form');
    };
    await choose('wan');
    assert.equal(await cdp.eval("document.querySelector('[name=wan1]').value"),'','uplink choice is explicit');
    await cdp.eval("for(const n of ['1','2']){document.querySelector('[name=wan'+n+']').value='ether'+n;document.querySelector('[name=wan'+n+']').dispatchEvent(new Event('change',{bubbles:true}))}");
    assert.equal(await cdp.eval("document.querySelector('[name=gateway1]').value"),'192.0.2.1','detected DHCP gateway');
    assert.equal(await cdp.eval("!!document.querySelector('[data-apply]')"),false,'no apply without review');
    await cdp.eval("document.querySelector('[data-form]').requestSubmit();document.querySelector('[name=gateway1]').dispatchEvent(new Event('input',{bubbles:true}))");
    await delay(350);
    assert.equal(await cdp.eval("!!document.querySelector('[data-apply]')"),false,'stale review response discarded');
    await cdp.eval("document.querySelector('[data-form]').requestSubmit()");
    await waitFor(()=>cdp.eval("!!document.querySelector('[data-apply]')"),'review');
    assert.equal(await cdp.eval("document.querySelector('[data-apply]').disabled"),true,'explicit consent needed');
    await cdp.eval("document.querySelector('[data-consent]').checked=true;document.querySelector('[data-consent]').dispatchEvent(new Event('change'));document.querySelector('[data-apply]').click()");
    await waitFor(()=>cdp.eval("document.querySelector('[data-review]').textContent.includes('Router settings verified')"),'verified result');
    assert.equal(writes.filter(w=>w.network_apply).length,1,'one apply');
    await choose('pppoe');
    assert.equal(await cdp.eval("document.querySelector('[name=gateway_cidr]').value"),'192.168.52.1/24','live-checked suggested subscriber prefix');
    await choose('wifi');
    assert.equal(await cdp.eval("document.querySelector('[name=ssid]').value"),'Business','live SSID loaded');
    assert.equal(await cdp.eval("document.querySelector('[name=password]').value"),'','existing password never returned');
    await cdp.eval("document.querySelector('[name=password]').value='new-fixture-secret';document.querySelector('[data-form]').requestSubmit()");
    await waitFor(()=>cdp.eval("!!document.querySelector('[data-apply]')"),'Wi-Fi review');
    assert.equal(await cdp.eval("document.querySelector('[data-review]').textContent.includes('new-fixture-secret')"),false,'review hides secrets');
    assert.equal(await cdp.eval("JSON.stringify(localStorage).includes('new-fixture-secret')"),false,'no password browser draft');
    await choose('existing');
    assert.equal(await cdp.eval("document.querySelector('[name=lease-time]').value"),'1h','existing DHCP fields loaded');
    await cdp.send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:2,mobile:true});
    assert.equal(await cdp.eval("document.documentElement.scrollWidth<=innerWidth+1"),true,'mobile has no horizontal overflow');
    console.log('Network controls browser acceptance passed: dedicated tasks, detected values, stale review protection, consent, verified apply, secret handling, existing records and mobile layout.');
  } else
  if (process.env.E2E_NETWORK_SETUP_ONLY) {
    await cdp.eval("loadView('network', document.querySelector('#main'), ME)");
    await waitFor(() => cdp.eval("!!document.querySelector('#network-router')"), "network workspace");
    assert.equal(await cdp.eval("!!document.querySelector('[data-view=network]')"), true, "network has a dedicated navigation item");
    await cdp.eval("document.querySelector('#network-router').value='dev-1';document.querySelector('#network-router').dispatchEvent(new Event('change'))");
    await waitFor(() => cdp.eval("!!document.querySelector('#gateway-setup-form')"), "suggested form");
    assert.equal(await cdp.eval("document.querySelector('[name=gateway_cidr]').value"), "192.168.52.1/24");
    assert.equal(await cdp.eval("document.querySelector('[name=wan_interface]').value"), "", "uplink is not guessed");
    assert.equal(await cdp.eval("document.querySelector('[name=lan_interface]').value"), "", "LAN port is not guessed");
    assert.equal(await cdp.eval("document.querySelector('#gateway-apply').hidden"), true, "no apply before preview");
    await cdp.eval("document.querySelector('[name=wan_interface]').value='ether1';document.querySelector('[name=lan_interface]').value='ether2';document.querySelector('#gateway-setup-form').requestSubmit()");
    await waitFor(() => cdp.eval("!document.querySelector('#gateway-apply').hidden"), "preview");
    await cdp.eval("document.querySelector('[name=name]').dispatchEvent(new Event('input',{bubbles:true}))");
    assert.equal(await cdp.eval("document.querySelector('#gateway-apply').hidden"), true, "edits invalidate preview");
    await cdp.eval("document.querySelector('#gateway-setup-form').requestSubmit()");
    await waitFor(() => cdp.eval("!document.querySelector('#gateway-apply').hidden"), "fresh preview");
    await cdp.eval("window.confirm=()=>true;document.querySelector('#gateway-apply').click()");
    await waitFor(() => cdp.eval("document.querySelector('#gateway-status').textContent.includes('applied and verified')"), "verified synthetic apply");
    await cdp.send("Emulation.setDeviceMetricsOverride", {width:390,height:844,deviceScaleFactor:2,mobile:true});
    assert.equal(await cdp.eval("document.documentElement.scrollWidth <= innerWidth"),true,"network setup fits mobile");
    console.log("Network setup browser acceptance passed: navigation, live suggestions, explicit ports, preview invalidation, synthetic verified apply, mobile layout.");
  } else if (process.env.E2E_PORTAL_DEPLOY_ONLY) {
    await portalDeploymentAcceptance(cdp);
    await cdp.send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
    assert.equal(await cdp.eval("document.documentElement.scrollWidth <= window.innerWidth"), true, "portal deployment fits phone viewport");
    console.log("✓ Portal deployment browser acceptance passed: gateway/profile discovery, review without writes, explicit deployment, verified result, replay prevention and phone layout.");
  } else {
  assert.equal(await cdp.eval("document.querySelectorAll('#plans-table tbody tr').length"), 1);
  assert.equal(await cdp.eval("document.querySelector('#grace-data').value"), "50", "default grace data is 50 MB");
  await cdp.eval(`(() => { document.querySelector('#grace-min').value='7'; document.querySelector('#grace-data').value='75'; })()`);
  await cdp.pointerClick("#plan-save");
  await waitFor(() => config.guest_grace_minutes === 7 && config.guest_grace_data_mb === 75, "custom grace limits saved");
  await waitFor(() => cdp.eval("document.querySelector('#plan-msg')?.textContent.includes('verified from the server')"), "grace limits server readback");

  // Magic Dude is a persistent popup, not a route-sized page. Opening and
  // chatting must leave the current Portal workflow intact underneath it.
  await cdp.pointerClick("#magic-dude-launcher");
  await waitFor(() => cdp.eval("!document.querySelector('#magic-dude-popup').hidden"), "Magic Dude popup");
  assert.equal(await cdp.eval("location.hash"), "#portal", "opening chat preserves the current view");
  assert.equal(await cdp.eval("!!document.querySelector('.md-layout')"), false, "legacy full-page chat layout is absent");
  assert.equal(await cdp.eval("!!document.querySelector('#magic-dude-nav')"), false, "Magic Dude has no sidebar page entry");
  assert.equal(await cdp.eval("document.querySelector('.md-popup-identity .md-avatar').naturalWidth > 0"), true, "Magic Dude mascot loads in the popup");
  assert.equal(await cdp.eval("document.querySelector('#magic-dude-launcher .md-avatar').naturalWidth > 0"), true, "Magic Dude mascot loads in the floating launcher");
  await cdp.eval(`(() => { const e=document.querySelector('#md-question'); e.value='What needs attention?'; e.dispatchEvent(new Event('input',{bubbles:true})); })()`);
  await cdp.pointerClick("#md-send");
  await waitFor(() => chatWrites.length === 1, "Magic Dude API write");
  await waitFor(() => cdp.eval("document.querySelector('#md-messages').textContent.includes('one offline lab router')"), "Magic Dude answer");
  await cdp.pointerClick("#md-close");
  assert.equal(await cdp.eval("document.querySelector('#magic-dude-popup').hidden"), true, "chat closes without navigating");

  // A slower page must never overwrite a newer navigation choice. The last
  // click wins even when the first route is still waiting for live data.
  await cdp.eval(`(() => {
    document.querySelector('[data-view=fleet]').click();
    document.querySelector('[data-view=templates]').click();
  })()`);
  await waitFor(() => cdp.eval("location.hash === '#templates' && document.querySelector('main h1')?.textContent.trim() === 'Templates'"), "latest navigation choice");
  await delay(350);
  assert.equal(await cdp.eval("location.hash === '#templates' && document.querySelector('main h1')?.textContent.trim() === 'Templates'"), true, "an older response cannot replace the latest page");

  // The recovered Revenue workspace must use real billing records and the
  // current decision endpoint, not placeholder chart data.
  await cdp.eval("loadView('revenue', document.querySelector('#main'), ME)");
  await waitFor(() => cdp.eval("!!document.querySelector('#revenue-export')"), "revenue workspace");
  assert.equal(await cdp.eval("document.querySelector('[data-portal-summary]').textContent.includes('0 MMK')"), true, "legacy business-pass revenue is excluded from guest sales");
  assert.equal(await cdp.eval("document.querySelectorAll('[data-purchase-action=approve]').length"), 1, "pending request exposes a real approval control");
  await cdp.pointerClick("[data-purchase-action=approve]");
  await waitFor(() => guestPurchaseRequests.find(r => r.id === "guest-pending")?.status === "approved", "guest revenue approval API write");
  await waitFor(() => cdp.eval("document.querySelector('[data-portal-summary]').textContent.includes('3,500 MMK')"), "guest-only revenue readback after approval");

  await cdp.eval("loadView('portal', document.querySelector('#main'), ME)");
  await waitFor(() => cdp.eval("!!document.querySelector('#plan-add')"), "portal editor after revenue");

  // Fleet uses the compact systems-board grammar from the owner's reference
  // video, but all values must remain traceable to current APIs.
  await cdp.eval("loadView('fleet', document.querySelector('#main'), ME)");
  await waitFor(() => cdp.eval("!!document.querySelector('.fleet-console')"), "fleet systems board");
  assert.equal(await cdp.eval("document.querySelector('.fleet-board-row').textContent.includes('CHR lab router')"), true, "fleet board shows the API router");
  assert.equal(await cdp.eval("document.querySelector('.fleet-board-row').textContent.includes('OFFLINE')"), true, "fleet board shows real reachability");
  assert.equal(await cdp.eval(`document.querySelector('.fleet-console').textContent.includes("Updated from each router's management path")`), true, "fleet board explains where its live status comes from");

  await cdp.eval("loadView('topology', document.querySelector('#main'), ME)");
  await waitFor(() => cdp.eval("!!document.querySelector('.topo-console')"), "topology systems board");
  assert.equal(await cdp.eval("document.querySelectorAll('.topo-path-row').length"), 2, "topology path index contains real mapped nodes");
  assert.equal(await cdp.eval("document.querySelector('.topo-console').textContent.includes('Select a node to inspect its details')"), true, "topology tells the operator how to inspect a device");
  await cdp.pointerClick(".topo-path-row:nth-child(2)");
  assert.equal(await cdp.eval("[...document.querySelectorAll('.topo-detail')].find(e => !e.hidden)?.textContent.includes('Lobby access point')"), true, "path selection opens the matching device inspector");
  assert.equal(await cdp.eval("!!document.querySelector('#topo-refresh') && !!document.querySelector('#topo-manage-routers')"), true, "topology operational controls are present");
  assert.equal(await cdp.eval("getComputedStyle(document.querySelector('.topo-device .topo-signal')).display"), "grid", "topology node renders segmented reachability signal");
  assert.equal(await cdp.eval("document.querySelector('.topo-modebar').textContent.includes('Ports · awaiting RouterOS inventory')"), true, "topology exposes the honest port-inventory boundary");

  // Recovered project areas are separate, visible workspaces. Their values
  // must be API-backed and unfinished engines must not produce fake actions.
  await cdp.eval("loadView('sites', document.querySelector('#main'), ME)");
  await waitFor(() => cdp.eval("!!document.querySelector('.site-ops-console')"), "sites operations workspace");
  assert.equal(await cdp.eval("document.querySelector('.site-map-pin').textContent.includes('Downtown Café')"), true, "site coordinate pin uses saved site data");
  await cdp.pointerClick(".site-location-card");
  await waitFor(() => cdp.eval("!!document.querySelector('.site-readiness')"), "site Go Live readiness");
  assert.equal(await cdp.eval("document.querySelector('.site-readiness').textContent.includes('Live signal')"), true, "site readiness includes the verified management signal");

  await cdp.eval("loadView('routers', document.querySelector('#main'), ME)");
  await waitFor(() => cdp.eval("!!document.querySelector('.routerboard-card')"), "RouterBOARD workspace");
  assert.equal(await cdp.eval("document.querySelector('.routerboard-card').textContent.includes('CHR lab router')"), true, "RouterBOARD card uses registered device inventory");
  assert.equal(await cdp.eval("document.querySelector('.routerboard-card').textContent.includes('WebFig status')"), true, "RouterBOARD capability state is visible");
  await cdp.pointerClick("[data-router-capability=webfig]");
  await waitFor(() => cdp.eval("document.querySelector('#router-capability-dialog').open"), "WebFig status dialog");
  assert.equal(await cdp.eval("document.querySelector('#router-capability-content').textContent.includes('Backup and configure secure WebFig')"), true, "WebFig offers a real secure configuration action");
  await cdp.eval("window.confirm=()=>true");
  await cdp.pointerClick("#webfig-secure");
  await waitFor(() => cdp.eval("document.querySelector('#webfig-status').textContent.includes('configured and verified')"), "WebFig secure apply result");
  await cdp.eval("document.querySelector('#router-capability-dialog').close()");
  await cdp.eval("loadView('network', document.querySelector('#main'), ME)");
  await waitFor(() => cdp.eval("!!document.querySelector('#network-router')"), "network workspace");
  await cdp.eval("document.querySelector('#network-router').value='dev-1';document.querySelector('#network-router').dispatchEvent(new Event('change'))");
  await waitFor(() => cdp.eval("!!document.querySelector('#gateway-setup-form')"), "functional gateway setup form");
  await cdp.eval(`(() => { const f=document.querySelector('#gateway-setup-form'); const values={name:'guest',wan_interface:'ether1',lan_interface:'ether2',gateway_cidr:'192.168.50.1/24',pool_start:'192.168.50.10',pool_end:'192.168.50.200'}; for(const [k,v] of Object.entries(values)) f.elements[k].value=v; })()`);
  await cdp.pointerClick("#gateway-preview");
  await waitFor(() => cdp.eval("!document.querySelector('#gateway-apply').hidden"), "gateway preview");
  assert.equal(await cdp.eval("document.querySelector('#gateway-status').textContent.includes('Create DHCP pool')"),true,"gateway preview shows concrete changes");
  await cdp.eval("document.querySelector('#gateway-setup-form').elements.name.dispatchEvent(new Event('input',{bubbles:true}))");
  assert.equal(await cdp.eval("document.querySelector('#gateway-apply').hidden"),true,"edited gateway settings invalidate the preview");
  await cdp.pointerClick("#gateway-preview");
  await waitFor(() => cdp.eval("!document.querySelector('#gateway-apply').hidden"), "fresh gateway preview");
  await cdp.eval("window.confirm=()=>true");
  await cdp.pointerClick("#gateway-apply");
  await waitFor(() => cdp.eval("document.querySelector('#gateway-status').textContent.includes('applied and verified')"), "gateway apply result");
  await cdp.eval("loadView('store', document.querySelector('#main'), ME)");
  await waitFor(() => cdp.eval("!!document.querySelector('.store-hero')"), "Store workspace");
  assert.equal(await cdp.eval("document.querySelector('.store-products').textContent.includes('350,000 MMK')"), true, "Store reads the published annual price");
  assert.equal(await cdp.eval("document.querySelector('.store-wallet').textContent.includes('24.00')"), true, "Store reads the real Magic Coin wallet");
  assert.equal(await cdp.eval("[...document.querySelectorAll('.store-product button')].some(button => button.textContent.includes('Payment account unavailable') && button.disabled)"), true, "checkout stays unavailable until a payment account exists");

  if (await cdp.eval("getComputedStyle(document.querySelector('#rail-toggle')).display !== 'none'")) await cdp.pointerClick("#rail-toggle");
  const profileHit = await cdp.eval(`(() => { const el=document.querySelector('.rail-profile'); el.scrollIntoView({block:'center'}); const r=el.getBoundingClientRect(); const hit=document.elementFromPoint(r.left+r.width/2,r.top+r.height/2); return {rect:{x:r.x,y:r.y,w:r.width,h:r.height},viewport:{w:innerWidth,h:innerHeight},tag:hit?.tagName, cls:hit?.className, text:hit?.textContent?.trim().slice(0,40)}; })()`);
  assert.equal(String(profileHit.cls || '').includes('rail-profile') || profileHit.cls === 'rail-profile-copy' || profileHit.cls === 'rail-profile-name' || profileHit.cls === 'rail-profile-role' || profileHit.cls === 'rail-profile-avatar', true, `profile pointer hit target (${JSON.stringify(profileHit)})`);
  await cdp.pointerClick(".rail-profile");
  await delay(100);
  assert.equal(await cdp.eval("location.hash"), "#profile", "sidebar identity click navigates to profile");
  await waitFor(() => cdp.eval("!!document.querySelector('#profile-form') || !!document.querySelector('.error-empty')"), "personal profile workspace");
  assert.equal(await cdp.eval("document.querySelector('.error-empty')?.textContent || ''"), "", "profile loads without an API/rendering error");
  assert.equal(await cdp.eval("location.hash"), "#profile", "sidebar identity opens the profile page");
  assert.equal(await cdp.eval(`(() => { const image=document.querySelector('#profile-cover-image'); image.hidden=false; const hidden=getComputedStyle(document.querySelector('.profile-cover-fallback')).display === 'none'; image.hidden=true; return hidden; })()`), true, "uploaded timeline artwork is not obscured by its placeholder");
  await cdp.eval(`(() => { const n=document.querySelector('#profile-name'); n.value='Portal QA Updated'; n.dispatchEvent(new Event('input',{bubbles:true})); document.querySelector('#profile-language').value='my'; })()`);
  await cdp.pointerClick("#profile-form button[type=submit]");
  await waitFor(() => profileWrites.length === 1, "profile API write");
  assert.equal(profileWrites[0].language, "my", "language preference is persisted");
  await cdp.eval(`(() => { document.querySelector('#current-password').value='current-secret'; document.querySelector('#new-password').value='new-secret-123'; })()`);
  await cdp.pointerClick("#password-form button[type=submit]");
  await waitFor(() => passwordWrites.length === 1, "password API write");
  assert.equal(passwordWrites[0].current_password, "current-secret", "password change sends current-password proof");
  await cdp.pointerClick("#view-guide");
  await waitFor(() => cdp.eval("!!document.querySelector('#guide-grid')"), "product guide workspace");
  assert.equal(await cdp.eval("document.querySelectorAll('[data-guide-card]').length >= 20"), true, "guide covers every customer workspace");
  await waitFor(() => cdp.eval("document.querySelector('.guide-hero img')?.complete && document.querySelector('.guide-hero img')?.naturalWidth > 0"), "Magic Dude guide artwork");
  assert.equal(await cdp.eval("document.querySelector('.guide-hero img').naturalWidth > 0"), true, "Magic Dude guide artwork loads");

  // Shared shell spacing applies to every view, and route changes reset the
  // previous scroll offset so headings cannot enter already clipped.
  await cdp.eval("window.scrollTo(0, 600)");
  await cdp.eval("loadView('sites', document.querySelector('#main'), ME)");
  await waitFor(() => cdp.eval("!!document.querySelector('.capacity-list')"), "sites spacing audit");
  assert.equal(await cdp.eval("window.scrollY === 0 && document.querySelector('#main').scrollTop === 0"), true, "route change resets retained scroll position");
  assert.equal(await cdp.eval(`(() => {
    const panels=[...document.querySelectorAll('#content > .panel + .panel')];
    return panels.length > 0 && panels.every(panel => panel.getBoundingClientRect().top - panel.previousElementSibling.getBoundingClientRect().bottom >= 15);
  })()`), true, "adjacent top-level panels use the shared 16px section rhythm");

  await cdp.eval("loadView('portal', document.querySelector('#main'), ME)");
  await waitFor(() => cdp.eval("!!document.querySelector('#plan-add')"), "portal editor after fleet");

  assert.equal(await cdp.eval("document.querySelector('#portal-voucher-enabled').checked"), true, "voucher access is owner-selectable");
  assert.equal(await cdp.eval("document.querySelector('#portal-payment-enabled').checked"), true, "mobile payment is owner-selectable");
  await cdp.pointerClick("#portal-payment-enabled");
  await cdp.pointerClick("#plan-save");
  await waitFor(() => config.mobile_payment_enabled === false, "voucher-only selection saved");
  await waitFor(() => cdp.eval("document.querySelector('#plan-msg')?.textContent.includes('verified from the server')"), "voucher-only server readback");
  await waitFor(() => cdp.eval("!!document.querySelector('#pb-frame').contentDocument?.querySelector('#voucher-submit')"), "voucher-only preview");
  await waitFor(() => cdp.eval("document.querySelector('#portal-payment-enabled')?.checked === false"), "voucher-only editor readback");
  assert.equal(await cdp.eval("!!document.querySelector('#pb-frame').contentDocument?.querySelector('#portal-go-payment')"), false, "voucher-only portal has no payment navigation");
  await cdp.pointerClick("#portal-payment-enabled");
  await cdp.pointerClick("#plan-save");
  await waitFor(() => config.mobile_payment_enabled === true, "combined access selection restored");
  await waitFor(() => cdp.eval("document.querySelector('#plan-msg')?.textContent.includes('verified from the server')"), "combined access server readback");

  await cdp.pointerClick("#plan-add");
  await waitFor(() => cdp.eval("document.querySelectorAll('#plans-table tbody tr').length===2"), "Add plan pointer click");
  await cdp.eval(`(() => { const row=document.querySelectorAll('#plans-table tbody tr')[1];
    const values={key:'day',label:'Day pass',minutes:'1440',amount:'3500',data_mb:'5120'};
    for(const input of row.querySelectorAll('input[data-k]')) { input.value=values[input.dataset.k]; input.dispatchEvent(new Event('input',{bubbles:true})); }
  })()`);
  await cdp.pointerClick("#plan-save");
  await waitFor(() => writes.some(w => w.guest_plans?.length === 2), "plans API write");
  await waitFor(() => cdp.eval("document.querySelector('#plan-msg')?.textContent.includes('verified from the server')"), "plans readback confirmation");
  assert.equal(config.guest_plans.length, 2);

  await cdp.pointerClick("#plans-table tbody tr:last-child button[data-act=del]");
  await waitFor(() => cdp.eval("document.querySelectorAll('#plans-table tbody tr').length===1"), "Remove pointer click");

  await cdp.eval(`(() => { const e=document.querySelector('#pb-name'); e.value='Acceptance Coffee Updated'; e.dispatchEvent(new Event('input',{bubbles:true})); })()`);
  await cdp.pointerClick("#pb-save");
  await waitFor(() => config.business_name === "Acceptance Coffee Updated", "branding API write");
  await waitFor(() => cdp.eval("document.querySelector('#pb-msg')?.textContent.includes('verified from the server')"), "branding readback confirmation");

  await cdp.pointerClick("#pb-lang-my");
  assert.equal(await cdp.eval("document.querySelector('#pb-lang-my').getAttribute('aria-pressed')"), "true");
  await waitFor(() => cdp.eval("document.querySelector('#pb-frame').contentDocument?.documentElement.lang === 'my-MM'"), "Myanmar preview");
  await cdp.pointerClick("#pb-lang-en");
  await waitFor(() => cdp.eval("document.querySelector('#pb-frame').contentDocument?.querySelector('h1')?.textContent === 'Connect to WiFi'"), "English preview");

  await waitFor(() => cdp.eval("!!document.querySelector('#pb-frame').contentDocument?.querySelector('#voucher-submit')"), "voucher-first preview");
  assert.equal(await cdp.eval("!!document.querySelector('#pb-frame').contentDocument?.querySelector('.plan')"), false, "plans are not mixed into the voucher screen");
  assert.equal(await cdp.eval("getComputedStyle(document.querySelector('#pb-frame').contentDocument.body).backgroundColor"), "rgb(5, 7, 15)", "guest portal restores the original deep mesh foundation");
  assert.equal(await cdp.eval("parseFloat(getComputedStyle(document.querySelector('#pb-frame').contentDocument.querySelector('.guest')).borderRadius) >= 24"), true, "guest experience uses the original rounded glass surface");
  assert.equal(await cdp.eval("parseFloat(getComputedStyle(document.querySelector('#pb-frame').contentDocument.querySelector('#voucher-code')).fontSize) >= 16"), true, "guest voucher input prevents iOS focus zoom");
  await cdp.eval(`(() => { const d=document.querySelector('#pb-frame').contentDocument; d.querySelector('#voucher-code').value='TEST-1234'; })()`);
  await cdp.activateInFrame("#voucher-submit");
  await waitFor(() => cdp.eval("document.querySelector('#pb-frame').contentDocument?.querySelector('#voucher-msg')?.textContent.includes('Preview only')"), "voucher simulated result");
  await cdp.activateInFrame("#portal-go-payment");
  await waitFor(() => cdp.eval("!!document.querySelector('#pb-frame').contentDocument?.querySelector('.plan')"), "separate payment-plan screen");
  assert.equal(await cdp.eval("!!document.querySelector('#pb-frame').contentDocument?.querySelector('#portal-back-voucher')"), true, "payment screen can return to voucher login");
  await waitFor(() => cdp.eval("(() => { const el=document.querySelector('#pb-frame').contentDocument?.querySelector('.plan'); if(!el) return false; el.click(); return true; })()"), "select payment plan");
  await waitFor(() => cdp.eval("!!document.querySelector('#pb-frame').contentDocument?.querySelector('#confirm')"), "payment preview flow");
  assert.equal(await cdp.eval("document.querySelector('#pb-frame').contentDocument.querySelector('#grace-allowance').textContent.includes('75 MB')"), true, "preview uses the saved data allowance");
  assert.equal(await cdp.eval("document.querySelector('#pb-frame').contentDocument.querySelector('#grace-allowance').textContent.includes('· 7 ')"), true, "preview uses the saved time allowance in either language");

  await cdp.eval("loadView('portal', document.querySelector('#main'), ME)");
  await waitFor(() => cdp.eval("!!document.querySelector('#plan-save')"), "portal editor after preview flow");

  failNextPut = true;
  await cdp.pointerClick("#plan-save");
  await waitFor(() => cdp.eval("document.querySelector('#plan-msg')?.textContent.includes('acceptance save failure')"), "visible save failure");
  assert.equal(await cdp.eval("document.querySelector('#plan-save').disabled"), false, "failed save re-enables its button");

  await portalDeploymentAcceptance(cdp);

  await cdp.send("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 2, mobile: true });
  await cdp.eval("loadView('topology', document.querySelector('#main'), ME)");
  await waitFor(() => cdp.eval("!!document.querySelector('.topo-console')"), "mobile topology board");
  assert.equal(await cdp.eval("document.documentElement.scrollWidth <= window.innerWidth"), true, "topology keeps wide graph scrolling inside its canvas on phones");
  assert.equal(await cdp.eval("getComputedStyle(document.querySelector('.topo-inspector')).display === 'block'"), true, "topology inspector stacks below the graph on phones");
  await cdp.eval("loadView('routers', document.querySelector('#main'), ME)");
  await waitFor(() => cdp.eval("!!document.querySelector('.capacity-list')"), "mobile capacity view");
  assert.equal(await cdp.eval("document.documentElement.scrollWidth <= window.innerWidth"), true, "routers page has no viewport-level horizontal overflow");
  assert.equal(await cdp.eval("getComputedStyle(document.querySelector('.mobile-device-list')).display !== 'none'"), true, "mobile device cards are shown");
  assert.equal(await cdp.eval("getComputedStyle(document.querySelector('.desktop-devices')).display === 'none'"), true, "desktop device table is hidden on phones");
  assert.equal(await cdp.eval("[...document.querySelectorAll('.capacity-row')].length > 0 && [...document.querySelectorAll('.capacity-row')].every(row => row.querySelectorAll('.capacity-metric').length === 3 && [...row.querySelectorAll('.capacity-metric')].every(metric => getComputedStyle(metric).display !== 'none'))"), true, "all capacity values remain visible");
  assert.equal(await cdp.eval("document.querySelector('.capacity-row .status.warn') !== null"), true, "exhausted router capacity is labelled at limit");
  assert.equal(await cdp.eval("[...document.querySelectorAll('.sites-actions .btn')].every(b => { const r=b.getBoundingClientRect(); return r.left >= 0 && r.right <= innerWidth; })"), true, "mobile actions stay inside the viewport");
  await cdp.pointerClick("#magic-dude-launcher");
  assert.equal(await cdp.eval(`(() => { const r=document.querySelector('#magic-dude-popup').getBoundingClientRect(); return r.left >= 0 && r.right <= innerWidth && r.top >= 0 && r.bottom <= innerHeight; })()`), true, "mobile chat popup stays inside the viewport");
  assert.equal(await cdp.eval(`(() => { const r=document.querySelector('#magic-dude-popup').getBoundingClientRect(); return r.left >= 10 && innerWidth-r.right >= 10 && innerHeight-r.bottom >= 60; })()`), true, "mobile chat remains an inset floating card rather than an edge drawer");
  assert.equal(await cdp.eval("getComputedStyle(document.querySelector('#magic-dude-popup')).borderBottomLeftRadius !== '0px'"), true, "floating chat keeps all four rounded corners");
  assert.equal(await cdp.eval("getComputedStyle(document.querySelector('#magic-dude-popup')).position"), "fixed", "chat remains available above every mobile view");
  assert.equal(await cdp.eval("parseFloat(getComputedStyle(document.querySelector('#md-question')).fontSize) >= 16"), true, "chat input prevents iOS focus zoom");
  await cdp.pointerClick("#md-close");

  console.log(`✓ UI acceptance passed: Fleet and Topology systems boards, Revenue approval/readback, popup Magic Dude chat, portal writes/readback/failure/preview, and 390px capacity/device layout.`);
  }
  if (process.env.E2E_HOLD_MS) await delay(Number(process.env.E2E_HOLD_MS) || 60000);
} finally {
  try { ws?.close(); } catch (_) {}
  if (browser && browser.exitCode === null) {
    const stopped = new Promise(resolveStop => browser.once("exit", resolveStop));
    try { browser.kill("SIGTERM"); } catch (_) {}
    await Promise.race([stopped, delay(3000)]);
  }
  await new Promise(r => server.close(r));
  if (profile) await rm(profile, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
}
