import unittest
import test_backend
from backend.models import SubscriptionInvoice,TenantSubscription,BillingEvent
from sqlalchemy import select,func
class BillingTests(unittest.TestCase):
 setUp=test_backend.AccountPaymentTests.setUp
 tearDown=test_backend.AccountPaymentTests.tearDown
 login=test_backend.AccountPaymentTests.login
 def invoice(self):
  platform=self.login('platform');owner=self.login('owner');tenant=self.client.get('/v1/me',headers=owner).json()['tenant_id'];plan=self.client.post('/v1/platform/subscription-plans',headers=platform,json={'name':'Billing test','monthly_price':10000,'currency':'MMK','site_limit':2,'router_limit':3}).json()
  body={'tenant_id':tenant,'plan_id':plan['id'],'subscription_revision':0,'period_start':100,'period_end':200,'destination':'Test platform counter','instructions':'Test manual payment only','request_key':'invoice-test-key-001'}
  r=self.client.post('/v1/platform/subscription-invoices',headers=platform,json=body);self.assertEqual(r.status_code,201);return r.json(),body,platform,owner
 def submission(self,revision=1,**extra):return {'expected_revision':revision,'method':'Cash','reference':'billing-test-ref','evidence':'Test receipt evidence','request_key':'billing-payment-key-001'}|extra
 def test_approval_is_atomic_and_retries_have_single_history_effects(self):
  v,body,p,o=self.invoice();self.assertTrue(self.client.post('/v1/platform/subscription-invoices',headers=p,json=body).json()['replayed']);id=v['id'];url='/v1/subscription/invoices/'+id+'/payment';r=self.client.post(url,headers=o,json=self.submission());self.assertEqual(r.status_code,200);self.assertTrue(self.client.post(url,headers=o,json=self.submission()).json()['replayed'])
  review={'expected_revision':2,'decision':'approved','reason':'Counter receipt confirmed (test)','confirmed_received':True};decision='/v1/platform/subscription-invoices/'+id+'/decision';self.assertEqual(self.client.post(decision,headers=p,json=review|{'confirmed_received':False}).status_code,422)
  r=self.client.post(decision,headers=p,json=review);self.assertEqual(r.status_code,200);self.assertEqual(r.json()['status'],'paid');self.assertTrue(self.client.post(decision,headers=p,json=review).json()['replayed'])
  subscription=self.client.get('/v1/subscription',headers=o).json();self.assertEqual(subscription['payment_source_invoice_id'],id);self.assertEqual(subscription['payment_confirmation'],'operator_confirmed');self.assertFalse(subscription['payment_verified'])
  with self.app.state.factory() as s:self.assertEqual(s.scalar(select(func.count()).select_from(BillingEvent)),3)
 def test_isolation_evidence_rounds_and_historical_key_replay(self):
  v,body,p,o=self.invoice();id=v['id'];url='/v1/subscription/invoices/'+id+'/payment'
  self.assertEqual(self.client.post(url,headers=self.login('other'),json=self.submission()).status_code,404);self.assertEqual(self.client.get('/v1/subscription/invoices',headers=self.login('guest')).status_code,403)
  self.assertEqual(self.client.post(url,headers=o,json=self.submission()).status_code,200)
  review='/v1/platform/subscription-invoices/'+id+'/decision';self.assertEqual(self.client.post(review,headers=o,json={'expected_revision':2,'decision':'rejected','reason':'Test'}).status_code,403)
  for revision in [2,4]:
   self.assertEqual(self.client.post(review,headers=p,json={'expected_revision':revision,'decision':'needs_evidence','reason':'More receipt details (test)'}).status_code,200)
   if revision==2:self.assertEqual(self.client.post(url,headers=o,json=self.submission(3,request_key='billing-payment-key-002',evidence='Second test receipt')).status_code,200)
  replay=self.client.post(url,headers=o,json=self.submission(5));self.assertTrue(replay.json()['replayed']);self.assertEqual(replay.json()['status'],'needs_evidence')
  self.assertEqual(self.client.post(url,headers=o,json=self.submission(5,evidence='Conflicting reuse')).status_code,409)
  events=self.client.get('/v1/subscription/invoices/'+id+'/history',headers=o).json();self.assertEqual(len(events),5)
 def test_subscription_change_blocks_approval_without_partial_payment(self):
  v,body,p,o=self.invoice();id=v['id'];self.client.post('/v1/subscription/invoices/'+id+'/payment',headers=o,json=self.submission())
  changed=self.client.post('/v1/platform/businesses/'+v['tenant_id']+'/subscription',headers=p,json={'expected_revision':0,'plan_id':v['plan_id'],'state':'active','period_start':50,'period_end':100,'reason':'Other operator action (test)'});self.assertEqual(changed.status_code,200)
  r=self.client.post('/v1/platform/subscription-invoices/'+id+'/decision',headers=p,json={'expected_revision':2,'decision':'approved','reason':'Test','confirmed_received':True});self.assertEqual(r.status_code,409)
  with self.app.state.factory() as s:self.assertEqual(s.get(SubscriptionInvoice,id).status,'pending');self.assertEqual(s.get(TenantSubscription,v['tenant_id']).revision,1);self.assertEqual(s.scalar(select(func.count()).select_from(BillingEvent)),2)
