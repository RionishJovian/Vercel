import unittest
import test_backend
from sqlalchemy import select,func
from backend.models import Voucher
class VoucherBatchTests(unittest.TestCase):
 setUp=test_backend.AccountPaymentTests.setUp
 tearDown=test_backend.AccountPaymentTests.tearDown
 login=test_backend.AccountPaymentTests.login
 package=test_backend.AccountPaymentTests.package
 def test_atomic_issue_replay_and_changed_count(self):
  h=self.login('owner');p=self.package(h);body={'package_id':p,'count':3,'expires_days':7,'request_key':'voucher-batch-test-001'}
  r=self.client.post('/v1/vouchers/batch',headers=h,json=body);self.assertEqual(r.status_code,201);items=r.json()['items'];self.assertEqual(len(items),3);self.assertEqual(len(set(v['code'] for v in items)),3)
  replay=self.client.post('/v1/vouchers/batch',headers=h,json=body);self.assertTrue(replay.json()['replayed']);self.assertTrue(all(v['code'] is None for v in replay.json()['items']))
  for changed in [{'count':2},{'expires_days':8}]:self.assertEqual(self.client.post('/v1/vouchers/batch',headers=h,json=body|changed).status_code,409)
  with self.app.state.factory() as s:
   self.assertEqual(s.scalar(select(func.count()).select_from(Voucher)),3)
   for v in s.scalars(select(Voucher)):self.assertNotIn(v.code_hash,[x['code'] for x in items])
 def test_permissions_limits_and_invalid_site_no_partial(self):
  h=self.login('owner');p=self.package(h);body={'package_id':p,'count':3,'expires_days':7,'request_key':'voucher-batch-test-002'}
  for name in ['guest','platform','other']:self.assertIn(self.client.post('/v1/vouchers/batch',headers=self.login(name),json=body).status_code,[403,404])
  for count in [0,101]:self.assertEqual(self.client.post('/v1/vouchers/batch',headers=h,json=body|{'count':count}).status_code,422)
  self.assertEqual(self.client.post('/v1/vouchers/batch',headers=h,json=body|{'site_id':'missing'}).status_code,404)
  with self.app.state.factory() as s:self.assertEqual(s.scalar(select(func.count()).select_from(Voucher)),0)
