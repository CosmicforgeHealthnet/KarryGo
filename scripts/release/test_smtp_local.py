"""Safety tests for direct operator-driven STAGING SMTP cutover."""
import contextlib
import io
import unittest
from unittest import mock

import smtp_local as cli


class LocalActivationTests(unittest.TestCase):
    def test_confirmation_required_before_any_mutation(self):
        with mock.patch.object(cli, "require_operator"), mock.patch.object(cli, "perform") as perform:
            with contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(1, cli.main(["activate", "--confirm", "WRONG"]))
            perform.assert_not_called()

    def test_root_denied_even_when_container_is_healthy(self):
        with mock.patch.object(cli.os, "geteuid", return_value=0):
            with self.assertRaises(ValueError):
                cli.require_operator()

    def test_non_deploy_denied(self):
        with mock.patch.object(cli.os, "geteuid", return_value=1010), mock.patch.object(cli.pwd, "getpwnam") as lookup:
            lookup.return_value.pw_uid = 1011
            with self.assertRaises(ValueError):
                cli.require_operator()

    def test_staging_prerequisite_failure_denies_activation(self):
        with mock.patch.object(cli, "exclusive_lock", return_value=contextlib.nullcontext()):
            with mock.patch.object(cli, "report", return_value=({"ready_for_activation": False}, {})):
                with mock.patch.object(cli.runtime, "transaction") as transaction:
                    with self.assertRaises(ValueError):
                        cli.perform("activate")
                    transaction.assert_not_called()

    def test_activation_uses_existing_transaction_and_backup(self):
        item = {"State": {"Health": {"Status": "healthy"}}}
        with mock.patch.object(cli, "exclusive_lock", return_value=contextlib.nullcontext()), \
             mock.patch.object(cli, "report", return_value=({"ready_for_activation": True, "resend_settings_loaded": False}, item)), \
             mock.patch.object(cli.time, "time_ns", return_value=123456789), \
             mock.patch.object(cli.runtime, "transaction", return_value="123456789") as transaction:
            with contextlib.redirect_stdout(io.StringIO()) as output:
                cli.perform("activate")
            transaction.assert_called_once_with({"operation": "activate", "run_id": "123456789"}, item)
            self.assertIn("123456789", output.getvalue())

    def test_rollback_requires_explicit_numeric_backup(self):
        item = {}
        with mock.patch.object(cli, "exclusive_lock", return_value=contextlib.nullcontext()), \
             mock.patch.object(cli, "report", return_value=({"ready_for_activation": True}, item)), \
             mock.patch.object(cli.runtime, "transaction") as transaction:
            with self.assertRaises(ValueError):
                cli.perform("rollback", "../bad")
            transaction.assert_not_called()

    def test_rollback_uses_original_backup_id(self):
        item = {}
        with mock.patch.object(cli, "exclusive_lock", return_value=contextlib.nullcontext()), \
             mock.patch.object(cli, "report", return_value=({"ready_for_activation": True}, item)), \
             mock.patch.object(cli.time, "time_ns", return_value=999), \
             mock.patch.object(cli.runtime, "transaction", return_value="123") as transaction:
            with contextlib.redirect_stdout(io.StringIO()):
                cli.perform("rollback", "123")
            transaction.assert_called_once_with(
                {"operation": "rollback", "run_id": "999", "backup_run_id": "123"}, item
            )

    def test_matching_config_skips_recreation(self):
        item = {"Config": {"Env": ["NOTIFICATION_SMTP_PASSWORD=somekey"]}}
        with mock.patch.object(cli, "exclusive_lock", return_value=contextlib.nullcontext()), \
             mock.patch.object(cli, "report", return_value=({"ready_for_activation": True, "resend_settings_loaded": True}, item)), \
             mock.patch.object(cli.runtime, "read_resend_key", return_value="somekey"), \
             mock.patch.object(cli.runtime, "smtp_auth") as smtp_auth, \
             mock.patch.object(cli.runtime, "transaction") as transaction:
            with contextlib.redirect_stdout(io.StringIO()):
                cli.perform("activate")
            smtp_auth.assert_called_once_with("somekey")
            transaction.assert_not_called()


if __name__ == "__main__":
    unittest.main()
