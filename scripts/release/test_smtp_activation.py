import copy
import json
import os
from pathlib import Path
import stat
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import yaml
import gate
import smtp_activate
import smtp_runtime as runtime
from test_gate import Fake, SHA, context, proof


class SMTPAuthorization(unittest.TestCase):
    def fake(self):
        api = Fake()
        api.payload["services"]["notification-service"] = copy.deepcopy(proof()["services"]["customer-service"])
        return api

    def authorize(self, api=None, ctx=None, services="notification-service"):
        return gate.authorize(api or self.fake(), ctx or context(), SHA, services, "smtp", "200")

    def test_uses_existing_full_release_gates(self):
        self.assertEqual(self.authorize()["services"], ["notification-service"])
        for change in [lambda a: a.info.update(protection_rules=[]), lambda a: a.secrets.clear(),
                       lambda a: a.review.clear(), lambda a: a.inherited.append({"name": "VPS_SSH_KEY"}),
                       lambda a: a.payload["services"].clear(), lambda a: a.run.update(path=".github/workflows/deploy-vendor-service.yml")]:
            api = self.fake(); change(api)
            with self.assertRaises(ValueError): self.authorize(api)
        for service in ["partner-web-service", "all", "customer-service", "notification-service,partner-web-service"]:
            with self.assertRaises(ValueError): self.authorize(services=service)
        for field, val in [("ref", "refs/heads/main"), ("event_name", "push"), ("run_attempt", "2")]:
            ctx = context(); ctx[field] = val
            with self.assertRaises(ValueError): self.authorize(ctx=ctx)

    def test_workflow_has_no_visible_credential_inputs_or_legacy_transport(self):
        workflow = yaml.load(Path(".github/workflows/deploy-platform.yml").read_text(), Loader=yaml.BaseLoader)
        self.assertEqual(set(workflow["on"]), {"workflow_dispatch"})
        self.assertEqual(set(workflow["on"]["workflow_dispatch"]["inputs"]), {"release_sha", "services", "restore_run_id", "operation", "backup_run_id"})
        self.assertEqual(workflow["concurrency"], {"group": "staging-release-runtime", "cancel-in-progress": "false"})
        job = workflow["jobs"]["smtp"]
        self.assertEqual(job["environment"], "staging")
        self.assertIn("refs/heads/staging", job["if"])
        self.assertNotIn("secrets.VPS_", str(workflow))
        self.assertNotIn("continue-on-error", str(workflow))

    def test_missing_protected_smtp_secret_never_starts_ssh(self):
        env = {"RELEASE_SHA": SHA, "RELEASE_SERVICES": "notification-service", "RESTORE_RUN_ID": "200",
               "SMTP_OPERATION": "activate", "GITHUB_RUN_ID": "100"}
        with patch.dict(os.environ, env), patch.object(gate, "authorize"), \
             patch.object(gate, "GitHub", return_value=self.fake()), patch.object(subprocess, "run") as process:
            with self.assertRaisesRegex(ValueError, "missing staging environment"):
                smtp_activate.main()
            process.assert_not_called()


