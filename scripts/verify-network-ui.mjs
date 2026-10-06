import {build} from 'esbuild';
import {mkdir,writeFile,rm} from 'node:fs/promises';
import assert from 'node:assert/strict';
const dir='.ui-check';await mkdir(dir,{recursive:true});
try{
 const bundle=await build({entryPoints:['src/NetworkRegistry.jsx'],bundle:true,platform:'node',format:'esm',external:['react'],write:false});await writeFile(dir+'/network.mjs',bundle.outputFiles[0].text);
 const {SiteRegistry,RouterRegistry}=await import('../'+dir+'/network.mjs');const React=await import('react');const {renderToStaticMarkup}=await import('react-dom/server');
 const sites=[{id:'test-site',name:'Sample cafe',timezone:'Asia/Yangon'}];const routers=[{id:'test-router',site_id:'test-site',name:'Gateway',model:'hAP ax3',reported_routeros:'Test version'}];
 const owner=renderToStaticMarkup(React.createElement(RouterRegistry,{sites,routers,act:()=>{},busy:true}));assert.match(owner,/Unverified/);assert.match(owner,/Not assigned/);assert.match(owner,/owner reported/);assert.match(owner,/Register router/);assert.doesNotMatch(owner,/Online/);
 const platform=renderToStaticMarkup(React.createElement(RouterRegistry,{sites,routers,platform:true}));assert.doesNotMatch(platform,/Register router/);assert.match(platform,/Platform router fleet/);
 const site=renderToStaticMarkup(React.createElement(SiteRegistry,{sites,busy:true}));assert.match(site,/Reporting timezone/);assert.match(site,/Create site/);
 console.log('Network UI checks passed: owner registration, platform read-only fleet, site fields and unverified connection labels.');
}finally{await rm(dir,{recursive:true,force:true})}
