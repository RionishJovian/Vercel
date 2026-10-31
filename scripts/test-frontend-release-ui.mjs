import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync,mkdirSync,writeFileSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {assertProductionFrontend,requiredFrontendFiles} from './frontend-release-contract.mjs';
function fixture() {
 const root=mkdtempSync(path.join(tmpdir(),'mm-web-release-'));
 for (const file of requiredFrontendFiles) { const p=path.join(root,file);mkdirSync(path.dirname(p),{recursive:true});writeFileSync(p,'fixture'); }
 return root;
}
test('release build rejects a missing shared asset instead of shipping a broken page',()=>{
 const root=fixture();try {rmSync(path.join(root,'assets/js/site.js'));assert.throws(()=>assertProductionFrontend(root),/Missing production web file.*site\.js/);}finally{rmSync(root,{recursive:true,force:true});}
});
test('release build rejects broken local asset references, including versioned URLs',()=>{
 const root=fixture();try {writeFileSync(path.join(root,'signin.html'),'<script src="/assets/js/no-such-module.js?v=7"></script>');assert.throws(()=>assertProductionFrontend(root),/Missing referenced frontend asset in signin\.html/);}finally{rmSync(root,{recursive:true,force:true});}
});
test('complete set of static pages passes the contract',()=>{
 const root=fixture();try {assert.doesNotThrow(()=>assertProductionFrontend(root));}finally{rmSync(root,{recursive:true,force:true});}
});
