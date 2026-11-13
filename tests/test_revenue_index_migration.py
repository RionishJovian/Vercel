import os,sqlite3,tempfile,unittest
from unittest.mock import patch
from pathlib import Path
from alembic import command
from alembic.config import Config
from backend.models import Base

INDEX='ix_financial_events_tenant_occurred'
ROOT=Path(__file__).resolve().parents[1]

def config():
 cfg=Config(str(ROOT/'alembic.ini'));cfg.set_main_option('script_location',str(ROOT/'backend/migrations'));return cfg
def index_columns(file,name=INDEX):
 with sqlite3.connect(file) as db:
  if not db.execute("SELECT 1 FROM sqlite_master WHERE type='index' AND name=?",(name,)).fetchone():return None
  return [r[2] for r in db.execute(f'PRAGMA index_info({name})').fetchall()]

class RevenueIndexMigrationTests(unittest.TestCase):
 def test_head_has_the_composite_index_and_the_model_declares_it(self):
  declared={i.name:[c.name for c in i.columns] for i in Base.metadata.tables['financial_events'].indexes}
  self.assertEqual(declared[INDEX],['tenant_id','occurred_at'])
  with tempfile.TemporaryDirectory() as folder:
   file=Path(folder)/'index.sqlite3'
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(file)}):
    command.upgrade(config(),'head')
    self.assertEqual(index_columns(file),['tenant_id','occurred_at'])
    # 007's single-column indexes are untouched: this migration only adds.
    self.assertEqual(index_columns(file,'ix_financial_events_tenant_id'),['tenant_id'])
    self.assertEqual(index_columns(file,'ix_financial_events_occurred_at'),['occurred_at'])
    # The query shape used by /v1/revenue and /v1/revenue/ledger is answered from the new index.
    with sqlite3.connect(file) as db:
     plan=' '.join(r[3] for r in db.execute("EXPLAIN QUERY PLAN SELECT id FROM financial_events WHERE tenant_id='t' AND occurred_at>=1 AND occurred_at<9 ORDER BY occurred_at DESC").fetchall())
    self.assertIn(INDEX,plan)

 def test_downgrade_drops_only_the_index_and_the_ledger_stays_guarded(self):
  cfg=config()
  with tempfile.TemporaryDirectory() as folder:
   file=Path(folder)/'ledger.sqlite3'
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(file)}):
    command.upgrade(cfg,'019')
    with sqlite3.connect(file) as db:
     db.execute("INSERT INTO tenants VALUES ('t','Cafe')")
     db.execute("INSERT INTO financial_events (id,tenant_id,source,source_id,kind,amount,currency,service,package_name,method,reference,occurred_at,timestamp_basis) VALUES ('e','t','guest_order','o','collection',1000,'MMK','internet','Plan','Cash','ref',100,'recorded_event')")
    command.upgrade(cfg,'head')
    self.assertIsNotNone(index_columns(file))
    # Rolling back the index must not touch the ledger.
    command.downgrade(cfg,'019')
    self.assertIsNone(index_columns(file))
    with sqlite3.connect(file) as db:self.assertEqual(db.execute('SELECT id,amount FROM financial_events').fetchall(),[('e',1000)])
    command.upgrade(cfg,'head')
    self.assertIsNotNone(index_columns(file))
    # The pre-existing destructive-downgrade protection still refuses to discard the ledger.
    with self.assertRaisesRegex(RuntimeError,'Financial events exist'):command.downgrade(cfg,'006')
    with sqlite3.connect(file) as db:self.assertEqual(db.execute('SELECT count(*) FROM financial_events').fetchone()[0],1)

 def test_shipped_migrations_are_never_edited(self):
  import subprocess
  # Migrations that exist on the base branch are history: adding a new forward migration is fine, changing, renaming
  # or deleting an existing one is not. Skipped when the base branch is unavailable (a shallow CI checkout).
  base=subprocess.run(['git','merge-base','HEAD','origin/main'],cwd=ROOT,capture_output=True,text=True)
  if base.returncode!=0:self.skipTest('no origin/main to compare against')
  out=subprocess.run(['git','diff','--name-status','--no-renames',base.stdout.strip(),'--','backend/migrations/versions'],cwd=ROOT,capture_output=True,text=True).stdout
  rewritten=[l for l in out.split('\n') if l and not l.startswith('A')]
  self.assertEqual(rewritten,[],'a shipped migration was edited or removed; add a new forward migration instead')

if __name__=='__main__':unittest.main()
