import copy
import json
from pathlib import Path
import unittest
from unittest.mock import patch

import gate
from test_gate import Fake, SHA, SERVICE, context

SUPPORT = "support-dispute-service"
ACTOR = "CosmicforgeHealthnet"


class DelegatedSupportRelease(unittest.TestCase):
    def setup_case(self):
        api = Fake()
        api.info["protection_rules"] = []
        api.review = []
        api.actors = [ACTOR]
        api.run["actor"]["login"] = ACTOR
        api.cert["actor"]["login"] = ACTOR
        api.payload["services"][SUPPORT] = copy.deepcopy(api.payload["services"][SERVICE])
        ctx = context()
        ctx.update(actor=ACTOR, triggering_actor=ACTOR)
        return api, ctx

    def authorize(self, api, ctx):
        return gate.authorize(api, ctx, SHA, SUPPORT, "platform", "200")

    def test_explicit_delegation_records_no_fictional_approver(self):
        api, ctx = self.setup_case()
        result = self.authorize(api, ctx)
        self.assertEqual(result["authorization"]["mode"], "delegated_operator")
        self.assertEqual(result["authorization"]["services"], [SUPPORT])
        self.assertEqual(result["approvers"], [])
        self.assertEqual(result["restore"]["run_id"], "200")

    def test_restore_certification_uses_the_same_explicit_delegation(self):
        api, ctx = self.setup_case()
        api.run["path"] = ".github/workflows/certify-staging-restore.yml"
        result = gate.authorize(api, ctx, SHA, SUPPORT, "certify", "", api.payload)
        self.assertEqual(result["authorization"]["mode"], "delegated_operator")
        self.assertEqual(result["approvers"], [])

    def test_other_services_still_require_independent_review(self):
        api, ctx = self.setup_case()
        for service in [SERVICE, "vendor-service"]:
            with self.subTest(service=service), self.assertRaisesRegex(ValueError, "separate reviewers"):
                kind = "vendor" if service == "vendor-service" else "platform"
                api.run["path"] = ".github/workflows/deploy-" + ("vendor-service" if kind == "vendor" else "platform") + ".yml"
                gate.authorize(api, ctx, SHA, service, kind, "200")

    def test_delegation_does_not_relax_remaining_denial_paths(self):
        mutations = [lambda a: a.info.update(can_admins_bypass=True),
                     lambda a: a.info.update(deployment_branch_policy=None),
                     lambda a: a.policies.append({"name": "*", "type": "tag"}),
                     lambda a: a.secrets.clear(),
                     lambda a: a.inherited.append({"name": "VPS_SSH_KEY"}),
                     lambda a: a.actors.clear(),
                     lambda a: setattr(a, "permission", "write"),
                     lambda a: setattr(a, "head", "f" * 40),
                     lambda a: a.ci.update(conclusion="failure"),
                     lambda a: a.jobs.clear(),
                     lambda a: setattr(a, "bad_digest", True),
                     lambda a: a.payload.update(operator_attested=False),
                     lambda a: a.payload["services"][SUPPORT].update(runtime_compatibility_attested=False),
                     lambda a: a.cert.update(path=".github/workflows/unrelated.yml")]
        for mutation in mutations:
            with self.subTest(mutation=mutation):
                api, ctx = self.setup_case()
                mutation(api)
                with self.assertRaises(ValueError):
                    self.authorize(api, ctx)

    def test_actor_must_be_named_in_both_delegation_and_environment_allowlist(self):
        with self.assertRaisesRegex(ValueError, "explicit Support deployment delegation"):
            gate.release_authorization([SUPPORT], "unlisted-user")

    def test_missing_delegation_returns_to_independent_review(self):
        with patch.object(Path, "is_file", return_value=False):
            self.assertEqual(gate.release_authorization([SUPPORT], ACTOR), {"mode": "independent_review"})
            api, ctx = self.setup_case()
            with self.assertRaisesRegex(ValueError, "separate reviewers"):
                self.authorize(api, ctx)

    def test_malformed_or_broadened_delegation_fails_closed(self):
        original = json.loads(Path("scripts/release/delegated-support.json").read_text())
        for edit in [{"services": ["all"]}, {"services": [SERVICE]}, {"schema": 2},
                     {"mode": "skip_checks"}, {"initiators": []}, {"authority_reference": ""},
                     {"unexpected": True}]:
            with self.subTest(edit=edit), patch.object(Path, "read_text", return_value=json.dumps({**original, **edit})):
                with self.assertRaises(ValueError):
                    gate.release_authorization([SUPPORT], ACTOR)
        with patch.object(Path, "read_text", return_value='{"schema":1,"schema":1}'):
            with self.assertRaisesRegex(ValueError, "duplicate"):
                gate.release_authorization([SUPPORT], ACTOR)


if __name__ == "__main__":
    unittest.main()