class SMTPConfiguration(unittest.TestCase):
    def test_minimal_env_update_preserves_unrelated_bytes(self):
        original = '# keep comment\nAPP_ENV=staging\nDATABASE_URL="postgres://secret"\nREDIS_PASSWORD=secret\nFLAG=false\nNOTIFICATION_SMTP_HOST=old.example\n'
        updated = runtime.patch_env(original, {**runtime.SETTINGS, "NOTIFICATION_SMTP_PASSWORD": "re_test_only"})
        for line in original.splitlines(keepends=True)[:-1]:
            self.assertIn(line, updated)
        self.assertEqual(runtime.value(updated, "NOTIFICATION_EMAIL_FROM"), "Karryam <no-reply@karryam.com>")
        self.assertEqual(updated.count("NOTIFICATION_SMTP_HOST="), 1)
        self.assertEqual(runtime.patch_env(updated, runtime.SETTINGS), updated)

    def test_ambiguous_or_injected_settings_fail_before_edit(self):
        for data in ['NOTIFICATION_SMTP_HOST=a\nNOTIFICATION_SMTP_HOST=b\n', 'NOTIFICATION_SMTP_HOST="unterminated\n']:
            with self.assertRaises(ValueError): runtime.patch_env(data, runtime.SETTINGS)
        for secret in ['re_x\nEVIL=true', 're_x"', 're_${PASSWORD}', 're_x\r']:
            with self.assertRaises(ValueError): runtime.patch_env('', {"NOTIFICATION_SMTP_PASSWORD": secret})

    @unittest.skipIf(os.name == "nt", "POSIX ownership/atomicity checked by Linux CI")
    def test_private_atomic_file_and_symlink_rejection(self):
        with tempfile.TemporaryDirectory() as directory:
            env = Path(directory) / "owner.env"
            env.write_text("original")
            env.chmod(0o600)
            info = runtime.private(env)
            runtime.atomic(env, b"replacement", info)
            self.assertEqual(env.read_bytes(), b"replacement")
            self.assertEqual(stat.S_IMODE(env.stat().st_mode), 0o600)
            env.chmod(0o644)
            with self.assertRaises(ValueError): runtime.private(env)
            link = Path(directory) / "link.env"
            link.symlink_to(env)
            with self.assertRaises(ValueError): runtime.private(link)

    def test_apply_captures_all_logs_and_uses_only_existing_script(self):
        with patch.object(runtime.subprocess, "run", return_value=subprocess.CompletedProcess([], 1, b"secret", b"otp")) as run:
            with self.assertRaises(ValueError): runtime.apply()
            args, kwargs = run.call_args
            self.assertEqual(args[0], ["bash", str(runtime.APPLY), "notification-service"])
            self.assertTrue(kwargs["capture_output"])

    def test_backups_rollback_and_failed_activation_restore(self):
        # Exercise the real transaction on disposable files; mock only host
        # permissions/atomic POSIX primitive and Docker/apply dependencies.
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            root.chmod(0o700)
            env = root / "notification.service.env"
            original = b"APP_ENV=staging\nDATABASE_URL=private\n"
            env.write_bytes(original)
            (root / "release.env").write_text("IMAGE=image@sha256:abc\n")
            item = {"Image": "sha256:abc", "State": {"Health": {"Status": "healthy"}}, "Config": {"Env": ["APP_ENV=staging"]}}
            request = {"operation": "activate", "run_id": "100", "password": "re_test_only"}
            with patch.object(runtime, "ENV", env), patch.object(runtime, "BACKUPS", root / ".smtp-backups"), \
                 patch.object(runtime, "APPLY", Path(__file__)), patch.object(runtime, "private", return_value=env.stat()), \
                 patch.object(runtime, "private_directory"), \
                 patch.object(runtime, "private_service_directory"), \
                 patch.object(runtime, "smtp_auth"), \
                 patch.object(runtime, "atomic", side_effect=lambda p, data, info: p.write_bytes(data)), \
                 patch.object(runtime, "apply") as apply, patch.object(runtime, "verify") as verify, \
                 patch.object(runtime, "container", return_value=item), \
                 patch.object(runtime.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, b'[{"Id":"sha256:abc"}]')):
                self.assertEqual(runtime.transaction(request, item), "100")
                self.assertEqual((root / ".smtp-backups/100/before.env").read_bytes(), original)
                activated = env.read_bytes()
                self.assertIn(b"DATABASE_URL=private", activated)
                self.assertEqual(runtime.transaction({"operation": "rollback", "run_id": "101", "backup_run_id": "100"}, item), "100")
                self.assertEqual(env.read_bytes(), original)
                env.write_bytes(activated + b"UNRELATED_CHANGE=true\n")
                with self.assertRaises(ValueError):
                    runtime.transaction({"operation": "rollback", "run_id": "102", "backup_run_id": "100"}, item)
                env.write_bytes(original)
                verify.side_effect = ValueError("wrong loaded settings")
                with self.assertRaisesRegex(ValueError, "previous configuration restored"):
                    runtime.transaction({**request, "run_id": "103"}, item)
                self.assertEqual(env.read_bytes(), original)
                self.assertEqual(apply.call_count, 4)  # activate, rollback, failed apply + restore
                require_before = env.read_bytes()
                with self.assertRaises(ValueError): runtime.transaction({**request, "run_id": "../escape"}, item)
                self.assertEqual(env.read_bytes(), require_before)

    def test_smtp_probe_uses_tls_and_auth_without_sending(self):
        with patch.object(runtime.smtplib, "SMTP_SSL") as smtp:
            runtime.smtp_auth("re_test_only")
            args, kwargs = smtp.call_args
            self.assertEqual(args, ("smtp.resend.com", 465))
            self.assertEqual(kwargs["timeout"], 15)
            client = smtp.return_value.__enter__.return_value
            client.login.assert_called_once_with("resend", "re_test_only")
            client.sendmail.assert_not_called()
            client.send_message.assert_not_called()


if __name__ == "__main__":
    unittest.main()
