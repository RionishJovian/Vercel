import fs from 'node:fs';
import vm from 'node:vm';
import assert from 'node:assert/strict';
import {JSDOM} from 'jsdom';
const source=fs.readFileSync(new URL('../web/app.html',import.meta.url),'utf8');
const dom=new JSDOM('<main><div id="content"></div></main>');
const users=[{user_id:'system',user_role:'DEVELOPER',is_active:true},{user_id:'staff',user_role:'ADMIN',is_active:true},{user_id:'legacy',user_role:'OPERATOR',is_active:true}];
const c=vm.createContext({document:dom.window.document,MM:{get:async()=>({team:users})},esc:x=>String(x??'')});
vm.runInContext(source.slice(source.indexOf('const TEAM_ROLES ='),source.indexOf('/* ---------- Backups')),c);
const main=dom.window.document.querySelector('main');
await c.renderTeam(main);
// System/platform (DEVELOPER) accounts are read-only in tenant team management.
assert(!main.querySelector('[data-role="system"]'));
assert(!main.querySelector('[data-save="system"]'));
assert(!main.querySelector('[data-disable="system"]'));
assert(main.textContent.includes('DEVELOPER'));
assert(main.textContent.includes('Platform-managed account'));
// A legacy/pruned role (OPERATOR) is no longer grantable, so it is read-only too.
assert(!main.querySelector('[data-role="legacy"]'));
assert(main.textContent.includes('OPERATOR'));
// A current grantable tenant role (ADMIN) stays editable.
assert.equal(main.querySelector('[data-role="staff"]').value,'ADMIN');
// The add-member role picker only offers grantable tenant roles, never DEVELOPER.
const addOpts=[...main.querySelector('#tm-role').options].map(x=>x.value);
assert(!addOpts.includes('DEVELOPER'));
assert.deepEqual(addOpts,['OWNER','ADMIN','VIEWER']);
// The page explains the role-vs-access-tier distinction to the operator.
assert(main.textContent.includes('access tier'));
console.log('Team UI passed: system/legacy roles read-only, grantable tenant staff editable, DEVELOPER not grantable, role-vs-tier notice present.');
