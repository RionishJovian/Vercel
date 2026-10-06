import unittest,time
from concurrent.futures import ThreadPoolExecutor
from sqlalchemy import select
import test_backend as fixture
from backend.app import HASHER
from backend.models import Voucher,Entitlement,User,Audit,PortalSettings

class VoucherTests(unittest.TestCase):
 setUp=fixture.AccountPaymentTests.setUp
 tearDown=fixture.AccountPaymentTests.tearDown
 login=fixture.AccountPaymentTests.login
 package=fixture.AccountPaymentTests.package
 def setup_voucher(self):
  self.owner=self.login('owner');self.guest=self.login('guest');self.second=self.login('guest2');self.pkg=self.package(self.owner)
  state=self.client.get('/v1/portal',headers=self.owner).json();cfg={**state['draft'],'voucher_login':True}
  self.client.post('/v1/portal/draft',headers=self.owner,json={'expected_revision':state['revision'],'settings':cfg})
  self.client.post('/v1/portal/publish',headers=self.owner,json={'expected_revision':state['revision']+1})
  self.body={'package_id':self.pkg,'expires_days':30,'request_key':'issue-test-voucher-001'}
  r=self.client.post('/v1/vouchers',headers=self.owner,json=self.body);self.assertEqual(r.status_code,201);return r.json()
 def redeem(self,code,header=None):return self.client.post('/v1/vouchers/redeem',headers=header or self.guest,json={'code':code})
 def test_issue_hash_snapshot_idempotency_and_no_revenue(self):
  v=self.setup_voucher();self.assertTrue(v['code']);again=self.client.post('/v1/vouchers',headers=self.owner,json=self.body).json();self.assertEqual(again['id'],v['id']);self.assertIsNone(again['code'])
  with self.app.state.factory() as s:
   stored=s.get(Voucher,v['id']);self.assertNotEqual(stored.code_hash,v['code'].replace('-',''));self.assertEqual(len(stored.code_hash),64)
  inventory=self.client.get('/v1/vouchers',headers=self.owner).json();self.assertNotIn('code',inventory[0]);self.assertNotIn('code_hash',inventory[0])
  self.assertEqual(self.client.get('/v1/revenue',headers=self.owner).json()['net_collections'],0)
  self.assertEqual(self.client.post('/v1/vouchers',headers=self.guest,json=self.body).status_code,403)
  self.assertEqual(self.client.post('/v1/vouchers',headers=self.owner,json={**self.body,'expires_days':2}).status_code,409)
 def test_redeem_once_replay_and_grant_source(self):
  v=self.setup_voucher();first=self.redeem(v['code'].lower().replace('-',' '));self.assertEqual(first.status_code,200);self.assertEqual(first.json()['fulfillment'],'pending_adapter')
  replay=self.redeem(v['code']);self.assertTrue(replay.json()['replayed']);self.assertEqual(replay.json()['id'],first.json()['id']);self.assertEqual(self.redeem(v['code'],self.second).status_code,400)
  passes=self.client.get('/v1/entitlements',headers=self.guest).json();self.assertEqual(len(passes),1);self.assertEqual(passes[0]['source'],'voucher');self.assertIsNone(passes[0]['order_id'])
  self.assertEqual(self.client.get('/v1/orders',headers=self.guest).json(),[]);self.assertEqual(self.client.get('/v1/revenue',headers=self.owner).json()['net_collections'],0)
 def test_expired_revoked_and_portal_disabled(self):
  v=self.setup_voucher()
  with self.app.state.factory() as s:row=s.get(Voucher,v['id']);row.created=int(time.time())-200;row.expires=int(time.time())-1;s.commit()
  self.assertEqual(self.redeem(v['code']).status_code,400)
  fresh=self.client.post('/v1/vouchers',headers=self.owner,json={**self.body,'request_key':'issue-test-voucher-002'}).json()
  self.client.post('/v1/vouchers/'+fresh['id']+'/revoke',headers=self.owner,json={'reason':'Lost code'});self.assertEqual(self.redeem(fresh['code']).status_code,400)
  newest=self.client.post('/v1/vouchers',headers=self.owner,json={**self.body,'request_key':'issue-test-voucher-003'}).json()
  state=self.client.get('/v1/portal',headers=self.owner).json();self.client.post('/v1/portal/draft',headers=self.owner,json={'expected_revision':state['revision'],'settings':{**state['draft'],'voucher_login':False}});self.client.post('/v1/portal/publish',headers=self.owner,json={'expected_revision':state['revision']+1})
  self.assertEqual(self.redeem(newest['code']).status_code,409)
 def test_revoke_redeemed_pass_and_tenant_isolation(self):
  v=self.setup_voucher();self.redeem(v['code']);other=self.login('other');platform=self.login('platform')
  with self.app.state.factory() as s:
   other_tenant=s.get(User,self.users['other']).tenant_id;cfg=s.get(PortalSettings,s.get(User,self.users['owner']).tenant_id).published.copy();s.add(PortalSettings(tenant_id=other_tenant,revision=1,draft=cfg,published=cfg,published_revision=1,updated=int(time.time())));s.add(User(username='foreignguest',role='guest',tenant_id=other_tenant,password_hash=HASHER.hash('test-password-only')));s.commit()
  foreign=self.login('foreignguest');self.assertEqual(self.redeem(v['code'],foreign).status_code,400)
  self.assertEqual(self.client.get('/v1/entitlements',headers=foreign).json(),[])
  self.assertEqual(self.client.get('/v1/vouchers',headers=other).json(),[]);self.assertEqual(self.client.get('/v1/vouchers',headers=platform).status_code,403)
  for headers,status in [(other,404),(self.guest,403)]:self.assertEqual(self.client.post('/v1/vouchers/'+v['id']+'/revoke',headers=headers,json={'reason':'Attempt'}).status_code,status)
  self.assertEqual(self.client.post('/v1/vouchers/'+v['id']+'/revoke',headers=self.owner,json={'reason':'Customer refund handled separately'}).status_code,200)
  self.assertTrue(self.client.get('/v1/entitlements',headers=self.guest).json()[0]['revoked']);self.assertEqual(self.redeem(v['code']).json()['fulfillment'],'revoked')
  with self.app.state.factory() as s:events=list(s.scalars(select(Audit).where(Audit.action=='voucher_revoked')));self.assertEqual(len(events),1)
 def test_concurrent_redemption_has_one_winner(self):
  v=self.setup_voucher()
  with ThreadPoolExecutor(max_workers=2) as pool:
   results=list(pool.map(lambda h:self.redeem(v['code'],h),[self.guest,self.second]))
  self.assertEqual(sum(r.status_code==200 for r in results),1);self.assertTrue(all(r.status_code in [200,400,409] for r in results))
  with self.app.state.factory() as s:self.assertEqual(len(list(s.scalars(select(Entitlement).where(Entitlement.voucher_id==v['id'])))),1)
 def test_invalid_code_attempts_are_limited(self):
  self.setup_voucher()
  for _ in range(10):self.assertEqual(self.redeem('A'*26).status_code,400)
  self.assertEqual(self.redeem('A'*26).status_code,429)
