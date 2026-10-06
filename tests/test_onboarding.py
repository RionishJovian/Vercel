import unittest,time
from sqlalchemy import select
import test_backend as fixture
from backend.models import User,Tenant,BusinessProvisioning,PortalSettings,ReviewCredential,PasswordResetRecord
from backend.app import HASHER
class OnboardingTests(unittest.TestCase):
 setUp=fixture.AccountPaymentTests.setUp
 tearDown=fixture.AccountPaymentTests.tearDown
 login=fixture.AccountPaymentTests.login
 def create(self,p,**extra):return self.client.post('/v1/platform/businesses',headers=p,json={'name':'New Cafe','owner_username':'new-owner','request_key':'business-create-key-001',**extra})
 def temp_login(self,d):
  r=self.client.post('/v1/auth/login',json={'username':d['owner']['username'],'password':d['temporary_password']});self.assertEqual(r.status_code,200,r.text);return {'Authorization':'Bearer '+r.json()['access_token']}
 def test_create_replay_isolation_and_first_password_gate(self):
  p=self.login('platform');r=self.create(p);self.assertEqual(r.status_code,201,r.text);d=r.json();h=self.temp_login(d)
  self.assertTrue(self.client.get('/v1/me',headers=h).json()['must_change_password'])
  for path in ['tenants','packages','portal','platform/businesses']:self.assertEqual(self.client.get('/v1/'+path,headers=h).status_code,403)
  self.assertEqual(self.create(p).json()['id'],d['id']);self.assertNotIn('temporary_password',self.create(p).json())
  self.assertEqual(self.create(p,name='Another business').status_code,409)
  changed=self.client.post('/v1/auth/password',headers=h,json={'current_password':d['temporary_password'],'new_password':'my-new-owner-password'});self.assertEqual(changed.status_code,200,changed.text);new={'Authorization':'Bearer '+changed.json()['access_token']}
  self.assertEqual(self.client.get('/v1/me',headers=h).status_code,401);self.assertFalse(self.client.get('/v1/me',headers=new).json()['must_change_password']);self.assertEqual(self.client.get('/v1/tenants',headers=new).json()[0]['id'],d['id'])
  portal=self.client.get('/v1/portal',headers=new).json();self.assertIsNone(portal['published']);self.assertFalse(portal['draft']['direct_purchase']);self.assertFalse(portal['draft']['voucher_login'])
  with self.app.state.factory() as s:
   a=s.get(User,d['owner']['id']);self.assertTrue(HASHER.verify(a.password_hash,'my-new-owner-password'));self.assertNotEqual(a.password_hash,d['temporary_password']);self.assertEqual(len(list(s.scalars(select(BusinessProvisioning)))),1)
 def test_platform_permissions_duplicate_username_and_no_orphan_business(self):
  p=self.login('platform')
  for name in ['owner','guest']:
   h=self.login(name);self.assertEqual(self.create(h).status_code,403);self.assertEqual(self.client.get('/v1/platform/businesses',headers=h).status_code,403)
  self.assertEqual(self.create(p,owner_username='OWNER').status_code,409)
  with self.app.state.factory() as s:self.assertEqual(len(list(s.scalars(select(Tenant)))),2)
  for username in ['bad name','!bad','a']:
   self.assertEqual(self.create(p,owner_username=username).status_code,422)
 def test_reset_revokes_sessions_and_external_credentials_and_preserves_business(self):
  p=self.login('platform');o=self.login('owner');credential=self.client.post('/v1/review-credentials',headers=o,json={'label':'Test service','provider':'custom','reviewer_identity':'identity','authorize_service':True}).json()
  business=self.client.get('/v1/platform/businesses',headers=p).json();b=next(b for b in business if any(a['username']=='owner' for a in b['owners']));a=next(a for a in b['owners'] if a['username']=='owner');url='/v1/platform/businesses/'+b['id']+'/owners/'+a['id']+'/reset-password'
  self.assertEqual(self.client.post(url,headers=o,json={'reason':'Recovery','confirmed_authority':True}).status_code,403)
  self.assertEqual(self.client.post(url,headers=p,json={'reason':'Recovery','confirmed_authority':False}).status_code,422)
  r=self.client.post(url,headers=p,json={'reason':'Verified owner recovery','confirmed_authority':True});self.assertEqual(r.status_code,200,r.text)
  self.assertEqual(self.client.get('/v1/me',headers=o).status_code,401);self.assertEqual(self.client.get('/v1/review-api/orders',headers={'Authorization':'Bearer '+credential['token']}).status_code,401)
  with self.app.state.factory() as s:self.assertEqual(s.get(ReviewCredential,credential['id']).revoke_reason,'Platform password reset: Verified owner recovery');self.assertEqual(len(list(s.scalars(select(PasswordResetRecord)))),1)
  h=self.temp_login(r.json());self.assertEqual(self.client.get('/v1/review-credentials',headers=h).status_code,403)
 def test_temporary_expiry_password_validation_and_attempt_limit(self):
  p=self.login('platform');d=self.create(p).json();h=self.temp_login(d)
  self.assertEqual(self.client.post('/v1/auth/password',headers=h,json={'current_password':d['temporary_password'],'new_password':'short'}).status_code,422)
  self.assertEqual(self.client.post('/v1/auth/password',headers=h,json={'current_password':d['temporary_password'],'new_password':d['temporary_password']}).status_code,422)
  for _ in range(8):self.assertEqual(self.client.post('/v1/auth/password',headers=h,json={'current_password':'wrong','new_password':'new-personal-password'}).status_code,400)
  self.assertEqual(self.client.post('/v1/auth/password',headers=h,json={'current_password':d['temporary_password'],'new_password':'new-personal-password'}).status_code,429)
  with self.app.state.factory() as s:s.get(User,d['owner']['id']).temporary_password_expires=int(time.time())-1;s.commit()
  self.assertEqual(self.client.get('/v1/me',headers=h).status_code,401)
  self.assertEqual(self.client.post('/v1/auth/login',json={'username':'new-owner','password':d['temporary_password']}).status_code,401)
  reset=self.client.post('/v1/platform/businesses/'+d['id']+'/owners/'+d['owner']['id']+'/reset-password',headers=p,json={'reason':'Verified recovery after expiry','confirmed_authority':True}).json();fresh=self.temp_login(reset)
  self.assertEqual(self.client.post('/v1/auth/password',headers=fresh,json={'current_password':reset['temporary_password'],'new_password':'recovered-owner-password'}).status_code,200)
 def test_existing_account_rotation_revokes_other_sessions_and_reset_history_is_private(self):
  o=self.login('owner');other_session=self.login('owner');g=self.login('guest')
  r=self.client.post('/v1/auth/password',headers=o,json={'current_password':'test-password-only','new_password':'changed-owner-password'});self.assertEqual(r.status_code,200,r.text)
  for h in [o,other_session]:self.assertEqual(self.client.get('/v1/me',headers=h).status_code,401)
  self.assertEqual(self.client.get('/v1/me',headers=g).status_code,200)
  new={'Authorization':'Bearer '+r.json()['access_token']};self.assertEqual(self.client.get('/v1/me',headers=new).json()['role'],'sme');self.assertFalse(self.client.get('/v1/me',headers=new).json()['must_change_password'])
  self.assertEqual(self.client.get('/v1/platform/password-resets',headers=new).status_code,403)
  p=self.login('platform');b=self.client.get('/v1/platform/businesses',headers=p).json();cafe=next(x for x in b if any(a['username']=='owner' for a in x['owners']));owner=next(a for a in cafe['owners'] if a['username']=='owner')
  self.client.post('/v1/platform/businesses/'+cafe['id']+'/owners/'+owner['id']+'/reset-password',headers=p,json={'reason':'Verified recovery history','confirmed_authority':True})
  history=self.client.get('/v1/platform/password-resets',headers=p).json();self.assertEqual(history[0]['reason'],'Verified recovery history');self.assertNotIn('temporary_password',history[0])
