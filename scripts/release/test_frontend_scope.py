"""Narrow negative tests for the backend-only scope guard's Dispatch preview exception."""
import unittest

from scripts.release.frontend_scope import validate_dispatch_preview, validate_preview


class FrontendScopeTest(unittest.TestCase):
    def setUp(self):
        self.manifest = (b"name: dispatch_provider\n"
                         b"  mobile_scanner: ^6.0.11\n"
                         b"  flutter_launcher_icons: ^0.14.4\n")

    def test_additive_widgetbook_dependency(self):
        modified = self.manifest.replace(
            b"  flutter_launcher_icons: ^0.14.4\n",
            b"  flutter_launcher_icons: ^0.14.4\n  widgetbook: ^3.23.0\n")
        self.assertTrue(validate_dispatch_preview(
            "apps/dispatch_provider/pubspec.yaml", self.manifest, modified))

    def test_manifest_dependency_removal_rejected(self):
        with self.assertRaises(ValueError):
            validate_dispatch_preview(
                "apps/dispatch_provider/pubspec.yaml", self.manifest,
                b"  flutter_launcher_icons: ^0.14.4\n  widgetbook: ^3.23.0\n")

    def test_new_preview_allowed(self):
        self.assertTrue(validate_dispatch_preview(
            "apps/dispatch_provider/lib/design_previews/auth/preview.dart",
            None, b"import 'package:flutter/material.dart';\n"))

    def test_runtime_source_is_not_exempt(self):
        self.assertFalse(validate_dispatch_preview(
            "apps/dispatch_provider/lib/app/dispatch_provider_app.dart", None, b"bad"))

    def test_existing_preview_cannot_be_replaced(self):
        with self.assertRaises(ValueError):
            validate_dispatch_preview(
                "apps/dispatch_provider/lib/design_system/karryam_tokens.dart",
                b"original", b"modified")

    def test_non_dart_in_preview_paths_rejected(self):
        with self.assertRaises(ValueError):
            validate_dispatch_preview(
                "apps/dispatch_provider/lib/widgetbook_catalog/secret.env",
                None, b"secret")

    def test_other_frontend_changes_remain_blocked(self):
        with self.assertRaises(ValueError):
            validate_preview("web/karryam-landing/src/main.tsx", b"old", b"new")


if __name__ == "__main__":
    unittest.main()
