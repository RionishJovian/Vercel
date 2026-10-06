"""Global platform-authored guidance, never device capability evidence."""
import time
from datetime import date
from typing import Literal
from urllib.parse import urlsplit
from fastapi import Depends,HTTPException
from pydantic import Field,field_validator
from sqlalchemy import select,update
from .schemas import Strict
from .models import KnowledgeCard
class CardIn(Strict):
 title:str=Field(min_length=1,max_length=120)
 category:Literal['MikroTik','Internet access','Media','Business']
 fact:str=Field(min_length=1,max_length=2000)
 why:str=Field(min_length=1,max_length=2000)
 requirements:str=Field(min_length=1,max_length=2000)
 action:Literal['Routers','Captive portal','Packages','Revenue','Media']
 source_url:str=Field(min_length=1,max_length=1000)
 verified_on:date
 @field_validator('title','fact','why','requirements')
 @classmethod
 def clean(cls,v):
  if not v.strip():raise ValueError('Text must not be blank')
  return v.strip()
 @field_validator('source_url')
 @classmethod
 def safe_url(cls,v):
  u=urlsplit(v)
  if u.scheme!='https' or not u.hostname or u.username or u.password or any(c.isspace() for c in v):raise ValueError('Use a public HTTPS source URL without credentials')
  return v
 @field_validator('verified_on')
 @classmethod
 def checked_date(cls,v):
  if v>date.today():raise ValueError('Verification date cannot be in the future')
  return v
class CardEdit(Strict):
 expected_revision:int=Field(gt=0)
 settings:CardIn
class CardState(Strict):
 expected_revision:int=Field(gt=0)
 operation:Literal['publish','unpublish','archive','restore']
def card_json(t):return {'id':t.id,**t.content,'published':t.published,'archived':t.archived,'revision':t.revision,'updated':t.updated,'compatibility':'not_checked'}
def register_knowledge(app,current,db,log):
 def platform(u):
  if u.role!='platform':raise HTTPException(403,'Platform publisher permission required')
 def own(s,u,id):
  platform(u);t=s.get(KnowledgeCard,id)
  if not t:raise HTTPException(404,'Guidance card not found')
  return t
 @app.get('/v1/knowledge')
 def list_cards(u=Depends(current),s=Depends(db)):
  if u.role not in ['platform','sme']:raise HTTPException(403,'Business guidance permission required')
  q=select(KnowledgeCard)
  if u.role=='sme':q=q.where(KnowledgeCard.published==True,KnowledgeCard.archived==False)
  return [card_json(t) for t in s.scalars(q.order_by(KnowledgeCard.updated.desc(),KnowledgeCard.id))]
 @app.post('/v1/knowledge',status_code=201)
 def create(b:CardIn,u=Depends(current),s=Depends(db)):
  platform(u);t=KnowledgeCard(content=b.model_dump(mode='json'),published=False,archived=False,revision=1,updated=int(time.time()));s.add(t);s.flush();log(s,u,'guidance_created',t.id);s.commit();return card_json(t)
 @app.post('/v1/knowledge/{id}')
 def edit(id:str,b:CardEdit,u=Depends(current),s=Depends(db)):
  own(s,u,id)
  changed=s.execute(update(KnowledgeCard).where(KnowledgeCard.id==id,KnowledgeCard.revision==b.expected_revision,KnowledgeCard.archived==False).values(content=b.settings.model_dump(mode='json'),published=False,revision=b.expected_revision+1,updated=int(time.time()))).rowcount
  if not changed:raise HTTPException(409,'Card changed or is archived. Refresh before editing.')
  log(s,u,'guidance_updated',id);s.commit();return card_json(s.get(KnowledgeCard,id))
 @app.post('/v1/knowledge/{id}/state')
 def state(id:str,b:CardState,u=Depends(current),s=Depends(db)):
  t=own(s,u,id)
  if (t.archived and b.operation!='restore') or (not t.archived and b.operation=='restore'):raise HTTPException(409,'Restore applies only to archived cards; restore before publishing.')
  if b.operation=='publish':CardIn.model_validate(t.content)
  values={'revision':b.expected_revision+1,'updated':int(time.time())}
  if b.operation=='publish':values['published']=True
  elif b.operation=='unpublish':values['published']=False
  elif b.operation=='archive':values.update(archived=True,published=False)
  else:values.update(archived=False,published=False)
  if not s.execute(update(KnowledgeCard).where(KnowledgeCard.id==id,KnowledgeCard.revision==b.expected_revision).values(**values)).rowcount:raise HTTPException(409,'Card changed. Refresh before changing publication.')
  log(s,u,'guidance_'+b.operation,id);s.commit();return card_json(s.get(KnowledgeCard,id))
