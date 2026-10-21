"""Build pinned release bundles; deploy only after a fresh API authorization.

Never invoked on push. Dispatch and Admin/Gateway/Partner are rejected before
any process launch. Host operations require the protected environment secrets.
"""
import argparse
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import tempfile

import gate


def run(args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)


def command_plan(evidence):
    services = gate.selected(",".join(evidence["services"]), evidence["kind"])
    gate.require(re.fullmatch(r"[0-9a-f]{40}", evidence["release_sha"]), "invalid release SHA")
    result = []
    for service in services:
        image = evidence["images"][service]
        gate.require(re.fullmatch(r"ghcr\.io/cosmicforgehealthnet/" + re.escape(service) + r"@sha256:[0-9a-f]{64}", image),
                     "immutable image digest required")
        if service == "vendor-service":
            # Stop the sole existing worker before migration; never auto-restart
            # an incompatible worker after a failure. API health precedes start.
            lines = ["set -euo pipefail", "cd " + shlex.quote(evidence["remote"]),
                     "export VENDOR_IMAGE=" + shlex.quote(image),
                     "export VENDOR_ENV_FILE=/opt/karrygo/vendor-service/vendor.env",
                     'test "$(stat -c %a "$VENDOR_ENV_FILE")" = 600',
                     'python3 check-admin-disabled.py "$VENDOR_ENV_FILE"',
                     "docker compose -f vendor.yml config --quiet",
                     "docker compose -f vendor.yml pull",
                     "docker compose -f vendor.yml stop vendor-worker",
                     "docker compose -f vendor.yml --profile operations run --rm vendor-migrate",
                     "docker compose -f vendor.yml up -d vendor-api",
                     "for i in $(seq 1 24); do test \"$(docker inspect -f '{{.State.Health.Status}}' karrygo-vendor-api-staging)\" != healthy || break; sleep 5; done",
                     "test \"$(docker inspect -f '{{.State.Health.Status}}' karrygo-vendor-api-staging)\" = healthy",
                     "docker compose -f vendor.yml up -d vendor-worker",
                     "test \"$(docker inspect -f '{{.State.Running}}' karrygo-vendor-worker-staging)\" = true",
                     "test -z \"$(docker port karrygo-vendor-worker-staging)\""]
        else:
            lines = ["set -euo pipefail", "cd " + shlex.quote(evidence["remote"] + "/platform"),
                     "bash bin/deploy-service.sh " + shlex.quote(service) + " " + shlex.quote(image)]
        result.append("\n".join(lines) + "\n")
    return result


def build(evidence):
    gate.selected(",".join(evidence["services"]), evidence["kind"])
    rows = {r.split("|")[0].strip(): r.split("|")[1].strip()
            for r in Path("deploy/platform/services.tsv").read_text().splitlines()
            if r.strip() and not r.lstrip().startswith("#")}
    images = {}
    for service in evidence["services"]:
        dockerfile = "services/vendor-service/Dockerfile" if service == "vendor-service" else rows[service]
        if dockerfile == "shared":
            dockerfile = "services/Dockerfile"
        tag = f"ghcr.io/cosmicforgehealthnet/{service}:{evidence['release_sha']}-release-{evidence['run_id']}"
        with tempfile.TemporaryDirectory() as directory:
            metadata = str(Path(directory) / "image.json")
            run(["docker", "buildx", "build", "--push", "--file", dockerfile,
                 "--build-arg", "SERVICE_DIR=" + service,
                 "--build-arg", "BUILD_COMMIT_SHA=" + evidence["release_sha"],
                 "--build-arg", "BUILD_VERSION=release-" + str(evidence["run_id"]),
                 "--tag", tag, "--metadata-file", metadata, "."])
            digest = json.loads(Path(metadata).read_text())["containerimage.digest"]
            gate.require(re.fullmatch(r"sha256:[0-9a-f]{64}", digest), "missing image digest")
            images[service] = f"ghcr.io/cosmicforgehealthnet/{service}@{digest}"
    evidence["images"] = images
    evidence["remote"] = f"/opt/karrygo/releases/{evidence['release_sha']}/{evidence['run_id']}"
    return evidence


