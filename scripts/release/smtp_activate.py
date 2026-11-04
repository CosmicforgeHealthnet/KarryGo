"""Existing protected staging SSH transport; never print credentials or host output."""
import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import tempfile

import gate

KEY = "KARRYGO_NOTIFICATIONS_RESEND_SMTP_PASSWORD"


def main():
    context = {k: os.environ.get("GITHUB_" + k.upper(), "") for k in
               ("repository", "event_name", "ref", "sha", "workflow_sha", "run_attempt", "run_id", "actor", "triggering_actor")}
    api = gate.GitHub(os.environ.get("RELEASE_AUDIT_TOKEN"))
    gate.authorize(api, context, os.environ["RELEASE_SHA"], os.environ["RELEASE_SERVICES"],
                   "smtp", os.environ["RESTORE_RUN_ID"])
    operation = os.environ["SMTP_OPERATION"]
    gate.require(operation in {"inspect", "activate", "rollback"}, "invalid SMTP operation")
    backup = os.environ.get("SMTP_BACKUP_RUN_ID", "")
    gate.require(operation != "rollback" or re.fullmatch(r"[1-9][0-9]*", backup), "rollback requires activation run ID")
    gate.require(re.fullmatch(r"[1-9][0-9]*", context["run_id"]), "invalid run ID")
    password = ""
    if operation == "activate":
        names = {s["name"] for s in api.pages("environments/staging/secrets", "secrets")}
        gate.require(KEY in names, "missing staging environment KarryGo Notifications SMTP secret")
        paths = ["actions/secrets"]
        if api.get("")["owner"]["type"] == "Organization":
            paths.append("actions/organization-secrets")
        for path in paths:
            gate.require(KEY not in {s["name"] for s in api.pages(path, "secrets")},
                         "SMTP credential must be environment-scoped only")
        password = os.environ.get(KEY, "")
        gate.require(re.fullmatch(r"re_[A-Za-z0-9_-]+", password), "missing/invalid protected Resend SMTP secret")
    host, user = os.environ["STAGING_DEPLOY_HOST"], os.environ["STAGING_DEPLOY_USER"]
    gate.require(re.fullmatch(r"[a-zA-Z0-9.-]+", host) and re.fullmatch(r"[a-z_][a-z0-9_-]*", user), "invalid SSH destination")
    gate.require(os.environ.get("STAGING_DEPLOY_SSH_KEY") and os.environ.get("STAGING_DEPLOY_KNOWN_HOSTS"),
                 "missing protected staging SSH credentials")
    payload = json.dumps({"operation": operation, "run_id": context["run_id"],
                          "backup_run_id": backup, "password": password})
    with tempfile.TemporaryDirectory() as directory:
        key = Path(directory) / "key"
        key.write_text(os.environ["STAGING_DEPLOY_SSH_KEY"] + "\n")
        key.chmod(0o600)
        hosts = Path(directory) / "known_hosts"
        hosts.write_text(os.environ["STAGING_DEPLOY_KNOWN_HOSTS"] + "\n")
        # The program is non-secret; the key travels ONLY on encrypted stdin,
        # never argv, shell interpolation, files, evidence or console output.
        program = Path("scripts/release/smtp_runtime.py").read_text()
        result = subprocess.run(["ssh", "-i", str(key), "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes",
                                 "-o", "ConnectTimeout=15", "-o", "UserKnownHostsFile=" + str(hosts),
                                 user + "@" + host, "python3 -c " + shlex.quote(program)],
                                input=payload, text=True, capture_output=True, timeout=600)
    # Never forward banners, command output, traceback or errors from the VPS.
    # Only schema-checked facts from the remote program can reach Actions logs.
    try:
        report = json.loads(result.stdout)
        gate.require(isinstance(report, dict) and set(report) == {"ok", "operation", "notification", "partner", "backup_run_id"},
                     "invalid host report")
        for name, fields in {"notification": {"present", "healthy", "staging", "settings_match", "password_present"},
                             "partner": {"present", "healthy", "staging", "settings_match", "password_present", "registered_env_file"}}.items():
            gate.require(isinstance(report[name], dict) and set(report[name]) == fields
                         and all(type(v) is bool for v in report[name].values()), "invalid host facts")
        gate.require(type(report["ok"]) is bool and report["operation"] == operation
                     and report["backup_run_id"] in {"", context["run_id"], backup}, "invalid host operation")
    except (ValueError, KeyError, TypeError):
        raise ValueError("SMTP host operation failed; output suppressed to protect secrets") from None
    print(json.dumps(report, sort_keys=True))
    gate.require(result.returncode == 0 and report["ok"],
                 "SMTP operation failed; inspect protected host backup and health; no provider delivery claimed")


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        # Validation errors are controlled strings. Never print a subprocess
        # exception containing argv/input or an external response body.
        print(str(error) if type(error) is ValueError else "SMTP activation blocked by unavailable authorization, SSH or host prerequisite")
        raise SystemExit(1)
