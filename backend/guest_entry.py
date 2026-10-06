"""Public published-site entry. Creates cloud accounts, never HotSpot sessions."""
import re,time
from fastapi import Depends,HTTPException,Request
from pydantic import BaseModel,Field,ConfigDict
from sqlalchemy import select,update
from sqlalchemy.exc import IntegrityError
from .models import Site,SitePortal,User,Package,LoginAttempt
from .portal import public_config
class Signup(BaseModel):
 model_config=ConfigDict(extra='forbid')
 username:str=Field(min_length=3,max_length=160)
 password:str=Field(min_length=12,max_length=256)
 revision:int
 accepted_terms:bool

def register_guest_entry(app,db,hasher,digest,log):
 def published(s,id):
  site=s.get(Site,id);portal=s.get(SitePortal,id)
  if not site or not portal or not portal.published or portal.published_revision is None:raise HTTPException(404,'Published site not found')
  if not (portal.published['direct_purchase'] or portal.published['voucher_login']):raise HTTPException(404,'Guest entry is disabled')
  return site,portal
 @app.get('/v1/public/sites/{id}')
 def entry(id:str,s=Depends(db)):
  site,portal=published(s,id)
  return {'site_id':id,'site_name':site.name,'revision':portal.published_revision,'settings':public_config(portal.published),'packages':[{'id':p.id,'policy':p.policy} for p in s.scalars(select(Package).where(Package.tenant_id==site.tenant_id,Package.active==True))] if portal.published['direct_purchase'] else [],'network_access':'not_connected'}
 @app.post('/v1/public/sites/{id}/register',status_code=201)
 def register(id:str,b:Signup,request:Request,s=Depends(db)):
  site,portal=published(s,id)
  now=int(time.time());key=digest('guest-signup-ip:'+(request.client.host if request.client else 'unknown'))
  bucket=s.get(LoginAttempt,key)
  if not bucket:bucket=LoginAttempt(key=key,failures=0,window_start=now);s.add(bucket)
  if now-bucket.window_start>=900:bucket.failures=0;bucket.window_start=now
  if bucket.failures>=8:raise HTTPException(429,'Too many registrations; try later')
  try:s.flush()
  except IntegrityError:s.rollback();raise HTTPException(429,'Registration busy; try again later')
  claimed=s.execute(update(LoginAttempt).where(LoginAttempt.key==key,LoginAttempt.failures<8).values(failures=LoginAttempt.failures+1))
  if not claimed.rowcount:raise HTTPException(429,'Too many registrations; try later')
  s.commit()
  if b.revision!=portal.published_revision:raise HTTPException(409,'Portal changed; reload and review the terms')
  name=b.username.strip().lower()
  if not re.fullmatch(r'[a-z0-9_.-]{3,160}',name) or not b.password.strip():raise HTTPException(422,'Use a valid username and a nonblank password')
  if not b.accepted_terms:raise HTTPException(422,'Accept the published terms before registering')
  u=User(username=name,password_hash=hasher.hash(b.password),role='guest',tenant_id=site.tenant_id)
  s.add(u)
  try:
   s.flush();log(s,u,'guest_registered',id+':revision:'+str(b.revision));s.commit()
  except IntegrityError:
   s.rollback();raise HTTPException(409,'Username unavailable; choose another username')
  return {'username':name,'site_id':id,'role':'guest','network_access':'not_connected'}
