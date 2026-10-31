import {existsSync,readFileSync} from 'node:fs';
import path from 'node:path';
// The console is the React app in app/, built into the web image (and the Vercel build) at /v2/.
// These are the static pages and shared assets that ship from web/.
export const requiredFrontendFiles = ['index.html','signin.html','pricing.html','portal.html',
 'assets/js/api.js','assets/js/site.js','assets/js/theme.js',
 'assets/css/landing.css','assets/css/site-pages.css'];
export function assertProductionFrontend(source) {
 for (const file of requiredFrontendFiles) if (!existsSync(path.join(source,file))) throw Error('Missing production web file: '+file);
 for (const page of ['index.html','signin.html','pricing.html','portal.html']) {
  const html=readFileSync(path.join(source,page),'utf8');
  for (const match of html.matchAll(/(?:src|href)=["'](\/assets\/[^"'?#]+)(?:[?#][^"']*)?["']/g)) {
   if (!existsSync(path.join(source,match[1].slice(1)))) throw Error('Missing referenced frontend asset in '+page+': '+match[1]);
  }
 }
}
