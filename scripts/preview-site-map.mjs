// Local UI verification only: fake tenant data, no production credentials/writes.
import {createServer} from "node:http";
import {readFile} from "node:fs/promises";
import path from "node:path";
const root = path.resolve(import.meta.dirname, "../web");
const mediaJobs = new Map();
const fixtures = {
  "/api/v1/me": {user:{email:"qa@example.test",full_name:"Map QA",user_role:process.env.MM_PREVIEW_ROLE || "DEVELOPER"},business:{business_id:"mock-business",company_name:"Map acceptance"}},
  "/api/v1/sites": {sites:[{site_id:"one",name:"Yangon lab",latitude:16.8409,longitude:96.1735,device_count:1},{site_id:"two",name:"No coordinates",latitude:null,longitude:null}]},
  "/api/v1/router-connections": {connections:[{site_id:"one",cloud_status:"online"}]},
  "/api/v1/device-allowances": {allowance:null},
  "/api/v1/routers": {routers:[{device_id:"gateway-test",hardware_name:"Gateway QA",deployment_layer:"MAIN_GATEWAY",vendor:"mikrotik"},{device_id:"switch-test",hardware_name:"Switch QA",deployment_layer:"DOWNSTREAM_SWITCH",vendor:"mikrotik"}]},
};
createServer(async (req,res) => {
  const url = new URL(req.url,"http://localhost");
  // Media UI fixtures only: no connection to Jellyfin and no live mutations.
  if (url.pathname === '/api/v1/media/connection') {
    res.writeHead(200,{'content-type':'application/json'});res.end(JSON.stringify({server_name:'Mock media PC',last_seen_at:new Date().toISOString(),expires_at:new Date(Date.now()+3600000).toISOString()}));return;
  }
  if (url.pathname === '/api/v1/media/pairing') {res.writeHead(201,{'content-type':'application/json'});res.end(JSON.stringify({pairing_code:'mock-code-not-a-real-credential',expires_in_seconds:600}));return;}
  if (url.pathname === '/api/v1/media/jobs' && req.method === 'POST') {
    let raw='';for await(const part of req) raw+=part;
    const job=JSON.parse(raw);
    const dashboard={collected_at:new Date().toISOString(),server:{ServerName:'Mock media PC',Version:'Test fixture'},operating_system:'Mock OS',counts:{MovieCount:0,SeriesCount:5,EpisodeCount:40,SongCount:12},active_sessions:2,active_users:1,playback:[{title:'Fixture film',user:'Test guest',device:'Test phone',method:'DirectPlay',paused:false}],host:{os:'Mock computer',cpu_percent:12,memory_percent:24,memory_total_bytes:17179869184,volumes:[{label:'Fixture drive',total_bytes:107374182400,free_bytes:53687091200}]}};
    mediaJobs.set(job.id,{...job,state:'succeeded',result:job.operation==='dashboard'?{dashboard}:job.operation==='libraries'?{libraries:[{Name:'Personal videos',CollectionType:'homevideos',Locations:['/Mock/Media']}]}:{accepted:true}});
    res.writeHead(202,{'content-type':'application/json'});res.end(JSON.stringify({id:job.id,state:'queued'}));return;
  }
  if (url.pathname.startsWith('/api/v1/media/jobs/')) {res.writeHead(200,{'content-type':'application/json'});res.end(JSON.stringify(mediaJobs.get(url.pathname.split('/').at(-1))||{}));return;}
  if (url.pathname.startsWith("/api/")) {res.writeHead(200,{"content-type":"application/json"}); res.end(JSON.stringify(fixtures[url.pathname] || {})); return;}
  const file = path.resolve(root,"." + (url.pathname === "/" ? "/app.html" : url.pathname));
  if (!file.startsWith(root + "/")) {res.writeHead(403); res.end(); return;}
  try {const data = await readFile(file); res.writeHead(200,{"content-type":({".html":"text/html",".mjs":"text/javascript",".js":"text/javascript",".css":"text/css",".png":"image/png",".svg":"image/svg+xml"})[path.extname(file)] || "application/octet-stream"}); res.end(data);}
  catch {res.writeHead(404); res.end();}
}).listen(8768,"127.0.0.1",() => console.log("Map preview: http://127.0.0.1:8768/app.html#sites"));
