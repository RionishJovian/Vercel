import unittest, sys, tempfile, json, urllib.request, urllib.error, threading, time, hmac, hashlib, os
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]));import server
class WorkflowTests(unittest.TestCase):
 @classmethod
 def setUpClass(cls):
  cls.tmp=tempfile.TemporaryDirectory();server.DB=Path(cls.tmp.name)/'test.db';server.read();cls.http=server.ThreadingHTTPServer(('127.0.0.1',0),server.Handler);cls.url='http://127.0.0.1:'+str(cls.http.server_port)+'/api/';threading.Thread(target=cls.http.serve_forever,daemon=True).start()
 @classmethod
 def tearDownClass(cls):cls.http.shutdown();cls.http.server_close();cls.tmp.cleanup()
 def call(self,path,body=None,token='',headers=None):
  req=urllib.request.Request(self.url+path,data=json.dumps(body).encode() if body is not None else None,headers={'Content-Type':'application/json','Authorization':'Bearer '+token,**(headers or {})})
  try:
   with urllib.request.urlopen(req) as r:return r.status,json.load(r)
  except urllib.error.HTTPError as e:return e.code,json.load(e)
 def login(self,role,tenant='cafe',guest='guest-test'):return self.call('demo-session',{'role':role,'tenant':tenant,'guest':guest})[1]['token']
 def test_payment_and_quota_journey(self):
  guest=self.login('guest');owner=self.login('sme');other=self.login('sme','lotus');o=self.call('orders',{'package':'p2','method':'Cash','reference':'test-txn','evidence':'counter receipt'},guest)[1]
  self.assertEqual(self.call('orders/decision',{'id':o['id'],'decision':'approved'},guest)[0],403)
  self.assertEqual(self.call('orders/decision',{'id':o['id'],'decision':'approved'},other)[0],403)
  approved=self.call('orders/decision',{'id':o['id'],'decision':'approved'},owner)[1];eid=approved['order']['entitlement'];self.assertTrue(self.call('orders/decision',{'id':o['id'],'decision':'approved'},owner)[1]['replayed'])
  self.assertEqual(self.call('check-access',{'id':eid},guest)[0],403)
  self.assertEqual(self.call('activate',{'id':eid},guest)[0],200)
  self.assertEqual(self.call('check-access',{'id':eid},guest)[0],200)
  self.call('demo-usage',{'id':eid,'gb':5},owner);self.assertEqual(self.call('check-access',{'id':eid},guest)[0],403)
  self.call('activate',{'id':eid},guest);self.assertEqual(self.call('check-access',{'id':eid},guest)[0],403)
  self.assertEqual(len([e for e in self.call('state',token=owner)[1]['entitlements'] if e['source']==o['id']]),1)
  self.assertFalse(any(x['id']==o['id'] for x in self.call('state',token=other)[1]['orders']))
 def test_voucher_single_redemption(self):
  owner=self.login('sme');guest=self.login('guest',guest='voucher-user');v=self.call('vouchers',{'package':'p1'},owner)[1];self.assertEqual(self.call('vouchers/redeem',{'code':v['code']},guest)[0],200);self.assertEqual(self.call('vouchers/redeem',{'code':v['code']},guest)[0],409)
 def test_media_title_and_expiry(self):
  s=server.seed();e=server.grant(s,'cafe','g',s['packages'][3],'o');e['activated']=time.time();self.assertTrue(server.access(e,'media','river')['allowed'])
  with self.assertRaises(server.Error):server.access(e,'media','cooking')
  with self.assertRaises(server.Error):server.access(e,'media','river',e['activated']+49*3600)
 def test_media_independent_of_internet(self):
  s=server.seed();e=server.grant(s,'cafe','g',s['packages'][4],'o');e['activated']=time.time()-4*3600
  with self.assertRaises(server.Error):server.access(e,'internet')
  self.assertTrue(server.access(e,'media','river')['allowed'])
 def test_refund_and_evidence(self):
  guest=self.login('guest');owner=self.login('sme');o=self.call('orders',{'package':'p3','method':'Cash','reference':'evidence-test','evidence':'unclear'},guest)[1]
  self.assertEqual(self.call('orders/decision',{'id':o['id'],'decision':'rejected'},owner)[0],400)
  self.call('orders/decision',{'id':o['id'],'decision':'needs_evidence','reason':'receipt missing'},owner)
  self.call('orders/evidence',{'id':o['id'],'evidence':'clear test receipt'},guest)
  e=self.call('orders/decision',{'id':o['id'],'decision':'approved'},owner)[1]['order']['entitlement'];self.call('activate',{'id':e},guest);self.call('orders/refund',{'id':o['id'],'reason':'recorded refund'},owner);self.assertEqual(self.call('check-access',{'id':e,'service':'media'},guest)[0],403)
 def test_callback_auth_and_replay(self):
  os.environ['PAYMENT_HOOK_SECRET']='test-only-secret';os.environ['PAYMENT_HOOK_TENANT']='cafe';g=self.login('guest');o=self.call('orders',{'package':'p1','method':'Cash','reference':'hook-test','evidence':'test'},g)[1];body={'id':o['id'],'decision':'approved'};raw=json.dumps(body).encode();stamp=str(int(time.time()));nonce='random-test-nonce-12345';sig=hmac.new(b'test-only-secret',stamp.encode()+b'.'+nonce.encode()+b'.'+raw,hashlib.sha256).hexdigest();heads={'X-Timestamp':stamp,'X-Nonce':nonce,'X-Signature':sig}
  self.assertEqual(self.call('hooks/payment-decision',body,headers={**heads,'X-Signature':'wrong'})[0],401);self.assertEqual(self.call('hooks/payment-decision',body,headers=heads)[0],200);self.assertEqual(self.call('hooks/payment-decision',body,headers=heads)[0],409)
  os.environ.pop('PAYMENT_HOOK_SECRET');os.environ.pop('PAYMENT_HOOK_TENANT')
 def test_session_required_and_theme_scope(self):
  self.assertEqual(self.call('state')[0],401);s=self.login('sme');self.assertEqual(self.call('themes',{'themes':server.seed()['themes']},s)[0],403)
if __name__=='__main__':unittest.main()
