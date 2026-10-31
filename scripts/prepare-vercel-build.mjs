import {cpSync,readdirSync,rmSync,mkdirSync,existsSync} from 'node:fs';
import {execFileSync} from 'node:child_process';
import path from 'node:path';
import {assertProductionFrontend} from './frontend-release-contract.mjs';
const root=process.cwd(),source=path.join(root,'web'),out=path.join(root,'vercel-public');
assertProductionFrontend(source);
rmSync(out,{recursive:true,force:true});mkdirSync(out,{recursive:true});
cpSync(source,out,{recursive:true,filter:filename=>!path.relative(source,filename).split(path.sep).some(p=>p.startsWith('.')||/\.(bak|backup)(\.|$)/i.test(p))});
// The console is the React app in app/, served at /v2/ exactly as in the web image.
const app=path.join(root,'app');
execFileSync('npm',['ci','--ignore-scripts'],{cwd:app,stdio:'inherit'});
execFileSync('npm',['run','build'],{cwd:app,stdio:'inherit'});
cpSync(path.join(app,'dist'),path.join(out,'v2'),{recursive:true});
console.log('Built the production web frontend and the /v2/ app, not the separate Sites prototype.');
