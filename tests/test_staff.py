import unittest
import test_backend
from sqlalchemy import select
from backend.models import User,StaffGrant
class StaffTests(unittest.TestCase):
 setUp=test_backend.AccountPaymentTests.setUp
 tearDown=test_backend.AccountPaymentTests.tearDown
 login=test_backend.AccountPaymentTests.login
 package=test_backend.AccountPaymentTests.package
 order=test_backend.AccountPaymentTests.order
 def create(self,profile):
  h=self.login('owner');r=self.client.post('/v1/staff',headers=h,json={'username':'staff-'+profile,'profile':profile,'request_key':'staff-request-key-'+profile});self.assertEqual(r.status_code,201)
  first=self.client.post('/v1/auth/login',json={'username':'staff-'+profile,'password':r.json()['temporary_password']});auth={'Authorization':'Bearer '+first.json()['access_token']};self.assertEqual(self.client.get('/v1/sites',headers=auth).status_code,403)
  changed=self.client.post('/v1/auth/password',headers=auth,json={'current_password':r.json()['temporary_password'],'new_password':'staff-test-personal-password'});self.assertEqual(changed.status_code,200);return r.json()['staff'],{'Authorization':'Bearer '+changed.json()['access_token']}
 def test_cashier_and_technician_cannot_escalate(self):
  for profile in ['cashier','technician','manager']:
   staff,h=self.create(profile);me=self.client.get('/v1/me',headers=h).json();self.assertEqual(me['staff_profile'],profile)
   for path in ['staff','review-credentials','platform/businesses']:self.assertEqual(self.client.get('/v1/'+path,headers=h).status_code,403)
   self.assertEqual(self.client.get('/v1/orders',headers=h).status_code,403 if profile=='technician' else 200)
   self.assertEqual(self.client.get('/v1/router-plans',headers=h).status_code,403 if profile=='cashier' else 200)
   self.assertEqual(self.client.post('/v1/orders/missing/refund',headers=h,json={'reason':'Test only'}).status_code,403 if profile!='manager' else 404)
 def test_create_retry_change_revision_and_session_revocation(self):
  staff,h=self.create('cashier');owner=self.login('owner');body={'username':'staff-cashier','profile':'cashier','request_key':'staff-request-key-cashier'}
  replay=self.client.post('/v1/staff',headers=owner,json=body).json();self.assertTrue(replay['replayed']);self.assertNotIn('temporary_password',replay)
  self.assertEqual(self.client.post('/v1/staff',headers=owner,json=body|{'profile':'manager'}).status_code,409)
  update={'expected_revision':1,'profile':'cashier','active':False,'reason':'Employment ended (test)'};r=self.client.post('/v1/staff/'+staff['id'],headers=owner,json=update);self.assertEqual(r.status_code,200);self.assertEqual(r.json()['last_reason'],update['reason']);self.assertEqual(self.client.get('/v1/me',headers=h).status_code,401)
  self.assertEqual(self.client.post('/v1/staff/'+staff['id'],headers=owner,json=update).status_code,409)
  with self.app.state.factory() as s:
   a=s.get(User,staff['id']);self.assertFalse(a.active);self.assertNotEqual(a.password_hash,'staff-test-personal-password')
  businesses=self.client.get('/v1/platform/businesses',headers=self.login('platform')).json();self.assertFalse(any(o['id']==staff['id'] for b in businesses for o in b['owners']))
 def test_recovery_requires_owner_confirmation_and_preserves_disable(self):
  staff,h=self.create('technician');owner=self.login('owner');url='/v1/staff/'+staff['id'];body={'expected_revision':1,'profile':'technician','active':False,'reason':'Recovery test disable'};self.assertEqual(self.client.post(url,headers=owner,json=body).status_code,200)
  reset=body|{'expected_revision':2,'confirmed_authority':True};self.assertEqual(self.client.post(url+'/reset-password',headers=owner,json=reset|{'confirmed_authority':False}).status_code,422)
  r=self.client.post(url+'/reset-password',headers=owner,json=reset);self.assertEqual(r.status_code,200);self.assertFalse(r.json()['staff']['active']);self.assertTrue(r.json()['staff']['must_change_password']);self.assertEqual(r.json()['staff']['revision'],3)
  self.assertEqual(self.client.post('/v1/auth/login',json={'username':'staff-technician','password':r.json()['temporary_password']}).status_code,401)
 def test_cashier_can_review_payment_and_record_counter_sale(self):
  staff,h=self.create('cashier');p=self.package(self.login('owner'));o=self.order(self.login('guest'),p).json()
  reviewed=self.client.post('/v1/orders/'+o['id']+'/decision',headers=h,json={'decision':'approved','expected_receipt_revision':0});self.assertEqual(reviewed.status_code,200);self.assertEqual(reviewed.json()['reviewer_id'],staff['id'])
  self.assertEqual(self.client.post('/v1/orders/'+o['id']+'/refund',headers=h,json={'reason':'Not allowed'}).status_code,403)
  voucher=self.client.post('/v1/vouchers',headers=h,json={'package_id':p,'request_key':'cashier-voucher-test-001'});self.assertEqual(voucher.status_code,201)
  sale=self.client.post('/v1/vouchers/'+voucher.json()['id']+'/sale',headers=h,json={'amount':3000,'method':'Cash','reference':'cashier-counter-test','evidence':'Counter confirmation (test)','confirmed_received':True});self.assertEqual(sale.status_code,200)
