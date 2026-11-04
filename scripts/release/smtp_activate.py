"""GitHub-side request/receipt client. No SSH, application or audit credentials."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import time
import uuid

import gate


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":")).encode()


def request():
    require = gate.require
    require(os.environ.get("GITHUB_REPOSITORY") == gate.REPOSITORY and
            os.environ.get("GITHUB_REF") == "refs/heads/staging" and
            os.environ.get("GITHUB_EVENT_NAME") == "workflow_dispatch", "manual canonical staging request required")
    sha = os.environ["RELEASE_SHA"]
    require(re.fullmatch(r"[0-9a-f]{40}", sha) and sha == os.environ["GITHUB_SHA"] == os.environ["GITHUB_WORKFLOW_SHA"],
            "workflow/ref/release SHA mismatch")
    require(os.environ["RELEASE_SERVICES"] == "notification-service", "Notification-only operation")
    require(os.environ.get("GITHUB_RUN_ATTEMPT") == "1", "fresh run required")
    operation = os.environ["SMTP_OPERATION"]
    require(operation in {"inspect", "activate", "rollback", "certify"}, "invalid operation")
    require(re.fullmatch(r"[1-9][0-9]*", os.environ["GITHUB_RUN_ID"]), "invalid run ID")
    restore = os.environ.get("RESTORE_RUN_ID", "")
    backup = os.environ.get("SMTP_BACKUP_RUN_ID", "")
    require(operation == "certify" or re.fullmatch(r"[1-9][0-9]*", restore), "valid restore run ID required")
    require(operation != "rollback" or re.fullmatch(r"[1-9][0-9]*", backup), "activation backup run ID required")
    proof = json.loads(os.environ["RESTORE_MANIFEST"]) if operation == "certify" else None
    return {"schema": 1, "repository": gate.REPOSITORY, "release_sha": sha,
            "services": "notification-service", "run_id": os.environ["GITHUB_RUN_ID"],
            "operation": operation, "restore_run_id": restore, "backup_run_id": backup,
            "nonce": uuid.uuid4().hex, "restore_manifest": proof}


def receipt_payload(item):
    return {"schema": 1, "run_id": item["run_id"], "operation": item["operation"],
            "request_sha256": hashlib.sha256(canonical(item)).hexdigest()}


def wait(item, api, writer, timeout=480):
    gate.require(writer and writer != "github-actions[bot]" and re.fullmatch(r"[A-Za-z0-9_-]+(?:\[bot\])?", writer),
                 "VPS receipt identity must be configured")
    payload = receipt_payload(item)
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        deployments = api.get("deployments?sha=" + item["release_sha"] + "&environment=staging&task=karrygo-smtp&per_page=100")
        matches = [d for d in deployments if d.get("payload") == payload and d.get("creator", {}).get("login") == writer]
        gate.require(len(matches) <= 1, "ambiguous VPS receipt")
        if matches:
            statuses = api.get(f"deployments/{matches[0]['id']}/statuses?per_page=100")
            statuses = [s for s in statuses if s.get("creator", {}).get("login") == writer]
            if statuses and statuses[0]["state"] in {"success", "failure", "error"}:
                gate.require(statuses[0]["state"] == "success", "VPS operation failed; protected rollback/receipt must be inspected")
                Path("smtp-receipt.json").write_bytes(canonical({**payload, "deployment_id": matches[0]["id"], "state": "success"}))
                if item["operation"] == "certify":
                    Path("restore-evidence.json").write_bytes(canonical(item["restore_manifest"]))
                return
        time.sleep(10)
    raise ValueError("No authenticated VPS receipt; executor, approval or restore prerequisite may be unavailable")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=["request", "wait"])
    args = parser.parse_args()
    if args.mode == "request":
        Path("smtp-request.json").write_bytes(canonical(request()))
    else:
        item = json.loads(Path("smtp-request.json").read_text())
        gate.require(item["run_id"] == os.environ["GITHUB_RUN_ID"], "foreign request")
        wait(item, gate.GitHub(os.environ.get("GITHUB_TOKEN")), os.environ.get("SMTP_RECEIPT_LOGIN"))
    print("Non-secret SMTP request published" if args.mode == "request" else "Authenticated VPS receipt confirmed")


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(str(error) if type(error) is ValueError else "SMTP request/receipt unavailable; no application credentials requested")
        raise SystemExit(1)
