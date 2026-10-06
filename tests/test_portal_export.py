import io,json,zipfile,unittest
import test_backend
import test_guest_entry
from backend.models import SitePortal
class PortalExportTests(unittest.TestCase):
 setUp=test_backend.AccountPaymentTests.setUp
 tearDown=test_backend.AccountPaymentTests.tearDown
 login=test_backend.AccountPaymentTests.login
 site=test_guest_entry.GuestEntryTests.site
 def test_export_is_private_scoped_and_non_executable(self):
  id=self.site();path='/v1/sites/'+id+'/portal/export';body={'expected_revision':1,'public_origin':'https://app.example.com'}
  for name in ['guest','other','platform']:self.assertIn(self.client.post(path,headers=self.login(name),json=body).status_code,[403,404])
  r=self.client.post(path,headers=self.login('owner'),json=body);self.assertEqual(r.status_code,200)
  with zipfile.ZipFile(io.BytesIO(r.content)) as z:
   self.assertEqual(set(z.namelist()),{'portal-plan.json','preview.html','README.txt'});plan=json.loads(z.read('portal-plan.json'));self.assertEqual(plan['guest_url'],'https://app.example.com/?guest_site='+id);self.assertEqual(plan['router_apply_state'],'not_connected');self.assertNotIn('guest_interface',plan['public_content']);self.assertIn(b'NOT a RouterOS',z.read('README.txt'))
 def test_revision_origin_publication_and_escaping(self):
  id=self.site();path='/v1/sites/'+id+'/portal/export';h=self.login('owner')
  for origin in ['http://app.example.com','https://u:p@app.example.com','https://app.example.com/path','https://app.example.com?x=1','javascript:alert(1)']:
   self.assertEqual(self.client.post(path,headers=h,json={'expected_revision':1,'public_origin':origin}).status_code,422)
  self.assertEqual(self.client.post(path,headers=h,json={'expected_revision':2,'public_origin':'https://app.example.com'}).status_code,409)
  with self.app.state.factory() as s:
   row=s.get(SitePortal,id);row.published={**row.published,'welcome':'<script>bad</script>'};row.draft={**row.draft,'allowed_hosts':['media.example.com']};s.commit()
  r=self.client.post(path,headers=h,json={'expected_revision':1,'public_origin':'https://app.example.com'})
  with zipfile.ZipFile(io.BytesIO(r.content)) as z:self.assertIn(b'&lt;script&gt;',z.read('preview.html'));self.assertEqual(json.loads(z.read('portal-plan.json'))['network_draft']['allowed_hosts'],['media.example.com'])
  with self.app.state.factory() as s:s.get(SitePortal,id).published=None;s.commit()
  self.assertEqual(self.client.post(path,headers=h,json={'expected_revision':1,'public_origin':'https://app.example.com'}).status_code,409)
