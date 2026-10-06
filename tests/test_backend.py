import unittest,tempfile,sys,time
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from fastapi.testclient import TestClient
from sqlalchemy import select
from sqlalchemy.schema import CreateTable
from sqlalchemy.dialects.postgresql import dialect
from backend.app import make_app,HASHER
from backend.models import Tenant,User,SessionToken,Entitlement,Outbox,Order,Base,PortalSettings
class AccountPaymentTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.app=make_app('sqlite:///'+self.tmp.name+'/accounts.db',initialize=True);self.client=TestClient(self.app);self.users={}
  with self.app.state.factory() as s:
   a=Tenant(name='Cafe');b=Tenant(name='Other');s.add_all([a,b]);s.flush()
   for username,role,tenant in [('owner','sme',a.id),('guest','guest',a.id),('other','sme',b.id),('guest2','guest',a.id),('platform','platform',None)]:
    u=User(username=username,role=role,tenant_id=tenant,password_hash=HASHER.hash('test-password-only'));s.add(u);s.flush();self.users[username]=u.id
   cfg={'business_name':'Test Cafe','welcome':'Test only','language':'en','terms':'','contact':'','direct_purchase':True,'voucher_login':False,'payment_destination':'Test counter','payment_instructions':'Test payment only','payment_methods':['Cash'],'hostname':'','guest_interface':'','guest_subnet':'','tls_required':True,'isolate_guests':True,'allowed_hosts':[]}
   s.add(PortalSettings(tenant_id=a.id,revision=1,draft=cfg,published=cfg,published_revision=1,updated=int(time.time())))
   s.commit()
 def tearDown(self):self.client.close();self.app.state.engine.dispose();self.tmp.cleanup()
 def login(self,name):
  r=self.client.post('/v1/auth/login',json={'username':name,'password':'test-password-only'});self.assertEqual(r.status_code,200);return {'Authorization':'Bearer '+r.json()['access_token']}
 def package(self,owner):
  r=self.client.post('/v1/packages',headers=owner,json={'name':'Combo','service':'combined','mode':'both','price':3000,'gb':5,'hours':3});self.assertEqual(r.status_code,201);return r.json()['id']
 def order(self,guest,p):
  return self.client.post('/v1/orders',headers=guest,json={'portal_revision':1,'package_id':p,'method':'Cash','reference':'receipt-test','evidence':'received at counter','request_key':'unique-test-request-001'})
 def test_auth_revocation_and_account_disable(self):
  self.assertEqual(self.client.get('/v1/me').status_code,401);h=self.login('owner');self.assertEqual(self.client.get('/v1/me',headers=h).json()['role'],'sme');self.client.post('/v1/auth/logout',headers=h);self.assertEqual(self.client.get('/v1/me',headers=h).status_code,401)
  h=self.login('owner')
  with self.app.state.factory() as s:u=s.get(User,self.users['owner']);u.active=False;s.commit()
  self.assertEqual(self.client.get('/v1/me',headers=h).status_code,401)
 def test_order_approval_is_atomic_and_pending_real_adapter(self):
  owner=self.login('owner');guest=self.login('guest');other=self.login('other');p=self.package(owner);r=self.order(guest,p);self.assertEqual(r.status_code,201);o=r.json();self.assertEqual(self.order(guest,p).json()['id'],o['id'])
  for h in [guest,other]:self.assertIn(self.client.post('/v1/orders/'+o['id']+'/decision',headers=h,json={'decision':'approved'}).status_code,[403,404])
  url='/v1/orders/'+o['id']+'/decision';r=self.client.post(url,headers=owner,json={'decision':'approved'});self.assertEqual(r.status_code,200);self.assertEqual(r.json()['fulfillment'],'pending_adapter');self.assertTrue(self.client.post(url,headers=owner,json={'decision':'approved'}).json()['replayed'])
  self.assertEqual(self.client.post(url,headers=owner,json={'decision':'rejected','reason':'conflict'}).status_code,409)
  with self.app.state.factory() as s:self.assertEqual(len(list(s.scalars(select(Entitlement)))),1);self.assertEqual(len(list(s.scalars(select(Outbox)))),2)
  self.assertEqual(self.client.get('/v1/revenue',headers=owner).json()['net_collections'],3000);self.assertEqual(self.client.get('/v1/orders',headers=other).json(),[])
 def test_guest_ownership_and_refund_revocation(self):
  owner=self.login('owner');guest=self.login('guest');guest2=self.login('guest2');p=self.package(owner);o=self.order(guest,p).json();url='/v1/orders/'+o['id'];self.client.post(url+'/decision',headers=owner,json={'decision':'approved'})
  self.assertEqual(self.client.get('/v1/orders',headers=guest2).json(),[]);self.assertEqual(self.client.get('/v1/entitlements',headers=guest2).json(),[])
  self.assertEqual(self.client.post(url+'/refund',headers=guest,json={'reason':'test'}).status_code,403);self.assertEqual(self.client.post(url+'/refund',headers=owner,json={'reason':'manual refund confirmed'}).status_code,200);self.assertTrue(self.client.get('/v1/entitlements',headers=guest).json()[0]['revoked']);self.assertEqual(self.client.get('/v1/revenue',headers=owner).json()['net_collections'],0)
 def test_policy_validation_and_no_privilege_claim(self):
  owner=self.login('owner');guest=self.login('guest');bad={'name':'Wrong','service':'internet','mode':'both','price':3000,'gb':0,'hours':0};self.assertEqual(self.client.post('/v1/packages',headers=owner,json=bad).status_code,422)
  self.assertEqual(self.client.post('/v1/packages',headers=guest,json={'name':'Try','service':'internet','mode':'data','price':1,'gb':1}).status_code,403)
  self.assertEqual(self.client.post('/v1/auth/login',json={'username':'guest','password':'test-password-only','role':'platform'}).status_code,422)
  self.assertEqual(self.client.post('/api/demo-session',json={'role':'platform'}).status_code,404)
 def test_login_lockout_and_no_plain_tokens_stored(self):
  for _ in range(8):self.assertEqual(self.client.post('/v1/auth/login',json={'username':'owner','password':'wrong'}).status_code,401)
  self.assertEqual(self.client.post('/v1/auth/login',json={'username':'owner','password':'wrong'}).status_code,429)
 def test_stored_session_digest_and_expiry(self):
  h=self.login('owner');raw=h['Authorization'][7:]
  with self.app.state.factory() as s:
   row=s.scalar(select(SessionToken));self.assertNotEqual(row.token_hash,raw);self.assertEqual(len(row.token_hash),64);row.expires=int(time.time())-1;s.commit()
  self.assertEqual(self.client.get('/v1/me',headers=h).status_code,401)
 def test_platform_does_not_implicitly_review_tenant_payments(self):
  h=self.login('platform');self.assertEqual(len(self.client.get('/v1/tenants',headers=h).json()),2);self.assertEqual(self.client.get('/v1/orders',headers=h).status_code,403)
 def test_evidence_review_and_audit_are_scoped(self):
  owner=self.login('owner');guest=self.login('guest');other=self.login('other');guest2=self.login('guest2');p=self.package(owner);o=self.order(guest,p).json();url='/v1/orders/'+o['id']
  self.assertEqual(self.client.post(url+'/decision',headers=owner,json={'decision':'needs_evidence','reason':'Receipt reference missing'}).status_code,200)
  self.assertEqual(self.client.post(url+'/evidence',headers=guest2,json={'evidence':'other customer'}).status_code,404)
  self.assertEqual(self.client.post(url+'/evidence',headers=guest,json={'evidence':'Reference and test receipt'}).json()['status'],'pending')
  self.assertEqual(self.client.post(url+'/decision',headers=owner,json={'decision':'approved'}).status_code,200)
  self.assertEqual(self.client.get('/v1/revenue',headers=owner).json()['approved_orders'],1)
  mine=self.client.get('/v1/audit',headers=guest).json();self.assertTrue(all(a['actor_id']==self.users['guest'] for a in mine))
  foreign=self.client.get('/v1/audit',headers=other).json();self.assertFalse(any(a['target']==o['id'] for a in foreign))
 def test_portal_draft_publish_isolated_and_revision_checked(self):
  owner=self.login('owner');guest=self.login('guest');other=self.login('other');platform=self.login('platform')
  old=self.client.get('/v1/portal',headers=owner).json();cfg={**old['draft'],'business_name':'Changed cafe','payment_destination':'New test counter','guest_interface':'bridge-guest','guest_subnet':'192.168.50.0/24','allowed_hosts':['catalogue.example.com','192.168.50.10']}
  body={'expected_revision':1,'settings':cfg}
  for h in [guest,platform]:self.assertEqual(self.client.post('/v1/portal/draft',headers=h,json=body).status_code,403)
  self.assertEqual(self.client.get('/v1/portal',headers=other).json()['revision'],0)
  self.assertEqual(self.client.get('/v1/portal/public',headers=other).json()['settings'],None)
  saved=self.client.post('/v1/portal/draft',headers=owner,json=body);self.assertEqual(saved.status_code,200);self.assertEqual(saved.json()['revision'],2)
  self.assertEqual(self.client.post('/v1/portal/draft',headers=owner,json=body).status_code,409)
  public=self.client.get('/v1/portal/public',headers=guest).json();self.assertEqual(public['settings']['business_name'],'Test Cafe')
  self.assertEqual(self.client.post('/v1/portal/publish',headers=owner,json={'expected_revision':1}).status_code,409)
  self.assertEqual(self.client.post('/v1/portal/publish',headers=owner,json={'expected_revision':2}).status_code,200)
  public=self.client.get('/v1/portal/public',headers=guest).json();self.assertEqual(public['settings']['business_name'],'Changed cafe');self.assertEqual(public['revision'],3)
  for key in ['guest_interface','guest_subnet','allowed_hosts','tls_required']:self.assertNotIn(key,public['settings'])
  self.assertEqual(self.client.get('/v1/portal',headers=guest).status_code,403)
 def test_portal_rejects_broad_hosts_and_invalid_payment_publication(self):
  h=self.login('owner');cfg=self.client.get('/v1/portal',headers=h).json()['draft']
  for change in [{'allowed_hosts':['*']},{'allowed_hosts':['*.example.com']},{'allowed_hosts':['https://example.com/movie']},{'guest_subnet':'192.168.50.5/24'},{'hostname':'999.999.999.999'}]:
   self.assertEqual(self.client.post('/v1/portal/draft',headers=h,json={'expected_revision':1,'settings':{**cfg,**change}}).status_code,422)
  bad={**cfg,'payment_destination':''};self.assertEqual(self.client.post('/v1/portal/draft',headers=h,json={'expected_revision':1,'settings':bad}).status_code,200)
  self.assertEqual(self.client.post('/v1/portal/publish',headers=h,json={'expected_revision':2}).status_code,422)
  self.assertEqual(self.client.get('/v1/portal/public',headers=h).json()['settings']['payment_destination'],'Test counter')
 def test_order_payment_snapshot_and_stale_guest_revision(self):
  owner=self.login('owner');guest=self.login('guest');p=self.package(owner);o=self.order(guest,p).json();self.assertEqual(o['payment_context']['destination'],'Test counter')
  cfg=self.client.get('/v1/portal',headers=owner).json()['draft'];cfg['payment_destination']='Replacement counter';cfg['payment_methods']=['Wallet']
  self.client.post('/v1/portal/draft',headers=owner,json={'expected_revision':1,'settings':cfg});self.client.post('/v1/portal/publish',headers=owner,json={'expected_revision':2})
  old={'package_id':p,'portal_revision':1,'method':'Cash','reference':'second','evidence':'test receipt','request_key':'second-request-test-001'}
  self.assertEqual(self.client.post('/v1/orders',headers=guest,json=old).status_code,409)
  self.assertEqual(self.client.post('/v1/orders',headers=guest,json={**old,'portal_revision':3}).status_code,422)
  new=self.client.post('/v1/orders',headers=guest,json={**old,'portal_revision':3,'method':'Wallet'});self.assertEqual(new.status_code,201);self.assertEqual(new.json()['payment_context']['destination'],'Replacement counter')
  self.assertEqual(self.order(guest,p).json()['id'],o['id'])
  stored=self.client.get('/v1/orders',headers=owner).json();self.assertEqual(next(x for x in stored if x['id']==o['id'])['payment_context']['destination'],'Test counter')
 def test_purchase_closed_without_published_portal(self):
  owner=self.login('owner');guest=self.login('guest');p=self.package(owner)
  with self.app.state.factory() as s:
   row=s.scalar(select(PortalSettings));row.published=None;row.published_revision=None;s.commit()
  self.assertEqual(self.order(guest,p).status_code,409)
  self.assertEqual(self.client.get('/v1/portal/public',headers=guest).json()['settings'],None)
 def test_disable_purchases_blocks_new_orders_without_removing_history(self):
  owner=self.login('owner');guest=self.login('guest');p=self.package(owner);first=self.order(guest,p).json()
  cfg=self.client.get('/v1/portal',headers=owner).json()['draft'];cfg['direct_purchase']=False
  self.client.post('/v1/portal/draft',headers=owner,json={'expected_revision':1,'settings':cfg});self.client.post('/v1/portal/publish',headers=owner,json={'expected_revision':2})
  new={'package_id':p,'portal_revision':3,'method':'Cash','reference':'new','evidence':'test','request_key':'another-test-request-001'}
  self.assertEqual(self.client.post('/v1/orders',headers=guest,json=new).status_code,409)
  self.assertEqual(self.order(guest,p).json()['id'],first['id'])
 def test_postgresql_schema_compiles(self):
  for table in Base.metadata.sorted_tables:self.assertIn('CREATE TABLE',str(CreateTable(table).compile(dialect=dialect())))
if __name__=='__main__':unittest.main()
