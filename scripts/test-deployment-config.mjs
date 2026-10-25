import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,mkdtempSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import path from 'node:path';
import {execFileSync} from 'node:child_process';
const script=readFileSync(new URL('../deploy/deploy-vps.sh',import.meta.url),'utf8');
const program=script.split('<<\'PY\'\nimport pathlib,sys,re,os\n')[1]?.split('\nPY')[0];
assert.ok(program,'Deployment must expose the exact image replacement program');
const python='import pathlib,sys,re,os\n'+program;
const api='ghcr.io/example/api@sha256:'+'a'.repeat(64),web='ghcr.io/example/web@sha256:'+'b'.repeat(64);
const fixture='name: mega-magic-go-restored\nservices:\n  db:\n    image: postgres:16\n    volumes: [db_data:/data]\n  api:\n    build:\n      context: ..\n    image: old-api:release\n    environment:\n      MM_MANAGEMENT_MIGRATION_ENABLED: "true"\n    volumes: [config:/app/config.yaml:ro]\n  web:\n    image: old-web:release\n    ports: [127.0.0.1:18081:80]\nvolumes:\n  db_data:\n    external: true\n';
test('Exact deployment transform changes only API/web image references',()=>{
 const dir=mkdtempSync(path.join(tmpdir(),'mm-compose-'));try{const p=path.join(dir,'compose.yaml');writeFileSync(p,fixture);execFileSync('python3',['-c',python,p,api,web]);assert.equal(readFileSync(p,'utf8'),fixture.replace('old-api:release',api).replace('old-web:release',web));}finally{rmSync(dir,{recursive:true,force:true});}
});
test('Missing service image aborts before changing original compose',()=>{
 const dir=mkdtempSync(path.join(tmpdir(),'mm-compose-'));try{const p=path.join(dir,'compose.yaml'),s=fixture.replace('    image: old-web:release\n','');writeFileSync(p,s);assert.throws(()=>execFileSync('python3',['-c',python,p,api,web],{stdio:'pipe'}));assert.equal(readFileSync(p,'utf8'),s);}finally{rmSync(dir,{recursive:true,force:true});}
});
