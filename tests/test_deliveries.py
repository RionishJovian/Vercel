import unittest,time
from sqlalchemy import select
import test_backend as fixture
from backend.models import EventDelivery,Outbox,ReviewCredential
class DeliveryTests(unittest.TestCase):
 setUp=fixture.AccountPaymentTests.setUp
 tearDown=fixture.AccountPaymentTests.tearDown
 login=fixture.AccountPaymentTests.login
 package=fixture.AccountPaymentTests.package
 order=fixture.AccountPaymentTests.order
 def credential(self,o,enabled=True):
  r=self.client.post('/v1/review-credentials',headers=o,json={'label':'Polling test service','provider':'custom','reviewer_identity':'test-reviewer','authorize_service':True,'receive_events':enabled});self.assertEqual(r.status_code,201,r.text);return r.json()
 def auth(self,c):return {'Authorization':'Bearer '+c['token']}
 def claim(self,h,**body):return self.client.post('/v1/review-api/events/claim',headers=h,json={'limit':10,'lease_seconds':30,**body})
 def complete(self,h,d,result='acknowledged',error=''):return self.client.post('/v1/review-api/events/'+d['id']+'/complete',headers=h,json={'lease_token':d['lease_token'],'result':result,'error':error})
 def test_per_service_acknowledgement_and_no_external_delivery_claim(self):
  o=self.login('owner');g=self.login('guest');c1=self.credential(o);c2=self.credential(o);id=self.order(g,self.package(o)).json()['id']
  d1=self.claim(self.auth(c1)).json()[0];d2=self.claim(self.auth(c2)).json()[0];self.assertEqual(d1['event'],'payment.submitted');self.assertEqual(d1['payload']['status'],'pending');self.assertEqual(d1['event_id'],d2['event_id']);self.assertNotEqual(d1['id'],d2['id'])
  self.assertEqual(self.claim(self.auth(c1)).json(),[])
  r=self.complete(self.auth(c1),d1);self.assertEqual(r.status_code,200,r.text);self.assertEqual(r.json()['state'],'acknowledged');self.assertTrue(self.complete(self.auth(c1),d1).json()['replayed'])
  self.assertEqual(self.complete(self.auth(c2),d1).status_code,404)
  with self.app.state.factory() as s:
   self.assertFalse(s.get(Outbox,d1['event_id']).delivered);self.assertEqual(s.get(EventDelivery,d2['id']).state,'leased');self.assertNotIn(d1['lease_token'],s.get(EventDelivery,d1['id']).lease_hash)
 def test_retry_backoff_lease_fencing_and_manual_failed_retry(self):
  o=self.login('owner');g=self.login('guest');c=self.credential(o);h=self.auth(c);self.order(g,self.package(o));d=self.claim(h).json()[0]
  r=self.complete(h,d,'retry','Provider temporarily unavailable');self.assertEqual(r.status_code,200);self.assertGreater(r.json()['available_at'],int(time.time()));self.assertEqual(self.claim(h).json(),[]);self.assertTrue(self.complete(h,d,'retry','Provider temporarily unavailable').json()['replayed'])
  with self.app.state.factory() as s:s.get(EventDelivery,d['id']).available_at=0;s.commit()
  next_d=self.claim(h).json()[0];self.assertNotEqual(next_d['lease_token'],d['lease_token']);self.assertEqual(self.complete(h,d).status_code,409)
  with self.app.state.factory() as s:
   row=s.get(EventDelivery,d['id']);row.lease_expires=0;row.cycle_attempts=5;s.commit()
  self.assertEqual(self.claim(h).json(),[])
  rows=self.client.get('/v1/event-deliveries?state=failed',headers=o).json();self.assertEqual(len(rows),1)
  retried=self.client.post('/v1/event-deliveries/'+d['id']+'/retry',headers=o,json={'reason':'Service recovered'});self.assertEqual(retried.status_code,200,retried.text);self.assertEqual(retried.json()['attempts'],2);self.assertEqual(retried.json()['cycle_attempts'],0)
  final=self.claim(h).json()[0];self.assertEqual(final['attempts'],3);self.assertEqual(self.complete(h,final).status_code,200)
 def test_repeated_evidence_rounds_emit_distinct_immutable_events(self):
  o=self.login('owner');g=self.login('guest');h=self.auth(self.credential(o));id=self.order(g,self.package(o)).json()['id']
  for n in range(2):
   self.assertEqual(self.client.post('/v1/orders/'+id+'/decision',headers=o,json={'decision':'needs_evidence','reason':f'Clarify round {n}'}).status_code,200)
   self.assertEqual(self.client.post('/v1/orders/'+id+'/evidence',headers=g,json={'evidence':f'Revised evidence {n}'}).status_code,200)
  events=self.claim(h).json();self.assertEqual(len(events),5);self.assertEqual([d['event_revision'] for d in events],[1,2,3,4,5]);self.assertEqual(events[0]['payload']['evidence'],'received at counter');self.assertEqual(events[-1]['payload']['evidence'],'Revised evidence 1')
  self.assertEqual(self.client.get('/v1/revenue',headers=o).json()['net_collections'],0)
 def test_pause_scope_revocation_and_no_retroactive_fanout(self):
  o=self.login('owner');g=self.login('guest');self.order(g,self.package(o));c=self.credential(o,False);h=self.auth(c)
  self.assertEqual(self.claim(h).status_code,403)
  self.client.post('/v1/review-credentials/'+c['id']+'/events',headers=o,json={'receive_events':True});self.assertEqual(self.claim(h).json(),[])
  foreign=self.login('other');self.assertEqual(self.client.get('/v1/event-deliveries',headers=foreign).json(),[]);self.assertEqual(self.client.post('/v1/review-credentials/'+c['id']+'/events',headers=foreign,json={'receive_events':True}).status_code,404)
  self.client.post('/v1/review-credentials/'+c['id']+'/revoke',headers=o,json={'reason':'Offboarding'});self.assertEqual(self.claim(h).status_code,401)
