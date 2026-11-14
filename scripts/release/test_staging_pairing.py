"""Acceptance tests for scripts/platform/staging-pairing.sh, focused on KEYGEN.

The script runs on the staging server against /opt/karrygo. Each test runs a
copy whose /opt/karrygo root and apply-env.sh are redirected into a temporary
directory, so the real parsing, refusal, backup and rollback logic is
exercised without a server.
"""

import base64
import os
import pathlib
import re
import shutil
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "platform" / "staging-pairing.sh"
MEDIA_VAR = "MEDIA_FILE_PRIVATE_ENCRYPTION_KEYS"


class StagingPairingKeygenTest(unittest.TestCase):
    def setUp(self):
        self.tmp = pathlib.Path(tempfile.mkdtemp(prefix="pairing-test-"))
        self.addCleanup(shutil.rmtree, self.tmp, True)
        opt = self.tmp / "opt"
        self.media_env = opt / "media-file-service" / "media.env"
        self.wallet_env = opt / "payment-wallet-service" / "wallet.env"
        for path, body in ((self.media_env, "APP_ENV=staging\nMEDIA_FILE_PRIVATE_API_ENABLED=true\n"),
                           (self.wallet_env, "APP_ENV=staging\n")):
            path.parent.mkdir(parents=True)
            path.write_text(body)
            path.chmod(0o600)
        self.apply_log = self.tmp / "apply.log"
        self.fail_marker = self.tmp / "fail-service"
        apply = self.tmp / "apply-env.sh"
        apply.write_text(
            "#!/usr/bin/env bash\n"
            f'echo "$1" >> "{self.apply_log}"\n'
            f'if [[ -f "{self.fail_marker}" && "$(cat "{self.fail_marker}")" == "$1" ]]; then\n'
            f'  rm -f "{self.fail_marker}"; exit 1\n'
            "fi\n")
        apply.chmod(0o700)
        script = SCRIPT.read_text()
        script = script.replace("apply=/opt/karrygo/platform/bin/apply-env.sh", f"apply={apply}")
        script = script.replace("/opt/karrygo/", f"{opt}/")
        self.script = self.tmp / "staging-pairing.sh"
        self.script.write_text(script)

    def run_pairing(self, desired):
        encoded = base64.b64encode(desired.encode()).decode()
        return subprocess.run(["bash", str(self.script), encoded], capture_output=True, text=True)

    def media_value(self):
        for line in self.media_env.read_text().splitlines():
            if line.startswith(MEDIA_VAR + "="):
                return line.split("=", 1)[1]
        return None

    def assert_valid_key(self, value):
        match = re.fullmatch(r"(k[0-9]{8}-[0-9a-f]{4}):([A-Za-z0-9+/=]+)", value or "")
        self.assertIsNotNone(match, "generated key is not keyId:base64")
        self.assertEqual(len(base64.b64decode(match.group(2))), 32)

    def test_generates_once_without_printing_and_never_rotates(self):
        first = self.run_pairing(f"KEYGEN media-file-service {MEDIA_VAR} 32\n")
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        value = self.media_value()
        self.assert_valid_key(value)
        self.assertIn("generated (value not shown)", first.stdout)
        self.assertNotIn(value.split(":", 1)[1], first.stdout + first.stderr)
        self.assertEqual(oct(self.media_env.stat().st_mode & 0o777), "0o600")
        self.assertIn("MEDIA_FILE_PRIVATE_API_ENABLED=true", self.media_env.read_text())

        second = self.run_pairing(f"KEYGEN media-file-service {MEDIA_VAR} 32\n")
        self.assertEqual(second.returncode, 0, second.stdout + second.stderr)
        self.assertIn("already present", second.stdout)
        self.assertEqual(self.media_value(), value)
        self.assertNotIn(value.split(":", 1)[1], second.stdout + second.stderr)

    def test_existing_key_is_left_alone(self):
        existing = "k1:" + base64.b64encode(b"\x01" * 32).decode()
        self.media_env.write_text(self.media_env.read_text() + f"{MEDIA_VAR}={existing}\n")
        result = self.run_pairing(f"KEYGEN media-file-service {MEDIA_VAR} 32\n")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.media_value(), existing)
        self.assertNotIn(existing, result.stdout + result.stderr)

    def test_refuses_unlisted_keygen_lines(self):
        for line in (f"KEYGEN media-file-service {MEDIA_VAR} 16",
                     "KEYGEN media-file-service MEDIA_FILE_DOWNLOAD_SIGNING_SECRET 32",
                     f"KEYGEN payment-wallet-service {MEDIA_VAR} 32",
                     f"KEYGEN media-file-service {MEDIA_VAR}"):
            result = self.run_pairing(line + "\n")
            self.assertNotEqual(result.returncode, 0, line)
            self.assertIn("Refusing unsupported line", result.stdout + result.stderr)
            self.assertIsNone(self.media_value())

    def test_unsafe_settings_file_is_blocked_and_untouched(self):
        self.media_env.chmod(0o644)
        before = self.media_env.read_text()
        result = self.run_pairing(f"KEYGEN media-file-service {MEDIA_VAR} 32\n")
        self.assertIn("BLOCKED: media-file-service", result.stdout)
        self.assertEqual(self.media_env.read_text(), before)
        self.assertEqual(oct(self.media_env.stat().st_mode & 0o777), "0o644")

    def test_rollback_keeps_generated_key_and_restores_other_changes(self):
        # Another touched service fails its health check: every file is
        # restored, but the generated key survives (data may be sealed with it).
        self.fail_marker.write_text("payment-wallet-service")
        result = self.run_pairing(
            f"KEYGEN media-file-service {MEDIA_VAR} 32\n"
            "SET media-file-service MEDIA_FILE_WORKERS_ENABLED=true\n"
            "SET payment-wallet-service PAYMENT_WALLET_FINANCE_OUTBOX_ENABLED=true\n")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("restoring every touched settings file", result.stdout)
        self.assertIn("kept through the restore (value not shown)", result.stdout)
        value = self.media_value()
        self.assert_valid_key(value)
        self.assertNotIn(value.split(":", 1)[1], result.stdout + result.stderr)
        self.assertNotIn("MEDIA_FILE_WORKERS_ENABLED", self.media_env.read_text())
        self.assertNotIn("PAYMENT_WALLET_FINANCE_OUTBOX_ENABLED", self.wallet_env.read_text())
        self.assertEqual(oct(self.media_env.stat().st_mode & 0o777), "0o600")

    def test_committed_pairings_include_media_keygen(self):
        desired = (ROOT / "deploy" / "platform" / "staging-pairings.txt").read_text()
        self.assertIn(f"KEYGEN media-file-service {MEDIA_VAR} 32", desired.splitlines())
        # The whole committed file parses; services without a settings file
        # here are BLOCKED, and the media key is still generated.
        result = self.run_pairing(desired)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("Refusing", result.stdout + result.stderr)
        self.assert_valid_key(self.media_value())


if __name__ == "__main__":
    unittest.main()
