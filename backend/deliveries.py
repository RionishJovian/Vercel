"""Transactional per-service events and leased pull delivery. No external networking."""
import time,secrets,hashlib
from typing import Literal
from fastapi import Depends,HTTPException
from pydantic import Field,model_validator
from sqlalchemy import select,update,or_,and_
from .models import Order,Outbox,ReviewCredential,EventDelivery,User
from .schemas import Strict

def emit_payment_event(s,o,event):
 s.flush();revision=s.execute(update(Order).where(Order.id==o.id).values(event_revision=Order.event_revision+1).returning(Order.event_revision)).scalar_one();now=int(time.time())
 payload={k:getattr(o,k) for k in ['id','site_id','guest_id','snapshot','payment_context','amount','currency','method','reference','evidence','status','reason','reviewer_id','receipt_revision','fulfillment']}
 e=Outbox(tenant_id=o.tenant_id,order_id=o.id,event=event,revision=revision,created=now,payload=payload);s.add(e);s.flush()
 q=select(ReviewCredential).join(User,User.id==ReviewCredential.user_id).where(ReviewCredential.tenant_id==o.tenant_id,ReviewCredential.receive_events==True,ReviewCredential.revoked_at==None,ReviewCredential.expires>now,User.active==True,User.must_change_password==False,User.role=='sme',User.tenant_id==o.tenant_id)
 for c in s.scalars(q):s.add(EventDelivery(tenant_id=o.tenant_id,credential_id=c.id,event_id=e.id,state='pending',attempts=0,cycle_attempts=0,available_at=now))
class SubscriptionIn(Strict):receive_events:bool
class ClaimIn(Strict):
 limit:int=Field(default=10,ge=1,le=50)
 lease_seconds:int=Field(default=120,ge=30,le=300)
class CompleteIn(Strict):
 lease_token:str=Field(min_length=30,max_length=100)
 result:Literal['acknowledged','retry']
 error:str=Field(default='',max_length=1000)
 @model_validator(mode='after')
 def retry_reason(self):
  if self.result=='retry' and not self.error:raise ValueError('Retry requires a failure description')
  return self
class RetryIn(Strict):reason:str=Field(min_length=1,max_length=1000)
def delivery_json(d,e,c=None,owner_available=True):return {k:getattr(d,k) for k in ['id','credential_id','event_id','state','attempts','cycle_attempts','available_at','lease_expires','acknowledged_at','last_error']}|{'order_id':e.order_id,'event':e.event,'event_revision':e.revision,'event_created':e.created,'service_label':c.label if c else None,'subscription_enabled':bool(c and c.receive_events),'credential_available':bool(owner_available and c and not c.revoked_at and c.expires>time.time())}

