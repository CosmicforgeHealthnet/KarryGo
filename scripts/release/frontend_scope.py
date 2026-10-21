"""Preserve the backend PR scope guard, allowing only new preview holds."""
import json
import re
import subprocess
import sys

PREVIEW = {"$schema": "https://openapi.vercel.sh/vercel.json", "git": {
    "deploymentEnabled": {"codex/staging-secure-admin-integration-20261020": False}}}
PATHS = {"apps/admin/vercel.json", "web/vercel.json", "web/fleet/vercel.json",
         "web/karryam-landing/vercel.json", "web/partner-web/vercel.json"}


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
        validate_preview(path, before.stdout if before.returncode == 0 else None, current)
    print("Backend scope verified; only exact new review-branch preview holds allowed")


if __name__ == "__main__":
    main(sys.argv[1])
