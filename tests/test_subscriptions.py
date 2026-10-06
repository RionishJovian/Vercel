import unittest
import test_backend
class SubscriptionTests(unittest.TestCase):
 setUp=test_backend.AccountPaymentTests.setUp
 tearDown=test_backend.AccountPaymentTests.tearDown
 login=test_backend.AccountPaymentTests.login
 def test_plan_assignment_snapshot_role_and_revision(self):
  h=self.login('platform');owner=self.login('owner');tenant=self.client.get('/v1/me',headers=owner).json()['tenant_id'];plan={'name':'Owner-defined test','monthly_price':10000,'currency':'MMK','site_limit':2,'router_limit':3}
  self.assertEqual(self.client.post('/v1/platform/subscription-plans',headers=owner,json=plan).status_code,403)
  p=self.client.post('/v1/platform/subscription-plans',headers=h,json=plan);self.assertEqual(p.status_code,201)
  body={'expected_revision':0,'plan_id':p.json()['id'],'state':'trial','period_start':100,'period_end':200,'reason':'Test assignment'};url='/v1/platform/businesses/'+tenant+'/subscription';r=self.client.post(url,headers=h,json=body);self.assertEqual(r.status_code,200);self.assertEqual(r.json()['plan_snapshot']['monthly_price'],10000);self.assertFalse(r.json()['payment_verified']);self.assertEqual(r.json()['enforcement'],'not_connected');self.assertEqual(self.client.post(url,headers=h,json=body).status_code,409)
  self.assertEqual(self.client.get('/v1/subscription',headers=owner).json()['state'],'trial');self.assertEqual(self.client.get('/v1/subscription',headers=self.login('other')).json()['state'],'unassigned');self.assertEqual(self.client.get('/v1/subscription',headers=self.login('guest')).status_code,403)
  self.assertEqual(self.client.post(url,headers=h,json=body|{'expected_revision':1,'period_end':50}).status_code,422)
