import assert from 'node:assert/strict';
import fs from 'node:fs';
import {createRequire} from 'node:module';
const {JSDOM}=createRequire(import.meta.url)('jsdom');
const css=fs.readFileSync('web/assets/css/layout.css','utf8');
const html=fs.readFileSync('web/app.html','utf8');
const deploy=fs.readFileSync('deploy.sh','utf8');
const dom=new JSDOM(`<style>${css}</style><body class="page-app">
<main><div class="page-head"><h1 class="page-title">Workspace</h1></div>
<div id="content"><section class="panel network-workspace">
<header><h2>Network</h2></header><label>Gateway<select class="input"><option>Choose device</option></select></label>
<input type="text"><input type="checkbox"><input type="radio"><button hidden>Apply</button>
</section><section class="panel"><div class="panel-head"><h2 class="panel-title">Business</h2></div>
<div class="panel-body"><select><option>Choose</option></select><textarea></textarea></div></section></div></main></body>`);
const {document,getComputedStyle}=dom.window;
for(const control of document.querySelectorAll('select,textarea,input[type="text"]')){
 assert.equal(getComputedStyle(control).minHeight,'44px','All text controls share a touch-sized shell');
}
for(const select of document.querySelectorAll('select')){
 assert.equal(getComputedStyle(select).appearance,'none','Safari must not render a thin native select');
 assert.equal(getComputedStyle(select).paddingRight,'36px','Leave room for the dropdown arrow');
}
for(const type of ['checkbox','radio']){
 assert.equal(parseFloat(getComputedStyle(document.querySelector(`input[type="${type}"]`)).minHeight),0,'Do not turn checkboxes into text fields');
}
assert.equal(getComputedStyle(document.querySelector('[hidden]')).display,'none','Shared rules cannot reveal an apply control');
assert.doesNotMatch(html,/max-width:1100px; margin-inline:auto/,'Remove the old centered network layer');
assert.equal((css.match(/\.panel\.network-workspace \{/g)||[]).length,2,'One network contract plus its mobile rule');
assert.match(deploy,/grep -rhoE '\/assets\/\(css\|js\)\//,'Discover every referenced shared asset for deployment versioning');
assert.ok(html.includes('/assets/css/layout.css?v='));
console.log('Workspace consistency passed: shared controls, native-select normalization, safe checkbox sizing, hidden actions, single network layer and cache versioning.');
