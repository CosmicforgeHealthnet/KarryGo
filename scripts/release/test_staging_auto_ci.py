import unittest
from unittest.mock import Mock, patch
import staging_auto_ci

SHA = "a" * 40


class AutoStagingCI(unittest.TestCase):
    def api(self):
        api = Mock()
        api.get.return_value = {"object": {"sha": SHA}}
        return api

    def test_green(self):
        with patch.object(staging_auto_ci.gate, "acceptance", return_value=[]):
            staging_auto_ci.wait_for_ci(self.api(), SHA, timeout=0)

    def test_not_green(self):
        with patch.object(staging_auto_ci.gate, "acceptance",
                          side_effect=ValueError("CI incomplete/failed: test")):
            with self.assertRaises(RuntimeError):
                staging_auto_ci.wait_for_ci(self.api(), SHA, timeout=0)

    def test_stale(self):
        api = self.api()
        api.get.return_value = {"object": {"sha": "b" * 40}}
        with patch.object(staging_auto_ci.gate, "acceptance") as acceptance:
            with self.assertRaises(ValueError):
                staging_auto_ci.wait_for_ci(api, SHA, timeout=0)
            acceptance.assert_not_called()

    def test_missing_or_failed_jobs(self):
        with patch.object(staging_auto_ci.gate, "acceptance",
                          side_effect=ValueError("CI jobs skipped/failed: test")):
            with self.assertRaises(ValueError):
                staging_auto_ci.wait_for_ci(self.api(), SHA, timeout=0)
