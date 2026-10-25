// DOM-only owner editor regression. No computer, library or live service changed.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {createRequire} from 'node:module';
const {JSDOM}=createRequire(import.meta.url)('jsdom');
const dom=new JSDOM('<main></main>',{url:'https://fixture.test/app.html',runScripts:'outside-only'}),w=dom.window;
const source=fs.readFileSync('web/assets/js/media-appearance.js','utf8').replace(/^export /gm,'');
w.eval(source+'\nglobalThis.mountAppearance=mountAppearance;');
const library='1'.repeat(32);let published=null,writes=0,libraryRequests=0;
const api={
 get:async path=>path==='/media/capabilities'?{connector:{available:true}}:path==='/media/publication'?{publication:published,paid_access_ready:true}:{state:'succeeded',result:{libraries:[{Name:'<img src=x> Fixture videos',ItemId:library,CollectionType:'homevideos'},{Name:'Private books',ItemId:'2'.repeat(32),CollectionType:'books'}]}},
 post:async(path,body)=>{libraryRequests++;assert.equal(path,'/media/jobs');assert.equal(body.operation,'libraries');return {};},
 put:async(path,design)=>{writes++;published={design,revision:writes,public_id:'a'.repeat(64)};return {publication:published,paid_access_ready:true};},
};
w.mountAppearance(w.document.querySelector('main'),{company_name:'Fixture cinema',business_id:'fixture'},{email:'fixture@test'},api);
const settle=()=>new Promise(resolve=>setTimeout(resolve,20));await settle();
assert.equal(writes,0);
w.document.querySelector('[data-media-libraries]').click();await settle();
const options=w.document.querySelector('[data-media-library-options]');assert.equal(options.querySelectorAll('input').length,1,'Books are not offered as playable videos');assert.equal(options.querySelector('img'),null,'Library names are not executable HTML');
const checkbox=options.querySelector('input');checkbox.checked=true;checkbox.dispatchEvent(new w.Event('change',{bubbles:true}));
w.document.querySelector('form').dispatchEvent(new w.Event('submit',{bubbles:true,cancelable:true}));await settle();
assert.deepEqual(Array.from(published.design.libraries),[library]);assert.equal(writes,1);
w.document.querySelector('[data-media-reload]').click();await settle();assert.equal(checkbox.checked,true,'Selected libraries survive reload');
assert.ok(w.document.querySelector('#media-design-status').textContent.includes('Paid playback checks'));
w.mountAppearance(w.document.querySelector('main'),{company_name:'Fixture cinema'},{}, {...api,get:async path=>path==='/media/capabilities'?{connector:{available:false}}:api.get(path)});await settle();const held=w.document.querySelector('[data-media-libraries]');assert.equal(held.disabled,true);held.click();await settle();assert.equal(libraryRequests,1,'Disabled connector must not enqueue jobs');
dom.window.close();console.log('Owner library DOM checks passed: explicit selection, safe names, publish and reload.');
