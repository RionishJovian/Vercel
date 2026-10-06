"""Delegated review API. Bot-platform identity verification belongs to the trusted adapter."""
import time,secrets,hashlib,json
from typing import Literal
from fastapi import Depends,HTTPException,Response
from fastapi.security import HTTPBearer,HTTPAuthorizationCredentials
from pydantic import Field
from sqlalchemy import select,update
from sqlalchemy.exc import IntegrityError
from .schemas import Strict,Decision
from .models import ReviewCredential,ExternalReviewAction,User,Order,ReceiptImage,Site
from .receipts import receipt_metadata

def fingerprint(value):return hashlib.sha256(json.dumps(value,sort_keys=True,separators=(',',':')).encode()).hexdigest()
class CredentialIn(Strict):
 label:str=Field(min_length=1,max_length=120)
 provider:Literal['telegram','discord','custom']
 reviewer_identity:str=Field(min_length=1,max_length=160)
 expires_days:int=Field(default=30,ge=1,le=90)
 receive_events:bool=False
 can_decide:bool=False
 authorize_service:Literal[True]
class ExternalDecision(Strict):
 decision:Decision
 context_hash:str=Field(pattern=r'^[a-f0-9]{64}$')
 reviewer_identity:str=Field(min_length=1,max_length=160)
 request_key:str=Field(min_length=16,max_length=80,pattern=r'^[A-Za-z0-9-]+$')
