"""One-command, operator-run STAGING Notification SMTP activation.

Runs only as the existing 'deploy' Unix user on the KarryGo VPS.
Reuses smtp_runtime's TLS/auth test, protected backup, narrow container
recreation, image/health verification and automatic rollback.
No GitHub runner, separate service, webhook or new credential is required.
"""
import argparse
import contextlib
import fcntl
import hmac
import json
import os
from pathlib import Path
import pwd
import stat
import sys
import time

import smtp_runtime as runtime


CONFIRM = "STAGING_NOTIFICATION_ONLY"
CONTAINER = "karrygo-notification-service"


def require_operator():
    if os.geteuid() == 0 or os.geteuid() != pwd.getpwnam("deploy").pw_uid:
        raise ValueError("run as the existing deploy user, not root")


def protected(path, directory=False):
    try:
        if directory:
            runtime.private_directory(path)
        else:
            runtime.private(path)
        return True
    except (OSError, ValueError):
        return False


def report():
    item = runtime.container(CONTAINER)
    container_env = runtime.env_of(item) if item else {}
    try:
        staging_file = runtime.value(runtime.ENV.read_text(), "APP_ENV") == "staging"
    except (OSError, ValueError, UnicodeError):
        staging_file = False
    env_private = protected(runtime.ENV)
    key_private = protected(runtime.RESEND_KEY) and (
        # The parent is a protected service .secrets directory, mode 700.
        protected(runtime.RESEND_KEY.parent, directory=True)
    )
    healthy = bool(item and item.get("State", {}).get("Health", {}).get("Status") == "healthy")
    staging_container = container_env.get("APP_ENV") == "staging"
    release_file = runtime.ENV.parent / "release.env"
    try:
        runtime.no_symlinks(release_file)
        release_present = release_file.is_file()
        runtime.no_symlinks(runtime.APPLY)
        apply_present = runtime.APPLY.is_file()
    except (OSError, ValueError):
        release_present = apply_present = False
    matching = all(container_env.get(k) == v for k, v in runtime.SETTINGS.items())
    ready = all((env_private, key_private, healthy, staging_file, staging_container,
                 release_present, apply_present))
    return {"service": "notification-service", "environment_file_staging": staging_file,
            "container_staging": staging_container, "container_healthy": healthy,
            "env_file_protected": env_private, "resend_key_protected": key_private,
            "release_file_present": release_present, "apply_script_present": apply_present,
            "resend_settings_loaded": matching,
            "smtp_password_loaded": bool(container_env.get("NOTIFICATION_SMTP_PASSWORD")),
            "ready_for_activation": ready}, item


@contextlib.contextmanager
def exclusive_lock():
    runtime.no_symlinks(runtime.BACKUPS)
    runtime.BACKUPS.mkdir(mode=0o700, exist_ok=True)
    runtime.private_directory(runtime.BACKUPS)
    lockfile = runtime.BACKUPS / ".local-operation.lock"
    fd = os.open(lockfile, os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600)
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o600 or info.st_uid != os.geteuid():
            raise ValueError("invalid operation lock ownership or permissions")
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        yield
    finally:
        os.close(fd)


def perform(operation, backup_id=None):
    with exclusive_lock():
        status, item = report()
        if not status["ready_for_activation"]:
            raise ValueError("staging prerequisites failed; run inspect")
        if operation == "activate" and status["resend_settings_loaded"]:
            local_password = runtime.read_resend_key()
            if hmac.compare_digest(runtime.env_of(item).get("NOTIFICATION_SMTP_PASSWORD", ""), local_password):
                runtime.smtp_auth(local_password)
                print("Already configured; Resend SMTP authentication verified. No container restart.")
                return
        run_id = str(time.time_ns())
        request = {"operation": operation, "run_id": run_id}
        if operation == "rollback":
            if not backup_id or not backup_id.isdecimal() or int(backup_id) < 1:
                raise ValueError("valid numeric activation backup ID required")
            request["backup_run_id"] = backup_id
        original_backup = runtime.transaction(request, item)
        if operation == "activate":
            print("STAGING SMTP activated; protected backup ID: " + original_backup)
            print("Container health and SMTP login verified; real inbox/OTP acceptance is still required.")
        else:
            print("STAGING SMTP configuration rolled back; original activation backup ID: " + original_backup)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="operation", required=True)
    commands.add_parser("inspect", help="print safe readiness booleans; no changes")
    activation = commands.add_parser("activate", help="activate SMTP only for staging Notification")
    activation.add_argument("--confirm", required=True)
    rollback = commands.add_parser("rollback", help="restore a specific activation backup")
    rollback.add_argument("--backup-id", required=True)
    rollback.add_argument("--confirm", required=True)
    args = parser.parse_args(argv)
    try:
        require_operator()
        if args.operation == "inspect":
            status, _ = report()
            print(json.dumps(status, indent=2, sort_keys=True))
            return 0 if status["ready_for_activation"] else 2
        if args.confirm != CONFIRM:
            raise ValueError("confirmation mismatch; expected " + CONFIRM)
        perform(args.operation, getattr(args, "backup_id", None))
        return 0
    except Exception:
        # Never print provider credentials, container env, or mail recipients.
        print("SMTP operation blocked or failed. Inspect the service and protected local backup before retrying.", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
