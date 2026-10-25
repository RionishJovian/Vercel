import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync,mkdirSync,writeFileSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {assertProductionFrontend,requiredFrontendFiles} from './frontend-release-contract.mjs';
function fixture() {
 const root=mkdtempSync(path.join(tmpdir(),'mm-web-release-'));
 for (const file of requiredFrontendFiles) { const p=path.join(root,file);mkdirSync(path.dirname(p),{recursive:true});writeFileSync(p,'fixture'); }
 writeFileSync(path.join(root,'app.html'),'<script src="/assets/js/account-presence.js?v=1"></script><script src="/assets/js/workspaces.js?v=1"></script><link href="/assets/css/jade-glass.css?v=1" rel="stylesheet">');
 return root;
}
test('release build rejects a missing runtime dependency instead of shipping broken UI',()=>{
 const root=fixture();try {rmSync(path.join(root,'assets/js/account-presence.js'));assert.throws(()=>assertProductionFrontend(root),/Missing production web file.*account-presence/);}finally{rmSync(root,{recursive:true,force:true});}
});
test('release build rejects broken local asset references, including versioned URLs',()=>{
 const root=fixture();try {writeFileSync(path.join(root,'app.html'),'<script src="/assets/js/no-such-module.js?v=7"></script>');assert.throws(()=>assertProductionFrontend(root),/Missing referenced frontend asset/);}finally{rmSync(root,{recursive:true,force:true});}
});
test('an older app entry cannot silently omit presence or workspace integration',()=>{
 const root=fixture();try {writeFileSync(path.join(root,'app.html'),'<main>Old app</main>');assert.throws(()=>assertProductionFrontend(root),/Production app does not load/);}finally{rmSync(root,{recursive:true,force:true});}
});
test('complete versioned entry passes the contract',()=>{
 const root=fixture();try {assert.doesNotThrow(()=>assertProductionFrontend(root));}finally{rmSync(root,{recursive:true,force:true});}
});
