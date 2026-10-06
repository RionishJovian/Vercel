import unittest
import test_backend as fixture
from sqlalchemy import select
from backend.models import Audit
class KnowledgeTests(unittest.TestCase):
 setUp=fixture.AccountPaymentTests.setUp
 tearDown=fixture.AccountPaymentTests.tearDown
 login=fixture.AccountPaymentTests.login
 def body(self):return {'title':'Local RADIUS','category':'MikroTik','fact':'User Manager hosts RADIUS.','why':'Understand local authentication.','requirements':'Check hardware and license.','action':'Routers','source_url':'https://manual.mikrotik.com/docs/introduction/','verified_on':'2026-10-05'}
 def create(self,h):
  r=self.client.post('/v1/knowledge',headers=h,json=self.body());self.assertEqual(r.status_code,201,r.text);return r.json()
 def state(self,h,c,operation):return self.client.post('/v1/knowledge/'+c['id']+'/state',headers=h,json={'expected_revision':c['revision'],'operation':operation})
 def test_role_publication_retirement_and_conflict(self):
  p=self.login('platform');o=self.login('owner');g=self.login('guest');other=self.login('other');c=self.create(p)
  self.assertEqual(self.client.get('/v1/knowledge').status_code,401)
  self.assertEqual(self.client.get('/v1/knowledge',headers=g).status_code,403)
  self.assertEqual(self.client.get('/v1/knowledge',headers=o).json(),[])
  for h in [o,g,other]:
   self.assertEqual(self.client.post('/v1/knowledge',headers=h,json=self.body()).status_code,403)
   self.assertEqual(self.state(h,c,'publish').status_code,403)
  pub=self.state(p,c,'publish').json();self.assertEqual(pub['compatibility'],'not_checked')
  self.assertEqual(len(self.client.get('/v1/knowledge',headers=other).json()),1)
  self.assertEqual(self.state(p,c,'archive').status_code,409)
  edited=self.client.post('/v1/knowledge/'+c['id'],headers=p,json={'expected_revision':pub['revision'],'settings':{**self.body(),'title':'Edited guidance'}}).json()
  self.assertFalse(edited['published']);self.assertEqual(self.client.get('/v1/knowledge',headers=o).json(),[])
  archived=self.state(p,edited,'archive').json();self.assertEqual(self.state(p,archived,'publish').status_code,409)
  restored=self.state(p,archived,'restore').json();self.assertFalse(restored['published']);self.assertFalse(restored['archived'])
  with self.app.state.factory() as s:self.assertEqual(len(list(s.scalars(select(Audit).where(Audit.action.like('guidance_%'))))),5)
 def test_source_date_action_and_blank_validation(self):
  p=self.login('platform')
  for changes in [{'source_url':'javascript:alert(1)'},{'source_url':'https://user:secret@example.com'},{'source_url':'http://example.com'},{'verified_on':'2099-01-01'},{'title':'   '},{'action':'Execute router command'},{'published':True}]:
   self.assertEqual(self.client.post('/v1/knowledge',headers=p,json={**self.body(),**changes}).status_code,422)
