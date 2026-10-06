import {build} from 'esbuild';
import {mkdir,writeFile,readFile,rm} from 'node:fs/promises';
import assert from 'node:assert/strict';
await mkdir('.ui-check',{recursive:true});
try{
 const bundle=await build({entryPoints:['src/Knowledge.jsx'],bundle:true,platform:'node',format:'esm',external:['react'],write:false});await writeFile('.ui-check/knowledge.mjs',bundle.outputFiles[0].text);
 const {default:Knowledge}=await import('../.ui-check/knowledge.mjs');const React=await import('react');const {renderToStaticMarkup}=await import('react-dom/server');
 const cards=JSON.parse(await readFile('backend/knowledge-seed.json','utf8')).map(c=>({...c,published:true,archived:false,revision:1}));
 const sme=renderToStaticMarkup(React.createElement(Knowledge,{cards,go:()=>{},explain:()=>{}}));assert.match(sme,/Explain with Magic Dude/);assert.match(sme,/Router compatibility: not checked/);assert.doesNotMatch(sme,/Save draft|Retire|Create guidance card/);
 const platform=renderToStaticMarkup(React.createElement(Knowledge,{cards,platform:true,busy:true,act:()=>{}}));assert.match(platform,/Save draft/);assert.match(platform,/Retire/);assert.match(platform,/disabled=""/);
 const unsafe=renderToStaticMarkup(React.createElement(Knowledge,{cards:[{...cards[0],title:'<script>bad</script>'}],go:()=>{},explain:()=>{}}));assert.match(unsafe,/&lt;script&gt;/);assert.doesNotMatch(unsafe,/<script>/);
 const css=await readFile('src/styles.css','utf8');await writeFile('../Did-you-know-preview.html',`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>MikroMagic · Did you know?</title><style>${css}\nbody{padding:24px}.preview{max-width:1100px;margin:auto}.preview fieldset{border:0;padding:0}.preview h1{font-size:42px}</style><body><main class="preview"><h1>Did you know?</h1><p>Static preview from actual React components. Sample guidance; controls are disabled here. Use the authenticated workspace for publishing and navigation.</p><fieldset disabled>${sme}</fieldset><h1>Platform guidance publishing</h1><fieldset disabled>${platform}</fieldset></main></body></html>`);
 console.log('Guidance UI: role controls, unchecked compatibility, source links and escaped content passed.');
}finally{await rm('.ui-check/knowledge.mjs',{force:true})}
