#!/usr/bin/env python3
"""Staging-only Vendor forward migration rehearsal on an isolated DB clone."""
import os
import re
import subprocess as sp
import sys
import tempfile
import time
import uuid
from pathlib import Path
from urllib.parse import parse_qsl, urlsplit, urlunsplit

def run(args, **kwargs):
    return sp.run(args, check=True, **kwargs)

def output(args):
    return sp.check_output(args, text=True, stderr=sp.DEVNULL).strip()

def main():
    if len(sys.argv) != 3:
        raise ValueError("expected candidate image and private backup")
    image, backup = sys.argv[1:]
    if not re.fullmatch(r"ghcr.io/cosmicforgehealthnet/vendor-service:[0-9a-f]{40}", image):
        raise ValueError("invalid staging Vendor image")
    source = "karrygo-vendor-staging-postgres"
    uid = uuid.uuid4().hex[:16]
    name = "karrygo-vendor-rehearsal-" + uid
    bootstrap = "restore_" + uid
    old = output(["docker", "inspect", "-f", "{{.Image}}", source])
    env = dict(line.split("=", 1) for line in Path("/opt/karrygo/vendor-service/vendor.env").read_text().splitlines()
               if re.match(r"^[A-Z][A-Z0-9_]*=", line))
    if env.get("APP_ENV", "").strip("'\"") != "staging":
        raise ValueError("not staging")
    url = urlsplit(env["VENDOR_DATABASE_URL"].strip("'\""))
    if url.scheme not in ("postgres", "postgresql") or not url.username or not url.path:
        raise ValueError("invalid Vendor database URL")
    host = url.hostname
    addresses = {"karrygo-vendor-staging-postgres"}
    import json
    for net in json.loads(output(["docker", "inspect", "-f", "{{json .NetworkSettings.Networks}}", source])).values():
        addresses.update(net.get("Aliases") or [])
        addresses.add(net.get("IPAddress"))
    if host not in addresses:
        raise ValueError("Vendor database URL does not identify the expected staging DB")
    if any(key.lower() in {"host", "hostaddr", "port", "service", "dbname"} for key, _ in parse_qsl(url.query)):
        raise ValueError("unexpected database endpoint override")
    local = urlunsplit((url.scheme, url.netloc.rsplit("@", 1)[0] + "@127.0.0.1:5432", url.path, url.query, ""))
    os.umask(0o077)
    with tempfile.TemporaryDirectory(prefix="vendor-rehearsal-") as td:
        envfile = Path(td) / "env"
        envfile.write_text("VENDOR_DATABASE_URL=" + local + "\nVENDOR_MIGRATIONS_DIR=/srv/vendor-service/migrations\n")
        with open(str(backup) + ".rehearsal.log", "w") as log:
            try:
                run(["docker", "run", "-d", "--name", name, "--network", "none",
                     "--memory", "1g", "--pids-limit", "256",
                     "-e", "POSTGRES_USER=" + bootstrap, "-e", "POSTGRES_PASSWORD=" + uuid.uuid4().hex,
                     "-e", "POSTGRES_DB=restore_bootstrap", old], stdout=log, stderr=log)
                for _ in range(60):
                    if sp.run(["docker", "exec", name, "pg_isready", "-h", "127.0.0.1",
                               "-U", bootstrap, "-d", "restore_bootstrap"], stdout=log, stderr=log).returncode == 0:
                        break
                    time.sleep(1)
                else:
                    raise ValueError("sandbox readiness timeout")
                import gzip
                with gzip.open(backup, "rb") as f:
                    p = sp.Popen(["docker", "exec", "-i", name, "psql", "-X", "-q", "-v", "ON_ERROR_STOP=1",
                                  "-U", bootstrap, "-d", "restore_bootstrap"], stdin=sp.PIPE, stdout=log, stderr=log)
                    import shutil
                    try:
                        shutil.copyfileobj(f, p.stdin)
                    finally:
                        p.stdin.close()
                        if p.wait() != 0:
                            raise ValueError("sandbox restore failed")
                # Candidate sees only the disposable DB loopback, no live
                # network, external providers, or published ports.
                run(["docker", "run", "--rm", "--network", "container:" + name,
                     "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
                     "--env-file", str(envfile), "--entrypoint", "/bin/vendor-migrate", image], stdout=log, stderr=log)
                print("VENDOR_REHEARSAL_OK restored backup and candidate migrations")
            finally:
                sp.run(["docker", "rm", "-f", "-v", name], stdout=log, stderr=log)

if __name__ == "__main__":
    try:
        main()
    except Exception:
        # DB URLs and migration detail stay in the mode-600 host log.
        print("Vendor restore/migration rehearsal failed; inspect private VPS backup rehearsal log", file=sys.stderr)
        sys.exit(1)
