"""Owner-delegated staff profiles; permissions checked on every account request."""
import json,re,secrets,time
from typing import Literal
from pydantic import Field
from fastapi import Depends,HTTPException
from sqlalchemy import select,update,delete
from sqlalchemy.exc import IntegrityError
from .schemas import Strict
from .models import StaffGrant,User,SessionToken,ReviewCredential,LoginAttempt
def staff_allowed(profile,method,path):
 if path in ['/v1/me','/v1/auth/password','/v1/auth/logout'] or path.startswith('/v1/account/sessions') or path.startswith('/v1/account/mfa'):return True
 if path in ['/v1/appearance/effective','/v1/tenants']:return method=='GET'
 if profile=='manager':return any(path=='/v1/'+x or path.startswith('/v1/'+x+'/') for x in ['tenants','packages','orders','entitlements','audit','portal','vouchers','sites','routers','hubs','router-plans','revenue','knowledge','media','subscription'])
 if profile=='cashier':
  if method=='GET':return path in ['/v1/packages','/v1/orders','/v1/vouchers','/v1/sites','/v1/audit','/v1/revenue'] or path.startswith('/v1/revenue/') or bool(re.fullmatch(r'/v1/orders/[^/]+/receipts(?:/[^/]+)?',path))
  return path in ['/v1/vouchers','/v1/vouchers/batch'] or bool(re.fullmatch(r'/v1/orders/[^/]+/decision',path)) or bool(re.fullmatch(r'/v1/vouchers/[^/]+/sale',path))
 if profile=='technician':
  if method=='GET':return path in ['/v1/sites','/v1/routers','/v1/hubs','/v1/router-plans','/v1/knowledge','/v1/audit'] or bool(re.fullmatch(r'/v1/sites/[^/]+/planning',path))
  return path in ['/v1/sites','/v1/routers','/v1/router-plans'] or bool(re.fullmatch(r'/v1/sites/[^/]+/planning',path))
 return False
class StaffCreate(Strict):
 username:str=Field(min_length=3,max_length=160,pattern=r'^[a-z0-9][a-z0-9_.@+-]{2,159}$')
 profile:Literal['manager','cashier','technician']
 request_key:str=Field(min_length=16,max_length=80)
class StaffChange(Strict):
 expected_revision:int=Field(ge=1)
 profile:Literal['manager','cashier','technician']
 active:bool
 reason:str=Field(min_length=1,max_length=1000)
class StaffReset(StaffChange):
 confirmed_authority:Literal[True]
def staff_json(g,a):return {'id':a.id,'username':a.username,'profile':g.profile,'revision':g.revision,'active':a.active,'must_change_password':a.must_change_password,'temporary_password_expires':a.temporary_password_expires,'last_reason':g.last_reason}
def register_staff(app,current,db,owner,log,hasher,digest):
 def full_owner(s,u):
  owner(u)
  if s.get(StaffGrant,u.id):raise HTTPException(403,'Only the business owner manages staff')
 def target(s,u,id):
  g=s.get(StaffGrant,id);a=s.get(User,id)
  if not g or not a or a.tenant_id!=u.tenant_id or g.tenant_id!=u.tenant_id:raise HTTPException(404,'Staff account not found')
  return g,a
 @app.get('/v1/staff')
 def staff(u=Depends(current),s=Depends(db)):
  full_owner(s,u);return [staff_json(g,a) for g,a in s.execute(select(StaffGrant,User).join(User,User.id==StaffGrant.user_id).where(StaffGrant.tenant_id==u.tenant_id).order_by(User.username))]
 @app.post('/v1/staff',status_code=201)
 def create(b:StaffCreate,u=Depends(current),s=Depends(db)):
  full_owner(s,u);fingerprint=digest(json.dumps(b.model_dump(exclude={'request_key'}),sort_keys=True));existing=s.scalar(select(StaffGrant).where(StaffGrant.created_by==u.id,StaffGrant.request_key==b.request_key))
  if existing:
   if existing.request_hash!=fingerprint:raise HTTPException(409,'Request key reused for different settings')
   return {'staff':staff_json(existing,s.get(User,existing.user_id)),'replayed':True,'shown_once':False}
  raw=secrets.token_urlsafe(18);now=int(time.time());a=User(username=b.username,password_hash=hasher.hash(raw),role='sme',tenant_id=u.tenant_id,active=True,must_change_password=True,temporary_password_expires=now+86400);s.add(a)
  try:
   s.flush();g=StaffGrant(user_id=a.id,tenant_id=u.tenant_id,profile=b.profile,revision=1,created_by=u.id,request_key=b.request_key,request_hash=fingerprint,created=now,updated=now);s.add(g);s.flush();log(s,u,'staff_created',a.id+':'+b.profile);s.commit()
  except IntegrityError:s.rollback();raise HTTPException(409,'Username or request key already exists; retry safely')
  return {'staff':staff_json(g,a),'temporary_password':raw,'shown_once':True,'replayed':False}
 @app.post('/v1/staff/{id}')
 def change(id:str,b:StaffChange,u=Depends(current),s=Depends(db)):
  full_owner(s,u);g,a=target(s,u,id)
  if not s.execute(update(StaffGrant).where(StaffGrant.user_id==id,StaffGrant.revision==b.expected_revision).values(profile=b.profile,revision=b.expected_revision+1,last_reason=b.reason,updated=int(time.time()))).rowcount:raise HTTPException(409,'Staff account changed; refresh')
  a.active=b.active;s.execute(delete(SessionToken).where(SessionToken.user_id==a.id));s.execute(update(ReviewCredential).where(ReviewCredential.user_id==a.id,ReviewCredential.revoked_at==None).values(revoked_at=int(time.time()),revoke_reason='Staff authority changed: '+b.reason[:970]));log(s,u,'staff_authority_changed',id+':'+b.profile+':'+str(b.active));s.commit();s.refresh(g)
  return staff_json(g,a)
 @app.post('/v1/staff/{id}/reset-password')
 def reset(id:str,b:StaffReset,u=Depends(current),s=Depends(db)):
  full_owner(s,u);g,a=target(s,u,id)
  if b.profile!=g.profile or b.active!=a.active:raise HTTPException(409,'Password reset cannot also change authority')
  if not s.execute(update(StaffGrant).where(StaffGrant.user_id==id,StaffGrant.revision==b.expected_revision).values(revision=b.expected_revision+1,last_reason=b.reason,updated=int(time.time()))).rowcount:raise HTTPException(409,'Staff account changed; refresh')
  raw=secrets.token_urlsafe(18);a.password_hash=hasher.hash(raw);a.must_change_password=True;a.temporary_password_expires=int(time.time())+86400
  s.execute(delete(SessionToken).where(SessionToken.user_id==id));s.execute(update(ReviewCredential).where(ReviewCredential.user_id==id,ReviewCredential.revoked_at==None).values(revoked_at=int(time.time()),revoke_reason='Staff password reset: '+b.reason[:970]));s.execute(delete(LoginAttempt).where(LoginAttempt.key.in_([digest('account:'+a.username),digest('password-change:'+a.id)])));log(s,u,'staff_password_reset',id);s.commit();s.refresh(g)
  return {'staff':staff_json(g,a),'temporary_password':raw,'shown_once':True}