class RevokeIn(Strict):reason:str=Field(min_length=1,max_length=1000)
def credential_json(c):return {k:getattr(c,k) for k in ['id','label','provider','reviewer_identity','can_decide','receive_events','created','expires','revoked_at','revoke_reason']}
def register_review_api(app,current,db,owner,order_for,order_json,perform_review,log):
 auth=HTTPBearer(auto_error=False)
 def service(credentials:HTTPAuthorizationCredentials|None=Depends(auth),s=Depends(db)):
  if not credentials:raise HTTPException(401,'Review credential required')
  c=s.scalar(select(ReviewCredential).where(ReviewCredential.token_hash==hashlib.sha256(credentials.credentials.encode()).hexdigest()))
  if not c or c.revoked_at or c.expires<=time.time():raise HTTPException(401,'Review credential expired or revoked')
  u=s.get(User,c.user_id)
  if not u or not u.active or u.must_change_password or u.role!='sme' or u.tenant_id!=c.tenant_id:raise HTTPException(401,'Authorizing owner account is unavailable')
  return c,u
 def lock_credential(s,c):
  if not s.execute(update(ReviewCredential).where(ReviewCredential.id==c.id,ReviewCredential.revoked_at==None,ReviewCredential.expires>int(time.time())).values(expires=ReviewCredential.expires)).rowcount:raise HTTPException(401,'Review credential expired or revoked')
 def context(s,u,o):
  data=order_json(o);site=s.get(Site,o.site_id) if o.site_id else None;guest=s.get(User,o.guest_id)
  data['site_name']=site.name if site else None;data['guest_username']=guest.username if guest else None
  data['receipts']=receipt_metadata(s,o.id)
  data['duplicate_reference_order_ids']=list(s.scalars(select(Order.id).where(Order.tenant_id==u.tenant_id,Order.reference==o.reference,Order.id!=o.id).limit(20)))
  # Only mutable review context needs a fresh decision; duplicate warnings are advisory.
  data['context_hash']=fingerprint(order_json(o))
  data['settlement_verified']=False;data['adapter_connected']=False
  return data
 from .deliveries import register_deliveries
 register_deliveries(app,current,db,owner,service,lock_credential,log)
 @app.get('/v1/review-credentials')
 def credentials(u=Depends(current),s=Depends(db)):
  owner(u);return [credential_json(c) for c in s.scalars(select(ReviewCredential).where(ReviewCredential.tenant_id==u.tenant_id,ReviewCredential.user_id==u.id).order_by(ReviewCredential.created.desc()))]
 @app.post('/v1/review-credentials',status_code=201)
 def create(b:CredentialIn,u=Depends(current),s=Depends(db)):
  owner(u);now=int(time.time());raw='mmreview_'+secrets.token_urlsafe(48);c=ReviewCredential(tenant_id=u.tenant_id,user_id=u.id,label=b.label,provider=b.provider,reviewer_identity=b.reviewer_identity,can_decide=b.can_decide,receive_events=b.receive_events,token_hash=hashlib.sha256(raw.encode()).hexdigest(),created=now,expires=now+b.expires_days*86400);s.add(c);s.flush();log(s,u,'review_credential_created',c.id);s.commit();return {**credential_json(c),'token':raw,'shown_once':True}
 @app.post('/v1/review-credentials/{id}/revoke')
 def revoke(id:str,b:RevokeIn,u=Depends(current),s=Depends(db)):
  owner(u);c=s.get(ReviewCredential,id)
  if not c or c.tenant_id!=u.tenant_id or c.user_id!=u.id:raise HTTPException(404,'Review credential not found')
  if not c.revoked_at:
   s.execute(update(ReviewCredential).where(ReviewCredential.id==id,ReviewCredential.revoked_at==None).values(revoked_at=int(time.time()),revoke_reason=b.reason));log(s,u,'review_credential_revoked',c.id);s.commit();s.refresh(c)
  return credential_json(c)
 @app.get('/v1/external-review-actions')
 def actions(u=Depends(current),s=Depends(db)):
  owner(u);return [{k:getattr(a,k) for k in ['id','credential_id','order_id','reviewer_identity','provider','service_label','decision','reason','created']} for a in s.scalars(select(ExternalReviewAction).where(ExternalReviewAction.tenant_id==u.tenant_id).order_by(ExternalReviewAction.created.desc()).limit(200))]
 @app.get('/v1/review-api/orders')
 def queue(status:Literal['pending','needs_evidence','approved','rejected','refunded']='pending',limit:int=30,offset:int=0,p=Depends(service),s=Depends(db)):
  c,u=p
  if not 1<=limit<=100 or not 0<=offset<=1000000:raise HTTPException(422,'Invalid queue pagination')
  return [context(s,u,o) for o in s.scalars(select(Order).where(Order.tenant_id==u.tenant_id,Order.status==status).order_by(Order.created,Order.id).limit(limit).offset(offset))]
 @app.get('/v1/review-api/orders/{id}')
 def detail(id:str,p=Depends(service),s=Depends(db)):
  c,u=p;return context(s,u,order_for(s,u,id))
 @app.get('/v1/review-api/orders/{id}/receipts/{receipt_id}')
 def image(id:str,receipt_id:str,p=Depends(service),s=Depends(db)):
  c,u=p;order_for(s,u,id);r=s.get(ReceiptImage,receipt_id)
  if not r or r.order_id!=id:raise HTTPException(404,'Receipt not found')
  log(s,u,'external_receipt_viewed',c.id+':'+r.id);s.commit();return Response(r.image,media_type='image/png',headers={'Content-Security-Policy':"default-src 'none'; sandbox",'Content-Disposition':'inline; filename="receipt.png"'})
 @app.post('/v1/review-api/orders/{id}/decision')
 def decide(id:str,b:ExternalDecision,p=Depends(service),s=Depends(db)):
  c,u=p
  if not c.can_decide:raise HTTPException(403,'This service has read-only permission')
  if b.reviewer_identity!=c.reviewer_identity:raise HTTPException(403,'Reviewer identity does not match the delegated credential')
  if b.decision.expected_receipt_revision is None:raise HTTPException(422,'Expected receipt revision is required')
  lock_credential(s,c);request_hash=fingerprint({'order_id':id,**b.model_dump()});existing=s.scalar(select(ExternalReviewAction).where(ExternalReviewAction.credential_id==c.id,ExternalReviewAction.request_key==b.request_key))
  if existing:
   if existing.request_hash!=request_hash:raise HTTPException(409,'Review request key reused with another decision')
   return {**existing.result,'external_action_id':existing.id,'replayed':True}
  o=order_for(s,u,id);s.execute(update(Order).where(Order.id==o.id).values(receipt_revision=Order.receipt_revision));s.refresh(o)
  if fingerprint(order_json(o))!=b.context_hash:raise HTTPException(409,'Payment context changed. Fetch the order again before deciding.')
  result=perform_review(s,u,id,b.decision,allow_replay=False)
  a=ExternalReviewAction(tenant_id=u.tenant_id,credential_id=c.id,order_id=id,request_key=b.request_key,request_hash=request_hash,reviewer_identity=c.reviewer_identity,provider=c.provider,service_label=c.label,decision=b.decision.decision,reason=b.decision.reason,result=result,created=int(time.time()));s.add(a);s.flush();log(s,u,'external_review_decision',a.id)
  try:s.commit()
  except IntegrityError:s.rollback();raise HTTPException(409,'Decision already processed; retry with the same request key')
  return {**result,'external_action_id':a.id,'replayed':False}
