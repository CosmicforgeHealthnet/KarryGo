import copy
import hashlib
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import zipfile

import yaml
import gate
import smtp_activate as client
import smtp_runtime as runtime
import smtp_vps as worker
from test_gate import Fake, SHA, context, proof


def request(operation="activate"):
    return {"schema": 1, "repository": gate.REPOSITORY, "release_sha": SHA,
            "services": "notification-service", "run_id": "100", "operation": operation,
            "restore_run_id": "200", "backup_run_id": "", "nonce": "a" * 32, "restore_manifest": None}


def api():
    result = Fake()
    result.secrets = []  # VPS mode requires no GitHub transport/audit/app secrets.
    result.payload["services"]["notification-service"] = copy.deepcopy(proof()["services"]["customer-service"])
    result.run.update(status="in_progress", run_attempt=1, head_branch="staging",
                      triggering_actor={"login": "initiator"}, actor={"login": "initiator"})
    return result


class VPSGates(unittest.TestCase):
    def test_certification_still_requires_real_restore_attestation(self):
        value = api()
        value.run["path"] = ".github/workflows/certify-staging-restore.yml"
        item = request("certify"); item["restore_manifest"] = value.payload
        self.assertEqual(worker.authorize(value, item)["kind"], "certify")
        item["restore_manifest"]["operator_attested"] = False
        with self.assertRaises(ValueError): worker.authorize(value, item)

    def test_loaded_unrelated_setting_change_is_rejected(self):
        baseline = {"APP_ENV": "staging", "JWT_SECRET": "test-only-original", **runtime.SETTINGS,
                    "NOTIFICATION_SMTP_PASSWORD": "re_test_only"}
        loaded = {**baseline, "JWT_SECRET": "test-only-changed"}
        item = {"Image": "image", "State": {"Health": {"Status": "healthy"}},
                "Config": {"Env": [k + "=" + v for k, v in loaded.items()]}}
        with patch.object(runtime, "container", return_value=item):
            with self.assertRaises(ValueError):
                runtime.verify({**runtime.SETTINGS, "NOTIFICATION_SMTP_PASSWORD": "re_test_only"}, "image", baseline)

    @unittest.skipIf(os.name == "nt", "POSIX secret permissions verified by Linux CI")
    def test_existing_vps_key_is_read_only_locally_and_rejects_unsafe_files(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "resend-smtp-key"
            path.write_text("re_test_only\n"); path.chmod(0o600)
            with patch.object(runtime, "RESEND_KEY", path):
                self.assertEqual(runtime.read_resend_key(), "re_test_only")
                path.chmod(0o644)
                with self.assertRaises(ValueError): runtime.read_resend_key()
                path.chmod(0o600)
                link = path.parent / "link"; link.symlink_to(path)
                with patch.object(runtime, "RESEND_KEY", link):
                    with self.assertRaises(ValueError): runtime.read_resend_key()

    def test_vps_credentials_do_not_remove_approval_ci_or_restore(self):
        self.assertEqual(worker.authorize(api(), request())["services"], ["notification-service"])
        for mutate in [lambda a: a.review.clear(), lambda a: a.info.update(protection_rules=[]),
                       lambda a: a.payload["services"].clear(), lambda a: a.ci.update(conclusion="failure"),
                       lambda a: a.inherited.append({"name": "VPS_SSH_KEY"}),
                       lambda a: a.inherited.append({"name": "KARRYGO_NOTIFICATIONS_RESEND_SMTP_PASSWORD"}),
                       lambda a: a.secrets.append({"name": "RELEASE_AUDIT_TOKEN"}),
                       lambda a: a.secrets.append({"name": "KARRYGO_NOTIFICATIONS_RESEND_SMTP_PASSWORD"}),
                       lambda a: a.run.update(status="completed"), lambda a: a.run.update(run_attempt=2),
                       lambda a: a.run.update(head_branch="main"),
                       lambda a: a.run.update(triggering_actor={"login": "other"})]:
            with self.subTest(mutate=mutate):
                value = api(); mutate(value)
                with self.assertRaises(ValueError): worker.authorize(value, request())
        with self.assertRaises(ValueError):
            gate.authorize(api(), context(), SHA, "customer-service", "platform", "200", credential_source="vps")

    def test_request_cannot_carry_secrets_shell_or_other_service(self):
        for field, value in [("password", "test-only"), ("command", "arbitrary"), ("services", "partner-web-service"),
                             ("operation", "deploy"), ("run_id", "../escape"), ("repository", "other/repo"),
                             ("restore_run_id", "unavailable")]:
            item = request(); item[field] = value
            with self.assertRaises(ValueError): worker.validate_request(item)

    def test_only_digest_verified_bounded_artifact_is_read(self):
        item = request()
        def archive(name="smtp-request.json"):
            output = io.BytesIO()
            with zipfile.ZipFile(output, "w") as z: z.writestr(name, json.dumps(item))
            return output.getvalue()
        for raw, digest in [(archive(), None), (archive("../bad"), "correct")]:
            class ArtifactAPI:
                def pages(self, *args):
                    return [{"id": 1, "name": "staging-smtp-request", "digest":
                             "sha256:" + hashlib.sha256(raw).hexdigest() if digest else "wrong"}]
                def raw(self, *args): return raw
            with self.assertRaises(ValueError): worker.download_request(ArtifactAPI(), "100")

    def test_non_root_or_deploy_writable_install_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "code.py"; path.write_text("code")
            if os.name != "nt": path.chmod(0o666)
            with self.assertRaises((ValueError, AttributeError)): worker.trusted(path)


class RequestAndReceipt(unittest.TestCase):
    def test_request_contains_no_credentials_and_requires_actual_certification(self):
        env = {"GITHUB_REPOSITORY": gate.REPOSITORY, "GITHUB_REF": "refs/heads/staging",
               "GITHUB_EVENT_NAME": "workflow_dispatch", "RELEASE_SHA": SHA, "GITHUB_SHA": SHA,
               "GITHUB_WORKFLOW_SHA": SHA, "RELEASE_SERVICES": "notification-service", "GITHUB_RUN_ATTEMPT": "1",
               "GITHUB_RUN_ID": "100", "SMTP_OPERATION": "activate", "RESTORE_RUN_ID": "200"}
        with patch.dict(os.environ, env):
            item = client.request(); worker.validate_request(item)
            self.assertEqual(set(item), worker.FIELDS)
            self.assertNotIn("password", item)
            with patch.dict(os.environ, {"RESTORE_RUN_ID": "unavailable"}):
                with self.assertRaises(ValueError): client.request()

    def test_forged_or_failed_receipt_cannot_report_success(self):
        item = request()
        class ReceiptAPI:
            creator = "attacker"
            status = "success"
            def get(self, path):
                if path.startswith("deployments?"):
                    return [{"id": 7, "payload": client.receipt_payload(item), "creator": {"login": self.creator}}]
                return [{"state": self.status, "creator": {"login": self.creator}}]
        with tempfile.TemporaryDirectory() as directory, patch.object(client.time, "sleep"):
            previous = Path.cwd(); os.chdir(directory)
            try:
                with patch.object(client.time, "monotonic", side_effect=[0, 0, 2]):
                    with self.assertRaisesRegex(ValueError, "No authenticated"): client.wait(item, ReceiptAPI(), "vps-receipt", timeout=1)
                value = ReceiptAPI(); value.creator = "vps-receipt"; value.status = "failure"
                with self.assertRaisesRegex(ValueError, "VPS operation failed"): client.wait(item, value, "vps-receipt")
                self.assertFalse(Path("smtp-receipt.json").exists())
                value.status = "success"
                client.wait(item, value, "vps-receipt")
                self.assertEqual(json.loads(Path("smtp-receipt.json").read_text())["state"], "success")
            finally: os.chdir(previous)

    def test_vps_authorizes_before_changes_and_never_replays_run(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "approved-revision").write_text(SHA)
            env = root / "owner.env"; env.write_text("APP_ENV=staging\n")
            current = {"Config": {"Env": ["APP_ENV=staging"]}}
            calls = []
            def post(writer, path, data):
                calls.append("post")
                return {"id": 7, "creator": {"login": "vps-receipt"}}
            with patch.object(worker, "INSTALL", root), patch.object(worker, "STATE", root / "state"), \
                 patch.object(worker, "trusted"), patch.object(worker, "authorize", side_effect=lambda *a: calls.append("gate") or {"initiator": "initiator"}), \
                 patch.object(worker, "post", side_effect=post), patch.object(runtime, "ENV", env), \
                 patch.object(runtime, "private"), patch.object(runtime, "private_directory"), \
                 patch.object(runtime, "container", return_value=current), \
                 patch.object(runtime, "transaction", side_effect=lambda *a: calls.append("transaction")) as transaction, \
                 patch.object(runtime, "atomic", side_effect=lambda p, data, info: p.write_bytes(data)), \
                 patch.object(worker.os, "fchmod", create=True):
                worker.handle(None, None, "vps-receipt", request())
                self.assertEqual(calls, ["gate", "post", "gate", "transaction", "post"])
                worker.handle(None, None, "vps-receipt", request())
                self.assertEqual(transaction.call_count, 1)
                with patch.object(worker, "authorize", side_effect=ValueError("no approval")):
                    with self.assertRaises(ValueError): worker.handle(None, None, "vps-receipt", request())
                self.assertEqual(transaction.call_count, 1)

    def test_certification_job_has_no_audit_or_shell_credentials(self):
        workflow = yaml.load(Path(".github/workflows/certify-staging-restore.yml").read_text(), Loader=yaml.BaseLoader)
        job = workflow["jobs"]["certify-notification"]
        self.assertNotIn("secrets.", str(job))
        self.assertEqual(job["environment"], "staging")
        self.assertEqual(job["permissions"]["deployments"], "read")
        self.assertIn("notification-service", job["if"])
        self.assertTrue(any(step.get("run") == "python scripts/release/smtp_activate.py wait" for step in job["steps"]))


if __name__ == "__main__": unittest.main()
