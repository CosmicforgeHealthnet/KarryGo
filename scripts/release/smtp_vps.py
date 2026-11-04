"""Outgoing-only, locally installed Notification release worker. No network listener.

Never executes downloaded code. An administrator installs/pins reviewed code;
GitHub artifacts contain only bounded requests. Tokens and SMTP keys stay here.
"""
import hashlib
import io
import json
import os
from pathlib import Path
import re
import stat
import urllib.request
import zipfile

import gate
import smtp_activate as protocol
import smtp_runtime as runtime

INSTALL = Path("/usr/local/lib/karrygo-smtp")
AUDIT_TOKEN = Path("/opt/karrygo/platform/.secrets/release-audit-token")
RECEIPT_TOKEN = Path("/opt/karrygo/platform/.secrets/release-receipt-token")
STATE = Path("/opt/karrygo/notification-service/.smtp-requests")
FIELDS = {"schema", "repository", "release_sha", "services", "run_id", "operation",
          "restore_run_id", "backup_run_id", "nonce", "restore_manifest"}


def trusted(path):
    runtime.no_symlinks(path)
    for current in [path, *path.parents]:
        info = current.stat()
        gate.require(info.st_uid == 0 and not stat.S_IMODE(info.st_mode) & 0o022,
                     "executor code/policy must be root-owned and not deploy-writable")


def read_token(path):
    runtime.private(path)
    runtime.private_service_directory(path.parent)
    result = path.read_text().strip()
    gate.require(re.fullmatch(r"(?:github_pat_|ghp_|ghs_)[A-Za-z0-9_]+", result),
                 "VPS GitHub credential has an unexpected type")
    return result


def validate_request(item):
    gate.require(type(item) is dict and set(item) == FIELDS and item["schema"] == 1,
                 "unexpected request fields")
    gate.require(item["repository"] == gate.REPOSITORY and item["services"] == "notification-service",
                 "Notification-only canonical request required")
    gate.require(re.fullmatch(r"[0-9a-f]{40}", item["release_sha"]) and
                 re.fullmatch(r"[1-9][0-9]*", item["run_id"]) and
                 re.fullmatch(r"[0-9a-f]{32}", item["nonce"]), "invalid request identity")
    gate.require(item["operation"] in {"inspect", "activate", "rollback", "certify"}, "invalid operation")
    gate.require(item["operation"] == "certify" or re.fullmatch(r"[1-9][0-9]*", item["restore_run_id"]),
                 "valid restore certification required")
    gate.require(item["operation"] != "rollback" or re.fullmatch(r"[1-9][0-9]*", item["backup_run_id"]),
                 "valid activation backup required")
    gate.require(item["operation"] == "certify" or item["restore_manifest"] is None,
                 "unexpected restore manifest")


def download_request(api, run_id):
    artifacts = api.pages(f"actions/runs/{run_id}/artifacts", "artifacts")
    matches = [a for a in artifacts if a.get("name") == "staging-smtp-request" and not a.get("expired")]
    if not matches:
        return None
    gate.require(len(matches) == 1, "ambiguous request artifact")
    data = api.raw(f"actions/artifacts/{matches[0]['id']}/zip")
    gate.require(matches[0].get("digest") == "sha256:" + hashlib.sha256(data).hexdigest(), "request digest mismatch")
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        gate.require(archive.namelist() == ["smtp-request.json"] and
                     archive.getinfo("smtp-request.json").file_size <= 32768, "invalid request archive")
        item = json.loads(archive.read("smtp-request.json"))
    validate_request(item)
    gate.require(item["run_id"] == str(run_id), "foreign request artifact")
    return item


def context(api, item):
    run = api.get("actions/runs/" + item["run_id"])
    gate.require(run.get("status") == "in_progress" and run.get("run_attempt") == 1
                 and run.get("head_branch") == "staging", "request run is not active staging attempt 1")
    actor = run["actor"]["login"]
    triggering = run["triggering_actor"]["login"]
    # Derive context from GitHub's API, never from an artifact's claimed actor.
    return {"repository": gate.REPOSITORY, "event_name": run["event"], "ref": "refs/heads/staging",
            "sha": run["head_sha"], "workflow_sha": run["head_sha"], "run_attempt": "1",
            "run_id": item["run_id"], "actor": actor, "triggering_actor": triggering}


def authorize(api, item):
    validate_request(item)
    kind = "certify" if item["operation"] == "certify" else "smtp"
    return gate.authorize(api, context(api, item), item["release_sha"], "notification-service", kind,
                          item["restore_run_id"], proof=item["restore_manifest"], credential_source="vps")


