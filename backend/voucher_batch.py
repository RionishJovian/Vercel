"""Atomic batch issuance; full codes are never stored and appear once."""
import base64,secrets,time
from fastapi import Depends,HTTPException
from pydantic import Field
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from .schemas import VoucherIssue
from .models import Voucher,Package
from .vouchers import digest,voucher_json
from .sites import allowed_site
from .media import ensure_media_offer
class BatchIn(VoucherIssue):
 count:int=Field(ge=1,le=100)
def register_voucher_batch(app,current,db,owner,log):
 @app.post('/v1/vouchers/batch',status_code=201)
 def issue(b:BatchIn,u=Depends(current),s=Depends(db)):
  owner(u);prefix='batch:'+digest(b.request_key)[:40]+':'
  previous=list(s.scalars(select(Voucher).where(Voucher.issued_by==u.id,Voucher.request_key.like(prefix+'%')).order_by(Voucher.request_key)))
  if previous:
   if len(previous)!=b.count or any(v.site_id!=b.site_id or v.package_id!=b.package_id or v.expires-v.created!=b.expires_days*86400 for v in previous):raise HTTPException(409,'Batch request key reused with different settings')
   return {'replayed':True,'items':[voucher_json(v)|{'code':None} for v in previous],'message':'Already issued. Codes are shown only once; revoke lost codes before issuing replacements.'}
  if b.site_id:allowed_site(s,u,b.site_id)
  package=s.get(Package,b.package_id)
  if not package or package.tenant_id!=u.tenant_id or not package.active:raise HTTPException(404,'Package not available')
  ensure_media_offer(s,u.tenant_id,package.policy);now=int(time.time());items=[]
  for i in range(b.count):
   raw=base64.b32encode(secrets.token_bytes(16)).decode().rstrip('=')
   v=Voucher(site_id=b.site_id,tenant_id=u.tenant_id,package_id=package.id,policy=package.policy.copy(),code_hash=digest(raw),code_hint=raw[-4:],issued_by=u.id,request_key=prefix+str(i).zfill(3),created=now,expires=now+b.expires_days*86400);s.add(v);items.append((v,'-'.join(raw[j:j+5] for j in range(0,len(raw),5))))
  try:
   s.flush()
   for v,_ in items:log(s,u,'voucher_batch_issued',v.id)
   s.commit()
  except IntegrityError:s.rollback();raise HTTPException(409,'Batch issuance raced; retry the same request key')
  return {'replayed':False,'items':[voucher_json(v)|{'code':code} for v,code in items]}
