"""Catalogue metadata and sales policies. No media files or playback authorization."""
import time
from typing import Literal
from pydantic import Field
from fastapi import Depends,HTTPException
from sqlalchemy import select,update
from .models import MediaTitle
from .schemas import Strict
class MediaIn(Strict):
 name:str=Field(min_length=1,max_length=120)
 description:str=Field(default='',max_length=4000)
 genre:str=Field(default='',max_length=80)
 runtime_minutes:int=Field(gt=0,le=1440)
 release_year:int|None=Field(default=None,ge=1888,le=2100)
class MediaUpdate(Strict):
 expected_revision:int=Field(gt=0)
 settings:MediaIn
class MediaState(Strict):
 expected_revision:int=Field(gt=0)
 operation:Literal['publish','unpublish','archive','restore']

def title_json(t):return {k:getattr(t,k) for k in ['id','name','description','genre','runtime_minutes','release_year','published','archived','revision','updated']}
def targeted(policy):return policy.get('service')=='media' and policy.get('mode') in ['title','hybrid']
def title_for(s,tenant,id,public=False):
 if not id:return None
 t=s.get(MediaTitle,id)
 if not t or t.tenant_id!=tenant or t.archived or (public and not t.published):return None
 return t
def package_snapshot(s,tenant,policy):
 if targeted(policy):
  t=title_for(s,tenant,policy['title'])
  if not t:raise HTTPException(404,'Choose an active title in your own catalogue')
  policy={**policy,'title_snapshot':{k:getattr(t,k) for k in ['id','name','runtime_minutes','release_year']}}
 return policy
def library_title(s,tenant):return s.scalar(select(MediaTitle).where(MediaTitle.tenant_id==tenant,MediaTitle.published==True,MediaTitle.archived==False).limit(1))
def media_offer_available(s,tenant,policy):
 if policy.get('service')=='media' and policy.get('mode')=='library':return bool(library_title(s,tenant))
 return not targeted(policy) or bool(title_for(s,tenant,policy.get('title'),public=True))
def ensure_media_offer(s,tenant,policy):
 candidate=title_for(s,tenant,policy.get('title'),public=True) if targeted(policy) else library_title(s,tenant) if policy.get('service')=='media' and policy.get('mode')=='library' else None
 if not targeted(policy) and not (policy.get('service')=='media' and policy.get('mode')=='library'):return
 if not candidate or not s.execute(update(MediaTitle).where(MediaTitle.id==candidate.id,MediaTitle.tenant_id==tenant,MediaTitle.published==True,MediaTitle.archived==False).values(revision=MediaTitle.revision)).rowcount:raise HTTPException(409,'Media offer is withdrawn or has no published titles. Ask the owner for an available package.')
def media_policy(p):
 if p.get('service') not in ['media','combined']:return None
 if p['service']=='combined':return {'mode':'library','library_hours':p.get('media_hours',24),'title_hours':None,'title':None,'activation':'awaiting_adapter','playback':'not_connected'}
 mode=p['mode'];return {'mode':mode,'library_hours':p['hours'] if mode in ['library','hybrid'] else None,'title_hours':p.get('title_hours',48) if mode=='hybrid' else p['hours'] if mode=='title' else None,'title':p.get('title_snapshot') or ({'id':p.get('title'),'name':p.get('title'),'legacy_unresolved':True} if mode in ['title','hybrid'] else None),'activation':'awaiting_adapter','playback':'not_connected'}

def register_media(app,current,db,owner,log):
 def own(s,u,id):
  owner(u);t=s.get(MediaTitle,id)
  if not t or t.tenant_id!=u.tenant_id:raise HTTPException(404,'Media title not found')
  return t
 @app.get('/v1/media/titles')
 def catalogue(u=Depends(current),s=Depends(db)):
  if u.role not in ['sme','guest']:raise HTTPException(403,'Business catalogue permission required')
  q=select(MediaTitle).where(MediaTitle.tenant_id==u.tenant_id)
  if u.role=='guest':q=q.where(MediaTitle.published==True,MediaTitle.archived==False)
  return [title_json(t) for t in s.scalars(q.order_by(MediaTitle.name))]
 @app.post('/v1/media/titles',status_code=201)
 def create(b:MediaIn,u=Depends(current),s=Depends(db)):
  owner(u);t=MediaTitle(tenant_id=u.tenant_id,**b.model_dump(),published=False,archived=False,revision=1,updated=int(time.time()));s.add(t);s.flush();log(s,u,'media_title_created',t.id);s.commit();return title_json(t)
 @app.post('/v1/media/titles/{id}')
 def edit(id:str,b:MediaUpdate,u=Depends(current),s=Depends(db)):
  own(s,u,id);changed=s.execute(update(MediaTitle).where(MediaTitle.id==id,MediaTitle.revision==b.expected_revision,MediaTitle.archived==False).values(**b.settings.model_dump(),published=False,revision=b.expected_revision+1,updated=int(time.time()))).rowcount
  if not changed:raise HTTPException(409,'Title changed or is archived. Refresh before editing.')
  log(s,u,'media_title_updated',id);s.commit();return title_json(s.get(MediaTitle,id))
 @app.post('/v1/media/titles/{id}/state')
 def state(id:str,b:MediaState,u=Depends(current),s=Depends(db)):
  t=own(s,u,id)
  if not t.archived and b.operation=='restore':raise HTTPException(409,'Title is not archived')
  if t.archived and b.operation!='restore':raise HTTPException(409,'Restore the archived title first')
  values={'revision':b.expected_revision+1,'updated':int(time.time())}
  if b.operation=='publish':values['published']=True
  elif b.operation=='unpublish':values['published']=False
  elif b.operation=='archive':values.update(archived=True,published=False)
  else:values.update(archived=False,published=False)
  changed=s.execute(update(MediaTitle).where(MediaTitle.id==id,MediaTitle.revision==b.expected_revision).values(**values)).rowcount
  if not changed:raise HTTPException(409,'Title changed. Refresh before changing publication.')
  log(s,u,'media_title_'+b.operation,id);s.commit();return title_json(s.get(MediaTitle,id))
