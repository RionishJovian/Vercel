import unittest
import test_backend
class AccountSessionTests(unittest.TestCase):
 setUp=test_backend.AccountPaymentTests.setUp
 tearDown=test_backend.AccountPaymentTests.tearDown
 login=test_backend.AccountPaymentTests.login
 def test_own_session_listing_and_scoped_revoke(self):
  first=self.login('owner');second=self.login('owner');other=self.login('other');rows=self.client.get('/v1/account/sessions',headers=first).json();self.assertEqual(len(rows),2);self.assertEqual(sum(r['current'] for r in rows),1);target=next(r for r in rows if not r['current'])
  self.assertEqual(self.client.post('/v1/account/sessions/revoke',headers=other,json={'session_ref':target['session_ref']}).status_code,404)
  self.assertEqual(self.client.post('/v1/account/sessions/revoke',headers=first,json={'session_ref':target['session_ref']}).status_code,200)
  self.assertEqual(self.client.get('/v1/me',headers=second).status_code,401);self.assertEqual(self.client.get('/v1/me',headers=first).status_code,200)
  self.assertEqual(self.client.post('/v1/account/sessions/revoke',headers=first,json={'session_ref':next(r for r in rows if r['current'])['session_ref']}).status_code,409)
 def test_end_others_requires_confirmation_and_preserves_current(self):
  for role in ['platform','guest']:
   first=self.login(role);second=self.login(role)
   self.assertEqual(self.client.post('/v1/account/sessions/revoke-others',headers=first,json={'confirmed':False}).status_code,422)
   r=self.client.post('/v1/account/sessions/revoke-others',headers=first,json={'confirmed':True});self.assertEqual(r.json()['revoked_count'],1);self.assertFalse(r.json()['service_credentials_changed']);self.assertEqual(self.client.get('/v1/me',headers=first).status_code,200);self.assertEqual(self.client.get('/v1/me',headers=second).status_code,401)
