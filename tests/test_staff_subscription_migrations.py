import os,sqlite3,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
from alembic import command
from alembic.config import Config
class StaffSubscriptionMigrations(unittest.TestCase):
 def config(self):
  root=Path(__file__).resolve().parents[1];cfg=Config(str(root/'alembic.ini'));cfg.set_main_option('script_location',str(root/'backend/migrations'));return cfg
 def test_staff_downgrade_prevents_privilege_escalation(self):
  with tempfile.TemporaryDirectory() as folder:
   file=Path(folder)/'staff.db';cfg=self.config()
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(file)}):
    command.upgrade(cfg,'head');command.downgrade(cfg,'015');command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:
     db.execute("INSERT INTO tenants VALUES ('t','Cafe')");db.execute("INSERT INTO users (id,username,password_hash,role,tenant_id,active) VALUES ('o','owner','test-only','sme','t',1),('s','staff','test-only','sme','t',1)");db.execute("INSERT INTO staff_grants VALUES ('s','t','cashier',1,'o','test-key','test-hash',1,1,'test reason')")
    with self.assertRaisesRegex(RuntimeError,'escalate'):command.downgrade(cfg,'016')
    with sqlite3.connect(file) as db:self.assertEqual(db.execute('SELECT profile FROM staff_grants').fetchone()[0],'cashier');self.assertEqual(db.execute('PRAGMA foreign_key_check').fetchall(),[])
 def test_subscription_plan_preservation(self):
  with tempfile.TemporaryDirectory() as folder:
   file=Path(folder)/'plans.db';cfg=self.config()
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(file)}):
    command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:db.execute("INSERT INTO subscription_plans VALUES ('p','{}',1)")
    with self.assertRaisesRegex(RuntimeError,'Subscription records exist'):command.downgrade(cfg,'015')
