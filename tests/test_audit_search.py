import unittest,time
import test_backend
from backend.models import Audit,User
class AuditSearchTests(unittest.TestCase):
 setUp=test_backend.AccountPaymentTests.setUp
 tearDown=test_backend.AccountPaymentTests.tearDown
 login=test_backend.AccountPaymentTests.login
 def test_scope_literal_search_and_page(self):
  with self.app.state.factory() as s:
   for name,target in [('owner','marker%'),('guest','marker_guest'),('other','marker_other')]:
    u=s.get(User,self.users[name]);s.add(Audit(actor_id=u.id,tenant_id=u.tenant_id,action='fixture',target=target,created=100))
   s.commit()
  h=self.login('owner');r=self.client.get('/v1/audit?search=marker&action=fixture',headers=h).json();self.assertEqual(len(r),2)
  self.assertEqual(len(self.client.get('/v1/audit?search=%25',headers=h).json()),1)
  self.assertEqual(len(self.client.get('/v1/audit?action=fixture',headers=self.login('guest')).json()),1)
  self.assertEqual(len(self.client.get('/v1/audit?action=fixture',headers=self.login('platform')).json()),3)
  a=self.client.get('/v1/audit?action=fixture&limit=1',headers=h).json();b=self.client.get('/v1/audit?action=fixture&limit=1&offset=1',headers=h).json();self.assertNotEqual(a[0]['id'],b[0]['id'])
  self.assertEqual(self.client.get('/v1/audit?start=101&action=fixture',headers=h).json(),[])
  self.assertEqual(self.client.get('/v1/audit?start=2&end=1',headers=h).status_code,422)
  self.assertEqual(self.client.get('/v1/audit?limit=201',headers=h).status_code,422)
