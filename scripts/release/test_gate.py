import copy
import datetime as dt
import hashlib
import io
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import zipfile

import yaml
import gate
import execute

SHA = "a" * 40
SERVICE = "customer-service"


def proof():
    return {"schema": 1, "environment": "staging", "release_sha": SHA,
            "completed_at": dt.datetime.now(dt.timezone.utc).isoformat(), "operator_attested": True,
            "services": {s: {"validated": True, "commands_succeeded": True, "backup_sha256": "b" * 64,
                             "validation_sha256": "c" * 64, "source_database_version": 16,
                             "restore_database_version": 16, "artifact_location": "protected-offsite/backup.sql.gz",
                             "validation_summary": "ledger, native rows, receipts, constraints and audit checked",
                             "runtime_compatibility_attested": True}
                         for s in [SERVICE, "vendor-service"]}}


class Fake:
    def __init__(self, kind="platform"):
        self.info = {"id": 1, "can_admins_bypass": False,
                     "protection_rules": [{"type": "required_reviewers", "reviewers": [{"type": "User", "reviewer": {"id": 2}}], "prevent_self_review": True}],
                     "deployment_branch_policy": {"protected_branches": False, "custom_branch_policies": True}}
        self.policies = [{"name": "staging", "type": "branch"}]
        self.secrets = [{"name": s} for s in gate.CREDENTIALS]
        self.inherited = []
        self.review = [{"state": "approved", "user": {"login": "approver", "id": 2}, "environments": [{"id": 1}]}]
        self.run = {"path": ".github/workflows/deploy-" + ("platform" if kind == "platform" else "vendor-service") + ".yml",
                    "event": "workflow_dispatch", "head_sha": SHA, "actor": {"login": "initiator"}}
        self.cert = {"path": ".github/workflows/certify-staging-restore.yml", "head_sha": SHA,
                     "head_branch": "staging", "event": "workflow_dispatch", "status": "completed",
                     "conclusion": "success", "run_attempt": 1, "actor": {"login": "initiator"}}
        self.ci = {"id": 300, "head_sha": SHA, "event": "pull_request", "status": "completed", "conclusion": "success", "run_attempt": 1}
        self.jobs = [{"conclusion": "success"}] * 30
        self.head = SHA
        self.actors = ["initiator"]
        self.permission = "maintain"
        self.payload = proof()
        self.artifact = {"id": 90, "name": "staging-restore-evidence", "expired": False}
        self.artifact_missing = False
        self.bad_digest = False
        self.missing_ci = False

    def get(self, path):
        if path == "": return {"owner": {"type": "User"}}
        if path == "environments/staging": return self.info
        if path.endswith("/STAGING_RELEASE_INITIATORS"): return {"value": json.dumps(self.actors)}
        if path == "git/ref/heads/staging": return {"object": {"sha": self.head}}
        if path == "actions/runs/100": return self.run
        if path == "actions/runs/200": return self.cert
        if path.endswith("/permission"): return {"permission": self.permission}
        if path.endswith("/approvals"): return self.review
        raise AssertionError(path)

    def pages(self, path, key):
        if path.endswith("deployment-branch-policies"): return self.policies
        if path == "environments/staging/secrets": return self.secrets
        if path == "actions/secrets": return self.inherited
        if "/artifacts" in path:
            raw = self.raw("actions/artifacts/90/zip")
            self.artifact["digest"] = "sha256:" + hashlib.sha256(raw).hexdigest() if not self.bad_digest else "wrong"
            return [] if self.artifact_missing else [self.artifact]
        if "/runs?head_sha=" in path: return [] if self.missing_ci else [self.ci]
        if path.endswith("/jobs"): return self.jobs
        raise AssertionError(path)

    def raw(self, path):
        data = io.BytesIO()
        with zipfile.ZipFile(data, "w") as archive:
            info = zipfile.ZipInfo("restore-evidence.json", date_time=(2026, 1, 1, 0, 0, 0))
            archive.writestr(info, json.dumps(self.payload))
        return data.getvalue()


def context():
    return {"repository": gate.REPOSITORY, "event_name": "workflow_dispatch", "ref": "refs/heads/staging",
            "sha": SHA, "workflow_sha": SHA, "run_attempt": "1", "run_id": "100",
            "actor": "initiator", "triggering_actor": "initiator"}