def post(api, path, data):
    req = urllib.request.Request("https://api.github.com/repos/" + gate.REPOSITORY + "/" + path,
        data=protocol.canonical(data), method="POST", headers={"Authorization": "Bearer " + api.token,
        "Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28", "Content-Type": "application/json"})
    with api.opener.open(req, timeout=30) as response:
        return json.load(response)


def handle(api, writer, identity, item):
    for name in ["gate.py", "smtp_activate.py", "smtp_runtime.py", "smtp_vps.py", "approved-revision", "receipt-identity",
                 "deploy/platform/services.tsv"]:
        trusted(INSTALL / name)
    gate.require((INSTALL / "approved-revision").read_text().strip() == item["release_sha"], "executor revision is not approved for this SHA")
    authorization = authorize(api, item)
    gate.require(identity and identity != authorization["initiator"] and identity != "github-actions[bot]",
                 "receipt credential must use an independent VPS-only identity")
    runtime.no_symlinks(STATE)
    STATE.mkdir(mode=0o700, exist_ok=True)
    runtime.private_directory(STATE)
    claim = STATE / (item["run_id"] + ".json")
    runtime.no_symlinks(claim)
    if claim.exists():
        return  # Exactly one attempt per run; crashed operations require review.
    # Persist before mutation. No tokens, SMTP keys or recipient data are stored.
    with claim.open("xb") as output:
        os.fchmod(output.fileno(), 0o600)
        output.write(protocol.canonical({**protocol.receipt_payload(item), "state": "claimed"}))
        output.flush()
        os.fsync(output.fileno())
    # Receipt record only, not an alternative authorization path: the complete
    # seven-workflow CI gate above remains mandatory before this API call.
    deployment = post(writer, "deployments", {"ref": item["release_sha"], "environment": "staging",
        "task": "karrygo-smtp", "auto_merge": False, "required_contexts": [],
        "description": "VPS Notification-only operation after independent release gate",
        "payload": protocol.receipt_payload(item), "production_environment": False, "transient_environment": False})
    gate.require(deployment.get("creator", {}).get("login") == identity, "untrusted receipt creator")
    error = None
    try:
        # Reauthorize after intent publication, immediately before protected data.
        authorize(api, item)
        if item["operation"] != "certify":
            runtime.private(runtime.ENV)
            before = runtime.ENV.read_text()
            current = runtime.container("karrygo-notification-service")
            gate.require(runtime.value(before, "APP_ENV") == "staging" and current and
                         runtime.env_of(current).get("APP_ENV") == "staging", "staging file/container confirmation failed")
            runtime.transaction(item, current)
        result = {**protocol.receipt_payload(item), "state": "success"}
    except Exception:
        # runtime.transaction restores the previous env/container on failure.
        error = True
        result = {**protocol.receipt_payload(item), "state": "failure"}
    runtime.atomic(claim, protocol.canonical(result), claim.stat())
    post(writer, f"deployments/{deployment['id']}/statuses", {"state": result["state"], "auto_inactive": False,
         "environment": "staging", "description": "VPS gate and operation verified" if not error else
         "VPS operation failed; inspect protected local backup and receipt"})


def main():
    import fcntl
    os.umask(0o077)
    trusted(INSTALL)
    os.chdir(INSTALL)
    # Directory lock serializes this worker with its own future timer ticks.
    lock = os.open(runtime.ENV.parent, os.O_RDONLY | os.O_DIRECTORY)
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        api = gate.GitHub(read_token(AUDIT_TOKEN))
        writer = gate.GitHub(read_token(RECEIPT_TOKEN))
        trusted(INSTALL / "receipt-identity")
        identity = (INSTALL / "receipt-identity").read_text().strip()
        runs = api.pages("actions/runs?event=workflow_dispatch&branch=staging&status=in_progress", "workflow_runs")
        for run in runs:
            if run.get("path", "").split("@")[0] not in {".github/workflows/deploy-platform.yml", ".github/workflows/certify-staging-restore.yml"}:
                continue
            try:
                item = download_request(api, run["id"])
                if item:
                    handle(api, writer, identity, item)
            except Exception:
                print("Notification SMTP request blocked; authorization/credential/restore prerequisites not satisfied")
    finally:
        os.close(lock)


if __name__ == "__main__":
    try:
        main()
    except Exception:
        print("Notification SMTP worker unavailable; inspect protected VPS setup")
        raise SystemExit(1) from None
