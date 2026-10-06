import os,sqlite3,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
from alembic import command
from alembic.config import Config
class BillingSecurityMigrations(unittest.TestCase):
 def test_empty_roundtrip_and_mfa_preservation(self):
  root=Path(__file__).resolve().parents[1];cfg=Config(str(root/'alembic.ini'));cfg.set_main_option('script_location',str(root/'backend/migrations'))
  with tempfile.TemporaryDirectory() as folder:
   file=Path(folder)/'records.db'
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(file)}):
    command.upgrade(cfg,'head');command.downgrade(cfg,'017');command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:
     db.execute("INSERT INTO users (id,username,password_hash,role,tenant_id,active) VALUES ('p','platform','test-only','platform',NULL,1)")
     db.execute("INSERT INTO account_mfa VALUES ('p',1,'test-encrypted',NULL,1,'[]',1)")
    with self.assertRaisesRegex(RuntimeError,'MFA records exist'):command.downgrade(cfg,'018')
    with sqlite3.connect(file) as db:self.assertEqual(db.execute('SELECT enabled FROM account_mfa').fetchone()[0],1);self.assertEqual(db.execute('PRAGMA foreign_key_check').fetchall(),[])
