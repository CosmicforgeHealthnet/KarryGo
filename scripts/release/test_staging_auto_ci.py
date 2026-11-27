import unittest
from unittest.mock import Mock, patch
import staging_auto_ci as ci

SHA = "a" * 40


class AutoStagingCI(unittest.TestCase):
    def api(self, conclusion="success", missing=False, jobs="success"):
        api = Mock()
        api.get.return_value = {"object": {"sha": SHA}}
        runs = [
            {"id": i, "head_sha": SHA, "head_branch": "staging", "event": "push",
             "path": ".github/workflows/" + path, "status": "completed",
             "conclusion": conclusion, "run_attempt": 1}
            for i, path in enumerate(ci.REQUIRED, 1)]
        if missing:
            runs.pop()
        api.pages.side_effect = lambda path, key: runs if key == "workflow_runs" else [
            {"conclusion": jobs}] * max(ci.REQUIRED.values())
        return api

    def test_green(self):
        ci.wait_for_ci(self.api(), SHA, timeout=0)

    def test_failed(self):
        with self.assertRaisesRegex(ValueError, "required CI failed"):
            ci.wait_for_ci(self.api(conclusion="failure"), SHA, timeout=0)

    def test_missing(self):
        with self.assertRaises(RuntimeError):
            ci.wait_for_ci(self.api(missing=True), SHA, timeout=0)

    def test_stale(self):
        api = self.api()
        api.get.return_value = {"object": {"sha": "b" * 40}}
        with self.assertRaises(ValueError):
            ci.wait_for_ci(api, SHA, timeout=0)
        api.pages.assert_not_called()

    def test_skipped_job(self):
        with self.assertRaises(ValueError):
            ci.wait_for_ci(self.api(jobs="skipped"), SHA, timeout=0)
