export function staffPaths(profile){
 if(profile==='cashier')return ['tenants','packages','orders','audit','appearance/effective','revenue','vouchers','sites'];
 if(profile==='technician')return ['tenants','audit','appearance/effective','knowledge','sites','routers','hubs','router-plans'];
 return ['tenants','packages','orders','entitlements','audit','appearance/effective','media/titles','knowledge','revenue','vouchers','sites','routers','hubs','router-plans'];
}
export function staffPages(profile){
 if(profile==='cashier')return ['Payments','Vouchers','Revenue','Audit','Account'];
 if(profile==='technician')return ['Sites','Network','Routers','Internet','Did you know?','Audit','Account'];
 return ['Overview','Revenue','Sites','Network','Routers','Internet','Captive portal','Packages','Vouchers','Payments','Media','Did you know?','Audit','Account'];
}
