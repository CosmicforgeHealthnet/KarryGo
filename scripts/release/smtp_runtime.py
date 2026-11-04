"""Runs on approved staging host. Fixed paths, protected backups, no raw output.

No SMTP send or DNS/provider/webhook change. Partner is inspection-only until
the existing Gateway split deployment hold is separately resolved.
"""
import contextlib
import hashlib
import json
import os
from pathlib import Path
import re
import smtplib
import ssl
import stat
import subprocess
import sys
import tempfile

ROOT = Path("/opt/karrygo")
ENV = ROOT / "notification-service/notification.service.env"
BACKUPS = ROOT / "notification-service/.smtp-backups"
APPLY = ROOT / "platform/bin/apply-env.sh"
SETTINGS = {
    "NOTIFICATION_EMAIL_PROVIDER": "smtp", "NOTIFICATION_SMTP_HOST": "smtp.resend.com",
    "NOTIFICATION_SMTP_PORT": "465", "NOTIFICATION_SMTP_TLS_MODE": "tls",
    "NOTIFICATION_SMTP_TIMEOUT_SECONDS": "15", "NOTIFICATION_SMTP_USERNAME": "resend",
    "NOTIFICATION_EMAIL_FROM": "Karryam <no-reply@karryam.com>",
}
PARTNER = {"PARTNER_SMTP_HOST": "smtp.resend.com", "PARTNER_SMTP_PORT": "465",
           "PARTNER_SMTP_TLS_MODE": "tls", "PARTNER_SMTP_USERNAME": "resend",
           "PARTNER_SMTP_FROM": "no-reply@karryam.com", "PARTNER_SMTP_FROM_NAME": "Karryam"}


def require(value):
    if not value:
        raise ValueError("host prerequisite failed")


def no_symlinks(path):
    for item in [path, *path.parents]:
        require(not item.is_symlink())


def private(path):
    no_symlinks(path)
    info = path.stat()
    require(stat.S_ISREG(info.st_mode) and stat.S_IMODE(info.st_mode) == 0o600 and info.st_uid == os.geteuid())
    return info


def private_directory(path):
    no_symlinks(path)
    info = path.stat()
    require(stat.S_ISDIR(info.st_mode) and stat.S_IMODE(info.st_mode) == 0o700 and info.st_uid == os.geteuid())


def private_service_directory(path):
    no_symlinks(path)
    info = path.stat()
    require(stat.S_ISDIR(info.st_mode) and info.st_uid == os.geteuid() and not stat.S_IMODE(info.st_mode) & 0o027)


def value(data, name):
    matches = re.findall(r"(?m)^[ \t]*(?:export[ \t]+)?" + re.escape(name) + r"[ \t]*=[ \t]*(.*)$", data)
    require(len(matches) <= 1)
    result = matches[0].strip() if matches else ""
    if result[:1] in {"'", '"'}:
        require(len(result) >= 2 and result[-1] == result[0])
        result = result[1:-1]
    return result


def patch_env(data, values):
    # Never source/eval the environment. Keep all unrelated bytes verbatim.
    for name, setting in values.items():
        require("\n" not in setting and "\r" not in setting and "$" not in setting and '"' not in setting)
        value(data, name)  # Reject duplicate/unterminated target assignments.
        pattern = r"(?m)^[ \t]*(?:export[ \t]+)?" + re.escape(name) + r"[ \t]*=.*$"
        line = name + '="' + setting + '"'
        if re.search(pattern, data):
            data = re.sub(pattern, lambda _: line, data)
        else:
            data += ("" if not data or data.endswith("\n") else "\n") + line + "\n"
    return data


def atomic(path, data, info):
    no_symlinks(path)
    fd, name = tempfile.mkstemp(prefix=".smtp-", dir=path.parent)
    try:
        with os.fdopen(fd, "wb") as output:
            os.fchmod(output.fileno(), 0o600)
            os.fchown(output.fileno(), info.st_uid, info.st_gid)
            output.write(data)
            output.flush()
            os.fsync(output.fileno())
        os.replace(name, path)
        directory = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(directory)
        finally:
            os.close(directory)
    finally:
        with contextlib.suppress(FileNotFoundError):
            os.unlink(name)


def container(name):
    result = subprocess.run(["docker", "inspect", name], capture_output=True, timeout=20)
    if result.returncode:
        return None
    return json.loads(result.stdout)[0]


def env_of(item):
    return dict(entry.split("=", 1) for entry in item["Config"]["Env"] if "=" in entry)


def facts(item, settings, password):
    env = env_of(item) if item else {}
    return {"present": item is not None, "healthy": bool(item and item["State"].get("Health", {}).get("Status") == "healthy"),
            "staging": env.get("APP_ENV") == "staging", "settings_match": all(env.get(k) == v for k, v in settings.items()),
            "password_present": bool(env.get(password))}


def smtp_auth(password):
    # A host-side TLS/authentication probe only; never sends an email and does
    # not establish container egress, domain verification or inbox delivery.
    context = ssl.create_default_context()
    context.minimum_version = ssl.TLSVersion.TLSv1_2
    with smtplib.SMTP_SSL("smtp.resend.com", 465, timeout=15, context=context) as smtp:
        smtp.login("resend", password)


def apply():
    # apply-env may print application logs on failure. Keep ALL its output off
    # the Actions stream, including provider errors and recipient information.
    result = subprocess.run(["bash", str(APPLY), "notification-service"], capture_output=True, timeout=180)
    require(result.returncode == 0)


