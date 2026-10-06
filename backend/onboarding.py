"""Platform business provisioning and temporary-password completion."""
import secrets,time,json,hashlib
from fastapi import Depends,HTTPException
from pydantic import Field,field_validator
from sqlalchemy import select,update,delete
from sqlalchemy.exc import IntegrityError
from argon2.exceptions import VerificationError,VerifyMismatchError
from .models import Tenant,User,SessionToken,LoginAttempt,PortalSettings,BusinessProvisioning,ReviewCredential,PasswordResetRecord,StaffGrant
from .schemas import Strict,PortalConfig
class BusinessIn(Strict):
 name:str=Field(min_length=1,max_length=160)
 owner_username:str=Field(min_length=3,max_length=160,pattern=r'^[a-z0-9][a-z0-9_.@+-]{2,159}$')
 request_key:str=Field(min_length=16,max_length=80,pattern=r'^[A-Za-z0-9-]+$')
 @field_validator('owner_username',mode='before')
 @classmethod
 def username(cls,v):return v.strip().lower() if isinstance(v,str) else v
class ResetIn(Strict):
 reason:str=Field(min_length=1,max_length=1000)
 confirmed_authority:bool
 @field_validator('confirmed_authority')
 @classmethod
 def authority(cls,v):
  if not v:raise ValueError('Confirm authority to reset this owner account')
  return v
class PasswordIn(Strict):
 model_config={'extra':'forbid','str_strip_whitespace':False}
 current_password:str=Field(min_length=1,max_length=256)
 new_password:str=Field(min_length=12,max_length=256)
 @field_validator('new_password')
 @classmethod
 def nonblank(cls,v):
  if not v.strip():raise ValueError('Password must not be blank')
  return v

