"""Optional RFC 6238 authenticators. Requires a separately configured encryption key."""
import base64,hashlib,hmac,secrets,struct,time
from urllib.parse import quote,urlencode
from cryptography.fernet import Fernet,InvalidToken
from fastapi import Depends,HTTPException
from pydantic import Field,ConfigDict
from sqlalchemy import update,delete
from argon2.exceptions import VerificationError,VerifyMismatchError
from .schemas import Strict
from .models import AccountMFA,User,SessionToken,LoginAttempt
def cipher(key):
 if not key:raise HTTPException(503,'Authenticator enrollment requires a configured server encryption key')
 try:return Fernet(key.encode() if isinstance(key,str) else key)
 except (ValueError,TypeError):raise HTTPException(503,'Server authenticator encryption key is invalid')
def totp(secret,step,digits=6):
 data=hmac.new(base64.b32decode(secret),struct.pack('>Q',step),hashlib.sha1).digest();offset=data[-1]&15;number=(struct.unpack('>I',data[offset:offset+4])[0]&0x7fffffff)%(10**digits);return str(number).zfill(digits)
def matching_step(secret,code,now):
 if len(code)!=6 or not code.isdigit():return None
 step=int(now)//30
 return next((i for i in [step,step-1,step+1] if i>=0 and hmac.compare_digest(totp(secret,i),code)),None)
def recovery_digest(code):return hashlib.sha256(('mfa-recovery:'+code).encode()).hexdigest()
def consume_mfa(s,u,code,key):
 # All consumers share the account row lock, including recovery-code consumption.
 if not s.execute(update(User).where(User.id==u.id,User.active==True).values(active=User.active)).rowcount:return False
 row=s.get(AccountMFA,u.id,populate_existing=True)
 if not row or not row.enabled:return True
 if not code:return False
 code=code.strip();hashed=recovery_digest(code)
 if any(hmac.compare_digest(hashed,x) for x in row.recovery_hashes):
  remaining=[x for x in row.recovery_hashes if not hmac.compare_digest(hashed,x)]
  return bool(s.execute(update(AccountMFA).where(AccountMFA.user_id==u.id,AccountMFA.enabled==True).values(recovery_hashes=remaining,updated=int(time.time()))).rowcount)
 try:secret=cipher(key).decrypt(row.encrypted_secret.encode()).decode()
 except (InvalidToken,AttributeError):raise HTTPException(503,'Authenticator key cannot decrypt this account; contact the operator')
 step=matching_step(secret,code,time.time())
 if step is None:return False
 return bool(s.execute(update(AccountMFA).where(AccountMFA.user_id==u.id,AccountMFA.enabled==True,AccountMFA.last_step<step).values(last_step=step,updated=int(time.time()))).rowcount)
class PasswordProof(Strict):
 model_config=ConfigDict(extra='forbid',str_strip_whitespace=False)
 password:str=Field(min_length=1,max_length=256)
class CodeProof(PasswordProof):
 code:str=Field(min_length=1,max_length=80)
