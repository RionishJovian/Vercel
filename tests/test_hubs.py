import base64,unittest
from concurrent.futures import ThreadPoolExecutor
import test_backend as fixture
from sqlalchemy import select
from backend.models import MagicHub,HubAssignment,RouterPlan,User,Audit

def key(n):return base64.b64encode(bytes([n])*32).decode()
class HubTests(unittest.TestCase):
 setUp=fixture.AccountPaymentTests.setUp
 tearDown=fixture.AccountPaymentTests.tearDown
 login=fixture.AccountPaymentTests.login
 def setup_hub(self):
  platform=self.login('platform');owner=self.login('owner')
  body={'name':'Magic Hub A','endpoint':'hub.example.com','pool':'10.90.1.0/24','public_key':key(1)}
  r=self.client.post('/v1/hubs',headers=platform,json=body);self.assertEqual(r.status_code,201,r.text);hub=r.json()['id']
  with self.app.state.factory() as s:tenant=s.get(User,self.users['owner']).tenant_id
  r=self.client.post('/v1/hubs/'+hub+'/assignments',headers=platform,json={'tenant_id':tenant});self.assertEqual(r.status_code,200)
  site=self.client.post('/v1/sites',headers=owner,json={'name':'Cafe'}).json()['id']
  return platform,owner,hub,site,tenant
 def router(self,owner,site,name='Gateway'):
  r=self.client.post('/v1/routers',headers=owner,json={'site_id':site,'name':name,'model':'hAP ax3','reported_routeros':'7.x'});self.assertEqual(r.status_code,201);return r.json()['id']
 def plan(self,owner,hub,router,n=2):return self.client.post('/v1/routers/'+router+'/plan',headers=owner,json={'hub_id':hub,'router_public_key':key(n)})
 def test_platform_admin_and_assigned_business_visibility(self):
  platform,owner,hub,site,tenant=self.setup_hub();other=self.login('other');guest=self.login('guest')
  for h in [owner,guest,other]:self.assertEqual(self.client.post('/v1/hubs',headers=h,json={'name':'Blocked','endpoint':'b.example.com','pool':'10.90.2.0/24','public_key':key(4)}).status_code,403)
  self.assertEqual(len(self.client.get('/v1/hubs',headers=owner).json()),1);self.assertEqual(self.client.get('/v1/hubs',headers=other).json(),[])
  self.assertEqual(self.client.get('/v1/hubs',headers=guest).status_code,403);self.assertEqual(self.client.get('/v1/hub-assignments',headers=owner).status_code,403)
  again=self.client.post('/v1/hubs/'+hub+'/assignments',headers=platform,json={'tenant_id':tenant});self.assertTrue(again.json()['replayed'])
  self.assertEqual(self.client.post('/v1/hubs/'+hub+'/assignments',headers=platform,json={'tenant_id':'missing'}).status_code,404)
  foreignsite=self.client.post('/v1/sites',headers=other,json={'name':'Other'}).json()['id'];foreignrouter=self.router(other,foreignsite)
  self.assertEqual(self.plan(other,hub,foreignrouter).status_code,404)
  self.assertEqual(self.plan(owner,hub,foreignrouter).status_code,404)
 def test_plan_retries_review_and_no_fabricated_connectivity(self):
  platform,owner,hub,site,tenant=self.setup_hub();router=self.router(owner,site);first=self.plan(owner,hub,router);self.assertEqual(first.status_code,201,first.text);p=first.json()
  self.assertEqual(p['address'],'10.90.1.2');self.assertEqual(p['state'],'awaiting_review');self.assertEqual(p['snapshot']['router']['peer_allowed_addresses'],['10.90.1.1/32']);self.assertFalse(p['snapshot']['guest_traffic_via_hub']);self.assertNotIn('private_key',p['snapshot']['router'])
  self.assertEqual(self.plan(owner,hub,router).json()['id'],p['id']);self.assertEqual(self.plan(owner,hub,router,3).status_code,409)
  for role in ['guest','other','platform']:
   self.assertIn(self.client.post('/v1/router-plans/'+p['id']+'/review',headers=self.login(role),json={'confirmed_review':True}).status_code,[403,404])
  self.assertEqual(self.client.post('/v1/router-plans/'+p['id']+'/review',headers=owner,json={'confirmed_review':False}).status_code,422)
  reviewed=self.client.post('/v1/router-plans/'+p['id']+'/review',headers=owner,json={'confirmed_review':True}).json();self.assertEqual(reviewed['state'],'awaiting_adapter');self.assertEqual(reviewed['connection_state'],'unverified');self.assertIsNone(reviewed['last_handshake'])
  self.assertTrue(self.client.post('/v1/router-plans/'+p['id']+'/review',headers=owner,json={'confirmed_review':True}).json()['replayed'])
  self.assertEqual(self.client.get('/v1/router-plans',headers=self.login('other')).json(),[]);self.assertEqual(self.client.get('/v1/router-plans',headers=self.login('guest')).status_code,403)
  self.assertEqual(len(self.client.get('/v1/router-plans',headers=platform).json()),1)
  fleet=self.client.get('/v1/routers',headers=owner).json()[0];self.assertEqual(fleet['hub_id'],hub);self.assertEqual(fleet['connection_state'],'unverified');self.assertIsNone(fleet['last_handshake'])
  with self.app.state.factory() as s:self.assertEqual(s.get(MagicHub,hub).next_host,3);self.assertEqual(len(list(s.scalars(select(Audit).where(Audit.action=='router_plan_reviewed')))),1)
 def test_validation_duplicates_and_secret_fields(self):
  platform=self.login('platform');body={'name':'Valid','endpoint':'hub.example.com','pool':'10.90.1.0/24','public_key':key(1)}
  for changed in [{'endpoint':'https://hub.example.com/path'},{'endpoint':'127.0.0.1'},{'endpoint':'10.0.0.1'},{'endpoint':'hub.local'},{'pool':'10.90.1.0/25'},{'pool':'10.90.1.1/24'},{'pool':'8.8.8.0/24'},{'port':0},{'public_key':key(0)},{'public_key':'invalid'},{'private_key':key(5)},{'operational_state':'online'}]:
   r=self.client.post('/v1/hubs',headers=platform,json=body|changed);self.assertEqual(r.status_code,422,r.text)
  self.assertEqual(self.client.post('/v1/hubs',headers=platform,json=body).status_code,201)
  self.assertEqual(self.client.post('/v1/hubs',headers=platform,json=body|{'name':'Another','endpoint':'another.example.com','public_key':key(4)}).status_code,409)
 def test_atomic_allocation_and_public_key_uniqueness(self):
  platform,owner,hub,site,tenant=self.setup_hub();a=self.router(owner,site,'A');b=self.router(owner,site,'B')
  with ThreadPoolExecutor(2) as pool:results=list(pool.map(lambda args:self.plan(owner,hub,*args),[(a,2),(b,3)]))
  self.assertEqual([r.status_code for r in results],[201,201]);self.assertEqual({r.json()['address'] for r in results},{'10.90.1.2','10.90.1.3'})
  c=self.router(owner,site,'C');self.assertEqual(self.plan(owner,hub,c,2).status_code,409)
  with self.app.state.factory() as s:self.assertEqual(s.get(MagicHub,hub).next_host,4)
  self.assertEqual(self.plan(owner,hub,c,1).status_code,422)
 def test_exhaustion_and_portal_overlap(self):
  platform,owner,hub,site,tenant=self.setup_hub();a=self.router(owner,site)
  cfg=self.client.get('/v1/sites/'+site+'/portal',headers=owner).json()['draft'];cfg['guest_subnet']='10.90.1.0/24'
  self.assertEqual(self.client.post('/v1/sites/'+site+'/portal/draft',headers=owner,json={'expected_revision':0,'settings':cfg}).status_code,200)
  self.assertEqual(self.plan(owner,hub,a).status_code,409)
  cfg['guest_subnet']='192.168.50.0/24';self.client.post('/v1/sites/'+site+'/portal/draft',headers=owner,json={'expected_revision':1,'settings':cfg})
  with self.app.state.factory() as s:h=s.get(MagicHub,hub);h.next_host=254;s.commit()
  self.assertEqual(self.plan(owner,hub,a).json()['address'],'10.90.1.254')
  b=self.router(owner,site,'Second');self.assertEqual(self.plan(owner,hub,b,3).status_code,409)
  self.assertEqual(self.client.get('/v1/hubs',headers=owner).json()[0]['remaining_addresses'],0)
 def test_concurrent_same_plan_reuses_allocation(self):
  platform,owner,hub,site,tenant=self.setup_hub();router=self.router(owner,site)
  with ThreadPoolExecutor(2) as pool:results=list(pool.map(lambda _:self.plan(owner,hub,router),range(2)))
  self.assertEqual([r.status_code for r in results],[201,201]);self.assertEqual(len({r.json()['id'] for r in results}),1)
  with self.app.state.factory() as s:self.assertEqual(s.get(MagicHub,hub).next_host,3)
