import {spawn} from 'node:child_process';
import {existsSync} from 'node:fs';
const python=existsSync('.venv/bin/python')?'.venv/bin/python':'python3';
const env={...process.env,...(python==='python3'&&existsSync('.runtime/python')?{PYTHONPATH:'.runtime/python'}:{}),DATABASE_URL:process.env.DATABASE_URL||'sqlite:///accounts.local.sqlite3'};
// Local development schema only; production migrations remain an explicit operator task.
function start(code){
 if(code){process.exit(code)}
 const demo=spawn('python3',['server.py'],{stdio:'inherit'});
 const account=spawn(python,['-m','uvicorn','backend.app:create_app','--factory','--host','127.0.0.1','--port','8788'],{stdio:'inherit',env});
 const vite=spawn(process.execPath,['node_modules/vite/bin/vite.js',...process.argv.slice(2)],{stdio:'inherit'});
 const children=[demo,account,vite];let stopping=false;
 const stop=code=>{if(stopping)return;stopping=true;children.forEach(p=>p.kill('SIGTERM'));process.exitCode=code||0};
 process.on('SIGTERM',()=>stop(0));process.on('SIGINT',()=>stop(0));
 children.forEach(p=>{p.on('error',e=>{console.error(e.message);stop(1)});p.on('exit',code=>stop(code))});
}
const migrate=process.env.DATABASE_URL?null:spawn(python,['-m','alembic','upgrade','head'],{stdio:'inherit',env});
if(migrate)migrate.on('exit',start);else start(0);
migrate?.on('error',e=>{console.error(e.message);process.exitCode=1});
