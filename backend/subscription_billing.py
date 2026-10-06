"""Manual payment review for SME-to-platform subscriptions, distinct from guest sales."""
import json,time,hashlib
from typing import Literal
from pydantic import Field,model_validator
from fastapi import Depends,HTTPException
from sqlalchemy import select,update
from sqlalchemy.exc import IntegrityError
from .schemas import Strict
from .models import SubscriptionInvoice,BillingEvent,SubscriptionPlan,TenantSubscription,Tenant,StaffGrant
from .subscriptions import plan_json,subscription_json
def fingerprint(data):return hashlib.sha256(json.dumps(data,sort_keys=True).encode()).hexdigest()
class InvoiceIn(Strict):
 tenant_id:str
 plan_id:str
 subscription_revision:int=Field(ge=0)
 period_start:int=Field(ge=0)
 period_end:int=Field(gt=0)
 destination:str=Field(min_length=1,max_length=500)
 instructions:str=Field(min_length=1,max_length=2000)
 request_key:str=Field(min_length=16,max_length=80)
 @model_validator(mode='after')
 def period(self):
  if self.period_end<=self.period_start:raise ValueError('End must follow start')
  return self
class SubmissionIn(Strict):
 expected_revision:int=Field(ge=1)
 method:str=Field(min_length=1,max_length=80)
 reference:str=Field(min_length=1,max_length=160)
 evidence:str=Field(min_length=1,max_length=4000)
 request_key:str=Field(min_length=16,max_length=80)
class ReviewIn(Strict):
 expected_revision:int=Field(ge=1)
 decision:Literal['approved','needs_evidence','rejected','void']
 reason:str=Field(min_length=1,max_length=1000)
 confirmed_received:bool=False
def invoice_json(v):
 return {k:getattr(v,k) for k in ['id','tenant_id','plan_id','snapshot','amount','currency','period_start','period_end','subscription_revision','revision','status','destination','instructions','method','reference','evidence','reviewed_by','reason','created','updated']}|{'settlement_verified':False,'automatic_renewal':False}
