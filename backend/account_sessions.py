"""Own-account session control. No device identity is inferred from tokens."""
import time
from fastapi import Depends,Request,HTTPException
from sqlalchemy import select,delete
from pydantic import Field
from .schemas import Strict
from .models import SessionToken
class RevokeSession(Strict):
 session_ref:str=Field(pattern=r'^[a-f0-9]{64}$')
class RevokeOthers(Strict):
 confirmed:bool
def register_account_sessions(app,current,db,log,digest):
 def current_hash(request):return digest(request.headers.get('authorization','').split(' ',1)[-1])
 def ref(value):return digest('session-ref:'+value)
 @app.get('/v1/account/sessions')
 def sessions(request:Request,u=Depends(current),s=Depends(db)):
  own=current_hash(request)
  return [{'session_ref':ref(t.token_hash),'expires':t.expires,'current':t.token_hash==own,'device_identity':'not_collected'} for t in s.scalars(select(SessionToken).where(SessionToken.user_id==u.id,SessionToken.expires>time.time()).order_by(SessionToken.expires.desc()))]
 @app.post('/v1/account/sessions/revoke')
 def revoke(b:RevokeSession,request:Request,u=Depends(current),s=Depends(db)):
  # Public references are domain-separated digests, never bearer tokens or their stored hashes.
  target=next((t for t in s.scalars(select(SessionToken).where(SessionToken.user_id==u.id,SessionToken.expires>time.time())) if ref(t.token_hash)==b.session_ref),None)
  if not target:raise HTTPException(404,'Active session not found')
  if target.token_hash==current_hash(request):raise HTTPException(409,'Use Sign out to end the current session')
  s.execute(delete(SessionToken).where(SessionToken.token_hash==target.token_hash,SessionToken.user_id==u.id));log(s,u,'other_session_revoked',b.session_ref);s.commit();return {'revoked':True}
 @app.post('/v1/account/sessions/revoke-others')
 def others(b:RevokeOthers,request:Request,u=Depends(current),s=Depends(db)):
  if not b.confirmed:raise HTTPException(422,'Confirm that other sessions should end')
  removed=s.execute(delete(SessionToken).where(SessionToken.user_id==u.id,SessionToken.token_hash!=current_hash(request))).rowcount
  log(s,u,'other_sessions_revoked',str(removed));s.commit();return {'revoked_count':removed,'current_session_preserved':True,'service_credentials_changed':False}
