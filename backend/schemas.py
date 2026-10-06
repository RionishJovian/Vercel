from pydantic import BaseModel, Field, ConfigDict, model_validator
from typing import Literal, Optional
class Strict(BaseModel):model_config=ConfigDict(extra='forbid',str_strip_whitespace=True)
class Login(Strict):
 model_config=ConfigDict(extra="forbid",str_strip_whitespace=False)
 username:str=Field(min_length=1,max_length=160)
 password:str=Field(min_length=1,max_length=256)
 mfa_code:Optional[str]=Field(default=None,min_length=1,max_length=80)
class PackageIn(Strict):
 name:str=Field(min_length=1,max_length=120)
 service:Literal['internet','media','combined']
 mode:Literal['data','time','both','library','title','hybrid']
 price:int=Field(gt=0,le=1_000_000_000)
 gb:float=Field(default=0,ge=0,le=100000)
 hours:float=Field(default=0,ge=0,le=8760)
 media_hours:float=Field(default=24,gt=0,le=8760)
 clock:Literal['elapsed','connected']='elapsed'
 devices:int=Field(default=1,ge=1,le=100)
 speed:int=Field(default=10,ge=1,le=10000)
 title:Optional[str]=Field(default=None,max_length=160)
 title_hours:float=Field(default=48,gt=0,le=8760)
 @model_validator(mode='after')
 def validate_policy(self):
  if self.service=='media':
   if self.mode not in ['library','title','hybrid'] or self.clock!='elapsed' or self.hours<=0:raise ValueError('Media needs a timed viewing window and library/title/hybrid mode')
   if self.mode in ['title','hybrid'] and not self.title:raise ValueError('Select the purchased title')
  else:
   if self.mode not in ['data','time','both']:raise ValueError('Internet needs data/time/both mode')
   if self.mode in ['data','both'] and self.gb<=0:raise ValueError('Positive data quota required')
   if self.mode in ['time','both'] and self.hours<=0:raise ValueError('Positive time quota required')
  return self
class OrderIn(Strict):
 site_id:Optional[str]=Field(default=None,min_length=1,max_length=36)
 portal_revision:int=Field(gt=0)
 package_id:str
 method:str=Field(min_length=1,max_length=80)
 reference:str=Field(min_length=1,max_length=160)
 evidence:str=Field(min_length=1,max_length=4000)
 request_key:str=Field(min_length=16,max_length=80)
class Decision(Strict):
 expected_receipt_revision:Optional[int]=Field(default=None,ge=0)
 decision:Literal['approved','rejected','needs_evidence']
 reason:str=Field(default='',max_length=1000)
 @model_validator(mode='after')
 def reasons(self):
  if self.decision!='approved' and not self.reason:raise ValueError('Reason required')
  return self
class EvidenceIn(Strict):evidence:str=Field(min_length=1,max_length=4000)
class Refund(Strict):reason:str=Field(min_length=1,max_length=1000)

from pydantic import field_validator
import ipaddress,re

def host_value(value):
 value=value.lower().rstrip('.')
 try:return str(ipaddress.ip_address(value))
 except ValueError:pass
 if len(value)>253 or '.' not in value or not all(re.fullmatch(r'[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?',part) for part in value.split('.')) or value.replace('.','').isdigit():
  raise ValueError('Use an explicit DNS hostname or IP address, without wildcard, port, path or URL scheme')
 return value

class PortalConfig(Strict):
 business_name:str=Field(default='My business',min_length=1,max_length=120)
 welcome:str=Field(default='Welcome. Choose internet or media access.',max_length=1000)
 language:Literal['en','my','both']='both'
 terms:str=Field(default='',max_length=4000)
 contact:str=Field(default='',max_length=200)
 direct_purchase:bool=False
 voucher_login:bool=True
 payment_destination:str=Field(default='',max_length=500)
 payment_instructions:str=Field(default='',max_length=2000)
 payment_methods:list[str]=Field(default_factory=lambda:['Cash at counter'],max_length=10)
 hostname:str=Field(default='',max_length=253)
 guest_interface:str=Field(default='',max_length=80,pattern=r'^[A-Za-z0-9_.:-]*$')
 guest_subnet:str=Field(default='',max_length=43)
 tls_required:bool=True
 isolate_guests:bool=True
 allowed_hosts:list[str]=Field(default_factory=list,max_length=50)
 @field_validator('hostname')
 @classmethod
 def hostname_valid(cls,v):return host_value(v) if v else v
 @field_validator('guest_subnet')
 @classmethod
 def subnet_valid(cls,v):
  if not v:return v
  try:n=ipaddress.ip_network(v,strict=True)
  except ValueError:raise ValueError('Use a network subnet, for example 192.168.50.0/24')
  if n.version!=4 or n.prefixlen<16 or n.prefixlen>30:raise ValueError('Guest IPv4 subnet must use /16 through /30; IPv6 policy requires router verification')
  return str(n)
 @field_validator('allowed_hosts')
 @classmethod
 def hosts_valid(cls,v):return list(dict.fromkeys(host_value(x.strip()) for x in v))
 @field_validator('payment_methods')
 @classmethod
 def methods_valid(cls,v):
  values=[x.strip() for x in v]
  if any(not x or len(x)>80 for x in values) or len(set(values))!=len(values):raise ValueError('Payment methods must be unique nonempty names, up to 80 characters each')
  return values
class PortalDraftIn(Strict):
 expected_revision:int=Field(ge=0)
 settings:PortalConfig
class PortalPublishIn(Strict):expected_revision:int=Field(gt=0)

class VoucherIssue(Strict):
 site_id:Optional[str]=Field(default=None,min_length=1,max_length=36)
 package_id:str
 expires_days:int=Field(default=30,ge=1,le=365)
 request_key:str=Field(min_length=16,max_length=80)
class VoucherRedeem(Strict):
 site_id:Optional[str]=Field(default=None,min_length=1,max_length=36)
 code:str=Field(min_length=26,max_length=64)
 @field_validator('code')
 @classmethod
 def normalize(cls,v):
  value=v.replace('-','').replace(' ','').upper()
  if not re.fullmatch('[A-Z2-7]{26}',value):raise ValueError('Enter the complete voucher code')
  return value
class VoucherRevoke(Strict):reason:str=Field(min_length=1,max_length=1000)

class VoucherSaleIn(Strict):
 amount:int=Field(gt=0,le=1_000_000_000)
 method:str=Field(min_length=1,max_length=80)
 reference:str=Field(min_length=1,max_length=160)
 evidence:str=Field(min_length=1,max_length=4000)
 confirmed_received:Literal[True]
class VoucherSaleRefund(Strict):
 reason:str=Field(min_length=1,max_length=1000)
 reference:str=Field(min_length=1,max_length=160)
 confirmed_returned:Literal[True]

class SiteIn(Strict):
 name:str=Field(min_length=1,max_length=120)
 timezone:str=Field(default='Asia/Yangon',max_length=80)
 @field_validator('timezone')
 @classmethod
 def valid_zone(cls,v):
  from zoneinfo import ZoneInfo,ZoneInfoNotFoundError
  try:ZoneInfo(v)
  except (ZoneInfoNotFoundError,ValueError):raise ValueError('Use a recognized IANA timezone')
  return v
class RouterIn(Strict):
 site_id:str=Field(min_length=1,max_length=36)
 name:str=Field(min_length=1,max_length=120)
 model:str=Field(min_length=1,max_length=120)
 reported_routeros:str=Field(default='',max_length=40)
