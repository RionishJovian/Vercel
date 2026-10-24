import {cpSync,readdirSync,rmSync,mkdirSync,existsSync} from 'node:fs';
import path from 'node:path';
const root=process.cwd(),source=path.join(root,'web'),out=path.join(root,'vercel-public');
for(const name of ['index.html','app.html','signin.html','portal.html','assets/js/api.js'])if(!existsSync(path.join(source,name)))throw Error('Missing production web file: '+name);
rmSync(out,{recursive:true,force:true});mkdirSync(out,{recursive:true});
cpSync(source,out,{recursive:true,filter:filename=>!path.relative(source,filename).split(path.sep).some(p=>p.startsWith('.')||/\.(bak|backup)(\.|$)/i.test(p))});
console.log('Built the production web frontend, not the separate Sites prototype.');
