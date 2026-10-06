import base64,time,unittest
from unittest.mock import patch
from cryptography.fernet import Fernet
import test_backend
from backend.mfa import totp
from backend.models import AccountMFA

class MFATests(unittest.TestCase):
 def setUp(self):
  self.key=Fernet.generate_key().decode()
  with patch.dict('os.environ',{'MIKROMAGIC_MFA_KEY':self.key}):test_backend.AccountPaymentTests.setUp(self)
 tearDown=test_backend.AccountPaymentTests.tearDown
 login=test_backend.AccountPaymentTests.login
 def enable(self,name='owner'):
  h=self.login(name);b={'password':'test-password-only'}
  r=self.client.post('/v1/account/mfa/enroll',headers=h,json=b);self.assertEqual(r.status_code,200);secret=r.json()['secret']
  r=self.client.post('/v1/account/mfa/confirm',headers=h,json=b|{'code':totp(secret,int(time.time())//30)});self.assertEqual(r.status_code,200)
  self.assertEqual(self.client.get('/v1/me',headers=h).status_code,401)
  return secret,r.json(),{'Authorization':'Bearer '+r.json()['access_token']}
 def test_rfc_vectors(self):
  secret=base64.b32encode(b'12345678901234567890').decode()
  for timestamp,expected in [(59,'94287082'),(1111111109,'07081804'),(1111111111,'14050471'),(1234567890,'89005924'),(2000000000,'69279037'),(20000000000,'65353130')]:self.assertEqual(totp(secret,timestamp//30,8),expected)
 def test_missing_server_key_blocks_enrollment(self):
  from backend.mfa import cipher
  from fastapi import HTTPException
  with self.assertRaises(HTTPException) as result:cipher(None)
  self.assertEqual(result.exception.status_code,503)
 def test_login_replay_and_single_use_recovery(self):
  secret,result,h=self.enable();body={'username':'owner','password':'test-password-only'}
  self.assertEqual(self.client.post('/v1/auth/login',json=body).status_code,401)
  self.assertEqual(self.client.post('/v1/auth/login',json=body|{'mfa_code':totp(secret,int(time.time())//30)}).status_code,401)
  with patch('backend.mfa.time.time',return_value=time.time()+60):
   code=totp(secret,int(time.time())//30)
   self.assertEqual(self.client.post('/v1/auth/login',json=body|{'mfa_code':code}).status_code,200)
   self.assertEqual(self.client.post('/v1/auth/login',json=body|{'mfa_code':code}).status_code,401)
  recovery=result['recovery_codes'][0]
  self.assertEqual(self.client.post('/v1/auth/login',json=body|{'mfa_code':recovery}).status_code,200)
  self.assertEqual(self.client.post('/v1/auth/login',json=body|{'mfa_code':recovery}).status_code,401)
  with self.app.state.factory() as s:
   row=s.get(AccountMFA,self.users['owner']);self.assertNotIn(secret,row.encrypted_secret);self.assertNotIn(recovery,row.recovery_hashes);self.assertEqual(len(row.recovery_hashes),9)
 def test_guest_disable_rotates_sessions_and_clears_secrets(self):
  secret,result,h=self.enable('guest');r=self.client.post('/v1/account/mfa/disable',headers=h,json={'password':'test-password-only','code':result['recovery_codes'][0]});self.assertEqual(r.status_code,200)
  self.assertEqual(self.client.get('/v1/me',headers=h).status_code,401);self.login('guest')
  with self.app.state.factory() as s:
   row=s.get(AccountMFA,self.users['guest']);self.assertFalse(row.enabled);self.assertIsNone(row.encrypted_secret);self.assertEqual(row.recovery_hashes,[])
 def test_enrollment_password_throttle_and_expiry(self):
  h=self.login('owner')
  for _ in range(8):self.assertEqual(self.client.post('/v1/account/mfa/enroll',headers=h,json={'password':'wrong'}).status_code,400)
  self.assertEqual(self.client.post('/v1/account/mfa/enroll',headers=h,json={'password':'test-password-only'}).status_code,429)
  h=self.login('guest');r=self.client.post('/v1/account/mfa/enroll',headers=h,json={'password':'test-password-only'});secret=r.json()['secret']
  with self.app.state.factory() as s:s.get(AccountMFA,self.users['guest']).enrollment_expires=1;s.commit()
  self.assertEqual(self.client.post('/v1/account/mfa/confirm',headers=h,json={'password':'test-password-only','code':totp(secret,int(time.time())//30)}).status_code,409)