def deploy(evidence):
    # Revalidate SHA, current environment/initiator/approval, CI and restore
    # immediately before host access; an approval from an older run is useless.
    context = {k: os.environ.get("GITHUB_" + k.upper(), "") for k in
               ("repository", "event_name", "ref", "sha", "workflow_sha", "run_attempt", "run_id", "actor", "triggering_actor")}
    gate.require(str(evidence["run_id"]) == context["run_id"], "foreign release bundle")
    gate.authorize(gate.GitHub(os.environ.get("RELEASE_AUDIT_TOKEN")), context,
                   evidence["release_sha"], ",".join(evidence["services"]), evidence["kind"], os.environ["RESTORE_RUN_ID"])
    commands = command_plan(evidence)
    host, user = os.environ["STAGING_DEPLOY_HOST"], os.environ["STAGING_DEPLOY_USER"]
    gate.require(re.fullmatch(r"[a-zA-Z0-9.-]+", host) and re.fullmatch(r"[a-z_][a-z0-9_-]*", user), "invalid SSH destination")
    with tempfile.TemporaryDirectory() as directory:
        scratch = Path(directory)
        key = scratch / "key"
        key.write_text(os.environ["STAGING_DEPLOY_SSH_KEY"] + "\n")
        key.chmod(0o600)
        hosts = scratch / "known_hosts"
        hosts.write_text(os.environ["STAGING_DEPLOY_KNOWN_HOSTS"] + "\n")
        opts = ["-i", str(key), "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes",
                "-o", "UserKnownHostsFile=" + str(hosts)]
        target = user + "@" + host
        remote = evidence["remote"]
        gate.require(re.fullmatch(r"/opt/karrygo/releases/[0-9a-f]{40}/[1-9][0-9]*", remote), "invalid release path")
        bundle = scratch / "bundle"
        bundle.mkdir()
        if evidence["kind"] == "vendor":
            shutil.copyfile("deploy/vendor-service/docker-compose.staging.yml", bundle / "vendor.yml")
            shutil.copyfile("deploy/platform/bin/check-admin-disabled.py", bundle / "check-admin-disabled.py")
        else:
            platform = bundle / "platform"
            shutil.copytree("deploy/platform/bin", platform / "bin")
            (platform / "compose").mkdir()
            shutil.copyfile("deploy/platform/services.tsv", platform / "services.tsv")
            for name in ["common"] + evidence["services"]:
                shutil.copyfile(f"deploy/platform/compose/{name}.yml", platform / "compose" / (name + ".yml"))
        # Existing /opt/karrygo/platform, Nginx and cutover config are untouched.
        run(["ssh", *opts, target, "mkdir -p " + shlex.quote(remote)])
        run(["scp", *opts, "-r", str(bundle) + "/.", target + ":" + remote + "/"])
        login = "docker login ghcr.io -u " + shlex.quote(os.environ["GITHUB_ACTOR"]) + " --password-stdin"
        run(["ssh", *opts, target, login], input=os.environ["GHCR_TOKEN"] + "\n", text=True)
        try:
            for image in evidence["images"].values():
                run(["ssh", *opts, target, "docker pull " + shlex.quote(image)])
            # Stop on the first error; no partial-success continuation.
            for script in commands:
                run(["ssh", *opts, target, "bash -s"], input=script, text=True)
        finally:
            run(["ssh", *opts, target, "docker logout ghcr.io"])


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=["build", "deploy"])
    args = parser.parse_args()
    path = Path("release-evidence.json")
    evidence = json.loads(path.read_text())
    if args.mode == "build":
        path.write_text(json.dumps(build(evidence), sort_keys=True, indent=2) + "\n")
    else:
        deploy(evidence)
