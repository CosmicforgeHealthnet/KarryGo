"""Private Analytics topology invariants; no host mutation or credentials."""
from pathlib import Path
import unittest
import yaml
ROOT=Path(__file__).resolve().parents[2]
class AnalyticsExposure(unittest.TestCase):
 def test_only_app_reaches_source_network(self):
  c=yaml.safe_load((ROOT/'deploy/platform/compose/analytics-service.yml').read_text())
  self.assertTrue(c['networks']['analytics-private']['internal'])
  self.assertTrue(c['networks']['source-readers']['external'])
  self.assertEqual(set(c['services']['app']['networks']), {'analytics-private','source-readers'})
  for name in ('db','redis'):
   self.assertEqual(c['services'][name]['networks'],['analytics-private'])
  for s in c['services'].values():
   self.assertFalse(s.get('ports'));self.assertFalse(s.get('privileged',False));self.assertTrue(s['read_only'])
   self.assertIn('no-new-privileges:true',s['security_opt'])
 def test_template_cannot_supply_development_credentials(self):
  text=(ROOT/'deploy/platform/env/analytics.env.example').read_text()
  self.assertNotIn('development-analytics-service-secret',text)
  for key in ('ANALYTICS_DATABASE_URL','ANALYTICS_REDIS_PASSWORD','ANALYTICS_SERVICE_SECRETS'):
   self.assertIn(key+'=\n',text)
if __name__=='__main__':unittest.main()
