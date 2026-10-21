"""Reject privileged Admin rollout flags before any release mutation.

Never print environment values. Missing flags preserve disabled code defaults.
"""
import re
import sys
from pathlib import Path


def check(path):
    for line in Path(path).read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        key, separator, value = line.removeprefix("export ").partition("=")
        key = key.strip()
        if separator and (key.startswith("ADMIN_") or "_ADMIN_" in key) and key.endswith("_ENABLED"):
            value = value.strip().strip("\"'")
            if value not in {"", "false"}:
                raise ValueError("privileged Admin release flag is not disabled: " + key)


if __name__ == "__main__":
    check(sys.argv[1])
