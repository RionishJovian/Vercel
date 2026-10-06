import unittest
import test_backend,test_guest_entry
class SitePlanningTests(unittest.TestCase):
 setUp=test_backend.AccountPaymentTests.setUp
 tearDown=test_backend.AccountPaymentTests.tearDown
 login=test_backend.AccountPaymentTests.login
 site=test_guest_entry.GuestEntryTests.site
 def test_scope_revisions_and_manual_budget(self):
  id=self.site();url='/v1/sites/'+id+'/planning';h=self.login('owner');initial=self.client.get(url,headers=h).json();self.assertEqual(initial['revision'],0)
  body={'expected_revision':0,'settings':{'mode':'failover','uplinks':[{'name':'Starlink'},{'name':'Backup','priority':2}],'budget':{'enabled':True,'limit_gb':500,'used_gb':250,'cycle_start':'2026-10-01','cycle_end':'2026-11-01','reading_date':'2026-10-06'},'notes':'Owner-entered test'}}
  r=self.client.post(url,headers=h,json=body);self.assertEqual(r.status_code,200);self.assertFalse(r.json()['usage_verified']);self.assertEqual(r.json()['revision'],1)
  self.assertEqual(self.client.post(url,headers=h,json=body).status_code,409)
  for name in ['guest','platform','other']:self.assertIn(self.client.get(url,headers=self.login(name)).status_code,[403,404])
 def test_invalid_cycle_uplinks_and_unknown_usage(self):
  id=self.site();url='/v1/sites/'+id+'/planning';h=self.login('owner')
  for settings in [{'mode':'balance','uplinks':[{'name':'Only'}]},{'uplinks':[{'name':'Same'},{'name':'same'}]},{'budget':{'enabled':True}},{'budget':{'used_gb':20,'cycle_start':'2026-10-01','cycle_end':'2026-11-01','reading_date':'2026-11-01'}}]:self.assertEqual(self.client.post(url,headers=h,json={'expected_revision':0,'settings':settings}).status_code,422)
  self.assertEqual(self.client.post(url,headers=h,json={'expected_revision':0,'settings':{}}).status_code,200)
 def test_topology_links_and_cross_editor_conflict(self):
  id=self.site();url='/v1/sites/'+id+'/planning';h=self.login('owner')
  topology={'nodes':[{'id':'r','name':'Router','kind':'router'},{'id':'m','name':'Local media','kind':'media_server'}],'links':[{'source':'r','target':'m','label':'LAN'}]}
  r=self.client.post(url,headers=h,json={'expected_revision':0,'settings':{'topology':topology}});self.assertEqual(r.status_code,200)
  settings=r.json()['settings'];settings['notes']='WAN editor preserves graph';self.assertEqual(self.client.post(url,headers=h,json={'expected_revision':1,'settings':settings}).status_code,200)
  self.assertEqual(self.client.post(url,headers=h,json={'expected_revision':1,'settings':settings}).status_code,409)
  for links in [[{'source':'r','target':'missing'}],[{'source':'r','target':'r'}],[{'source':'r','target':'m'},{'source':'m','target':'r'}]]:self.assertEqual(self.client.post(url,headers=h,json={'expected_revision':2,'settings':{'topology':{**topology,'links':links}}}).status_code,422)
