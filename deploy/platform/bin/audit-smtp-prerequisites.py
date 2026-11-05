"""Authorized VPS administration: read-only metadata; never prints secret values."""
import json
import os
from pathlib import Path
import pwd
import stat
import subprocess

root = Path("/opt/karrygo")
owner = pwd.getpwnam("deploy").pw_uid
paths = [root / "notification-service/notification.service.env",
         root / "notification-service/.secrets",
         root / "notification-service/.secrets/resend-smtp-key",
         root / "platform/.secrets/release-audit-token",
         root / "platform/.secrets/release-receipt-token",
         root / "platform/.secrets",
         Path("/usr/local/lib/karrygo-smtp/approved-revision")]
metadata = []
for path in paths:
    info = path.lstat() if path.exists() or path.is_symlink() else None
    metadata.append({"path": str(path), "present": info is not None,
                     "symlink": path.is_symlink(), "deploy_owned": bool(info and info.st_uid == owner),
                     "mode": format(stat.S_IMODE(info.st_mode), "03o") if info else None,
                     "owner_only": bool(info and stat.S_IMODE(info.st_mode) == 0o600)})
env_path = paths[0]
staging_file = False
if env_path.is_file() and not env_path.is_symlink():
    lines = env_path.read_text().splitlines()
    values = [line.split("=", 1)[1].strip().strip('"\'') for line in lines if line.startswith("APP_ENV=")]
    staging_file = values == ["staging"]
result = subprocess.run(["docker", "inspect", "karrygo-notification-service"], capture_output=True, timeout=20)
item = json.loads(result.stdout)[0] if result.returncode == 0 else {}
env = dict(part.split("=", 1) for part in item.get("Config", {}).get("Env", []) if "=" in part)
candidates = []
for base in [root / "platform", root / "notification-service", root / "backups", Path("/etc/karrygo")]:
    for directory, children, files in os.walk(base, followlinks=False):
        children[:] = [name for name in children if name not in {".smtp-backups", ".smtp-requests", "node_modules", ".git"}
                       and not Path(directory, name).is_symlink()]
        if len(Path(directory).relative_to(base).parts) >= 3:
            children[:] = []
        for name in files:
            lower = name.lower()
            if ("audit" in lower or "restore" in lower) and len(candidates) < 30:
                candidates.append(str(Path(directory, name)))
print(json.dumps({"files": metadata, "staging_env_file": staging_file,
                  "staging_container": env.get("APP_ENV") == "staging",
                  "healthy": item.get("State", {}).get("Health", {}).get("Status") == "healthy",
                  "resend_host_loaded": env.get("NOTIFICATION_SMTP_HOST") == "smtp.resend.com",
                  "target_sender_loaded": env.get("NOTIFICATION_EMAIL_FROM") == "Karryam <no-reply@karryam.com>",
                  "candidate_audit_or_restore_paths": candidates}, indent=2))
