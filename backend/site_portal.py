import time
from fastapi import Depends,HTTPException
from sqlalchemy import select,update
from sqlalchemy.exc import IntegrityError
from .models import SitePortal,Site,PortalSettings
from .schemas import PortalDraftIn,PortalPublishIn,PortalConfig
from .sites import allowed_site
from .portal import portal_state,public_config

def selected_portal(s,u,site_id=None):
 if site_id:
  allowed_site(s,u,site_id);return s.get(SitePortal,site_id)
 return s.get(PortalSettings,u.tenant_id)

def register_site_portals(app,current,db,owner,log):
 @app.get('/v1/guest-sites')
 def guest_sites(u=Depends(current),s=Depends(db)):
  if u.role not in ['guest','sme']:raise HTTPException(403,'Business context required')
  return [{'id':site.id,'name':site.name} for site,portal in s.execute(select(Site,SitePortal).join(SitePortal,SitePortal.site_id==Site.id).where(Site.tenant_id==u.tenant_id,SitePortal.published_revision.is_not(None)))]
 @app.get('/v1/sites/{id}/portal')
 def get_portal(id:str,u=Depends(current),s=Depends(db)):
  owner(u);site=allowed_site(s,u,id);return portal_state(s.get(SitePortal,id),site.name)|{'site_id':id}
 @app.post('/v1/sites/{id}/portal/draft')
 def draft(id:str,b:PortalDraftIn,u=Depends(current),s=Depends(db)):
  owner(u);allowed_site(s,u,id);row=s.get(SitePortal,id);now=int(time.time())
  if row:
   changed=s.execute(update(SitePortal).where(SitePortal.site_id==id,SitePortal.revision==b.expected_revision).values(draft=b.settings.model_dump(),revision=b.expected_revision+1,updated=now))
   if not changed.rowcount:raise HTTPException(409,'Site portal changed. Refresh before editing again.')
  else:
   if b.expected_revision!=0:raise HTTPException(409,'Site portal revision does not match')
   s.add(SitePortal(site_id=id,revision=1,draft=b.settings.model_dump(),updated=now))
  try:s.flush();log(s,u,'site_portal_draft_saved',id);s.commit()
  except IntegrityError:s.rollback();raise HTTPException(409,'Site portal changed. Refresh and review.')
  s.expire_all();return portal_state(s.get(SitePortal,id))|{'site_id':id}
 @app.post('/v1/sites/{id}/portal/publish')
 def publish(id:str,b:PortalPublishIn,u=Depends(current),s=Depends(db)):
  owner(u);allowed_site(s,u,id);row=s.get(SitePortal,id)
  if not row or row.revision!=b.expected_revision:raise HTTPException(409,'Site portal changed. Refresh and review.')
  cfg=PortalConfig.model_validate(row.draft)
  if cfg.direct_purchase and (not cfg.payment_destination or not cfg.payment_instructions or not cfg.payment_methods):raise HTTPException(422,'Direct purchases need destination, instructions and a method')
  revision=b.expected_revision+1
  changed=s.execute(update(SitePortal).where(SitePortal.site_id==id,SitePortal.revision==b.expected_revision).values(published=cfg.model_dump(),published_revision=revision,revision=revision,updated=int(time.time())))
  if not changed.rowcount:raise HTTPException(409,'Site portal changed. Refresh and review.')
  log(s,u,'site_portal_published',id);s.commit();s.expire_all();return portal_state(s.get(SitePortal,id))|{'site_id':id}
