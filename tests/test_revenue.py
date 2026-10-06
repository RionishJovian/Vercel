import unittest
from datetime import datetime,timezone
from unittest.mock import patch
from sqlalchemy import select
import test_backend as fixture
from backend.models import FinancialEvent,Order
class RevenueTests(unittest.TestCase):
 setUp=fixture.AccountPaymentTests.setUp
 tearDown=fixture.AccountPaymentTests.tearDown
 login=fixture.AccountPaymentTests.login
 package=fixture.AccountPaymentTests.package
 order=fixture.AccountPaymentTests.order
 def collect(self,site=None):
  owner=self.login('owner');guest=self.login('guest');p=self.package(owner)
  b={'package_id':p,'portal_revision':1,'method':'Cash','reference':'Receipt','evidence':'Received at counter','request_key':'revenue-order-001'}
  if site:b['site_id']=site
  o=self.client.post('/v1/orders',headers=guest,json=b);self.assertEqual(o.status_code,201,o.text)
  r=self.client.post('/v1/orders/'+o.json()['id']+'/decision',headers=owner,json={'decision':'approved'});self.assertEqual(r.status_code,200,r.text)
  return owner,o.json()['id']
 def ts(self,text):return int(datetime.fromisoformat(text).replace(tzinfo=timezone.utc).timestamp())
 def test_event_dates_refund_period_and_retry(self):
  owner,id=self.collect()
  with self.app.state.factory() as s:
   e=s.scalar(select(FinancialEvent));e.occurred_at=self.ts('2026-09-30T18:00:00');s.commit()
  # Receipt is Sep 30 UTC, but Oct 1 in Myanmar.
  r=self.client.get('/v1/revenue',headers=owner,params={'start':'2026-10-01','end':'2026-10-01'}).json();self.assertEqual(r['gross_collections'],3000);self.assertEqual(r['collection_count'],1);self.assertEqual(r['by_day'][0]['date'],'2026-10-01')
  r=self.client.get('/v1/revenue',headers=owner,params={'start':'2026-09-30','end':'2026-09-30','timezone':'UTC'}).json();self.assertEqual(r['gross_collections'],3000)
  self.client.post('/v1/orders/'+id+'/decision',headers=owner,json={'decision':'approved'})
  self.assertEqual(self.client.post('/v1/orders/'+id+'/refund',headers=owner,json={'reason':'Returned at counter'}).status_code,200)
  with self.app.state.factory() as s:e=s.scalar(select(FinancialEvent).where(FinancialEvent.kind=='refund'));e.occurred_at=self.ts('2026-10-03T10:00:00');s.commit()
  r=self.client.get('/v1/revenue',headers=owner,params={'start':'2026-10-01','end':'2026-10-01'}).json();self.assertEqual(r['net_collections'],3000)
  r=self.client.get('/v1/revenue',headers=owner,params={'start':'2026-10-03','end':'2026-10-03'}).json();self.assertEqual(r['net_collections'],-3000);self.assertEqual(r['refund_count'],1);self.assertEqual(r['collection_count'],0)
  r=self.client.get('/v1/revenue',headers=owner).json();self.assertEqual(r['net_collections'],0);self.assertEqual(r['gross_collections'],3000);self.assertEqual(r['refunds'],3000)
  self.assertEqual(self.client.get('/v1/revenue/ledger',headers=owner).json()['total'],2)
 def test_inclusive_dates_boundaries_and_validation(self):
  owner,id=self.collect()
  with self.app.state.factory() as s:e=s.scalar(select(FinancialEvent));e.occurred_at=self.ts('2026-10-01T17:30:00');s.commit()
  # Oct 2 00:00 Myanmar: excluded from Oct 1; included in Oct 2.
  self.assertEqual(self.client.get('/v1/revenue',headers=owner,params={'start':'2026-10-01','end':'2026-10-01'}).json()['net_collections'],0)
  self.assertEqual(self.client.get('/v1/revenue',headers=owner,params={'start':'2026-10-02','end':'2026-10-02'}).json()['net_collections'],3000)
  for params in [{'start':'2026-10-01'},{'end':'2026-10-01'},{'timezone':'Bad/Zone'},{'start':'2026-10-02','end':'2026-10-01'},{'start':'2020-01-01','end':'2026-10-01'},{'start':'invalid','end':'2026-10-01'},{'start':'9999-12-31','end':'9999-12-31'}]:self.assertEqual(self.client.get('/v1/revenue',headers=owner,params=params).status_code,422)
 def test_role_site_scope_and_ledger_pagination(self):
  owner,id=self.collect();other=self.login('other')
  for name in ['guest','platform']:
   for path in ['revenue','revenue/ledger']:self.assertEqual(self.client.get('/v1/'+path,headers=self.login(name)).status_code,403)
  self.assertEqual(self.client.get('/v1/revenue/ledger',headers=other).json()['total'],0)
  site=self.client.post('/v1/sites',headers=owner,json={'name':'Main cafe'}).json()['id']
  with self.app.state.factory() as s:e=s.scalar(select(FinancialEvent));e.site_id=site;s.commit()
  self.assertEqual(self.client.get('/v1/revenue',headers=owner,params={'site_id':site}).json()['net_collections'],3000)
  self.assertEqual(self.client.get('/v1/revenue',headers=owner,params={'site_id':'unassigned'}).json()['net_collections'],0)
  self.assertEqual(self.client.get('/v1/revenue',headers=other,params={'site_id':site}).status_code,404)
  self.assertEqual(self.client.get('/v1/revenue/ledger',headers=other,params={'site_id':site}).status_code,404)
  self.client.post('/v1/orders/'+id+'/refund',headers=owner,json={'reason':'Returned'})
  a=self.client.get('/v1/revenue/ledger',headers=owner,params={'limit':1}).json();b=self.client.get('/v1/revenue/ledger',headers=owner,params={'limit':1,'offset':1}).json()
  self.assertTrue(a['has_more']);self.assertFalse(b['has_more']);self.assertNotEqual(a['items'][0]['id'],b['items'][0]['id'])
  self.assertEqual(self.client.get('/v1/revenue/ledger',headers=owner,params={'limit':10001}).status_code,422)
 def test_financial_snapshot_survives_source_changes(self):
  owner,id=self.collect()
  with self.app.state.factory() as s:o=s.get(Order,id);o.reference='Edited';o.snapshot={**o.snapshot,'name':'Changed'};s.commit()
  row=self.client.get('/v1/revenue/ledger',headers=owner).json()['items'][0];self.assertEqual(row['package_name'],'Combo');self.assertEqual(row['reference'],'Receipt');self.assertEqual(row['timestamp_basis'],'recorded_event');self.assertEqual(row['signed_amount'],3000)
 def test_pending_workload_uses_submission_time_not_collections(self):
  owner=self.login('owner');p=self.package(owner);guest=self.login('guest');o=self.order(guest,p).json()
  with self.app.state.factory() as s:order=s.get(Order,o['id']);order.created=self.ts('2026-09-01T10:00:00');s.commit()
  r=self.client.get('/v1/revenue',headers=owner,params={'start':'2026-09-01','end':'2026-09-01'}).json();self.assertEqual(r['pending_orders'],1);self.assertEqual(r['pending_amount'],3000);self.assertEqual(r['pending_over_24h'],1);self.assertEqual(r['gross_collections'],0)
  self.assertEqual(self.client.get('/v1/revenue',headers=owner,params={'start':'2026-10-01','end':'2026-10-01'}).json()['pending_orders'],0)
