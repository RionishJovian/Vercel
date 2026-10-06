import unittest,time
from sqlalchemy import select
import test_backend as fixture
from backend.models import ReviewCredential,User,ExternalReviewAction,Entitlement,FinancialEvent,Outbox
class ReviewApiTests(unittest.TestCase):
 setUp=fixture.AccountPaymentTests.setUp
 tearDown=fixture.AccountPaymentTests.tearDown
 login=fixture.AccountPaymentTests.login
 package=fixture.AccountPaymentTests.package
 order=fixture.AccountPaymentTests.order
 def credential(self,h,can_decide=True):
  r=self.client.post('/v1/review-credentials',headers=h,json={'label':'Test bot','provider':'telegram','reviewer_identity':'123456','expires_days':1,'can_decide':can_decide,'authorize_service':True});self.assertEqual(r.status_code,201,r.text);return r.json()
 def setup_order(self):
  o=self.login('owner');g=self.login('guest');return o,g,self.order(g,self.package(o)).json()['id']
 def service(self,c):return {'Authorization':'Bearer '+c['token']}
 def decision(self,h,id,key='external-review-key-001',**changes):
  context=self.client.get('/v1/review-api/orders/'+id,headers=h).json();body={'request_key':key,'reviewer_identity':'123456','context_hash':context['context_hash'],'decision':{'decision':'approved','reason':'Manually verified payment','expected_receipt_revision':context['receipt_revision']},**changes};return self.client.post('/v1/review-api/orders/'+id+'/decision',headers=h,json=body),body
 def test_context_approval_retry_and_audited_service_identity(self):
  o,g,id=self.setup_order();c=self.credential(o);h=self.service(c)
  q=self.client.get('/v1/review-api/orders',headers=h);self.assertEqual(q.status_code,200,q.text);context=q.json()[0]
  for field in ['snapshot','payment_context','guest_username','site_name','reference','evidence','receipts','duplicate_reference_order_ids','context_hash']:self.assertIn(field,context)
  self.assertFalse(context['settlement_verified']);r,b=self.decision(h,id);self.assertEqual(r.status_code,200,r.text);self.assertFalse(r.json()['replayed'])
  replay=self.client.post('/v1/review-api/orders/'+id+'/decision',headers=h,json=b);self.assertTrue(replay.json()['replayed']);self.assertEqual(replay.json()['external_action_id'],r.json()['external_action_id'])
  self.assertEqual(self.client.post('/v1/review-api/orders/'+id+'/decision',headers=h,json={**b,'reviewer_identity':'wrong'}).status_code,403)
  changed={**b,'decision':{**b['decision'],'reason':'Different claim'}};self.assertEqual(self.client.post('/v1/review-api/orders/'+id+'/decision',headers=h,json=changed).status_code,409)
  actions=self.client.get('/v1/external-review-actions',headers=o).json();self.assertEqual(actions[0]['reviewer_identity'],'123456');self.assertEqual(actions[0]['service_label'],'Test bot')
  with self.app.state.factory() as s:
   self.assertEqual(len(list(s.scalars(select(Entitlement)))),1);self.assertEqual(len(list(s.scalars(select(FinancialEvent)))),1);self.assertEqual(len(list(s.scalars(select(ExternalReviewAction)))),1);self.assertEqual(len(list(s.scalars(select(Outbox)))),2)
 def test_scope_read_only_and_separate_login_tokens(self):
  o,g,id=self.setup_order();c=self.credential(o,False);h=self.service(c);other=self.login('other');foreign=self.service(self.credential(other))
  self.assertEqual(self.client.get('/v1/me',headers=h).status_code,401);self.assertEqual(self.client.get('/v1/review-api/orders',headers=o).status_code,401)
  self.assertEqual(self.client.get('/v1/review-api/orders/'+id,headers=foreign).status_code,404);self.assertEqual(self.client.get('/v1/review-api/orders',headers=foreign).json(),[])
  r,b=self.decision(h,id);self.assertEqual(r.status_code,403)
  for name in ['guest','platform']:
   user=self.login(name);self.assertEqual(self.client.get('/v1/review-credentials',headers=user).status_code,403)
  listing=self.client.get('/v1/review-credentials',headers=o).json();self.assertNotIn('token',listing[0]);self.assertNotIn('token_hash',listing[0])
  self.assertEqual(self.client.post('/v1/review-credentials/'+c['id']+'/revoke',headers=other,json={'reason':'foreign'}).status_code,404)
 def test_revoke_expiry_owner_disable_and_hashed_storage(self):
  o,g,id=self.setup_order();c=self.credential(o);h=self.service(c)
  with self.app.state.factory() as s:
   stored=s.get(ReviewCredential,c['id']);self.assertNotEqual(stored.token_hash,c['token']);self.assertEqual(len(stored.token_hash),64)
  self.client.post('/v1/review-credentials/'+c['id']+'/revoke',headers=o,json={'reason':'Rotating service key'})
  self.assertEqual(self.client.get('/v1/review-api/orders',headers=h).status_code,401)
  c=self.credential(o);h=self.service(c)
  with self.app.state.factory() as s:
   expired=s.get(ReviewCredential,c['id']);expired.created=int(time.time())-200;expired.expires=int(time.time())-1;s.commit()
  self.assertEqual(self.client.get('/v1/review-api/orders',headers=h).status_code,401)
  c=self.credential(o);h=self.service(c)
  with self.app.state.factory() as s:s.get(User,self.users['owner']).active=False;s.commit()
  self.assertEqual(self.client.get('/v1/review-api/orders',headers=h).status_code,401)
 def test_stale_context_and_receipts_cannot_be_approved(self):
  o,g,id=self.setup_order();h=self.service(self.credential(o));ctx=self.client.get('/v1/review-api/orders/'+id,headers=h).json()
  self.client.post('/v1/orders/'+id+'/decision',headers=o,json={'decision':'needs_evidence','reason':'Clarify evidence'})
  self.client.post('/v1/orders/'+id+'/evidence',headers=g,json={'evidence':'Updated payment evidence'})
  r,b=self.decision(h,id,context_hash=ctx['context_hash']);self.assertEqual(r.status_code,409)
  r,b=self.decision(h,id);self.assertEqual(r.status_code,200,r.text)
 def test_reasons_and_explicit_authorization_validation(self):
  o,g,id=self.setup_order();h=self.service(self.credential(o))
  ctx=self.client.get('/v1/review-api/orders/'+id,headers=h).json()
  for decision in [{'decision':'rejected','reason':'','expected_receipt_revision':0},{'decision':'approved'}]:
   r,b=self.decision(h,id,decision=decision);self.assertEqual(r.status_code,422)
  self.assertEqual(self.client.get('/v1/review-api/orders?limit=101',headers=h).status_code,422)
 def test_private_receipt_context_and_external_request_evidence_then_reject(self):
  from io import BytesIO
  from PIL import Image
  o,g,id=self.setup_order();h=self.service(self.credential(o));old=self.client.get('/v1/review-api/orders/'+id,headers=h).json()
  img=BytesIO();Image.new('RGB',(10,10),'red').save(img,format='PNG')
  receipt=self.client.post('/v1/orders/'+id+'/receipts',headers={**g,'Content-Type':'application/octet-stream','X-Upload-Key':'external-receipt-key-001'},content=img.getvalue()).json()
  view='/v1/review-api/orders/'+id+'/receipts/'+receipt['id'];self.assertEqual(self.client.get(view,headers=h).status_code,200);self.assertEqual(self.client.get(view,headers=o).status_code,401)
  foreign=self.service(self.credential(self.login('other')));self.assertEqual(self.client.get(view,headers=foreign).status_code,404)
  r,b=self.decision(h,id,context_hash=old['context_hash']);self.assertEqual(r.status_code,409)
  r,b=self.decision(h,id,decision={'decision':'needs_evidence','reason':'Please send full reference','expected_receipt_revision':1});self.assertEqual(r.status_code,200,r.text)
  self.client.post('/v1/orders/'+id+'/evidence',headers=g,json={'evidence':'Reference corrected'})
  r,b=self.decision(h,id,key='external-review-key-002',decision={'decision':'rejected','reason':'Payment not found after manual verification','expected_receipt_revision':1});self.assertEqual(r.status_code,200,r.text)
  self.assertEqual(self.client.get('/v1/revenue',headers=o).json()['net_collections'],0);self.assertEqual(len(self.client.get('/v1/external-review-actions',headers=o).json()),2)
