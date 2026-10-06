import os,sqlite3,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
from alembic import command
from alembic.config import Config
class PlanningMigrationTests(unittest.TestCase):
 def test_plan_preservation_and_empty_roundtrip(self):
  root=Path(__file__).resolve().parents[1];cfg=Config(str(root/'alembic.ini'));cfg.set_main_option('script_location',str(root/'backend/migrations'))
  with tempfile.TemporaryDirectory() as folder:
   path=Path(folder)/'planning.db'
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(path)}):
    command.upgrade(cfg,'head');command.downgrade(cfg,'014');command.upgrade(cfg,'head')
    with sqlite3.connect(path) as db:
     db.execute("INSERT INTO tenants VALUES ('t','Cafe')");db.execute("INSERT INTO sites VALUES ('s','t','Branch','Asia/Yangon',1)");db.execute("INSERT INTO site_planning VALUES ('s',1,'{}',1)")
    with self.assertRaisesRegex(RuntimeError,'Site plans exist'):command.downgrade(cfg,'014')
    with sqlite3.connect(path) as db:self.assertEqual(db.execute('SELECT revision FROM site_planning').fetchone()[0],1);self.assertEqual(db.execute('PRAGMA foreign_key_check').fetchall(),[])
