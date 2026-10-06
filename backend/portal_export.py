"""Reviewable portal handoff, deliberately without executable router commands."""
import io,json,zipfile
from html import escape
from urllib.parse import urlsplit
from fastapi import Depends,HTTPException,Response
from pydantic import Field,field_validator
from .schemas import Strict,PortalConfig
from .models import PortalSettings,SitePortal,Tenant
from .sites import allowed_site
from .portal import public_config

class ExportRequest(Strict):
 expected_revision:int=Field(gt=0)
 public_origin:str=Field(max_length=300)
 @field_validator('public_origin')
 @classmethod
 def origin(cls,value):
  parsed=urlsplit(value)
  if parsed.scheme!='https' or not parsed.hostname or parsed.username or parsed.password or parsed.path not in ('','/') or parsed.query or parsed.fragment:
   raise ValueError('Use an HTTPS webapp origin without credentials, path, query or fragment')
  try:parsed.port
  except ValueError:raise ValueError('Invalid port')
  from .schemas import host_value
  host_value(parsed.hostname)
  return value.rstrip('/')

def export_bundle(row,name,site_id,origin):
 cfg=PortalConfig.model_validate(row.published)
 guest_url=origin+'/?guest_site='+site_id if site_id else None
 plan={'format':'mikromagic.portal-review.v1','scope':{'site_id':site_id,'business_name':name},'saved_revision':row.revision,'published_revision':row.published_revision,'guest_url':guest_url,'public_content':public_config(row.published),'network_draft':{k:getattr(cfg,k) for k in ['hostname','guest_interface','guest_subnet','tls_required','isolate_guests','allowed_hosts']},'webapp_host':urlsplit(origin).hostname,'required_review':['Confirm HTTPS certificate and all portal asset/API hosts','Review allowed hosts: no wildcards, external CDNs or unprotected stream endpoints','Verify DNS, IPv4 and IPv6 guest isolation and unauthorized-path blocking','Select one quota/accounting authority and activation clock per site','Back up router configuration; review diff and rollback before any application'],'router_apply_state':'not_connected','authentication_state':'cloud_accounts_only','media_enforcement':'not_connected'}
 body=f'<h1>{escape(cfg.business_name)}</h1><p>{escape(cfg.welcome)}</p><p>{escape(cfg.contact)}</p><h2>Terms</h2><p>{escape(cfg.terms)}</p>'
 body+=f'<a href="{escape(guest_url,quote=True)}">Open published guest portal</a>' if guest_url else '<p>Business-wide anonymous guest entry is not available. Publish a site portal for a guest link.</p>'
 body+='<p>Preview/handoff only. No router login, RADIUS credentials, media unlock or firewall rules are included.</p>'
 preview='<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Portal handoff preview</title><style>body{background:#f0ece2;color:#16382b;font:18px system-ui;padding:32px;max-width:780px;margin:auto}a{color:#2d6554}p{white-space:pre-wrap}</style><body>'+body+'</body></html>'
 notes='MikroMagic portal review bundle\n\nThis is NOT a RouterOS installation bundle. Do not upload preview.html as a HotSpot login page.\n\nportal-plan.json contains published guest content plus the CURRENT SAVED network draft. Network settings are not validated against hardware and may differ from the publication revision.\n\nThe webapp origin is supplied by the owner, not proof of a deployed or reachable service. No network requests are made during export.\n\nLocal media should expose only the catalogue pre-login, never unprotected Jellyfin playback. Payment destinations are instructions, not verified settlement accounts.\n\n'+ '\n'.join('- '+x for x in plan['required_review'])+'\n'
 output=io.BytesIO()
 # Guest publication and private saved network draft have independent lifecycles.
 draft=PortalConfig.model_validate(row.draft)
 plan['network_draft']={k:getattr(draft,k) for k in plan['network_draft']}
 with zipfile.ZipFile(output,'w',zipfile.ZIP_DEFLATED) as z:
  z.writestr('portal-plan.json',json.dumps(plan,ensure_ascii=False,indent=2));z.writestr('preview.html',preview);z.writestr('README.txt',notes)
 return output.getvalue()

def register_portal_exports(app,current,db,owner,log):
 def export(b,u,s,site_id=None):
  owner(u)
  site=allowed_site(s,u,site_id) if site_id else None
  row=s.get(SitePortal,site_id) if site_id else s.get(PortalSettings,u.tenant_id)
  if not row or row.revision!=b.expected_revision:raise HTTPException(409,'Portal changed; refresh before exporting')
  if not row.published or row.published_revision is None:raise HTTPException(409,'Publish guest content before exporting')
  payload=export_bundle(row,site.name if site else s.get(Tenant,u.tenant_id).name,site_id,b.public_origin)
  log(s,u,'portal_review_exported',site_id or u.tenant_id);s.commit()
  return Response(payload,media_type='application/zip',headers={'Content-Disposition':'attachment; filename="MikroMagic-portal-review.zip"'})
 @app.post('/v1/portal/export')
 def business_export(b:ExportRequest,u=Depends(current),s=Depends(db)):return export(b,u,s)
 @app.post('/v1/sites/{id}/portal/export')
 def site_export(id:str,b:ExportRequest,u=Depends(current),s=Depends(db)):return export(b,u,s,id)
