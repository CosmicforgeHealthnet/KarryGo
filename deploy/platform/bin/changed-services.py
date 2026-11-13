"""Decide which services a set of changed files actually affects.

    changed-services.py <candidate keys...>  < changed-file-list

Prints the affected keys, space separated, in the order given.

A service's image is built from exactly two inputs (services/Dockerfile and
the per-service Dockerfiles copy nothing else): its own folder
services/<key>/ and the shared module shared/go/. So:

- services/<key>/...            -> that service.
- go.work, go.work.sum          -> nothing. Neither file is copied into any
                                   image; each service builds from its own
                                   go.mod/go.sum. (A checksum-only go.work.sum
                                   change used to redeploy every service.)
- services/Dockerfile           -> every service built with the shared
                                   Dockerfile (deploy/platform/services.tsv).
- shared/go/go.mod or go.sum    -> every service that requires the shared
                                   module (its dependency versions changed).
- shared/go/<pkg>/... (Go code) -> only services whose build imports that
                                   package, directly or transitively, measured
                                   with `go list -deps` on the service module.
- shared/go/<non-Go file>       -> services importing the package that owns
                                   the folder (embedded files live there).

If the import graph cannot be computed (go missing, a module that does not
load), the script fails rather than guessing: deploying too little is worse
than a red run that someone reads.
"""
import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
SHARED_MODULE = "cosmicforge/logistics/shared/go"


def shared_dockerfile_services():
    keys = set()
    for line in (ROOT / "deploy/platform/services.tsv").read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        cols = [c.strip() for c in line.split("|")]
        if len(cols) > 1 and cols[1] == "shared":
            keys.add(cols[0])
    return keys


def requires_shared(key):
    mod = ROOT / "services" / key / "go.mod"
    return mod.exists() and SHARED_MODULE in mod.read_text()


def imported_shared_packages(key):
    """Every shared/go package in the service's build graph (all main packages and tests excluded)."""
    service_dir = ROOT / "services" / key
    if not (service_dir / "go.mod").exists():
        return set()
    env = dict(os.environ, GOWORK="off", GOFLAGS="-mod=mod")
    result = subprocess.run(
        ["go", "list", "-deps", "-f", "{{.ImportPath}}", "./..."],
        cwd=service_dir, env=env, capture_output=True, text=True,
    )
    if result.returncode != 0:
        sys.stderr.write(result.stderr)
        raise SystemExit(f"cannot compute the import graph of {key}")
    return {p for p in result.stdout.split() if p == SHARED_MODULE or p.startswith(SHARED_MODULE + "/")}


def main():
    candidates = sys.argv[1:]
    changed = [line.strip() for line in sys.stdin if line.strip()]
    shared_dockerfile = shared_dockerfile_services()

    picked = set()
    shared_packages = set()
    shared_module_files = False
    for path in changed:
        if path in ("go.work", "go.work.sum"):
            continue
        if path == "services/Dockerfile":
            picked |= {k for k in candidates if k in shared_dockerfile}
            continue
        if path in ("shared/go/go.mod", "shared/go/go.sum"):
            shared_module_files = True
            continue
        if path.startswith("shared/go/"):
            folder = os.path.dirname(path[len("shared/go/"):])
            shared_packages.add(SHARED_MODULE + ("/" + folder if folder else ""))
            continue
        for k in candidates:
            if path.startswith(f"services/{k}/"):
                picked.add(k)

    if shared_module_files:
        picked |= {k for k in candidates if requires_shared(k)}
    if shared_packages:
        for k in candidates:
            if k in picked or not requires_shared(k):
                continue
            if imported_shared_packages(k) & shared_packages:
                picked.add(k)

    print(" ".join(k for k in candidates if k in picked))


if __name__ == "__main__":
    main()
