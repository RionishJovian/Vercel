"""Append-only manual cash events and timezone-aware SME reporting."""
import time
from datetime import date,datetime,time as day_time,timedelta
from zoneinfo import ZoneInfo,ZoneInfoNotFoundError
from fastapi import Depends,HTTPException,Query
from sqlalchemy import select,func
from .models import FinancialEvent,Order,Site

def record_event(s,*,u,source,record,site_id,kind='collection',reference=None,occurred_at=None):
 s.add(FinancialEvent(tenant_id=u.tenant_id,site_id=site_id,source=source,source_id=record.id,kind=kind,amount=record.amount,currency=record.currency,service=record.snapshot['service'],package_name=record.snapshot['name'],method=record.method,reference=reference if reference is not None else record.reference,actor_id=u.id,occurred_at=occurred_at if occurred_at is not None else int(time.time()),timestamp_basis='recorded_event'))

def register_revenue(app,current,db,owner):
 def filters(s,u,start,end,timezone,site_id):
  owner(u)
  try:tz=ZoneInfo(timezone)
  except (ZoneInfoNotFoundError,ValueError):raise HTTPException(422,'Choose a valid reporting timezone')
  if (start is None)!=(end is None):raise HTTPException(422,'Provide both start and end dates, or neither for all time')
  if start and (end<start or (end-start).days>365):raise HTTPException(422,'Date range must be ordered and no longer than 366 days')
  if site_id and site_id!='unassigned':
   site=s.get(Site,site_id)
   if not site or site.tenant_id!=u.tenant_id:raise HTTPException(404,'Site not found')
  lower=int(datetime.combine(start,day_time.min,tz).timestamp()) if start else None
  try:upper=int(datetime.combine(end+timedelta(days=1),day_time.min,tz).timestamp()) if end else None
  except (ValueError,OverflowError):raise HTTPException(422,'End date is outside supported range')
  def restrict(q,model,clock):
   q=q.where(model.tenant_id==u.tenant_id)
   if site_id:q=q.where(model.site_id.is_(None) if site_id=='unassigned' else model.site_id==site_id)
   if lower is not None:q=q.where(clock>=lower,clock<upper)
   return q
  return tz,restrict,{'start':start.isoformat() if start else None,'end':end.isoformat() if end else None,'timezone':timezone,'site_id':site_id or None,'date_basis':'financial_event_time','pending_date_basis':'order_submission_time'}
 def inputs(start:date|None=None,end:date|None=None,timezone:str=Query(default='Asia/Yangon',max_length=80),site_id:str|None=Query(default=None,max_length=36)):
  return start,end,timezone,site_id
 @app.get('/v1/revenue')
 def revenue(params=Depends(inputs),u=Depends(current),s=Depends(db)):
  tz,restrict,applied=filters(s,u,*params);events=list(s.scalars(restrict(select(FinancialEvent),FinancialEvent,FinancialEvent.occurred_at)));pending=list(s.scalars(restrict(select(Order).where(Order.status=='pending'),Order,Order.created)))
  collect=[e for e in events if e.kind=='collection'];refund=[e for e in events if e.kind=='refund'];gross=sum(e.amount for e in collect);returned=sum(e.amount for e in refund);net=gross-returned
  def signed(e):return e.amount if e.kind=='collection' else -e.amount
  def net_source(source):return sum(signed(e) for e in events if e.source==source)
  def count_source(source):return max(0,sum(1 if e.kind=='collection' else -1 for e in events if e.source==source))
  days={};methods={};sites={}
  site_names={x.id:x.name for x in s.scalars(select(Site).where(Site.tenant_id==u.tenant_id))}
  for e in events:
   day=datetime.fromtimestamp(e.occurred_at,tz).date().isoformat()
   for group,key in [(days,day),(methods,e.method),(sites,e.site_id or 'unassigned')]:
    bucket=group.setdefault(key,{'gross_collections':0,'refunds':0,'net_collections':0,'collection_count':0,'refund_count':0})
    bucket['gross_collections' if e.kind=='collection' else 'refunds']+=e.amount;bucket['collection_count' if e.kind=='collection' else 'refund_count']+=1;bucket['net_collections']+=signed(e)
  return {'currency':'MMK','filters':applied,'net_collections':net,'gross_collections':gross,'refunds':returned,'collection_count':len(collect),'refund_count':len(refund),'average_receipt':round(gross/len(collect),2) if collect else 0,'approved_orders':count_source('guest_order'),'confirmed_payments':max(0,len(collect)-len(refund)),'voucher_sales':count_source('voucher_sale'),'pending_orders':len(pending),'pending_amount':sum(o.amount for o in pending),'pending_over_24h':sum(o.created<int(time.time())-86400 for o in pending),'estimated_timestamp_events':sum(e.timestamp_basis=='legacy_submission_estimate' for e in events),'by_source':{'guest_orders':net_source('guest_order'),'voucher_counter_sales':net_source('voucher_sale')},'by_service':{k:sum(signed(e) for e in events if e.service==k) for k in sorted({'internet','media','combined'}|{e.service for e in events})},'by_day':[{'date':day,**days[day]} for day in sorted(days)],'by_method':[{'method':method,**methods[method]} for method in sorted(methods)],'by_site':[{'site_id':id if id!='unassigned' else None,'site_name':site_names.get(id,'Business-wide / unassigned'),**sites[id]} for id in sorted(sites)]}
 @app.get('/v1/revenue/ledger')
 def ledger(params=Depends(inputs),limit:int=Query(default=100,ge=1,le=10000),offset:int=Query(default=0,ge=0,le=1000000),u=Depends(current),s=Depends(db)):
  tz,restrict,applied=filters(s,u,*params);q=restrict(select(FinancialEvent),FinancialEvent,FinancialEvent.occurred_at);total=s.scalar(select(func.count()).select_from(q.subquery()));rows=list(s.scalars(q.order_by(FinancialEvent.occurred_at.desc(),FinancialEvent.id.desc()).limit(limit+1).offset(offset)))
  has_more=len(rows)>limit;rows=rows[:limit]
  names={x.id:x.name for x in s.scalars(select(Site).where(Site.tenant_id==u.tenant_id))}
  items=[{k:getattr(e,k) for k in ['id','site_id','source','source_id','kind','amount','currency','service','package_name','method','reference','actor_id','occurred_at','timestamp_basis']}|{'site_name':names.get(e.site_id,'Business-wide / unassigned'),'signed_amount':e.amount if e.kind=='collection' else -e.amount,'local_time':datetime.fromtimestamp(e.occurred_at,tz).isoformat()} for e in rows]
  return {'filters':applied,'total':total,'offset':offset,'limit':limit,'has_more':has_more,'items':items}