def register_deliveries(app,current,db,owner,service,lock_credential,log):
 @app.post('/v1/review-credentials/{id}/events')
 def subscribe(id:str,b:SubscriptionIn,u=Depends(current),s=Depends(db)):
  owner(u);c=s.get(ReviewCredential,id)
  if not c or c.user_id!=u.id or c.tenant_id!=u.tenant_id:raise HTTPException(404,'Credential not found')
  lock_credential(s,c);c.receive_events=b.receive_events;log(s,u,'event_subscription_enabled' if b.receive_events else 'event_subscription_paused',c.id);s.commit();return {'receive_events':c.receive_events}
 @app.get('/v1/event-deliveries')
 def list_deliveries(state:Literal['pending','leased','acknowledged','failed']|None=None,limit:int=100,offset:int=0,u=Depends(current),s=Depends(db)):
  owner(u)
  if not 1<=limit<=200 or not 0<=offset<=1000000:raise HTTPException(422,'Invalid delivery pagination')
  q=select(EventDelivery,Outbox,ReviewCredential,User).join(Outbox,Outbox.id==EventDelivery.event_id).join(ReviewCredential,ReviewCredential.id==EventDelivery.credential_id).join(User,User.id==ReviewCredential.user_id).where(EventDelivery.tenant_id==u.tenant_id)
  if state:q=q.where(EventDelivery.state==state)
  return [delivery_json(d,e,c,authorizer.active and not authorizer.must_change_password and authorizer.role=='sme' and authorizer.tenant_id==c.tenant_id) for d,e,c,authorizer in s.execute(q.order_by(Outbox.created.desc(),Outbox.revision.desc(),EventDelivery.id).limit(limit).offset(offset))]
 @app.post('/v1/review-api/events/claim')
 def claim(b:ClaimIn,p=Depends(service),s=Depends(db)):
  c,u=p;lock_credential(s,c);s.refresh(c)
  if not c.receive_events:raise HTTPException(403,'Event subscription is paused')
  now=int(time.time());q=select(EventDelivery).join(Outbox,Outbox.id==EventDelivery.event_id).where(EventDelivery.credential_id==c.id,EventDelivery.available_at<=now,or_(EventDelivery.state=='pending',and_(EventDelivery.state=='leased',EventDelivery.lease_expires<=now))).order_by(EventDelivery.available_at,Outbox.created,Outbox.revision,EventDelivery.id).limit(b.limit)
  claimed=[]
  for d in s.scalars(q):
   if d.cycle_attempts>=5:d.state='failed';d.lease_expires=None;d.last_error='Delivery lease expired after five attempts';continue
   raw=secrets.token_urlsafe(32);d.state='leased';d.attempts+=1;d.cycle_attempts+=1;d.lease_hash=hashlib.sha256(raw.encode()).hexdigest();d.lease_expires=now+b.lease_seconds;d.completion=None
   e=s.get(Outbox,d.event_id);claimed.append({**delivery_json(d,e,c),'lease_token':raw,'payload':e.payload})
  s.commit();return claimed
 @app.post('/v1/review-api/events/{id}/complete')
 def complete(id:str,b:CompleteIn,p=Depends(service),s=Depends(db)):
  c,u=p;lock_credential(s,c);s.refresh(c)
  if not c.receive_events:raise HTTPException(403,'Event subscription is paused')
  d=s.get(EventDelivery,id)
  if not d or d.credential_id!=c.id or d.tenant_id!=u.tenant_id:raise HTTPException(404,'Delivery not found')
  if not d.lease_hash or not secrets.compare_digest(d.lease_hash,hashlib.sha256(b.lease_token.encode()).hexdigest()):raise HTTPException(409,'Lease replaced or unavailable')
  completion={'result':b.result,'error':b.error}
  if d.completion==completion:return {**delivery_json(d,s.get(Outbox,d.event_id),c),'replayed':True}
  now=int(time.time())
  if d.state!='leased' or not d.lease_expires or d.lease_expires<=now:raise HTTPException(409,'Lease expired. Claim the event again.')
  d.completion=completion;d.lease_expires=None
  if b.result=='acknowledged':d.state='acknowledged';d.acknowledged_at=now;d.last_error=None
  else:
   d.last_error=b.error;d.state='failed' if d.cycle_attempts>=5 else 'pending';d.available_at=now+min(3600,15*2**d.cycle_attempts)
  log(s,u,'event_delivery_'+b.result,d.id);s.commit();return {**delivery_json(d,s.get(Outbox,d.event_id),c),'replayed':False}
 @app.post('/v1/event-deliveries/{id}/retry')
 def retry(id:str,b:RetryIn,u=Depends(current),s=Depends(db)):
  owner(u);d=s.get(EventDelivery,id)
  if not d or d.tenant_id!=u.tenant_id:raise HTTPException(404,'Delivery not found')
  c=s.get(ReviewCredential,d.credential_id);lock_credential(s,c);s.refresh(d)
  authorizer=s.get(User,c.user_id)
  if not authorizer or not authorizer.active or authorizer.must_change_password or authorizer.role!='sme' or authorizer.tenant_id!=u.tenant_id:raise HTTPException(409,'Authorizing service owner is unavailable')
  if d.state!='failed':raise HTTPException(409,'Manual retry applies only to failed delivery')
  if not c.receive_events:raise HTTPException(409,'Enable the credential event subscription first')
  if not s.execute(update(EventDelivery).where(EventDelivery.id==id,EventDelivery.state=='failed').values(state='pending',cycle_attempts=0,available_at=int(time.time()),lease_hash=None,lease_expires=None,completion=None,last_error='Manual retry: '+b.reason[:980])).rowcount:raise HTTPException(409,'Delivery changed; refresh before retrying')
  log(s,u,'event_delivery_manual_retry',id);s.commit();s.refresh(d);return delivery_json(d,s.get(Outbox,d.event_id),c)
