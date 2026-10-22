"""Negative scope regression tests for the single content-pinned Dispatch quality helper."""
import subprocess
import unittest

from scripts.release.frontend_scope import validate_dispatch_quality, validate_preview

PATH = "apps/dispatch_provider/lib/features/profile/ui/selfie_image_quality.dart"

class ScopeTest(unittest.TestCase):
    def test_exact_new_helper_is_accepted(self):
        content = subprocess.check_output(["git", "show", "HEAD:" + PATH])
        self.assertTrue(validate_dispatch_quality(PATH, None, content))

    def test_modified_helper_denied(self):
        content = subprocess.check_output(["git", "show", "HEAD:" + PATH])
        with self.assertRaises(ValueError):
            validate_dispatch_quality(PATH, None, content + b"\n// silently modified\n")

    def test_replacement_denied(self):
        content = subprocess.check_output(["git", "show", "HEAD:" + PATH])
        with self.assertRaises(ValueError):
            validate_dispatch_quality(PATH, b"old", content)

    def test_other_flutter_runtime_is_not_exempt(self):
        self.assertFalse(validate_dispatch_quality(
            "apps/dispatch_provider/lib/app/dispatch_provider_app.dart", None, b"changed"))

    def test_unrelated_frontend_denied(self):
        with self.assertRaises(ValueError):
            validate_preview("web/karryam-landing/src/main.tsx", b"previous", b"changed")

if __name__ == "__main__":
    unittest.main()
