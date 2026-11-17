"""Immutable WireGuard management plans. No command dispatch or trusted telemetry yet."""
import base64,ipaddress,re,time
from fastapi import Depends,HTTPException
from pydantic import Field,field_validator
from typing import Literal
from sqlalchemy import select,update
from sqlalchemy.exc import IntegrityError
from .schemas import Strict
from .models import MagicHub,HubAssignment,RouterPlan,RouterDevice,Tenant,SitePortal,PortalSettings

PRIVATE=[ipaddress.ip_network(x) for x in ['10.0.0.0/8','172.16.0.0/12','192.168.0.0/16']]
def public_key(value):
 try:
  raw=base64.b64decode(value,validate=True)
  if len(raw)!=32 or raw==bytes(32) or base64.b64encode(raw).decode()!=value:raise ValueError()
 except (ValueError,TypeError):raise ValueError('Use a canonical base64 WireGuard public key (32 bytes)')
 return value
class HubIn(Strict):
 name:str=Field(min_length=1,max_length=120)
 endpoint:str=Field(min_length=1,max_length=253)
 port:int=Field(ge=1,le=65535,default=51820)
 pool:str=Field(min_length=1,max_length=18)
 public_key:str=Field(min_length=44,max_length=44)
 @field_validator('public_key')
 @classmethod
 def key(cls,v):return public_key(v)
 @field_validator('pool')
 @classmethod
 def network(cls,v):
  try:n=ipaddress.IPv4Network(v,strict=True)
  except ValueError:raise ValueError('Use an aligned private IPv4 /24 network')
  if n.prefixlen!=24 or not any(n.subnet_of(p) for p in PRIVATE):raise ValueError('Use an RFC1918 private IPv4 /24 network')
  return str(n)
 @field_validator('endpoint')
 @classmethod
 def host(cls,v):
  v=v.lower()
  try:
   a=ipaddress.ip_address(v)
   if not isinstance(a,ipaddress.IPv4Address) or not a.is_global:raise ValueError('Use a public IPv4 address or DNS name')
  except ValueError:
   if not re.fullmatch(r'(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?',v) or v.endswith(('.local','.localhost','.internal','.invalid')):raise ValueError('Use a public IPv4 address or DNS name without URL, path or port')
  return v
class AssignmentIn(Strict):tenant_id:str=Field(min_length=1,max_length=36)
class PlanIn(Strict):
 hub_id:str=Field(min_length=1,max_length=36)
 router_public_key:str=Field(min_length=44,max_length=44)
 @field_validator('router_public_key')
 @classmethod
 def key(cls,v):return public_key(v)
class PlanReview(Strict):confirmed_review:Literal[True]

def hub_json(h):
 n=ipaddress.ip_network(h.pool)
 return {k:getattr(h,k) for k in ['id','name','endpoint','port','pool','public_key','created']}|{'hub_address':str(n.network_address+1),'remaining_addresses':max(0,255-h.next_host),'operational_state':'unverified'}
def plan_json(p):
 return {k:getattr(p,k) for k in ['id','tenant_id','router_id','hub_id','address','snapshot','created','reviewed_at']}|{'state':'awaiting_adapter' if p.reviewed_at else 'awaiting_review','connection_state':'unverified','last_handshake':None}
def conflict_portals(s,router,pool):
 configs=[]
 for row in [s.get(SitePortal,router.site_id),s.get(PortalSettings,router.tenant_id)]:
  if row:configs.extend([row.draft,row.published])
 for cfg in configs:
  if cfg and cfg.get('guest_subnet') and ipaddress.ip_network(cfg['guest_subnet']).overlaps(pool):raise HTTPException(409,'Hub pool overlaps this business portal subnet; resolve addressing before planning')

