"""Runs on approved staging host. Fixed paths, protected backups, no raw output.

No SMTP send or DNS/provider/webhook change. This library is invoked only by
the locally installed Notification worker after independent authorization.
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
import tempfile

ROOT = Path("/opt/karrygo")
ENV = ROOT / "notification-service/notification.service.env"
BACKUPS = ROOT / "notification-service/.smtp-backups"
APPLY = ROOT / "platform/bin/apply-env.sh"
RESEND_KEY = ROOT / "notification-service/.secrets/resend-smtp-key"
SETTINGS = {
    "NOTIFICATION_EMAIL_PROVIDER": "smtp", "NOTIFICATION_SMTP_HOST": "smtp.resend.com",
    "NOTIFICATION_SMTP_PORT": "465", "NOTIFICATION_SMTP_TLS_MODE": "tls",
    "NOTIFICATION_SMTP_TIMEOUT_SECONDS": "15", "NOTIFICATION_SMTP_USERNAME": "resend",
    "NOTIFICATION_EMAIL_FROM": "Karryam <no-reply@karryam.com>",
}


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


def read_resend_key():
    private(RESEND_KEY)
    private_service_directory(RESEND_KEY.parent)
    password = RESEND_KEY.read_text().rstrip("\r\n")
    require(re.fullmatch(r"re_[A-Za-z0-9_-]+", password))
    return password


def unrelated(data):
    # Exact bytes of every unrelated assignment/comment must survive.
    names = "|".join([*SETTINGS, "NOTIFICATION_SMTP_PASSWORD"])
    return re.sub(r"(?m)^[ \t]*(?:export[ \t]+)?(?:" + names + r")[ \t]*=.*(?:\n|$)", "", data)


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


def verify(expected, image, baseline=None):
    current = container("karrygo-notification-service")
    require(current and current["Image"] == image and facts(current, SETTINGS, "NOTIFICATION_SMTP_PASSWORD")["healthy"])
    loaded = env_of(current)
    require(all(loaded.get(k, "") == v for k, v in expected.items()))
    if baseline is not None:
        changed = set(expected)
        require({k: v for k, v in loaded.items() if k not in changed} ==
                {k: v for k, v in baseline.items() if k not in changed})


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
        require("password" not in request)
        password = read_resend_key()
        smtp_auth(password)
        expected = {**SETTINGS, "NOTIFICATION_SMTP_PASSWORD": password}
        new = patch_env(text, expected).encode("utf-8")
        require(unrelated(text).rstrip("\n") == unrelated(new.decode()).rstrip("\n"))
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
        verify(expected, item["Image"], env_of(item))
        if operation == "activate":
            smtp_auth(expected["NOTIFICATION_SMTP_PASSWORD"])
    except Exception:
        atomic(ENV, old, info)
        apply()
        restored = container("karrygo-notification-service")
        require(restored and restored["Image"] == item["Image"] and restored["State"].get("Health", {}).get("Status") == "healthy")
        require(env_of(restored) == env_of(item))
        raise ValueError("configuration apply failed; previous configuration restored") from None
    return source_id


if __name__ == "__main__":
    raise SystemExit("Use the installed, authorized VPS request worker")
