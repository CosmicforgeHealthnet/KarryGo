"""Preserve backend scope guard; permit explicitly bounded Dispatch Widgetbook previews."""
import json
import re
import subprocess
import sys

PREVIEW = {"$schema": "https://openapi.vercel.sh/vercel.json", "git": {
    "deploymentEnabled": {"codex/staging-secure-admin-integration-20261020": False}}}
PATHS = {"apps/admin/vercel.json", "web/vercel.json", "web/fleet/vercel.json",
         "web/karryam-landing/vercel.json", "web/partner-web/vercel.json"}


def validate_dispatch_preview(path, previous, current):
    """Allow ONLY additive, isolated Dispatch design previews; no app runtime changes."""
    root = "apps/dispatch_provider/"
    if path == root + "pubspec.yaml":
        if previous is None:
            raise ValueError("existing Dispatch manifest required")
        token = b"  flutter_launcher_icons: ^0.14.4\n"
        if previous.count(token) != 1 or b"  widgetbook:" in previous:
            raise ValueError("unexpected original Dispatch manifest")
        expected = previous.replace(token, token + b"  widgetbook: ^3.23.0\n")
        if current != expected:
            raise ValueError("Dispatch manifest must add only Widgetbook dev dependency")
        return True
    prefixes = (root + "lib/design_previews/", root + "lib/design_system/",
                root + "lib/widgetbook_catalog/")
    if path == root + "lib/widgetbook.dart" or path.startswith(prefixes):
        if previous is not None or not path.endswith(".dart"):
            raise ValueError("Dispatch preview scope permits only new Dart source files")
        return True
    return False


def validate_preview(path, previous, current):
    if path not in PATHS or previous is not None or json.loads(current) != PREVIEW:
        raise ValueError("frontend implementation/configuration change outside backend scope: " + path)


def main(base):
    if not re.fullmatch(r"[0-9a-f]{40}", base):
        raise ValueError("exact comparison SHA required")
    subprocess.run(["git", "cat-file", "-e", base + "^{commit}"], check=True)
    changes = subprocess.check_output([
        "git", "diff", "--name-only", base, "HEAD", "--", "apps/**", "web/**",
        "frontend/**", "mobile/**", ":!apps/dispatch_provider/test/**"], text=True)
    for path in changes.splitlines():
        before = subprocess.run(["git", "show", base + ":" + path], capture_output=True, check=False)
        if before.returncode and subprocess.run(
                ["git", "ls-tree", base, "--", path], capture_output=True, check=True).stdout:
            raise ValueError("cannot inspect existing frontend file")
        current = subprocess.check_output(["git", "show", "HEAD:" + path])
        previous = before.stdout if before.returncode == 0 else None
        if not validate_dispatch_preview(path, previous, current):
            validate_preview(path, previous, current)
    print("Frontend scope verified: no runtime implementation changes, only bounded preview additions")


if __name__ == "__main__":
    main(sys.argv[1])
