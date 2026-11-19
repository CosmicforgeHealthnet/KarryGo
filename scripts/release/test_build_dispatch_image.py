"""The Dispatch build workflow must stay build-only.

Dispatch's migration-070 transition is an operator-run, separately approved
procedure (docs/deployment/dispatch-070-transition-package.md). The build
workflow only produces the image digest that approval records, so it must
never be able to reach the staging server.
"""

import pathlib
import unittest

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / ".github" / "workflows" / "build-dispatch-image.yml"


class BuildDispatchImageTest(unittest.TestCase):
    def setUp(self):
        self.text = WORKFLOW.read_text()
        self.data = yaml.load(self.text, Loader=yaml.BaseLoader)

    def test_manual_only_on_staging(self):
        self.assertEqual(set(self.data["on"]), {"workflow_dispatch"})
        job = self.data["jobs"]["build"]
        self.assertIn("refs/heads/staging", job["if"])

    def test_cannot_reach_the_server(self):
        job = self.data["jobs"]["build"]
        self.assertNotIn("environment", job, "an environment would expose the deploy SSH key")
        for forbidden in ("STAGING_DEPLOY", "ssh ", "scp ", "deploy-service.sh", "rollback.sh", "migrate.sh"):
            self.assertNotIn(forbidden, self.text)

    def test_builds_only_dispatch_and_records_digest(self):
        self.assertIn("svc=driver-dispatch-delivery-service", self.text)
        self.assertIn("containerimage.digest", self.text)
        self.assertEqual(self.data["permissions"], {"contents": "read"})


if __name__ == "__main__":
    unittest.main()
