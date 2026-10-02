import fs from 'node:fs';
import vm from 'node:vm';
import assert from 'node:assert/strict';

const source = fs.readFileSync(new URL('../web/app.html', import.meta.url), 'utf8');
const section = (start, end) => source.slice(source.indexOf(start), source.indexOf(end, source.indexOf(start)));
const context = vm.createContext({Date, Map, Number, String, encodeURIComponent,
  ME: {user: {user_role: 'OWNER'}}, SITE_EDITOR_ROLES: ['OWNER','ADMIN','DEVELOPER'],
  MMTheme: {icon:()=>''}, topologyLayerLabel:x=>x,
  asArray: x => Array.isArray(x) ? x : [], esc: x => String(x ?? ''), badge: x => x});
vm.runInContext(section('function connectionIsFresh(', 'function connPanel('), context);
const now = Date.now();
assert.equal(context.connectionIsFresh({cloud_status:'online'}, now), false);
assert.equal(context.connectionIsFresh({cloud_status:'online',cloud_last_seen_at:new Date(now).toISOString()}, now), true);
assert.equal(context.connectionIsFresh({cloud_status:'online',cloud_last_seen_at:new Date(now-600000).toISOString()}, now), false);
vm.runInContext(section('function routerCapabilityButton(', 'async function renderSites('), context);
const device = {device_id:'test', hardware_name:'QA', deployment_layer:'DOWNSTREAM_SWITCH'};
let html = context.routerBoardPanel([device], {});
assert.ok(!html.includes('Configure gateway'), 'switches must not show gateway configuration');
assert.ok(!html.includes('Hotspot readiness'));
context.ME.user.user_role = 'VIEWER';
html = context.routerBoardPanel([{...device,deployment_layer:'MAIN_GATEWAY'}], {});
assert.ok(!html.includes('data-edit=')); assert.ok(!html.includes('data-reboot='));
assert.ok(html.includes('Gateway settings'));

const element = () => ({hidden:true, disabled:false, isConnected:true, listeners:{},
  addEventListener(event, fn) {this.listeners[event]=fn;}});
const status=element(), apply=element(), preview=element(), form=element();
form.querySelector = selector => ({'#gateway-status':status,'#gateway-apply':apply,'#gateway-preview':preview})[selector];
form.elements = {radius_secret:{value:''}};
let resolve, posts=0;
context.FormData = class {get(name) {return name==='vlan_id'?'10':name==='dns_servers'?'1.1.1.1':'fixture';} has() {return false;}};
context.MM = {post() {posts++; return new Promise(r=>resolve=r);}};
context.confirm = () => true;
vm.runInContext(section('function wireGatewaySetup(', '// Inline edit form for one router'), context);
context.wireGatewaySetup({querySelector:()=>form}, device);
const pending = form.listeners.submit({preventDefault(){}});
form.listeners.input();
resolve({plan:{confirmation:'old',steps:[]}}); await pending;
assert.equal(apply.hidden,true, 'an input change during preview must invalidate its response');
await apply.listeners.click(); assert.equal(posts,1,'stale preview must not authorize apply');
const current = form.listeners.submit({preventDefault(){}});
resolve({plan:{confirmation:'current',steps:[]}}); await current;
assert.equal(apply.hidden,false,'unchanged preview should enable review/apply');
form.listeners.input(); assert.equal(apply.hidden,true);
assert.ok(source.includes('document.getElementById("site-add")?.addEventListener'), 'downstream/viewer rendering must tolerate absent add control');
assert.ok(!section('function requestPanel(', 'function wireAllowance(').includes('id="site-add"'), 'capacity panel must not duplicate device-add control');
assert.ok(!section('function siteLocationPicker(', 'async function wireSiteLocationPicker(').includes('site-location-map'), 'site edit must not mount a second map');
console.log('Network workspace regressions passed: roles, capabilities, freshness, preview race, single-map form.');
