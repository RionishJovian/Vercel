"""Owner-authored WAN and ISP data-budget plans, with no router execution."""
import time
from datetime import date
from typing import Literal
from pydantic import Field,model_validator
from fastapi import Depends,HTTPException
from sqlalchemy import update
from sqlalchemy.exc import IntegrityError
from .models import SitePlanning
from .schemas import Strict
from .sites import allowed_site
class Uplink(Strict):
 name:str=Field(min_length=1,max_length=80)
 provider:str=Field(default='Other',max_length=80)
 enabled:bool=True
 priority:int=Field(default=1,ge=1,le=100)
 weight:int=Field(default=1,ge=1,le=100)
class Budget(Strict):
 enabled:bool=False
 limit_gb:float=Field(default=1000,gt=0,le=10000000,allow_inf_nan=False)
 used_gb:float|None=Field(default=None,ge=0,le=100000000,allow_inf_nan=False)
 cycle_start:date|None=None
 cycle_end:date|None=None
 reading_date:date|None=None
 warning_percent:int=Field(default=80,ge=1,le=99)
 exhaustion_action:Literal['notify','throttle','block']='notify'
 @model_validator(mode='after')
 def valid_cycle(self):
  if self.enabled or self.used_gb is not None:
   if not self.cycle_start or not self.cycle_end or not 1<=(self.cycle_end-self.cycle_start).days<=366:raise ValueError('Provide an ordered budget cycle of 1–366 days; end is exclusive')
  if self.used_gb is not None and (not self.reading_date or not self.cycle_start<=self.reading_date<self.cycle_end):raise ValueError('Manual usage needs a reading date inside this cycle')
  return self
class Node(Strict):
 id:str=Field(min_length=1,max_length=36,pattern=r'^[A-Za-z0-9_-]+$')
 name:str=Field(min_length=1,max_length=80)
 kind:Literal['router','switch','ap','media_server','upstream','other']='other'
 x:int=Field(default=100,ge=70,le=730)
 y:int=Field(default=100,ge=40,le=560)
class Link(Strict):
 source:str=Field(min_length=1,max_length=36)
 target:str=Field(min_length=1,max_length=36)
 label:str=Field(default='',max_length=80)
class Topology(Strict):
 nodes:list[Node]=Field(default_factory=list,max_length=30)
 links:list[Link]=Field(default_factory=list,max_length=100)
 @model_validator(mode='after')
 def valid_links(self):
  ids={n.id for n in self.nodes}
  if len(ids)!=len(self.nodes):raise ValueError('Topology node IDs must be unique')
  seen=set()
  for link in self.links:
   if link.source not in ids or link.target not in ids or link.source==link.target:raise ValueError('Links must connect two existing distinct nodes')
   pair=tuple(sorted([link.source,link.target]))
   if pair in seen:raise ValueError('Only one link per node pair is supported')
   seen.add(pair)
  return self
class PlanningConfig(Strict):
 mode:Literal['disabled','failover','balance']='disabled'
 uplinks:list[Uplink]=Field(default_factory=list,max_length=8)
 budget:Budget=Field(default_factory=Budget)
 notes:str=Field(default='',max_length=2000)
 topology:Topology=Field(default_factory=Topology)
 @model_validator(mode='after')
 def valid_uplinks(self):
  if len({x.name.lower() for x in self.uplinks})!=len(self.uplinks):raise ValueError('Uplink names must be unique')
  if self.mode!='disabled' and sum(x.enabled for x in self.uplinks)<2:raise ValueError('Failover/load balancing need at least two enabled uplinks')
  return self
class PlanningSave(Strict):
 expected_revision:int=Field(ge=0)
 settings:PlanningConfig
def state(row,site):return {'site_id':site.id,'timezone':site.timezone,'revision':row.revision if row else 0,'settings':row.settings if row else PlanningConfig().model_dump(mode='json'),'updated':row.updated if row else None,'router_apply_state':'not_connected','usage_source':'owner_entered','usage_verified':False}
def register_site_planning(app,current,db,owner,log):
 @app.get('/v1/sites/{id}/planning')
 def get(id:str,u=Depends(current),s=Depends(db)):
  owner(u);site=allowed_site(s,u,id);return state(s.get(SitePlanning,id),site)
 @app.post('/v1/sites/{id}/planning')
 def save(id:str,b:PlanningSave,u=Depends(current),s=Depends(db)):
  owner(u);site=allowed_site(s,u,id);row=s.get(SitePlanning,id);settings=b.settings.model_dump(mode='json');now=int(time.time())
  if row:
   changed=s.execute(update(SitePlanning).where(SitePlanning.site_id==id,SitePlanning.revision==b.expected_revision).values(revision=b.expected_revision+1,settings=settings,updated=now)).rowcount
   if not changed:raise HTTPException(409,'Plan changed; refresh before saving')
  else:
   if b.expected_revision!=0:raise HTTPException(409,'Plan revision does not match')
   s.add(SitePlanning(site_id=id,revision=1,settings=settings,updated=now))
  try:s.flush();log(s,u,'site_wan_budget_plan_saved',id);s.commit()
  except IntegrityError:s.rollback();raise HTTPException(409,'Plan changed; refresh before saving')
  s.expire_all();return state(s.get(SitePlanning,id),site)
