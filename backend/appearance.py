"""Platform-controlled palettes; role scopes affect effective account themes."""
import copy,time
from typing import Literal
from fastapi import Depends,HTTPException,Response
from pydantic import Field,field_validator
from sqlalchemy import update
from sqlalchemy.exc import IntegrityError
from .models import ThemeSettings
from .schemas import Strict
DEFAULT={'light':{'background':'#f0ece2','surface':'#fffaf0','text':'#16382b','primary':'#2d6554','accent':'#91b29d'},'dark':{'background':'#062320','surface':'#123c33','text':'#f3f3ed','primary':'#93b99e','accent':'#6e9e88'},'scopes':['guest','platform','sme']}
class Palette(Strict):
 background:str=Field(pattern=r'^#[0-9a-fA-F]{6}$')
 surface:str=Field(pattern=r'^#[0-9a-fA-F]{6}$')
 text:str=Field(pattern=r'^#[0-9a-fA-F]{6}$')
 primary:str=Field(pattern=r'^#[0-9a-fA-F]{6}$')
 accent:str=Field(pattern=r'^#[0-9a-fA-F]{6}$')
 @field_validator('*')
 @classmethod
 def lowercase(cls,v):return v.lower()
class ThemeConfig(Strict):
 light:Palette
 dark:Palette
 scopes:list[Literal['platform','sme','guest']]=Field(max_length=3)
 @field_validator('scopes')
 @classmethod
 def unique(cls,v):
  if len(set(v))!=len(v):raise ValueError('Choose each role once')
  return sorted(v)
class ThemeDraft(Strict):
 expected_revision:int=Field(ge=0)
 settings:ThemeConfig
class ThemeRevision(Strict):expected_revision:int=Field(gt=0)
def rgb(hex):return [int(hex[i:i+2],16)/255 for i in [1,3,5]]
def mix(a,b,f):return [x*f+y*(1-f) for x,y in zip(a,b)]
def luminance(rgb):return sum(w*(v/12.92 if v<=.04045 else ((v+.055)/1.055)**2.4) for w,v in zip([.2126,.7152,.0722],rgb))
def ratio(a,b):
 x,y=sorted([luminance(a),luminance(b)]);return (y+.05)/(x+.05)
def contrast_checks(settings):
 result=[]
 for mode in ['light','dark']:
  c={k:rgb(v) for k,v in settings[mode].items()};panel=mix(c['surface'],c['background'],.7);muted=mix(c['text'],c['background'],.7)
  pairs=[('Text / page',c['text'],c['background']),('Text / surface',c['text'],c['surface']),('Text / panel',c['text'],panel),('Primary / page and button',c['primary'],c['background']),('Primary / surface',c['primary'],c['surface']),('Muted / page',muted,c['background']),('Muted / surface',muted,c['surface']),('Muted / panel',muted,panel),('Selected navigation',c['text'],mix(c['accent'],c['background'],.3)),('Badge text',c['text'],mix(c['accent'],panel,.22))]
  result.extend({'mode':mode,'pair':name,'ratio':round(ratio(a,b),2),'passes':ratio(a,b)>=4.5} for name,a,b in pairs)
 return result

def state_json(x):
 draft=x.draft if x else copy.deepcopy(DEFAULT)
 return {'revision':x.revision if x else 0,'draft':draft,'published_revision':x.published_revision if x else None,'has_previous':bool(x and x.previous),'has_unpublished_changes':not x or not x.published or x.draft!=x.published,'checks':contrast_checks(draft)}
def effective(x,role):
 config=x.published if x and x.published else DEFAULT;scoped=role in config['scopes']
 return {'light':config['light'] if scoped else DEFAULT['light'],'dark':config['dark'] if scoped else DEFAULT['dark'],'source':'published' if scoped and x and x.published else 'default','published_revision':x.published_revision if x else None}

def register_appearance(app,current,db,log):
 def platform(u):
  if u.role!='platform':raise HTTPException(403,'Platform appearance permission required')
 @app.get('/v1/appearance/public')
 def public(response:Response,s=Depends(db)):
  response.headers['Cache-Control']='no-store';return effective(s.get(ThemeSettings,'platform'),'platform')
 @app.get('/v1/appearance/effective')
 def account(response:Response,u=Depends(current),s=Depends(db)):
  response.headers['Cache-Control']='no-store';return effective(s.get(ThemeSettings,'platform'),u.role)
 @app.get('/v1/appearance')
 def settings(u=Depends(current),s=Depends(db)):
  platform(u);return state_json(s.get(ThemeSettings,'platform'))
 @app.post('/v1/appearance/draft')
 def draft(b:ThemeDraft,u=Depends(current),s=Depends(db)):
  platform(u);x=s.get(ThemeSettings,'platform');now=int(time.time())
  if not x:
   if b.expected_revision!=0:raise HTTPException(409,'Appearance draft changed; refresh before saving')
   x=ThemeSettings(id='platform',revision=1,draft=b.settings.model_dump(),updated=now);s.add(x)
  else:
   changed=s.execute(update(ThemeSettings).where(ThemeSettings.id=='platform',ThemeSettings.revision==b.expected_revision).values(revision=b.expected_revision+1,draft=b.settings.model_dump(),updated=now)).rowcount
   if not changed:raise HTTPException(409,'Appearance draft changed; refresh before saving')
  try:s.flush();log(s,u,'appearance_draft_saved','platform');s.commit();s.refresh(x)
  except IntegrityError:s.rollback();raise HTTPException(409,'Appearance draft changed; refresh before saving')
  return state_json(x)
 @app.post('/v1/appearance/publish')
 def publish(b:ThemeRevision,u=Depends(current),s=Depends(db)):
  platform(u);x=s.get(ThemeSettings,'platform')
  if not x or x.revision!=b.expected_revision:raise HTTPException(409,'Save and refresh the current draft before publishing')
  if x.published and x.draft==x.published:raise HTTPException(409,'Draft matches the current publication; no palette change to publish')
  failed=[c for c in contrast_checks(x.draft) if not c['passes']]
  if failed:raise HTTPException(422,'Improve contrast before publication: '+', '.join(c['mode']+' '+c['pair'] for c in failed))
  changed=s.execute(update(ThemeSettings).where(ThemeSettings.id=='platform',ThemeSettings.revision==b.expected_revision).values(revision=b.expected_revision+1,published=x.draft,previous=x.published or copy.deepcopy(DEFAULT),published_revision=b.expected_revision+1,updated=int(time.time()))).rowcount
  if not changed:raise HTTPException(409,'Appearance changed; refresh before publishing')
  log(s,u,'appearance_published','platform');s.commit();s.refresh(x);return state_json(x)
 @app.post('/v1/appearance/restore')
 def restore(b:ThemeRevision,u=Depends(current),s=Depends(db)):
  platform(u);x=s.get(ThemeSettings,'platform')
  if not x or not x.previous:raise HTTPException(409,'No previous published palette')
  if any(not c['passes'] for c in contrast_checks(x.previous)):raise HTTPException(422,'Previous palette needs contrast correction')
  changed=s.execute(update(ThemeSettings).where(ThemeSettings.id=='platform',ThemeSettings.revision==b.expected_revision).values(revision=b.expected_revision+1,draft=x.previous,published=x.previous,previous=x.published,published_revision=b.expected_revision+1,updated=int(time.time()))).rowcount
  if not changed:raise HTTPException(409,'Appearance changed; refresh before restoring')
  log(s,u,'appearance_restored','platform');s.commit();s.refresh(x);return state_json(x)