def register_hubs(app,current,db,owner,log):
 def platform(u):
  if u.role!='platform':raise HTTPException(403,'Platform hub administration required')
 def visible_hub(s,u,id):
  h=s.get(MagicHub,id)
  if not h or (u.role!='platform' and not s.get(HubAssignment,(id,u.tenant_id))):raise HTTPException(404,'Assigned hub not found')
  return h
 @app.get('/v1/hubs')
 def hubs(u=Depends(current),s=Depends(db)):
  if u.role not in ['sme','platform']:raise HTTPException(403,'Hub permission required')
  q=select(MagicHub)
  if u.role=='sme':q=q.join(HubAssignment).where(HubAssignment.tenant_id==u.tenant_id)
  return [hub_json(h) for h in s.scalars(q.order_by(MagicHub.created))]
 @app.post('/v1/hubs',status_code=201)
 def create(b:HubIn,u=Depends(current),s=Depends(db)):
  platform(u);h=MagicHub(**b.model_dump(),created=int(time.time()),next_host=2);s.add(h)
  try:s.flush();log(s,u,'hub_registered',h.id);s.commit()
  except IntegrityError:s.rollback();raise HTTPException(409,'Hub name, pool, endpoint/port or public key already registered')
  return hub_json(h)
 @app.get('/v1/hub-assignments')
 def assignments(u=Depends(current),s=Depends(db)):
  platform(u);return [{'hub_id':x.hub_id,'tenant_id':x.tenant_id,'created':x.created} for x in s.scalars(select(HubAssignment))]
 @app.post('/v1/hubs/{id}/assignments')
 def assign(id:str,b:AssignmentIn,u=Depends(current),s=Depends(db)):
  platform(u);visible_hub(s,u,id)
  if not s.get(Tenant,b.tenant_id):raise HTTPException(404,'Business not found')
  key=(id,b.tenant_id)
  if s.get(HubAssignment,key):return {'hub_id':id,'tenant_id':b.tenant_id,'replayed':True}
  s.add(HubAssignment(hub_id=id,tenant_id=b.tenant_id,created=int(time.time())))
  try:s.flush();log(s,u,'hub_assigned',id+':'+b.tenant_id);s.commit()
  except IntegrityError:s.rollback();return {'hub_id':id,'tenant_id':b.tenant_id,'replayed':True}
  return {'hub_id':id,'tenant_id':b.tenant_id,'replayed':False}
 @app.get('/v1/router-plans')
 def plans(u=Depends(current),s=Depends(db)):
  if u.role not in ['sme','platform']:raise HTTPException(403,'Router plan permission required')
  q=select(RouterPlan)
  if u.role=='sme':q=q.where(RouterPlan.tenant_id==u.tenant_id)
  return [plan_json(p) for p in s.scalars(q.order_by(RouterPlan.created))]
 @app.post('/v1/routers/{id}/plan',status_code=201)
 def plan(id:str,b:PlanIn,u=Depends(current),s=Depends(db)):
  owner(u);r=s.get(RouterDevice,id)
  if not r or r.tenant_id!=u.tenant_id:raise HTTPException(404,'Router not found')
  h=visible_hub(s,u,b.hub_id)
  old=s.scalar(select(RouterPlan).where(RouterPlan.router_id==id))
  if old:
   if old.hub_id!=b.hub_id or old.router_public_key!=b.router_public_key:raise HTTPException(409,'Router already has an immutable plan; migration or key rotation needs an explicit workflow')
   return plan_json(old)|{'replayed':True}
  if b.router_public_key==h.public_key:raise HTTPException(422,'Router and hub public keys must differ')
  pool=ipaddress.ip_network(h.pool);conflict_portals(s,r,pool)
  # One atomic counter update serializes allocation across SQLite and PostgreSQL.
  index=s.execute(update(MagicHub).where(MagicHub.id==h.id,MagicHub.next_host<255).values(next_host=MagicHub.next_host+1).returning(MagicHub.next_host)).scalar_one_or_none()
  if index is None:raise HTTPException(409,'Hub address pool exhausted')
  address=str(pool.network_address+index-1);hub=hub_json(h);hub_address=hub['hub_address']
  snapshot={'version':1,'purpose':'router_management','hub':{k:hub[k] for k in ['id','name','endpoint','port','public_key','hub_address']},'router':{'id':r.id,'site_id':r.site_id,'public_key':b.router_public_key,'interface':'mm-'+r.id[:8],'address':address+'/32','peer_allowed_addresses':[hub_address+'/32'],'routes':[{'destination':hub_address+'/32','interface':'mm-'+r.id[:8]}],'persistent_keepalive_seconds':25},'hub_peer':{'public_key':b.router_public_key,'allowed_addresses':[address+'/32']},'private_key_location':'router_only','guest_traffic_via_hub':False,'checks_required':['Verify RouterOS WireGuard capabilities and existing interfaces/routes','Compare all live LAN/WAN/VPN subnets for address overlap','Take a recoverable router backup and review configuration diff','Register peer on hub through authenticated adapter','Review hub peer isolation and management-service firewall/TLS restrictions','Confirm recent handshake, tunnel reachability and management-service authorization'],'dispatch':'not_implemented'}
  p=RouterPlan(tenant_id=u.tenant_id,router_id=id,hub_id=h.id,router_public_key=b.router_public_key,address=address,snapshot=snapshot,created=int(time.time()));s.add(p)
  try:s.flush();log(s,u,'router_plan_created',p.id);s.commit()
  except IntegrityError:
   s.rollback();old=s.scalar(select(RouterPlan).where(RouterPlan.router_id==id))
   if old and old.hub_id==b.hub_id and old.router_public_key==b.router_public_key:return plan_json(old)|{'replayed':True}
   raise HTTPException(409,'Router public key or plan already allocated')
  return plan_json(p)
 @app.post('/v1/router-plans/{id}/review')
 def review(id:str,b:PlanReview,u=Depends(current),s=Depends(db)):
  owner(u);p=s.get(RouterPlan,id)
  if not p or p.tenant_id!=u.tenant_id:raise HTTPException(404,'Router plan not found')
  changed=s.execute(update(RouterPlan).where(RouterPlan.id==id,RouterPlan.reviewed_at.is_(None)).values(reviewed_at=int(time.time()))).rowcount
  if changed:log(s,u,'router_plan_reviewed',id)
  s.commit();s.refresh(p);return plan_json(p)|{'replayed':not bool(changed)}
