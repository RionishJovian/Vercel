import unittest
from io import BytesIO
from PIL import Image,PngImagePlugin
import test_backend as fixture
class ReceiptTests(unittest.TestCase):
 setUp=fixture.AccountPaymentTests.setUp
 tearDown=fixture.AccountPaymentTests.tearDown
 login=fixture.AccountPaymentTests.login
 package=fixture.AccountPaymentTests.package
 order=fixture.AccountPaymentTests.order
 def image(self,color='red'):
  b=BytesIO();meta=PngImagePlugin.PngInfo();meta.add_text('private_note','remove-me');Image.new('RGB',(20,30),color).save(b,format='PNG',pnginfo=meta);return b.getvalue()
 def setup_order(self):
  o=self.login('owner');g=self.login('guest');return o,g,self.order(g,self.package(o)).json()['id']
 def upload(self,g,id,key='receipt-request-key-001',data=None):return self.client.post('/v1/orders/'+id+'/receipts',headers={**g,'Content-Type':'application/octet-stream','X-Upload-Key':key},content=data if data is not None else self.image())
 def test_private_reads_and_guest_upload_permissions(self):
  o,g,id=self.setup_order();r=self.upload(g,id);self.assertEqual(r.status_code,201,r.text);rid=r.json()['id'];url='/v1/orders/'+id+'/receipts/'+rid
  self.assertEqual(self.client.get(url).status_code,401)
  for name in ['other','guest2','platform']:
   h=self.login(name);self.assertEqual(self.client.get(url,headers=h).status_code,404);self.assertEqual(self.client.get('/v1/orders/'+id+'/receipts',headers=h).status_code,404)
  self.assertEqual(self.upload(o,id).status_code,403)
  for h in [o,g]:
   viewed=self.client.get(url,headers=h);self.assertEqual(viewed.status_code,200);self.assertEqual(viewed.headers['cache-control'],'no-store');self.assertEqual(viewed.headers['content-type'],'image/png')
   with Image.open(BytesIO(viewed.content)) as im:self.assertEqual(im.size,(20,30));self.assertNotIn('private_note',im.info)
  self.assertNotIn('image',self.client.get('/v1/orders/'+id+'/receipts',headers=o).json()[0])
 def test_retry_limit_and_stale_review(self):
  o,g,id=self.setup_order();first=self.upload(g,id).json();self.assertEqual(self.upload(g,id).json()['id'],first['id'])
  self.assertEqual(self.upload(g,id,data=self.image('blue')).status_code,409)
  path='/v1/orders/'+id+'/decision'
  self.assertEqual(self.client.post(path,headers=o,json={'decision':'approved'}).status_code,409)
  self.assertEqual(self.client.post(path,headers=o,json={'decision':'approved','expected_receipt_revision':0}).status_code,409)
  for i in [2,3]:self.assertEqual(self.upload(g,id,key=f'receipt-request-key-00{i}').status_code,201)
  self.assertEqual(self.upload(g,id,key='receipt-request-key-004').status_code,409)
  self.assertEqual(self.client.post(path,headers=o,json={'decision':'approved','expected_receipt_revision':1}).status_code,409)
  self.assertEqual(self.client.post(path,headers=o,json={'decision':'approved','expected_receipt_revision':3}).status_code,200)
  self.assertEqual(self.upload(g,id,key='receipt-request-key-005').status_code,409)
  self.assertTrue(self.upload(g,id).json()['replayed'])
 def test_invalid_and_oversized_images_dont_change_revision(self):
  o,g,id=self.setup_order()
  for data in [b'<svg><script>bad</script></svg>',b'not-a-png',b'']:
   self.assertIn(self.upload(g,id,data=data).status_code,[413,422])
  self.assertEqual(self.upload(g,id,data=b'x'*(5*1024*1024+1)).status_code,413)
  b=BytesIO();Image.new('1',(3000,3000)).save(b,format='PNG');self.assertEqual(self.upload(g,id,data=b.getvalue()).status_code,422)
  self.assertEqual(self.client.get('/v1/orders',headers=o).json()[0]['receipt_revision'],0)
  self.assertEqual(self.client.get('/v1/orders/'+id+'/receipts',headers=o).json(),[])
 def test_attachment_does_not_resubmit_evidence_or_approve_payment(self):
  o,g,id=self.setup_order();path='/v1/orders/'+id
  self.client.post(path+'/decision',headers=o,json={'decision':'needs_evidence','reason':'Send receipt'})
  self.assertEqual(self.upload(g,id).status_code,201)
  self.assertEqual(self.client.get('/v1/orders',headers=o).json()[0]['status'],'needs_evidence')
  self.assertEqual(self.client.post(path+'/evidence',headers=g,json={'evidence':'Attached receipt and corrected reference'}).status_code,200)
  self.assertEqual(self.client.get('/v1/revenue',headers=o).json()['net_collections'],0)
