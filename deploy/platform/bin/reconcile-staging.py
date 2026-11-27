"""Reconcile application image revisions observed on staging against HEAD."""
import re
import subprocess
import sys
from collections import defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
REGISTRY = ROOT / "deploy/platform/services.tsv"
BLOCKED = {"api-gateway", "partner-web-service", "admin-backoffice-service",
           "driver-dispatch-delivery-service", "analytics-service"}


def candidates():
    for line in REGISTRY.read_text().splitlines():
        if line.strip() and not line.lstrip().startswith("#"):
            key = line.split("|")[0].strip()
            if key not in BLOCKED:
                yield key


def targets(key):
    names = []
    service = ""
    for line in (ROOT / "deploy/platform/compose" / (key + ".yml")).read_text().splitlines():
        match = re.fullmatch(r"  ([a-z][a-z0-9_-]*):\s*", line)
        if match:
            service = match.group(1)
        match = re.fullmatch(r"    container_name:\s*([a-zA-Z0-9_-]+)\s*", line)
        if match and service in {"app", "worker"}:
            names.append(match.group(1))
    if not names:
        raise ValueError("no application containers for " + key)
    return names


def select(observed, keys):
    groups = defaultdict(list)
    picked = set()
    for key in keys:
        revisions = [observed[(key, name)] for name in targets(key)]
        if len(set(revisions)) != 1 or not re.fullmatch(r"[0-9a-f]{40}", revisions[0]):
            picked.add(key)
            continue
        revision = revisions[0]
        if subprocess.run(["git", "merge-base", "--is-ancestor", revision, "HEAD"],
                          cwd=ROOT, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode:
            raise ValueError("unrecognized or non-ancestor runtime revision for " + key)
        groups[revision].append(key)
    for revision, group in groups.items():
        changes = subprocess.check_output(["git", "diff", "--name-only", revision, "HEAD"], cwd=ROOT, text=True)
        result = subprocess.check_output(
            [sys.executable, str(ROOT / "deploy/platform/bin/changed-services.py"), *group],
            cwd=ROOT, input=changes, text=True)
        picked.update(result.split())
        changed = set(changes.splitlines())
        for key in group:
            if f"deploy/platform/compose/{key}.yml" in changed or "deploy/platform/compose/common.yml" in changed:
                picked.add(key)
    return [key for key in keys if key in picked]


def main():
    keys = list(candidates())
    if sys.argv[1:] == ["targets"]:
        print(" ".join(f"{key}:{name}" for key in keys for name in targets(key)))
    elif sys.argv[1:] == ["select"]:
        observed = {}
        for line in sys.stdin:
            key, name, revision = line.split()
            if (key, name) in observed:
                raise ValueError("duplicate runtime evidence")
            observed[(key, name)] = revision
        expected = {(key, name) for key in keys for name in targets(key)}
        if set(observed) != expected:
            raise ValueError("incomplete runtime evidence")
        print(" ".join(select(observed, keys)))
    else:
        raise SystemExit("usage: reconcile-staging.py targets|select")


if __name__ == "__main__":
    main()
