import fs from 'node:fs';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const {JSDOM}=createRequire(import.meta.url)('jsdom');
const html=fs.readFileSync('web/app.html','utf8');
const document=new JSDOM(html).window.document;
const advanced=document.querySelector('#rail-advanced');
assert.ok(advanced && !advanced.open,'Advanced tools start collapsed');
for(const route of ['templates','termlog','aiusage','i18n','platform']){
 const buttons=document.querySelectorAll(`.nav-item[data-view="${route}"]`);
 assert.equal(buttons.length,1,'No duplicate navigation entry');
 assert.equal(buttons[0].closest('details'),advanced,'Technical tools belong under Advanced');
}
assert.equal(document.querySelector('.nav-item[data-view="templates"]').textContent,'RouterOS reference');
assert.match(html,/if \(advanced\) advanced\.open = true/,'Direct routes reveal the active advanced item');
assert.match(html,/\["platform", "i18n"\].*DEVELOPER/,'Developer route authorization remains');
console.log('Navigation cleanup passed: collapsed tools, no duplicates, honest reference label, active-route reveal and developer restriction.');
