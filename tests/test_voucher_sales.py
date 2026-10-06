import unittest
import test_vouchers as fixture
from sqlalchemy import select
from backend.models import VoucherSale,Entitlement
class VoucherSaleTests(unittest.TestCase):
 setUp=fixture.VoucherTests.setUp
 tearDown=fixture.VoucherTests.tearDown
 login=fixture.VoucherTests.login
 package=fixture.VoucherTests.package
 setup_voucher=fixture.VoucherTests.setup_voucher
 redeem=fixture.VoucherTests.redeem
 def payment(self,id,headers=None,**overrides):return self.client.post('/v1/vouchers/'+id+'/sale',headers=headers or self.owner,json={'amount':3000,'method':'Cash','reference':'receipt-01','evidence':'Confirmed at test counter','confirmed_received':True,**overrides})
 def refund(self,id,**overrides):return self.client.post('/v1/vouchers/'+id+'/sale/refund',headers=self.owner,json={'reason':'Returned at counter','reference':'refund-01','confirmed_returned':True,**overrides})
 def test_sale_retries_count_once_and_redemption_does_not_add_revenue(self):
  v=self.setup_voucher();self.assertEqual(self.payment(v['id']).status_code,200);self.assertTrue(self.payment(v['id']).json()['replayed'])
  self.redeem(v['code']);r=self.client.get('/v1/revenue',headers=self.owner).json();self.assertEqual(r['net_collections'],3000);self.assertEqual(r['voucher_sales'],1);self.assertEqual(r['approved_orders'],0);self.assertEqual(r['by_service']['combined'],3000)
  self.assertEqual(self.payment(v['id'],reference='different').status_code,409)
  with self.app.state.factory() as s:self.assertEqual(len(list(s.scalars(select(VoucherSale)))),1);self.assertEqual(len(list(s.scalars(select(Entitlement)))),1)
 def test_refund_reduces_collections_and_revokes_redeemed_pass(self):
  v=self.setup_voucher();self.payment(v['id']);self.redeem(v['code']);self.assertEqual(self.refund(v['id']).status_code,200);self.assertTrue(self.refund(v['id']).json()['replayed'])
  r=self.client.get('/v1/revenue',headers=self.owner).json();self.assertEqual(r['net_collections'],0);self.assertEqual(r['gross_collections'],3000);self.assertEqual(r['refunds'],3000);self.assertTrue(self.client.get('/v1/entitlements',headers=self.guest).json()[0]['revoked']);self.assertEqual(self.refund(v['id'],reference='changed').status_code,409)
 def test_permissions_amount_and_receipt_confirmation(self):
  v=self.setup_voucher();other=self.login('other');self.assertEqual(self.payment(v['id'],headers=other).status_code,404);self.assertEqual(self.payment(v['id'],headers=self.guest).status_code,403)
  self.assertEqual(self.payment(v['id'],amount=2000).status_code,422);self.assertEqual(self.payment(v['id'],confirmed_received=False).status_code,422);self.assertEqual(self.payment(v['id'],reference='').status_code,422)
  self.assertEqual(self.client.get('/v1/revenue',headers=self.owner).json()['net_collections'],0)
 def test_revocation_does_not_imply_refund_and_no_sale_for_revoked_code(self):
  v=self.setup_voucher();self.payment(v['id']);self.client.post('/v1/vouchers/'+v['id']+'/revoke',headers=self.owner,json={'reason':'Access stopped'})
  self.assertEqual(self.client.get('/v1/revenue',headers=self.owner).json()['net_collections'],3000);self.assertEqual(self.refund(v['id'],confirmed_returned=False).status_code,422)
  fresh=self.client.post('/v1/vouchers',headers=self.owner,json={**self.body,'request_key':'new-test-voucher-key'}).json();self.client.post('/v1/vouchers/'+fresh['id']+'/revoke',headers=self.owner,json={'reason':'Lost'})
  self.assertEqual(self.payment(fresh['id']).status_code,409)

 def test_concurrent_sale_does_not_duplicate_collections(self):
  from concurrent.futures import ThreadPoolExecutor
  v=self.setup_voucher()
  with ThreadPoolExecutor(max_workers=2) as pool:results=list(pool.map(lambda _:self.payment(v['id']),range(2)))
  self.assertTrue(any(r.status_code==200 for r in results));self.assertTrue(all(r.status_code in [200,409] for r in results))
  with self.app.state.factory() as s:self.assertEqual(len(list(s.scalars(select(VoucherSale)))),1)
  self.assertEqual(self.client.get('/v1/revenue',headers=self.owner).json()['net_collections'],3000)

 def test_guest_order_and_voucher_sale_are_counted_separately(self):
  v=self.setup_voucher();self.payment(v['id'])
  order=self.client.post('/v1/orders',headers=self.guest,json={'package_id':self.pkg,'portal_revision':3,'method':'Cash','reference':'guest-payment','evidence':'Test guest receipt','request_key':'separate-guest-order-001'}).json()
  self.client.post('/v1/orders/'+order['id']+'/decision',headers=self.owner,json={'decision':'approved'})
  r=self.client.get('/v1/revenue',headers=self.owner).json();self.assertEqual(r['net_collections'],6000);self.assertEqual(r['by_source'],{'guest_orders':3000,'voucher_counter_sales':3000});self.assertEqual(r['by_service']['combined'],6000)
  self.client.post('/v1/orders/'+order['id']+'/refund',headers=self.owner,json={'reason':'Test manual return'})
  r=self.client.get('/v1/revenue',headers=self.owner).json();self.assertEqual(r['net_collections'],3000);self.assertEqual(r['refunds'],3000);self.assertEqual(r['voucher_sales'],1)
