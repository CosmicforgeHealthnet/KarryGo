import copy
import datetime as dt
import json
from pathlib import Path
import unittest
from unittest.mock import patch
import gate
from test_gate import Fake, SHA, SERVICE, context
SERVICES = ['notification-service', 'verification-compliance-service']
ACTOR = 'CosmicforgeHealthnet'

class DelegatedScopedFeeds(unittest.TestCase):
    def policy(self):
        value = json.loads(Path('scripts/release/delegated-scoped-feeds.json').read_text())
        value['expires_at'] = (dt.datetime.now(dt.timezone.utc)+dt.timedelta(hours=1)).isoformat()
        return value

    def fixture(self):
        api=Fake(); api.info['protection_rules']=[]; api.review=[]; api.actors=[ACTOR]
        api.run['actor']['login']=ACTOR; api.cert['actor']['login']=ACTOR
        api.payload['services']={s:copy.deepcopy(api.payload['services'][SERVICE]) for s in SERVICES}
        ctx=context();ctx.update(actor=ACTOR,triggering_actor=ACTOR)
        return api,ctx

    def authorize(self,api,ctx):
        return gate.authorize(api,ctx,SHA,','.join(SERVICES),'platform','200')

    def test_exact_release_records_no_fictional_approver(self):
        policy=self.policy();api,ctx=self.fixture()
        with patch.object(gate,'scoped_feed_release_authorization',return_value=policy):
            result=self.authorize(api,ctx)
        self.assertEqual(result['approvers'],[])
        self.assertEqual(result['authorization'],policy)
        self.assertEqual(result['restore']['run_id'],'200')

    def test_subsets_and_other_services_are_not_delegated(self):
        for services in [[SERVICES[0]],[SERVICES[1]],SERVICES+['customer-service'],['all'],['vendor-service'],['analytics-service'],['driver-dispatch-delivery-service']]:
            with self.subTest(services=services):
                self.assertIsNone(gate.scoped_feed_release_authorization(services,ACTOR))

    def test_invalid_policy_actor_expiry_and_duplicates_are_rejected(self):
        original=self.policy()
        edits=[{'expires_at':'2000-01-01T00:00:00Z'},{'expires_at':(dt.datetime.now(dt.timezone.utc)+dt.timedelta(days=8)).isoformat()},{'expires_at':'invalid'},{'schema':True},{'schema':2},{'services':['all']},{'initiators':['other']},{'mode':'skip_checks'},{'authority_reference':''},{'unexpected':True}]
        for edit in edits:
            with self.subTest(edit=edit),patch.object(Path,'read_text',return_value=json.dumps({**original,**edit})):
                with self.assertRaises((ValueError,TypeError)):
                    gate.scoped_feed_release_authorization(SERVICES,ACTOR)
        with patch.object(Path,'read_text',return_value=json.dumps(original)):
            with self.assertRaisesRegex(ValueError,'actor lacks'):
                gate.scoped_feed_release_authorization(SERVICES,'other')
        with patch.object(Path,'is_file',return_value=False):
            self.assertIsNone(gate.scoped_feed_release_authorization(SERVICES,ACTOR))
        with patch.object(Path,'read_text',return_value='{"schema":1,"schema":1}'):
            with self.assertRaisesRegex(ValueError,'duplicate'):
                gate.scoped_feed_release_authorization(SERVICES,ACTOR)

    def test_other_release_guards_still_apply(self):
        policy=self.policy()
        mutations=[lambda a:a.info.update(can_admins_bypass=True),lambda a:a.info.update(deployment_branch_policy=None),lambda a:a.secrets.clear(),lambda a:a.actors.clear(),lambda a:setattr(a,'permission','write'),lambda a:setattr(a,'head','f'*40),lambda a:a.ci.update(conclusion='failure'),lambda a:a.jobs.clear(),lambda a:setattr(a,'bad_digest',True),lambda a:a.payload.update(operator_attested=False),lambda a:a.payload['services'][SERVICES[1]].update(runtime_compatibility_attested=False),lambda a:a.cert.update(path='.github/workflows/unrelated.yml')]
        for mutation in mutations:
            api,ctx=self.fixture();mutation(api)
            with self.subTest(mutation=mutation),patch.object(gate,'scoped_feed_release_authorization',return_value=policy):
                with self.assertRaises(ValueError):self.authorize(api,ctx)

if __name__=='__main__':unittest.main()