def register_subscription_billing(app,current,db,log):
 def platform(u):
  if u.role!='platform':raise HTTPException(403,'Platform operator required')
 def owner(s,u):
  if u.role!='sme' or s.get(StaffGrant,u.id):raise HTTPException(403,'Business owner required')
 def scoped(s,u,id):
  v=s.get(SubscriptionInvoice,id)
  if not v:raise HTTPException(404,'Invoice not found')
  if u.role!='platform':
   owner(s,u)
   if v.tenant_id!=u.tenant_id:raise HTTPException(404,'Invoice not found')
  return v
 def history(s,u,v,event,payload):
  s.add(BillingEvent(invoice_id=v.id,actor_id=u.id,event=event,payload=payload,created=int(time.time())));log(s,u,'subscription_'+event,v.id)
 @app.get('/v1/subscription/invoices')
 def listing(u=Depends(current),s=Depends(db)):
  q=select(SubscriptionInvoice)
  if u.role!='platform':owner(s,u);q=q.where(SubscriptionInvoice.tenant_id==u.tenant_id)
  return [invoice_json(v) for v in s.scalars(q.order_by(SubscriptionInvoice.created.desc(),SubscriptionInvoice.id).limit(200))]
 @app.get('/v1/subscription/invoices/{id}/history')
 def events(id:str,u=Depends(current),s=Depends(db)):
  v=scoped(s,u,id);return [{'id':e.id,'actor_id':e.actor_id,'event':e.event,'payload':e.payload,'created':e.created} for e in s.scalars(select(BillingEvent).where(BillingEvent.invoice_id==v.id).order_by(BillingEvent.created,BillingEvent.id))]
 @app.post('/v1/platform/subscription-invoices',status_code=201)
 def create(b:InvoiceIn,u=Depends(current),s=Depends(db)):
  platform(u);key=fingerprint(b.model_dump(exclude={'request_key'}));previous=s.scalar(select(SubscriptionInvoice).where(SubscriptionInvoice.created_by==u.id,SubscriptionInvoice.request_key==b.request_key))
  if previous:
   if previous.request_hash!=key:raise HTTPException(409,'Request key reused for another invoice')
   return invoice_json(previous)|{'replayed':True}
  p=s.get(SubscriptionPlan,b.plan_id);t=s.get(Tenant,b.tenant_id);sub=s.get(TenantSubscription,b.tenant_id)
  if not p or not t:raise HTTPException(404,'Business or plan not found')
  if p.settings['monthly_price']<=0:raise HTTPException(422,'Zero-price plans use manual assignment rather than payment invoices')
  if (sub.revision if sub else 0)!=b.subscription_revision:raise HTTPException(409,'Subscription changed; review its current period first')
  now=int(time.time());v=SubscriptionInvoice(tenant_id=t.id,plan_id=p.id,snapshot=plan_json(p)|{'business_name':t.name},amount=p.settings['monthly_price'],currency=p.settings['currency'],period_start=b.period_start,period_end=b.period_end,subscription_revision=b.subscription_revision,revision=1,status='unpaid',destination=b.destination,instructions=b.instructions,created_by=u.id,request_key=b.request_key,request_hash=key,created=now,updated=now);s.add(v)
  try:s.flush();history(s,u,v,'invoice_issued',invoice_json(v));s.commit()
  except IntegrityError:s.rollback();raise HTTPException(409,'Invoice creation raced; retry the same request key')
  return invoice_json(v)|{'replayed':False}
 @app.post('/v1/subscription/invoices/{id}/payment')
 def submit(id:str,b:SubmissionIn,u=Depends(current),s=Depends(db)):
  owner(s,u);v=scoped(s,u,id);key=fingerprint(b.model_dump(exclude={'expected_revision','request_key'}))
  prior=s.scalar(select(BillingEvent).where(BillingEvent.invoice_id==id,BillingEvent.event=='payment_submitted',BillingEvent.payload['request_key'].as_string()==b.request_key))
  if prior:
   if prior.payload['fingerprint']!=key:raise HTTPException(409,'Submission key reused with different payment evidence')
   return invoice_json(v)|{'replayed':True}
  if v.submission_key==b.request_key:
   if v.submission_hash!=key:raise HTTPException(409,'Submission key reused with different payment evidence')
   return invoice_json(v)|{'replayed':True}
  values={k:getattr(b,k) for k in ['method','reference','evidence']};values.update(status='pending',revision=b.expected_revision+1,submission_key=b.request_key,submission_hash=key,updated=int(time.time()),reviewed_by=None,reason=None)
  if not s.execute(update(SubscriptionInvoice).where(SubscriptionInvoice.id==id,SubscriptionInvoice.revision==b.expected_revision,SubscriptionInvoice.status.in_(['unpaid','needs_evidence'])).values(**values)).rowcount:raise HTTPException(409,'Invoice changed or is not accepting evidence; refresh')
  history(s,u,v,'payment_submitted',b.model_dump()|{'fingerprint':key});s.commit();s.refresh(v);return invoice_json(v)|{'replayed':False}
 @app.post('/v1/platform/subscription-invoices/{id}/decision')
 def decide(id:str,b:ReviewIn,u=Depends(current),s=Depends(db)):
  platform(u);v=scoped(s,u,id);status='paid' if b.decision=='approved' else b.decision
  if b.decision=='approved' and not b.confirmed_received:raise HTTPException(422,'Confirm receipt independently before approving')
  if v.revision==b.expected_revision+1 and v.status==status and v.reviewed_by==u.id and v.reason==b.reason:return invoice_json(v)|{'replayed':True}
  allowed=['unpaid','pending','needs_evidence','rejected'] if status=='void' else ['pending']
  if not s.execute(update(SubscriptionInvoice).where(SubscriptionInvoice.id==id,SubscriptionInvoice.revision==b.expected_revision,SubscriptionInvoice.status.in_(allowed)).values(status=status,revision=b.expected_revision+1,reviewed_by=u.id,reason=b.reason,updated=int(time.time()))).rowcount:raise HTTPException(409,'Invoice changed or already reviewed; refresh')
  if status=='paid':
   sub=s.get(TenantSubscription,v.tenant_id)
   if (sub.revision if sub else 0)!=v.subscription_revision:raise HTTPException(409,'Subscription changed since invoice issuance; resolve before approving')
   if sub and sub.state=='active' and v.period_start<sub.period_end:raise HTTPException(409,'Renewal overlaps an active period; resolve before approving')
   values={'revision':v.subscription_revision+1,'plan_id':v.plan_id,'plan_snapshot':v.snapshot,'state':'active','period_start':v.period_start,'period_end':v.period_end,'updated':int(time.time()),'reason':'Approved invoice '+v.id,'payment_source_invoice_id':v.id}
   if sub:
    if not s.execute(update(TenantSubscription).where(TenantSubscription.tenant_id==v.tenant_id,TenantSubscription.revision==v.subscription_revision).values(**values)).rowcount:raise HTTPException(409,'Subscription changed; refresh')
   else:s.add(TenantSubscription(tenant_id=v.tenant_id,**values))
  history(s,u,v,'invoice_'+status,b.model_dump())
  try:s.commit()
  except IntegrityError:s.rollback();raise HTTPException(409,'Subscription changed concurrently; refresh')
  s.refresh(v);return invoice_json(v)|{'replayed':False}
