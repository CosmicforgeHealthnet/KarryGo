"""Preserve backend scope guard; permit exactly pinned new Dispatch quality helper."""
import json
import re
import subprocess
import sys

PREVIEW = {"$schema": "https://openapi.vercel.sh/vercel.json", "git": {
    "deploymentEnabled": {"codex/staging-secure-admin-integration-20261020": False}}}
PATHS = {"apps/admin/vercel.json", "web/vercel.json", "web/fleet/vercel.json",
         "web/karryam-landing/vercel.json", "web/partner-web/vercel.json"}


# One content-pinned, new-file-only Dispatch quality helper from the reviewed
# stabilization branch.  This does not permit general Flutter changes.
DISPATCH_QUALITY_SOURCE = {
    "apps/dispatch_provider/lib/features/profile/ui/selfie_image_quality.dart":
        "defbe6f4a3dcefa6eb2f7936d20df7c45a09c8cf",
}


def validate_dispatch_quality(path, previous, current):
    expected = DISPATCH_QUALITY_SOURCE.get(path)
    if expected is None:
        return False
    if previous is not None:
        raise ValueError("cannot replace existing Dispatch quality helper")
    actual = subprocess.check_output(["git", "hash-object", "--stdin"], input=current).decode().strip()
    if actual != expected:
        raise ValueError("Dispatch quality helper content does not match reviewed source")
    return True


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
        if not validate_dispatch_quality(path, previous, current):
            validate_preview(path, previous, current)
    print("Backend scope verified; only exact approved new helper or review preview holds allowed")


if __name__ == "__main__":
    main(sys.argv[1])