class Gates(unittest.TestCase):
    def authorize(self, api=None, ctx=None, sha=SHA, services=SERVICE, kind="platform", restore_id="200"):
        return gate.authorize(api or Fake(), ctx or context(), sha, services, kind, restore_id)

    def test_separately_approved_platform_and_vendor(self):
        for kind, service in [("platform", SERVICE), ("vendor", "vendor-service")]:
            result = self.authorize(Fake(kind), services=service, kind=kind)
            self.assertEqual(result["approvers"], ["approver"])
            result["images"] = {service: f"ghcr.io/cosmicforgehealthnet/{service}@sha256:" + "d" * 64}
            result["remote"] = "/opt/karrygo/releases/" + SHA + "/100"
            calls = []
            # Execute the approved command plan against a no-network disposable
            # executor, not a host. Genuine database restore is tested separately.
            with patch.object(execute, "run", side_effect=lambda args, **kw: calls.append((args, kw))):
                for plan in execute.command_plan(result): execute.run(["sandbox-bash"], input=plan)
            self.assertEqual(len(calls), 1)
            self.assertNotIn("@sha256:staging", calls[0][1]["input"])

    def test_unapproved_contexts(self):
        for field, value in [("event_name", "push"), ("event_name", "workflow_call"), ("ref", "refs/heads/evil"),
                             ("sha", "f" * 40), ("workflow_sha", "f" * 40), ("run_attempt", "2"),
                             ("triggering_actor", "other"), ("repository", "attacker/fork")]:
            with self.subTest(field=field, value=value):
                ctx = context(); ctx[field] = value
                with self.assertRaises(ValueError): self.authorize(ctx=ctx)

    def test_unprotected_or_unapproved_environment(self):
        mutations = [lambda a: a.info.update(protection_rules=[]), lambda a: a.info.update(can_admins_bypass=True),
                     lambda a: a.info["protection_rules"][0].update(prevent_self_review=False),
                     lambda a: a.info["protection_rules"][0].update(reviewers=[]),
                     lambda a: a.info.update(deployment_branch_policy=None),
                     lambda a: a.policies.append({"name": "*", "type": "tag"}),
                     lambda a: a.secrets.clear(), lambda a: a.inherited.append({"name": "VPS_SSH_KEY"}),
                     lambda a: a.inherited.append({"name": "STAGING_DEPLOY_SSH_KEY"}),
                     lambda a: a.actors.clear(), lambda a: a.actors.append("other") or a.actors.remove("initiator"),
                     lambda a: setattr(a, "permission", "write"), lambda a: a.review.clear(),
                     lambda a: a.review[0]["user"].update(login="initiator"),
                     lambda a: a.review[0]["user"].update(id=999),
                     lambda a: a.review[0].update(state="rejected")]
        for mutation in mutations:
            with self.subTest(mutation=mutation):
                api = Fake(); mutation(api)
                with self.assertRaises(ValueError): self.authorize(api)

    def test_api_denial_fails_closed(self):
        with patch.object(Fake, "get", side_effect=PermissionError("API cannot verify")):
            with self.assertRaises(PermissionError): self.authorize()

    def test_stale_sha_failed_missing_and_skipped_ci(self):
        mutations = [lambda a: setattr(a, "head", "f" * 40), lambda a: a.ci.update(conclusion="failure"),
                     lambda a: a.ci.update(status="in_progress"), lambda a: setattr(a, "missing_ci", True),
                     lambda a: a.ci.update(head_sha="f" * 40), lambda a: a.jobs[0].update(conclusion="skipped"),
                     lambda a: a.jobs.clear()]
        for mutation in mutations:
            with self.subTest(mutation=mutation):
                api = Fake(); api.jobs = copy.deepcopy(api.jobs); mutation(api)
                with self.assertRaises(ValueError): self.authorize(api)
        with self.assertRaises(ValueError): self.authorize(sha="abc")

    def test_backup_and_certification_provenance_failures(self):
        mutations = [lambda a: setattr(a, "artifact_missing", True), lambda a: setattr(a, "bad_digest", True),
                     lambda a: a.cert.update(path=".github/workflows/attacker.yml"),
                     lambda a: a.cert.update(head_sha="f" * 40), lambda a: a.cert.update(conclusion="failure"),
                     lambda a: a.payload.update(environment="disposable"), lambda a: a.payload.update(operator_attested=False),
                     lambda a: a.payload.update(completed_at="2020-01-01T00:00:00Z"),
                     lambda a: a.payload["services"].clear(),
                     lambda a: a.payload["services"][SERVICE].update(validated=False),
                     lambda a: a.payload["services"][SERVICE].update(runtime_compatibility_attested=False),
                     lambda a: a.payload["services"][SERVICE].update(restore_database_version=18)]
        for mutation in mutations:
            with self.subTest(mutation=mutation):
                api = Fake(); mutation(api)
                with self.assertRaises(ValueError): self.authorize(api)
        with self.assertRaises(ValueError): self.authorize(restore_id="")

    def test_broad_and_cutover_selectors_never_spawn(self):
        for service in ["all", "*", "", SERVICE + "," + SERVICE, "vendor-service", "customer-service;echo evil", *gate.FORBIDDEN]:
            with self.subTest(service=service), patch.object(execute, "run") as process:
                with self.assertRaises(ValueError): self.authorize(services=service)
                process.assert_not_called()
        with self.assertRaises(ValueError): self.authorize(Fake("vendor"), kind="vendor", services=SERVICE)

    def test_digest_cannot_be_replaced_by_a_tag(self):
        result = self.authorize(); result["images"] = {SERVICE: "ghcr.io/cosmicforgehealthnet/customer-service:staging"}
        with self.assertRaises(ValueError): execute.command_plan(result)

    def test_modified_release_bundle_cannot_change_approved_service_selection(self):
        result = self.authorize()
        result["services"] = ["fleet-partner-service"]
        with patch.dict(os.environ, {"GITHUB_RUN_ID": "100", "RELEASE_SHA": SHA, "RELEASE_SERVICES": SERVICE}), \
             patch.object(gate, "GitHub") as api, patch.object(execute, "run") as command:
            with self.assertRaisesRegex(ValueError, "selection differs"):
                execute.deploy(result)
            api.assert_not_called()
            command.assert_not_called()

    def test_workflow_surfaces_and_credentials(self):
        for name in ["deploy-platform.yml", "deploy-vendor-service.yml", "certify-staging-restore.yml"]:
            data = yaml.load(Path(".github/workflows", name).read_text(), Loader=yaml.BaseLoader)
            self.assertEqual(set(data["on"]), {"workflow_dispatch"})
            self.assertEqual(data["concurrency"]["group"], "staging-release-runtime")
            self.assertEqual(data["concurrency"]["cancel-in-progress"], "false")
            job = next(iter(data["jobs"].values()))
            self.assertEqual(job["environment"], "staging")
            self.assertIn("github.ref == 'refs/heads/staging'", job["if"])
            self.assertNotIn("skip_tests", str(data))
            self.assertNotIn("secrets.VPS_", str(data))
            self.assertNotIn("continue-on-error", str(data))
            inputs = data["on"]["workflow_dispatch"]["inputs"]
            self.assertNotIn("default", inputs["services"])
            self.assertEqual(data["permissions"], {"contents": "read"})

    def test_actual_dispatch_scripts_refuse_deploy_and_rollback_before_host_tools(self):
        bash = "C:/Program Files/Git/bin/bash.exe" if os.name == "nt" else shutil.which("bash")
        root = Path.cwd()
        with tempfile.TemporaryDirectory() as directory:
            env = dict(os.environ, KARRYGO_ROOT=directory, PLATFORM_DIR=(root / "deploy/platform").as_posix())
            for name, args in [("deploy-service.sh", ["driver-dispatch-delivery-service", "ghcr.io/example@sha256:" + "a" * 64]),
                               ("rollback.sh", ["driver-dispatch-delivery-service"])]:
                result = subprocess.run([bash, (root / "deploy/platform/bin" / name).as_posix(), *args],
                                        env=env, capture_output=True, text=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("BLOCKED", result.stderr)
                self.assertEqual(list(Path(directory).iterdir()), [])

    def test_enabled_admin_flags_block_replacement_without_printing_values(self):
        spec = importlib.util.spec_from_file_location("admin_flags", "deploy/platform/bin/check-admin-disabled.py")
        module = importlib.util.module_from_spec(spec); spec.loader.exec_module(module)
        with tempfile.TemporaryDirectory() as directory:
            env = Path(directory) / "owner.env"
            env.write_text("TAXI_ADMIN_READ_ENABLED=false\nCUSTOMER_ADMIN_RESOURCE_READ_ENABLED=false\n")
            module.check(env)
            env.write_text("TAXI_ADMIN_READ_ENABLED=true\n")
            with self.assertRaises(ValueError): module.check(env)

    def test_review_branch_vercel_previews_are_disabled_only_for_this_branch(self):
        for directory in [".", "apps/admin", "web", "web/fleet", "web/karryam-landing", "web/partner-web"]:
            data = json.loads((Path(directory) / "vercel.json").read_text())
            self.assertEqual(data["git"]["deploymentEnabled"], {"codex/staging-secure-admin-integration-20261020": False})


if __name__ == "__main__":
    unittest.main()