def verify(expected, image):
    current = container("karrygo-notification-service")
    require(current and current["Image"] == image and facts(current, SETTINGS, "NOTIFICATION_SMTP_PASSWORD")["healthy"])
    loaded = env_of(current)
    require(all(loaded.get(k, "") == v for k, v in expected.items()))


def transaction(request, item):
    operation = request["operation"]
    if operation == "inspect":
        return ""
    require(operation in {"activate", "rollback"})
    info = private(ENV)
    private_service_directory(ENV.parent)
    old = ENV.read_bytes()
    text = old.decode("utf-8")
    require(value(text, "APP_ENV") == "staging" and env_of(item).get("APP_ENV") == "staging")
    require(item["State"].get("Health", {}).get("Status") == "healthy")
    release = ENV.parent / "release.env"
    no_symlinks(release)
    image = value(release.read_text(), "IMAGE")
    require(image and not image.startswith("-"))
    resolved = subprocess.run(["docker", "image", "inspect", image], capture_output=True, timeout=20)
    require(resolved.returncode == 0 and json.loads(resolved.stdout)[0]["Id"] == item["Image"])
    no_symlinks(APPLY)
    require(APPLY.is_file())
    no_symlinks(BACKUPS)
    BACKUPS.mkdir(mode=0o700, exist_ok=True)
    private_directory(BACKUPS)
    run_id = request["run_id"]
    require(re.fullmatch(r"[1-9][0-9]*", run_id))
    source_id = request.get("backup_run_id", "") if operation == "rollback" else run_id
    require(re.fullmatch(r"[1-9][0-9]*", source_id))
    directory = BACKUPS / source_id
    no_symlinks(directory)
    if operation == "activate":
        password = request["password"]
        require(re.fullmatch(r"re_[A-Za-z0-9_-]+", password))
        smtp_auth(password)
        expected = {**SETTINGS, "NOTIFICATION_SMTP_PASSWORD": password}
        new = patch_env(text, expected).encode("utf-8")
        directory.mkdir(mode=0o700)  # Never overwrite a backup from another run.
        atomic(directory / "before.env", old, info)
        manifest = {"before": hashlib.sha256(old).hexdigest(), "after": hashlib.sha256(new).hexdigest(), "image": item["Image"]}
        atomic(directory / "manifest.json", json.dumps(manifest).encode(), info)
    else:
        private_directory(directory)
        private(directory / "before.env")
        private(directory / "manifest.json")
        manifest = json.loads((directory / "manifest.json").read_text())
        new = (directory / "before.env").read_bytes()
        require(hashlib.sha256(old).hexdigest() == manifest["after"] and hashlib.sha256(new).hexdigest() == manifest["before"])
        require(item["Image"] == manifest["image"])  # Do not roll config across a different binary.
        expected = {k: value(new.decode(), k) for k in [*SETTINGS, "NOTIFICATION_SMTP_PASSWORD"]}
        # Preserve the current config too, so a failed rollback is reversible.
        undo = BACKUPS / run_id
        undo.mkdir(mode=0o700)
        atomic(undo / "before-rollback.env", old, info)
    # Always recreate, even if file values already match: container may be stale.
    atomic(ENV, new, info)
    try:
        apply()
        verify(expected, item["Image"])
    except Exception:
        atomic(ENV, old, info)
        apply()
        restored = container("karrygo-notification-service")
        require(restored and restored["Image"] == item["Image"] and restored["State"].get("Health", {}).get("Status") == "healthy")
        require(env_of(restored) == env_of(item))
        raise ValueError("configuration apply failed; previous configuration restored") from None
    return source_id


def main():
    import fcntl  # Linux host only. Keep pure functions importable for local tests.
    request = json.loads(sys.stdin.read(16384))
    require(request.get("operation") in {"inspect", "activate", "rollback"})
    no_symlinks(ENV)
    # Lock the service directory inode; read-only inspection creates no files.
    lock = os.open(ENV.parent, os.O_RDONLY | os.O_DIRECTORY)
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        item = container("karrygo-notification-service")
        partner = container("karrygo-partner-web-service")
        partner_facts = facts(partner, PARTNER, "PARTNER_SMTP_PASSWORD")
        # Discover the actual Compose env_file, rather than assuming template
        # paths reflect an already completed Partner/Gateway split.
        partner_facts["registered_env_file"] = False
        if partner:
            labels = partner["Config"].get("Labels") or {}
            files = labels.get("com.docker.compose.project.config_files", "").split(",")
            if files == [str(ROOT / "platform/compose/partner-web-service.yml")]:
                partner_facts["registered_env_file"] = "/opt/karrygo/partner-web-service/partner.env" in Path(files[0]).read_text()
        report = {"ok": False, "operation": request["operation"], "notification": facts(item, SETTINGS, "NOTIFICATION_SMTP_PASSWORD"),
                  "partner": partner_facts, "backup_run_id": ""}
        try:
            private(ENV)
            report["backup_run_id"] = transaction(request, item)
            report["notification"] = facts(container("karrygo-notification-service"), SETTINGS, "NOTIFICATION_SMTP_PASSWORD")
            report["ok"] = True
        finally:
            with contextlib.suppress(Exception):
                report["notification"] = facts(container("karrygo-notification-service"), SETTINGS, "NOTIFICATION_SMTP_PASSWORD")
            print(json.dumps(report))
        return 0
    finally:
        os.close(lock)


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception:
        # No exception message/traceback, env, provider reply or container logs.
        raise SystemExit(1) from None