def register_mfa(app,current,db,log,hasher,digest,key):
 def proof(s,u,password):
  # Serialize password/code attempts before reading or creating the throttle row.
  if not s.execute(update(User).where(User.id==u.id,User.password_hash==u.password_hash,User.active==True).values(active=User.active)).rowcount:raise HTTPException(409,'Account changed; sign in again')
  now=int(time.time());bucket_key=digest('mfa-operation:'+u.id);bucket=s.get(LoginAttempt,bucket_key)
  if not bucket:bucket=LoginAttempt(key=bucket_key,failures=0,window_start=now);s.add(bucket)
  if now-bucket.window_start>=900:bucket.failures=0;bucket.window_start=now
  if bucket.failures>=8:raise HTTPException(429,'Too many authenticator attempts; try later')
  try:ok=hasher.verify(u.password_hash,password)
  except (VerificationError,VerifyMismatchError):ok=False
  if not ok:bucket.failures+=1;s.commit();raise HTTPException(400,'Password is incorrect')
  if not s.execute(update(User).where(User.id==u.id,User.password_hash==u.password_hash,User.active==True).values(active=User.active)).rowcount:raise HTTPException(409,'Account changed; sign in again')
  return bucket
 def fail(s,bucket,message):bucket.failures+=1;s.commit();raise HTTPException(400,message)
 def rotate(s,u):
  s.execute(delete(SessionToken).where(SessionToken.user_id==u.id));raw=secrets.token_urlsafe(48);s.add(SessionToken(token_hash=digest(raw),user_id=u.id,expires=int(time.time())+3600));return raw
 @app.get('/v1/account/mfa')
 def status(u=Depends(current),s=Depends(db)):
  row=s.get(AccountMFA,u.id);return {'enabled':bool(row and row.enabled),'enrollment_available':bool(key),'pending':bool(row and not row.enabled and row.enrollment_expires and row.enrollment_expires>time.time()),'recovery_codes_remaining':len(row.recovery_hashes) if row and row.enabled else 0}
 @app.post('/v1/account/mfa/enroll')
 def enroll(b:PasswordProof,u=Depends(current),s=Depends(db)):
  encryption=cipher(key);bucket=proof(s,u,b.password);row=s.get(AccountMFA,u.id,populate_existing=True)
  if row and row.enabled:raise HTTPException(409,'Disable the existing authenticator before replacing it')
  secret=base64.b32encode(secrets.token_bytes(20)).decode();now=int(time.time())
  values={'enabled':False,'encrypted_secret':encryption.encrypt(secret.encode()).decode(),'enrollment_expires':now+600,'last_step':-1,'recovery_hashes':[],'updated':now}
  if row:s.execute(update(AccountMFA).where(AccountMFA.user_id==u.id).values(**values))
  else:s.add(AccountMFA(user_id=u.id,**values))
  log(s,u,'mfa_enrollment_started',u.id);s.commit();return {'secret':secret,'provisioning_uri':'otpauth://totp/'+quote('MikroMagic:'+u.username,safe='')+'?'+urlencode({'secret':secret,'issuer':'MikroMagic','algorithm':'SHA1','digits':6,'period':30}),'expires':now+600,'shown_once':True}
 @app.post('/v1/account/mfa/confirm')
 def confirm(b:CodeProof,u=Depends(current),s=Depends(db)):
  bucket=proof(s,u,b.password);row=s.get(AccountMFA,u.id,populate_existing=True)
  if not row or row.enabled or not row.enrollment_expires or row.enrollment_expires<=time.time():raise HTTPException(409,'Enrollment expired or unavailable; start again')
  try:secret=cipher(key).decrypt(row.encrypted_secret.encode()).decode()
  except InvalidToken:raise HTTPException(503,'Enrollment key unavailable')
  step=matching_step(secret,b.code.strip(),time.time())
  if step is None:fail(s,bucket,'Authenticator code is incorrect')
  raw_codes=[secrets.token_hex(16) for _ in range(10)];s.execute(update(AccountMFA).where(AccountMFA.user_id==u.id,AccountMFA.enabled==False).values(enabled=True,last_step=step,enrollment_expires=None,recovery_hashes=[recovery_digest(c) for c in raw_codes],updated=int(time.time())));bucket.failures=0;token=rotate(s,u);log(s,u,'mfa_enabled',u.id);s.commit();return {'enabled':True,'recovery_codes':raw_codes,'shown_once':True,'access_token':token,'expires_in':3600}
 @app.post('/v1/account/mfa/disable')
 def disable(b:CodeProof,u=Depends(current),s=Depends(db)):
  bucket=proof(s,u,b.password);row=s.get(AccountMFA,u.id,populate_existing=True)
  if not row or not row.enabled:raise HTTPException(409,'Authenticator is not enabled')
  if not consume_mfa(s,u,b.code,key):fail(s,bucket,'Authenticator or recovery code is incorrect or already used')
  s.execute(update(AccountMFA).where(AccountMFA.user_id==u.id).values(enabled=False,encrypted_secret=None,enrollment_expires=None,last_step=-1,recovery_hashes=[],updated=int(time.time())));bucket.failures=0;token=rotate(s,u);log(s,u,'mfa_disabled',u.id);s.commit();return {'enabled':False,'access_token':token,'expires_in':3600}
