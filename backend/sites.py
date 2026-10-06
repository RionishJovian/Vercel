"""Tenant-scoped registration; user-reported router metadata is not telemetry."""
import time
from fastapi import Depends,HTTPException
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from .models import Site,RouterDevice,RouterPlan
from .schemas import SiteIn,RouterIn

def allowed_site(s,u,id):
 site=s.get(Site,id)
 if not site or site.tenant_id!=u.tenant_id:raise HTTPException(404,'Site not found')
 return site

def register_sites(app,current,db,owner,log):
 @app.get('/v1/sites')
 def sites(u=Depends(current),s=Depends(db)):
  if u.role=='guest':raise HTTPException(403,'Guest site discovery uses the published portal list')
  q=select(Site)
  if u.role!='platform':q=q.where(Site.tenant_id==u.tenant_id)
  return [{k:getattr(x,k) for k in ['id','tenant_id','name','timezone','created']} for x in s.scalars(q.order_by(Site.name))]
 @app.post('/v1/sites',status_code=201)
 def create_site(b:SiteIn,u=Depends(current),s=Depends(db)):
  owner(u);x=Site(tenant_id=u.tenant_id,**b.model_dump(),created=int(time.time()));s.add(x)
  try:s.flush();log(s,u,'site_registered',x.id);s.commit()
  except IntegrityError:s.rollback();raise HTTPException(409,'A site with this name already exists')
  return {'id':x.id,'name':x.name,'timezone':x.timezone}
 @app.get('/v1/routers')
 def routers(u=Depends(current),s=Depends(db)):
  if u.role not in ['sme','platform']:raise HTTPException(403,'Router registry permission required')
  q=select(RouterDevice)
  if u.role=='sme':q=q.where(RouterDevice.tenant_id==u.tenant_id)
  planned={p.router_id:p.hub_id for p in s.scalars(select(RouterPlan).where(RouterPlan.tenant_id==u.tenant_id) if u.role=='sme' else select(RouterPlan))}
  return [{k:getattr(x,k) for k in ['id','tenant_id','site_id','name','model','reported_routeros','created']}|{'connection_state':'unverified','hub_id':planned.get(x.id),'last_handshake':None,'metadata_source':'owner_reported'} for x in s.scalars(q.order_by(RouterDevice.created.desc()))]
 @app.post('/v1/routers',status_code=201)
 def register_router(b:RouterIn,u=Depends(current),s=Depends(db)):
  owner(u);allowed_site(s,u,b.site_id);x=RouterDevice(tenant_id=u.tenant_id,**b.model_dump(),registered_by=u.id,created=int(time.time()));s.add(x)
  try:s.flush();log(s,u,'router_registered',x.id);s.commit()
  except IntegrityError:s.rollback();raise HTTPException(409,'Router name already registered at this site')
  return {'id':x.id,'connection_state':'unverified','hub_id':None}
