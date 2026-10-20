import fs from 'node:fs';
import vm from 'node:vm';
import assert from 'node:assert/strict';
import {JSDOM} from 'jsdom';

const source = fs.readFileSync(new URL('../web/app.html', import.meta.url), 'utf8');
const routes = fs.readFileSync(new URL('../internal/server/server.go', import.meta.url), 'utf8');
const dom = new JSDOM('<main><div id="content"></div></main>');
const incident = {id:'incident/a', severity:'warning', kind:'offline', suggested_action:{kind:'reboot'}};
const posts = [];
const context = vm.createContext({
  document:dom.window.document, ME:{user:{user_role:'OWNER'}},
  asArray:x=>Array.isArray(x)?x:[], esc:x=>String(x??''), ts:x=>x??'',
  MM:{get:async()=>({incidents:[incident]}),post:async(path)=>{posts.push(path);}},
  confirm:()=>false,
});
vm.runInContext(source.slice(source.indexOf('const FIX_KINDS ='), source.indexOf('async function renderPulse(')),context);
vm.runInContext(source.slice(source.indexOf('async function renderIncidents('), source.indexOf('/* ---------- Translations')),context);
const main=dom.window.document.querySelector('main');
// Current role scheme after OPERATOR/TECHNICIAN/SUPPORT were pruned: one tenant
// account == one business, roles VIEWER (the app user) < ADMIN (read/write incl.
// payments) < OWNER, plus the platform DEVELOPER. The server gates every incident
// mutation on RequireMinRole(RoleAdmin), so acknowledge, resolve and apply-fix all
// share the single ADMIN+ boundary — VIEWER sees none of them.
for(const role of ['VIEWER','ADMIN','OWNER','DEVELOPER']) {
  context.ME.user.user_role=role;
  await context.renderIncidents(main);
  const manage=['ADMIN','OWNER','DEVELOPER'].includes(role);
  assert.equal(!!main.querySelector('[data-act="acknowledge"]'),manage,role);
  assert.equal(!!main.querySelector('[data-act="resolve"]'),manage,role);
  assert.equal(!!main.querySelector('[data-fix]'),manage,role);
}
for(const action of ['acknowledge','resolve']) {
  await context.renderIncidents(main);
  main.querySelector(`[data-act="${action}"]`).click();
  await new Promise(resolve=>setImmediate(resolve));
  assert.equal(posts.at(-1),`/incidents/incident%2Fa/${action}`);
  assert(routes.includes(`incidents.POST("/:id/${action}"`),'UI action missing from production routes');
}
console.log('Incident workflow passed: real DOM clicks reach registered actions; role visibility matches server policy.');