def owner_json(u):return {k:getattr(u,k) for k in ['id','username','active','must_change_password','temporary_password_expires']}
def register_onboarding(app,current,db,log,hasher,digest,check):
 def platform(u):check(u.role=='platform','Platform operator permission required')
 @app.get('/v1/platform/businesses')
 def businesses(u=Depends(current),s=Depends(db)):
  platform(u);owners=list(s.scalars(select(User).where(User.role=='sme',~User.id.in_(select(StaffGrant.user_id))).order_by(User.username)))
  return [{'id':t.id,'name':t.name,'owners':[owner_json(a) for a in owners if a.tenant_id==t.id]} for t in s.scalars(select(Tenant).order_by(Tenant.name,Tenant.id))]
 @app.get('/v1/platform/password-resets')
 def resets(u=Depends(current),s=Depends(db)):
  platform(u);q=select(PasswordResetRecord,User).join(User,User.id==PasswordResetRecord.user_id).order_by(PasswordResetRecord.created.desc(),PasswordResetRecord.id).limit(200)
  return [{'id':r.id,'user_id':r.user_id,'tenant_id':a.tenant_id,'username':a.username,'actor_id':r.actor_id,'reason':r.reason,'created':r.created} for r,a in s.execute(q)]
 @app.post('/v1/platform/businesses',status_code=201)
 def create(b:BusinessIn,u=Depends(current),s=Depends(db)):
  platform(u);fingerprint=hashlib.sha256(json.dumps({'name':b.name,'owner_username':b.owner_username},sort_keys=True).encode()).hexdigest()
  existing=s.scalar(select(BusinessProvisioning).where(BusinessProvisioning.created_by==u.id,BusinessProvisioning.request_key==b.request_key))
  if existing:
   check(existing.request_hash==fingerprint,'Request key reused for another business',409);t=s.get(Tenant,existing.tenant_id);a=s.get(User,existing.owner_id);return {'id':t.id,'name':t.name,'owner':owner_json(a),'replayed':True,'shown_once':False}
  check(not s.scalar(select(User.id).where(User.username==b.owner_username)),'Username is already in use',409)
  try:
   raw=secrets.token_urlsafe(18);now=int(time.time());t=Tenant(name=b.name);s.add(t);s.flush();a=User(username=b.owner_username,role='sme',tenant_id=t.id,password_hash=hasher.hash(raw),active=True,must_change_password=True,temporary_password_expires=now+86400);s.add(a);s.flush()
   cfg=PortalConfig(business_name=b.name[:120],direct_purchase=False,voucher_login=False).model_dump();s.add(PortalSettings(tenant_id=t.id,revision=1,draft=cfg,published=None,published_revision=None,updated=now))
   s.add(BusinessProvisioning(tenant_id=t.id,owner_id=a.id,created_by=u.id,request_key=b.request_key,request_hash=fingerprint,created=now));log(s,u,'business_created',t.id);log(s,u,'business_owner_created',a.id)
   s.commit()
  except IntegrityError:s.rollback();raise HTTPException(409,'Business or username created concurrently; retry with the same request key')
  return {'id':t.id,'name':t.name,'owner':owner_json(a),'temporary_password':raw,'shown_once':True,'replayed':False}
 @app.post('/v1/platform/businesses/{id}/owners/{owner_id}/reset-password')
 def reset(id:str,owner_id:str,b:ResetIn,u=Depends(current),s=Depends(db)):
  platform(u);a=s.get(User,owner_id);check(a and a.role=='sme' and a.tenant_id==id and not s.get(StaffGrant,owner_id),'Business owner not found',404)
  raw=secrets.token_urlsafe(18);now=int(time.time())
  # Claim account row before invalidating sessions and service authority.
  s.execute(update(User).where(User.id==a.id).values(password_hash=hasher.hash(raw),must_change_password=True,temporary_password_expires=now+86400));s.execute(delete(SessionToken).where(SessionToken.user_id==a.id));s.execute(update(ReviewCredential).where(ReviewCredential.user_id==a.id,ReviewCredential.revoked_at==None).values(revoked_at=now,revoke_reason='Platform password reset: '+b.reason[:970]));s.execute(delete(LoginAttempt).where(LoginAttempt.key.in_([digest('account:'+a.username),digest('password-change:'+a.id)])));log(s,u,'owner_password_reset',a.id)
  # The reason remains in a dedicated reset record; no password enters audit data.
  s.add(PasswordResetRecord(user_id=a.id,actor_id=u.id,reason=b.reason,created=now));s.commit();s.refresh(a);return {'owner':owner_json(a),'temporary_password':raw,'shown_once':True}
 @app.post('/v1/auth/password')
 def password(b:PasswordIn,u=Depends(current),s=Depends(db)):
  now=int(time.time());key=digest('password-change:'+u.id);bucket=s.get(LoginAttempt,key)
  if not bucket:bucket=LoginAttempt(key=key,failures=0,window_start=now);s.add(bucket)
  if now-bucket.window_start>=900:bucket.failures=0;bucket.window_start=now
  check(bucket.failures<8,'Too many password attempts; try again later',429)
  try:ok=hasher.verify(u.password_hash,b.current_password)
  except (VerificationError,VerifyMismatchError):ok=False
  if not ok:bucket.failures+=1;s.commit();raise HTTPException(400,'Current password is incorrect')
  check(b.new_password!=b.current_password,'Choose a different password',422)
  changed=s.execute(update(User).where(User.id==u.id,User.password_hash==u.password_hash).values(password_hash=hasher.hash(b.new_password),must_change_password=False,temporary_password_expires=None)).rowcount;check(changed==1,'Password changed elsewhere. Sign in again.',409)
  bucket.failures=0;s.execute(delete(SessionToken).where(SessionToken.user_id==u.id));raw=secrets.token_urlsafe(48);s.add(SessionToken(token_hash=digest(raw),user_id=u.id,expires=now+3600));log(s,u,'password_changed',u.id);s.commit();return {'access_token':raw,'token_type':'bearer','expires_in':3600,'password_changed':True}
