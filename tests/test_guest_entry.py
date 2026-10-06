import unittest,time
import test_backend
from backend.models import Site,SitePortal,User
from sqlalchemy import select
class GuestEntryTests(unittest.TestCase):
 setUp=test_backend.AccountPaymentTests.setUp
 tearDown=test_backend.AccountPaymentTests.tearDown
 login=test_backend.AccountPaymentTests.login
 package=test_backend.AccountPaymentTests.package
 def site(self):
  with self.app.state.factory() as s:
   u=s.get(User,self.users['owner']);site=Site(tenant_id=u.tenant_id,name='Guest branch',timezone='Asia/Yangon',created=int(time.time()));s.add(site);s.flush()
   from backend.models import PortalSettings
   cfg=s.get(PortalSettings,u.tenant_id).published.copy();s.add(SitePortal(site_id=site.id,revision=1,published_revision=1,draft=cfg,published=cfg,updated=int(time.time())));s.commit();return site.id
 def body(self,**extra):return {'username':'new-guest','password':'long-test-password','accepted_terms':True,'revision':1}|extra
 def test_public_projection_and_signup_scope(self):
  id=self.site();self.package(self.login('owner'));r=self.client.get('/v1/public/sites/'+id);self.assertEqual(r.status_code,200);self.assertNotIn('guest_subnet',r.json()['settings']);self.assertEqual(len(r.json()['packages']),1)
  r=self.client.post('/v1/public/sites/'+id+'/register',json=self.body());self.assertEqual(r.status_code,201)
  token=self.client.post('/v1/auth/login',json={'username':'new-guest','password':'long-test-password'}).json()['access_token'];u=self.client.get('/v1/me',headers={'Authorization':'Bearer '+token}).json();self.assertEqual(u['role'],'guest')
  with self.app.state.factory() as s:self.assertEqual(u['tenant_id'],s.get(Site,id).tenant_id)
  self.assertEqual(self.client.post('/v1/public/sites/'+id+'/register',json=self.body()).status_code,409)
 def test_terms_revision_role_and_unpublished(self):
  id=self.site();url='/v1/public/sites/'+id+'/register'
  for body,code in [(self.body(accepted_terms=False),422),(self.body(revision=0),409),(self.body(role='sme'),422)]:self.assertEqual(self.client.post(url,json=body).status_code,code)
  with self.app.state.factory() as s:s.get(SitePortal,id).published=None;s.commit()
  self.assertEqual(self.client.get('/v1/public/sites/'+id).status_code,404);self.assertEqual(self.client.post(url,json=self.body()).status_code,404)
 def test_registration_budget(self):
  id=self.site();url='/v1/public/sites/'+id+'/register'
  for i in range(8):self.assertEqual(self.client.post(url,json=self.body(username='g'+str(i)+'x')).status_code,201)
  self.assertEqual(self.client.post(url,json=self.body()).status_code,429)
