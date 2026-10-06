"""Tenant portal drafts and guest-facing publication; never applies router rules."""
import time
from fastapi import Depends,HTTPException
from sqlalchemy import update
from sqlalchemy.exc import IntegrityError
from .models import PortalSettings,Tenant
from .schemas import PortalConfig,PortalDraftIn,PortalPublishIn
PUBLIC_FIELDS=('business_name','welcome','language','terms','contact','direct_purchase','voucher_login','payment_destination','payment_instructions','payment_methods')
def public_config(settings):return {k:settings[k] for k in PUBLIC_FIELDS}
def portal_state(row,name='My business'):
 return {'revision':row.revision if row else 0,'draft':row.draft if row else PortalConfig(business_name=name).model_dump(),'published':public_config(row.published) if row and row.published else None,'published_revision':row.published_revision if row else None,'router_apply_state':'not_connected'}
def register_portal(app,current,db,owner,log):
 @app.get('/v1/portal')
 def get_portal(u=Depends(current),s=Depends(db)):
  owner(u);return portal_state(s.get(PortalSettings,u.tenant_id),s.get(Tenant,u.tenant_id).name)
 @app.get('/v1/portal/public')
 def guest_portal(site_id:str|None=None,u=Depends(current),s=Depends(db)):
  if u.role not in ['guest','sme']:raise HTTPException(403,'Business context required')
  from .site_portal import selected_portal
  row=selected_portal(s,u,site_id)
  return {'site_id':site_id,'revision':row.published_revision if row else None,'settings':public_config(row.published) if row and row.published else None,'network_access':'not_connected','voucher_redemption':'stored_pass_only'}
 @app.post('/v1/portal/draft')
 def save_draft(b:PortalDraftIn,u=Depends(current),s=Depends(db)):
  owner(u);now=int(time.time());row=s.get(PortalSettings,u.tenant_id)
  if row:
   result=s.execute(update(PortalSettings).where(PortalSettings.tenant_id==u.tenant_id,PortalSettings.revision==b.expected_revision).values(draft=b.settings.model_dump(),revision=b.expected_revision+1,updated=now))
   if result.rowcount!=1:raise HTTPException(409,'Portal changed in another session. Refresh before editing again.')
  else:
   if b.expected_revision!=0:raise HTTPException(409,'Portal revision does not match. Refresh before editing again.')
   s.add(PortalSettings(tenant_id=u.tenant_id,revision=1,draft=b.settings.model_dump(),updated=now))
  try:
   s.flush();log(s,u,'portal_draft_saved',str(b.expected_revision+1));s.commit()
  except IntegrityError:
   s.rollback();raise HTTPException(409,'Portal changed in another session. Refresh before editing again.')
  s.expire_all();return portal_state(s.get(PortalSettings,u.tenant_id))
 @app.post('/v1/portal/publish')
 def publish(b:PortalPublishIn,u=Depends(current),s=Depends(db)):
  owner(u);row=s.get(PortalSettings,u.tenant_id)
  if not row or row.revision!=b.expected_revision:raise HTTPException(409,'Portal changed. Refresh and review the latest draft.')
  cfg=PortalConfig.model_validate(row.draft)
  if cfg.direct_purchase and (not cfg.payment_destination or not cfg.payment_methods or not cfg.payment_instructions):raise HTTPException(422,'Direct purchases need a payment destination, instructions and at least one method.')
  new_revision=b.expected_revision+1
  result=s.execute(update(PortalSettings).where(PortalSettings.tenant_id==u.tenant_id,PortalSettings.revision==b.expected_revision).values(published=cfg.model_dump(),published_revision=new_revision,revision=new_revision,updated=int(time.time())))
  if result.rowcount!=1:raise HTTPException(409,'Portal changed. Refresh and review the latest draft.')
  log(s,u,'portal_published',str(new_revision));s.commit();s.expire_all();return portal_state(s.get(PortalSettings,u.tenant_id))
