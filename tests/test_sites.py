import unittest
import test_backend as fixture
from sqlalchemy import select
from backend.models import Site,RouterDevice
class SiteTests(unittest.TestCase):
 setUp=fixture.AccountPaymentTests.setUp
 tearDown=fixture.AccountPaymentTests.tearDown
 login=fixture.AccountPaymentTests.login
 package=fixture.AccountPaymentTests.package
 def site(self,owner,name):
  response=self.client.post('/v1/sites',headers=owner,json={'name':name,'timezone':'Asia/Yangon'});self.assertEqual(response.status_code,201);return response.json()['id']
 def publish(self,owner,site,recipient):
  cfg=self.client.get('/v1/sites/'+site+'/portal',headers=owner).json()['draft'];cfg.update(direct_purchase=True,voucher_login=True,payment_destination=recipient,payment_instructions='Test receipt only',payment_methods=['Cash'])
  self.assertEqual(self.client.post('/v1/sites/'+site+'/portal/draft',headers=owner,json={'expected_revision':0,'settings':cfg}).status_code,200)
  self.assertEqual(self.client.post('/v1/sites/'+site+'/portal/publish',headers=owner,json={'expected_revision':1}).status_code,200)
 def test_registry_is_scoped_and_cannot_claim_live_state(self):
  owner=self.login('owner');other=self.login('other');guest=self.login('guest');platform=self.login('platform');site=self.site(owner,'Main branch')
  self.assertEqual(self.client.post('/v1/sites',headers=guest,json={'name':'Bad'}).status_code,403);self.assertEqual(self.client.post('/v1/sites',headers=owner,json={'name':'Main branch'}).status_code,409)
  body={'site_id':site,'name':'Gateway','model':'hAP ax3','reported_routeros':'owner reported'}
  self.assertEqual(self.client.post('/v1/routers',headers=other,json=body).status_code,404)
  self.assertEqual(self.client.post('/v1/routers',headers=owner,json={**body,'connection_state':'online'}).status_code,422)
  response=self.client.post('/v1/routers',headers=owner,json=body);self.assertEqual(response.status_code,201);self.assertEqual(response.json()['connection_state'],'unverified')
  self.assertEqual(self.client.get('/v1/routers',headers=other).json(),[]);self.assertEqual(self.client.get('/v1/routers',headers=guest).status_code,403)
  fleet=self.client.get('/v1/routers',headers=platform).json();self.assertEqual(len(fleet),1);self.assertIsNone(fleet[0]['last_handshake']);self.assertIsNone(fleet[0]['hub_id'])
 def test_site_portals_are_independent_and_do_not_replace_legacy(self):
  owner=self.login('owner');guest=self.login('guest');other=self.login('other');a=self.site(owner,'A');b=self.site(owner,'B');self.publish(owner,a,'Counter A');self.publish(owner,b,'Counter B')
  def public(site):return self.client.get('/v1/portal/public',headers=guest,params={'site_id':site}).json()
  self.assertEqual(public(a)['settings']['payment_destination'],'Counter A');self.assertEqual(public(b)['settings']['payment_destination'],'Counter B');self.assertNotIn('guest_interface',public(a)['settings'])
  self.assertEqual(self.client.get('/v1/portal/public',headers=guest).json()['settings']['payment_destination'],'Test counter')
  self.assertEqual(self.client.get('/v1/sites/'+a+'/portal',headers=other).status_code,404);self.assertEqual(self.client.get('/v1/sites/'+a+'/portal',headers=guest).status_code,403)
  self.assertEqual(len(self.client.get('/v1/guest-sites',headers=guest).json()),2)
 def test_branch_order_and_entitlement_retain_site(self):
  owner=self.login('owner');guest=self.login('guest');other=self.login('other');a=self.site(owner,'A');foreign=self.site(other,'Other');self.publish(owner,a,'Counter A');p=self.package(owner)
  body={'site_id':a,'package_id':p,'portal_revision':2,'method':'Cash','reference':'branch-receipt','evidence':'Test evidence','request_key':'branch-order-test-001'}
  order=self.client.post('/v1/orders',headers=guest,json=body);self.assertEqual(order.status_code,201);self.assertEqual(order.json()['site_id'],a);self.assertEqual(order.json()['payment_context']['destination'],'Counter A')
  self.assertEqual(self.client.post('/v1/orders',headers=guest,json={**body,'site_id':foreign,'request_key':'foreign-order-test-001'}).status_code,404)
  self.client.post('/v1/orders/'+order.json()['id']+'/decision',headers=owner,json={'decision':'approved'});self.assertEqual(self.client.get('/v1/entitlements',headers=guest).json()[0]['site_id'],a)
  self.assertEqual(self.client.get('/v1/revenue',headers=owner,params={'site_id':a}).json()['net_collections'],3000)
  self.assertEqual(self.client.get('/v1/revenue/ledger',headers=owner).json()['items'][0]['site_id'],a)
 def test_branch_voucher_cannot_redeem_at_another_portal(self):
  owner=self.login('owner');guest=self.login('guest');a=self.site(owner,'A');b=self.site(owner,'B');self.publish(owner,a,'A');self.publish(owner,b,'B');p=self.package(owner)
  voucher=self.client.post('/v1/vouchers',headers=owner,json={'site_id':a,'package_id':p,'expires_days':1,'request_key':'site-voucher-test-001'}).json()
  sale=self.client.post('/v1/vouchers/'+voucher['id']+'/sale',headers=owner,json={'amount':3000,'method':'Cash','reference':'branch-sale','evidence':'Confirmed receipt','confirmed_received':True});self.assertEqual(sale.status_code,200)
  self.assertEqual(self.client.get('/v1/revenue',headers=owner,params={'site_id':a}).json()['net_collections'],3000)
  self.assertEqual(self.client.get('/v1/revenue',headers=owner,params={'site_id':b}).json()['net_collections'],0)
  self.assertEqual(self.client.post('/v1/vouchers/redeem',headers=guest,json={'site_id':b,'code':voucher['code']}).status_code,400)
  right=self.client.post('/v1/vouchers/redeem',headers=guest,json={'site_id':a,'code':voucher['code']});self.assertEqual(right.status_code,200);self.assertEqual(self.client.get('/v1/entitlements',headers=guest).json()[0]['site_id'],a)
 def test_bad_timezone_and_unpublished_sites(self):
  owner=self.login('owner');guest=self.login('guest');self.assertEqual(self.client.post('/v1/sites',headers=owner,json={'name':'Invalid','timezone':'not/a-zone'}).status_code,422)
  a=self.site(owner,'Unpublished');self.assertEqual(self.client.get('/v1/guest-sites',headers=guest).json(),[]);self.assertIsNone(self.client.get('/v1/portal/public',headers=guest,params={'site_id':a}).json()['settings'])
