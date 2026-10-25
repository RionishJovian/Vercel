import {existsSync,readFileSync} from 'node:fs';
import path from 'node:path';
// These are existing production capabilities, not the separate Sites prototype.
export const requiredFrontendFiles = ['index.html','app.html','signin.html','portal.html',
 'assets/js/api.js','assets/js/account-presence.js','assets/js/theme.js','assets/js/workflow-revamp.js',
 'assets/js/workspaces.js','assets/js/hub-management.js','assets/js/management-migration.js',
 'assets/css/jade-glass.css','assets/css/workspaces.css'];
export function assertProductionFrontend(source) {
 for (const file of requiredFrontendFiles) if (!existsSync(path.join(source,file))) throw Error('Missing production web file: '+file);
 const html=readFileSync(path.join(source,'app.html'),'utf8');
 for (const match of html.matchAll(/(?:src|href)=["'](\/assets\/[^"'?#]+)(?:[?#][^"']*)?["']/g)) {
  if (!existsSync(path.join(source,match[1].slice(1)))) throw Error('Missing referenced frontend asset: '+match[1]);
 }
 for (const asset of ['assets/js/account-presence.js','assets/js/workspaces.js','assets/css/jade-glass.css']) {
  if (!html.includes('/'+asset)) throw Error('Production app does not load '+asset);
 }
}
