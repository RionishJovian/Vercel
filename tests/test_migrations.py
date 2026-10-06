import os,sqlite3,tempfile,unittest
from unittest.mock import patch
from pathlib import Path
from alembic import command
from alembic.config import Config

class MigrationTests(unittest.TestCase):
 def test_portal_migration_preserves_legacy_order_and_round_trips(self):
  root=Path(__file__).resolve().parents[1]
  with tempfile.TemporaryDirectory() as folder:
   file=Path(folder)/'migration.sqlite3';cfg=Config(str(root/'alembic.ini'));cfg.set_main_option('script_location',str(root/'backend/migrations'))
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(file)}):
    command.upgrade(cfg,'001')
    with sqlite3.connect(file) as db:
     db.execute("INSERT INTO tenants VALUES ('t','Legacy cafe')")
     db.execute("INSERT INTO users (id,username,password_hash,role,tenant_id,active) VALUES ('u','legacy','test-only-hash','guest','t',1)")
     db.execute("INSERT INTO packages VALUES ('p','t','{}',1)")
     db.execute("INSERT INTO orders VALUES ('o','t','u','p','{}',1000,'MMK','Cash','legacy-ref','legacy evidence','approved','pending_adapter',NULL,NULL,1,'legacy-request-key')")
    with sqlite3.connect(file) as db:db.execute("INSERT INTO entitlements VALUES ('e','o','t','u','{}',NULL,0,0,0)")
    command.upgrade(cfg,'006')
    with sqlite3.connect(file) as db:
     self.assertEqual(db.execute('SELECT amount,reference,payment_context FROM orders').fetchone(),(1000,'legacy-ref',None))
     self.assertEqual(db.execute('SELECT order_id,voucher_id,site_id FROM entitlements').fetchone(),('o',None,None))
     with self.assertRaises(sqlite3.IntegrityError):db.execute('UPDATE orders SET amount=-1')
     self.assertEqual(db.execute('PRAGMA foreign_key_check').fetchall(),[])
     self.assertIsNotNone(db.execute("SELECT name FROM sqlite_master WHERE name='portal_settings'").fetchone())
    command.downgrade(cfg,'001');command.upgrade(cfg,'006');command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:self.assertEqual(db.execute('SELECT count(*) FROM orders').fetchone()[0],1)

 def test_voucher_downgrade_refuses_to_discard_records(self):
  root=Path(__file__).resolve().parents[1]
  with tempfile.TemporaryDirectory() as folder:
   file=Path(folder)/'voucher.sqlite3';cfg=Config(str(root/'alembic.ini'));cfg.set_main_option('script_location',str(root/'backend/migrations'))
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(file)}):
    command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:
     db.execute("INSERT INTO tenants VALUES ('t','Cafe')")
     db.execute("INSERT INTO users (id,username,password_hash,role,tenant_id,active) VALUES ('u','owner','test-only-hash','sme','t',1)")
     db.execute("INSERT INTO packages VALUES ('p','t','{}',1)")
     db.execute("INSERT INTO vouchers (id,tenant_id,package_id,policy,code_hash,code_hint,issued_by,request_key,created,expires,redeemed_by,redeemed_at,revoked,reason) VALUES ('v','t','p','{}','test-digest','ABCD','u','test-request-key',1,100,NULL,NULL,0,NULL)")
    with self.assertRaisesRegex(RuntimeError,'Voucher records exist'):command.downgrade(cfg,'002')
    with sqlite3.connect(file) as db:self.assertEqual(db.execute('SELECT id FROM vouchers').fetchone()[0],'v')

    command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:
     db.execute("INSERT INTO voucher_sales (id,voucher_id,tenant_id,snapshot,amount,currency,method,reference,evidence,recorded_by,created) VALUES ('s','v','t','{}',1000,'MMK','Cash','test-ref','test-evidence','u',1)")
    with self.assertRaisesRegex(RuntimeError,'Voucher sale records exist'):command.downgrade(cfg,'003')
    with sqlite3.connect(file) as db:self.assertEqual(db.execute('SELECT amount FROM voucher_sales').fetchone()[0],1000)

 def test_hub_migration_preserves_records_and_constraints(self):
  root=Path(__file__).resolve().parents[1]
  with tempfile.TemporaryDirectory() as folder:
   file=Path(folder)/'hub.sqlite3';cfg=Config(str(root/'alembic.ini'));cfg.set_main_option('script_location',str(root/'backend/migrations'))
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(file)}):
    command.upgrade(cfg,'005');command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:
     db.execute("INSERT INTO magic_hubs VALUES ('h','Hub','hub.example.com',51820,'10.90.1.0/24','test-public-key',2,1)")
     with self.assertRaises(sqlite3.IntegrityError):db.execute("UPDATE magic_hubs SET next_host=256")
     with self.assertRaises(sqlite3.IntegrityError):db.execute("UPDATE magic_hubs SET port=0")
    with self.assertRaisesRegex(RuntimeError,'Hub records exist'):command.downgrade(cfg,'005')
    with sqlite3.connect(file) as db:
     self.assertEqual(db.execute('SELECT id,next_host FROM magic_hubs').fetchone(),('h',2));self.assertEqual(db.execute('PRAGMA foreign_key_check').fetchall(),[])

 def test_cash_ledger_backfill_dates_and_preservation(self):
  root=Path(__file__).resolve().parents[1]
  with tempfile.TemporaryDirectory() as folder:
   file=Path(folder)/'ledger.sqlite3';cfg=Config(str(root/'alembic.ini'));cfg.set_main_option('script_location',str(root/'backend/migrations'))
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(file)}):
    command.upgrade(cfg,'006')
    with sqlite3.connect(file) as db:
     db.execute("INSERT INTO tenants VALUES ('t','Cafe')")
     db.execute("INSERT INTO users (id,username,password_hash,role,tenant_id,active) VALUES ('u','owner','test-only-hash','sme','t',1)")
     db.execute("INSERT INTO packages VALUES ('p','t','{}',1)")
     for id,status in [('paid','approved'),('returned','refunded')]:
      db.execute("INSERT INTO orders (id,tenant_id,guest_id,package_id,snapshot,amount,currency,method,reference,evidence,status,fulfillment,reviewer_id,created,request_key) VALUES (?,?,?,?,?,1000,'MMK','Cash','legacy-ref','evidence',?,'pending_adapter','u',100,?)",(id,'t','u','p','{"name":"Plan","service":"internet"}',status,id+'-request'))
     db.execute("INSERT INTO audit_events VALUES ('audit','t','u','payment_approved','paid',200)")
     db.execute("INSERT INTO vouchers (id,tenant_id,package_id,policy,code_hash,code_hint,issued_by,request_key,created,expires,revoked) VALUES ('v','t','p','{}','test-hash','ABCD','u','voucher-key',1,10000,0)")
     db.execute("INSERT INTO voucher_sales (id,voucher_id,tenant_id,snapshot,amount,currency,method,reference,evidence,recorded_by,created,refunded_at,refunded_by,refund_reason,refund_reference) VALUES ('s','v','t','{}',1000,'MMK','Cash','receipt','evidence','u',400,500,'u','Returned','refund-receipt')")
    command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:
     rows=db.execute('SELECT source_id,kind,occurred_at,timestamp_basis,actor_id FROM financial_events ORDER BY source_id,kind').fetchall()
     self.assertEqual(len(rows),5)
     self.assertIn(('paid','collection',200,'migrated_audit','u'),rows)
     self.assertIn(('returned','collection',100,'legacy_submission_estimate','u'),rows)
     self.assertIn(('returned','refund',100,'legacy_submission_estimate',None),rows)
     self.assertIn(('s','collection',400,'migrated_record','u'),rows);self.assertIn(('s','refund',500,'migrated_record','u'),rows)
     self.assertEqual(db.execute("SELECT sum(CASE WHEN kind='collection' THEN amount ELSE -amount END) FROM financial_events").fetchone()[0],1000)
     self.assertEqual(db.execute('PRAGMA foreign_key_check').fetchall(),[])
    command.upgrade(cfg,'head')
    with self.assertRaisesRegex(RuntimeError,'Financial events exist'):command.downgrade(cfg,'006')
    with sqlite3.connect(file) as db:self.assertEqual(db.execute('SELECT count(*) FROM financial_events').fetchone()[0],5)

 def test_appearance_migration_keeps_published_palettes(self):
  root=Path(__file__).resolve().parents[1]
  with tempfile.TemporaryDirectory() as folder:
   file=Path(folder)/'appearance.sqlite3';cfg=Config(str(root/'alembic.ini'));cfg.set_main_option('script_location',str(root/'backend/migrations'))
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(file)}):
    command.upgrade(cfg,'007');command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:db.execute("INSERT INTO theme_settings VALUES ('platform',2,'{}','{}','{}',2,1)")
    with self.assertRaisesRegex(RuntimeError,'Appearance records exist'):command.downgrade(cfg,'007')
    with sqlite3.connect(file) as db:self.assertEqual(db.execute('SELECT revision,published_revision FROM theme_settings').fetchone(),(2,2))

 def test_media_migration_preserves_catalogue(self):
  root=Path(__file__).resolve().parents[1]
  with tempfile.TemporaryDirectory() as folder:
   file=Path(folder)/'media.sqlite3';cfg=Config(str(root/'alembic.ini'));cfg.set_main_option('script_location',str(root/'backend/migrations'))
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(file)}):
    command.upgrade(cfg,'008');command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:
     db.execute("INSERT INTO tenants VALUES ('t','Cafe')")
     db.execute("INSERT INTO media_titles VALUES ('m','t','Film','Description','Drama',90,2020,1,0,2,1)")
     with self.assertRaises(sqlite3.IntegrityError):db.execute('UPDATE media_titles SET runtime_minutes=0')
    with self.assertRaisesRegex(RuntimeError,'Media catalogue records exist'):command.downgrade(cfg,'008')
    with sqlite3.connect(file) as db:self.assertEqual(db.execute('SELECT name,published FROM media_titles').fetchone(),('Film',1));self.assertEqual(db.execute('PRAGMA foreign_key_check').fetchall(),[])

 def test_guidance_seed_and_edited_content_preservation(self):
  root=Path(__file__).resolve().parents[1]
  with tempfile.TemporaryDirectory() as folder:
   file=Path(folder)/'guidance.sqlite3';cfg=Config(str(root/'alembic.ini'));cfg.set_main_option('script_location',str(root/'backend/migrations'))
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(file)}):
    command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:
     self.assertEqual(db.execute('SELECT count(*) FROM knowledge_cards WHERE published=1').fetchone()[0],3)
    command.downgrade(cfg,'009');command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:db.execute("UPDATE knowledge_cards SET revision=2 WHERE id='guide-wireguard'")
    with self.assertRaisesRegex(RuntimeError,'Guidance records exist'):command.downgrade(cfg,'009')
    with sqlite3.connect(file) as db:self.assertEqual(db.execute("SELECT revision FROM knowledge_cards WHERE id='guide-wireguard'").fetchone()[0],2)

 def test_receipt_migration_preserves_images_and_order_constraints(self):
  root=Path(__file__).resolve().parents[1]
  with tempfile.TemporaryDirectory() as folder:
   file=Path(folder)/'receipt.sqlite3';cfg=Config(str(root/'alembic.ini'));cfg.set_main_option('script_location',str(root/'backend/migrations'))
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(file)}):
    command.upgrade(cfg,'head');command.downgrade(cfg,'010');command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:
     db.execute("INSERT INTO tenants VALUES ('t','Cafe')");db.execute("INSERT INTO users (id,username,password_hash,role,tenant_id,active) VALUES ('u','guest','test-only-hash','guest','t',1)");db.execute("INSERT INTO packages VALUES ('p','t','{}',1)")
     db.execute("INSERT INTO orders (id,tenant_id,guest_id,package_id,snapshot,amount,currency,method,reference,evidence,status,fulfillment,created,request_key) VALUES ('o','t','u','p','{}',1000,'MMK','Cash','ref','text','pending','pending_adapter',1,'test-request-key')")
     db.execute("INSERT INTO receipt_images VALUES ('r','o','u','test-upload-key','digest',?,20,30,1)",(b'test-image',))
     self.assertEqual(db.execute("SELECT receipt_revision FROM orders WHERE id='o'").fetchone()[0],0)
     with self.assertRaises(sqlite3.IntegrityError):db.execute('UPDATE orders SET amount=0')
    with self.assertRaisesRegex(RuntimeError,'Receipt records exist'):command.downgrade(cfg,'010')
    with sqlite3.connect(file) as db:self.assertEqual(db.execute('SELECT image FROM receipt_images').fetchone()[0],b'test-image');self.assertEqual(db.execute('PRAGMA foreign_key_check').fetchall(),[])

 def test_review_api_migration_preserves_credentials(self):
  root=Path(__file__).resolve().parents[1]
  with tempfile.TemporaryDirectory() as folder:
   file=Path(folder)/'review-api.sqlite3';cfg=Config(str(root/'alembic.ini'));cfg.set_main_option('script_location',str(root/'backend/migrations'))
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(file)}):
    command.upgrade(cfg,'head');command.downgrade(cfg,'011');command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:
     db.execute("INSERT INTO tenants VALUES ('t','Cafe')");db.execute("INSERT INTO users (id,username,password_hash,role,tenant_id,active) VALUES ('u','owner','test-only-hash','sme','t',1)")
     db.execute("INSERT INTO review_credentials (id,tenant_id,user_id,label,provider,reviewer_identity,can_decide,token_hash,created,expires,revoked_at,revoke_reason) VALUES ('c','t','u','Test bot','custom','user-id',0,'digest',1,100,NULL,NULL)")
    with self.assertRaisesRegex(RuntimeError,'Review integration records exist'):command.downgrade(cfg,'011')
    with sqlite3.connect(file) as db:self.assertEqual(db.execute('SELECT label FROM review_credentials').fetchone()[0],'Test bot');self.assertEqual(db.execute('PRAGMA foreign_key_check').fetchall(),[])

 def test_delivery_migration_preserves_legacy_events_and_new_history(self):
  root=Path(__file__).resolve().parents[1]
  with tempfile.TemporaryDirectory() as folder:
   file=Path(folder)/'delivery.sqlite3';cfg=Config(str(root/'alembic.ini'));cfg.set_main_option('script_location',str(root/'backend/migrations'))
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(file)}):
    command.upgrade(cfg,'012')
    with sqlite3.connect(file) as db:
     db.execute("INSERT INTO tenants VALUES ('t','Cafe')");db.execute("INSERT INTO users (id,username,password_hash,role,tenant_id,active) VALUES ('u','owner','test-only-hash','sme','t',1)");db.execute("INSERT INTO packages VALUES ('p','t','{}',1)")
     db.execute("INSERT INTO orders (id,tenant_id,guest_id,package_id,snapshot,amount,currency,method,reference,evidence,status,fulfillment,created,request_key) VALUES ('o','t','u','p','{}',1000,'MMK','Cash','ref','text','pending','pending_adapter',1,'test-request-key')")
     db.execute("INSERT INTO outbox_events VALUES ('legacy','t','o','payment.needs_evidence',0)")
    command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:self.assertEqual(db.execute('SELECT revision,created,payload FROM outbox_events').fetchone(),(None,None,None))
    command.downgrade(cfg,'012');command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:
     with self.assertRaises(sqlite3.IntegrityError):db.execute('UPDATE orders SET amount=0')
     db.execute("INSERT INTO outbox_events (id,tenant_id,order_id,event,delivered,revision,created,payload) VALUES ('new','t','o','payment.needs_evidence',0,1,10,'{}')")
    with self.assertRaisesRegex(RuntimeError,'Delivery/event records exist'):command.downgrade(cfg,'012')
    with sqlite3.connect(file) as db:self.assertEqual(db.execute('SELECT count(*) FROM outbox_events').fetchone()[0],2);self.assertEqual(db.execute('PRAGMA foreign_key_check').fetchall(),[])

 def test_onboarding_migration_preserves_existing_users_and_new_accounts(self):
  root=Path(__file__).resolve().parents[1]
  with tempfile.TemporaryDirectory() as folder:
   file=Path(folder)/'onboarding.sqlite3';cfg=Config(str(root/'alembic.ini'));cfg.set_main_option('script_location',str(root/'backend/migrations'))
   with patch.dict(os.environ,{'DATABASE_URL':'sqlite:///'+str(file)}):
    command.upgrade(cfg,'013')
    with sqlite3.connect(file) as db:
     db.execute("INSERT INTO users (id,username,password_hash,role,tenant_id,active) VALUES ('p','platform','existing-hash','platform',NULL,1)")
    command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:self.assertEqual(db.execute('SELECT must_change_password,temporary_password_expires FROM users').fetchone(),(0,None))
    command.downgrade(cfg,'013');command.upgrade(cfg,'head')
    with sqlite3.connect(file) as db:
     self.assertEqual(db.execute('SELECT password_hash FROM users').fetchone()[0],'existing-hash')
     with self.assertRaises(sqlite3.IntegrityError):db.execute("INSERT INTO users (id,username,password_hash,role,active) VALUES ('invalid','invalid','hash','sme',1)")
     db.execute("INSERT INTO tenants VALUES ('t','New business')")
     db.execute("INSERT INTO users (id,username,password_hash,role,tenant_id,active,must_change_password,temporary_password_expires) VALUES ('u','owner','hash','sme','t',1,1,100)")
     db.execute("INSERT INTO business_provisioning VALUES ('r','t','u','p','unique-key','digest',1)")
    with self.assertRaisesRegex(RuntimeError,'Onboarding/password records exist'):command.downgrade(cfg,'013')
    with sqlite3.connect(file) as db:self.assertEqual(db.execute('SELECT count(*) FROM business_provisioning').fetchone()[0],1);self.assertEqual(db.execute('PRAGMA foreign_key_check').fetchall(),[])
