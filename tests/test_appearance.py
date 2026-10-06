import copy,unittest
from concurrent.futures import ThreadPoolExecutor
import test_backend as fixture
from backend.appearance import DEFAULT,contrast_checks
from sqlalchemy import select
from backend.models import Audit
class AppearanceTests(unittest.TestCase):
 setUp=fixture.AccountPaymentTests.setUp
 tearDown=fixture.AccountPaymentTests.tearDown
 login=fixture.AccountPaymentTests.login
 def draft(self,h,c,rev=0):return self.client.post('/v1/appearance/draft',headers=h,json={'expected_revision':rev,'settings':c})
 def test_drafts_stay_private_and_publication_scopes_are_enforced(self):
  h=self.login('platform');c=copy.deepcopy(DEFAULT);c['light']['accent']='#9abbac';c['scopes']=['guest']
  r=self.draft(h,c);self.assertEqual(r.status_code,200,r.text);self.assertEqual(r.json()['revision'],1)
  self.assertEqual(self.client.get('/v1/appearance/public').json()['light']['accent'],DEFAULT['light']['accent'])
  self.assertEqual(self.client.get('/v1/appearance',headers=self.login('owner')).status_code,403)
  p=self.client.post('/v1/appearance/publish',headers=h,json={'expected_revision':1});self.assertEqual(p.status_code,200,p.text)
  self.assertEqual(self.client.get('/v1/appearance/effective',headers=self.login('guest')).json()['light']['accent'],'#9abbac')
  self.assertEqual(self.client.get('/v1/appearance/effective',headers=self.login('owner')).json()['source'],'default')
  self.assertEqual(self.client.get('/v1/appearance/effective',headers=h).json()['source'],'default')
  public=self.client.get('/v1/appearance/public');self.assertEqual(public.headers['cache-control'],'no-store');self.assertNotIn('draft',public.json());self.assertNotIn('previous',public.json())
 def test_role_permissions_and_css_input_validation(self):
  for role in ['owner','guest','other']:
   h=self.login(role);self.assertEqual(self.draft(h,DEFAULT).status_code,403)
   for path in ['publish','restore']:self.assertEqual(self.client.post('/v1/appearance/'+path,headers=h,json={'expected_revision':1}).status_code,403)
  h=self.login('platform')
  for value in ['red','#fff','#123456; background:url(x)','url(x)']:
   c=copy.deepcopy(DEFAULT);c['light']['background']=value;self.assertEqual(self.draft(h,c).status_code,422)
  for changed in [{'scopes':['platform','platform']},{'scopes':['admin']},{'tenant_override':True}]:self.assertEqual(self.draft(h,{**DEFAULT,**changed}).status_code,422)
 def test_low_contrast_draft_can_be_saved_but_not_published(self):
  h=self.login('platform');c=copy.deepcopy(DEFAULT);c['dark']['text']=c['dark']['surface']
  self.assertEqual(self.draft(h,c).status_code,200);r=self.client.post('/v1/appearance/publish',headers=h,json={'expected_revision':1});self.assertEqual(r.status_code,422);self.assertIn('dark Text / surface',r.json()['detail']);self.assertIsNone(self.client.get('/v1/appearance/public').json()['published_revision'])
 def test_revision_conflicts_restore_and_audit(self):
  h=self.login('platform');c=copy.deepcopy(DEFAULT);c['light']['accent']='#9abbac';self.draft(h,c)
  self.assertEqual(self.draft(h,DEFAULT,0).status_code,409)
  self.assertEqual(self.client.post('/v1/appearance/publish',headers=h,json={'expected_revision':9}).status_code,409)
  self.client.post('/v1/appearance/publish',headers=h,json={'expected_revision':1})
  self.assertEqual(self.client.get('/v1/appearance/public').json()['light']['accent'],'#9abbac')
  self.assertEqual(self.client.post('/v1/appearance/restore',headers=h,json={'expected_revision':1}).status_code,409)
  r=self.client.post('/v1/appearance/restore',headers=h,json={'expected_revision':2});self.assertEqual(r.status_code,200,r.text);self.assertEqual(r.json()['revision'],3);self.assertEqual(r.json()['draft'],DEFAULT)
  self.assertEqual(self.client.get('/v1/appearance/public').json()['light']['accent'],DEFAULT['light']['accent'])
  self.assertEqual(self.client.post('/v1/appearance/restore',headers=h,json={'expected_revision':3}).status_code,200)
  self.assertEqual(self.client.get('/v1/appearance/public').json()['light']['accent'],'#9abbac')
  with self.app.state.factory() as s:self.assertEqual(len(list(s.scalars(select(Audit).where(Audit.action=='appearance_restored')))),2)
 def test_concurrent_initial_draft_only_one_wins(self):
  h=self.login('platform')
  with ThreadPoolExecutor(2) as p:results=list(p.map(lambda _:self.draft(h,DEFAULT),range(2)))
  self.assertEqual(sorted(r.status_code for r in results),[200,409]);self.assertEqual(self.client.get('/v1/appearance',headers=h).json()['revision'],1)
 def test_default_checks_and_empty_scope_reset(self):
  self.assertTrue(all(c['passes'] for c in contrast_checks(DEFAULT)));h=self.login('platform');c=copy.deepcopy(DEFAULT);c['scopes']=[];self.draft(h,c)
  self.assertEqual(self.client.post('/v1/appearance/publish',headers=h,json={'expected_revision':1}).status_code,200)
  self.assertEqual(self.client.get('/v1/appearance/effective',headers=h).json()['source'],'default')
 def test_duplicate_publication_preserves_restore_point(self):
  h=self.login('platform');c=copy.deepcopy(DEFAULT);c['light']['accent']='#9abbac';self.draft(h,c)
  self.client.post('/v1/appearance/publish',headers=h,json={'expected_revision':1})
  self.assertEqual(self.client.post('/v1/appearance/publish',headers=h,json={'expected_revision':2}).status_code,409)
  self.assertFalse(self.client.get('/v1/appearance',headers=h).json()['has_unpublished_changes'])
  self.client.post('/v1/appearance/restore',headers=h,json={'expected_revision':2})
  self.assertEqual(self.client.get('/v1/appearance/public').json()['light'],DEFAULT['light'])
