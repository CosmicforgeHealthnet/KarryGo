#!/usr/bin/env python3
"""Routine Admin staging replacement using the existing private enrollment.

The automatic workflow must have passed exact-revision CI. This path does not
write the operator certification list, enroll principals, activate features,
or alter firewall rules. Test migration and both binaries on an isolated
database copy, then replace only the already-enrolled app.
"""
import gzip
import ipaddress
import json
import os
from pathlib import Path
import re
import shutil
import ssl
import socket
import subprocess as sp
import sys
import tempfile
import time
import uuid
from urllib.parse import parse_qsl, urlsplit, urlunsplit

ROOT = Path("/opt/karrygo/admin-backoffice-service")
APP = "karrygo-admin-backoffice-service"
DB = "karrygo-admin-db"
BIN = "/bin/cosmicforge-logistics-service"

def require(ok, message):
    if not ok:
        raise ValueError(message)

def inspect(name, kind="container"):
    return json.loads(sp.check_output(["docker", kind, "inspect", name], stderr=sp.DEVNULL))[0]

def envmap(items):
    return dict(x.split("=", 1) for x in items if "=" in x)

def dotenv(path):
    return {k: v.strip().strip("'\"") for k, v in
            (line.split("=", 1) for line in path.read_text().splitlines()
             if re.match(r"^[A-Z][A-Z0-9_]*=", line))}

def capture(args, **kw):
    return sp.check_output(args, stderr=sp.DEVNULL, **kw)

def wait_probe(name, log):
    for _ in range(36):
        if sp.run(["docker", "exec", name, BIN, "--probe"], stdout=log, stderr=log).returncode == 0:
            return
        time.sleep(5)
    raise ValueError("Admin local readiness failed")

def private_mounts(container):
    mounts = set()
    for m in container["Mounts"]:
        require(m["Type"] == "bind" and not m["RW"], "unexpected Admin mount")
        source = m["Source"]
        require(source.startswith(("/opt/karrygo/admin-backoffice-service/", "/etc/cosmicforge/staging/karryam")),
                "unexpected Admin mount source")
        require(m["Destination"].startswith("/run/"), "unexpected Admin mount target")
        mounts.add((source, m["Destination"]))
    return mounts

def ports(container):
    return {(p, x["HostIp"], x["HostPort"]) for p, entries in
            (container["HostConfig"]["PortBindings"] or {}).items() for x in entries or []}

def plan(image):
    old = inspect(APP)
    require(old["State"]["Running"] and old["State"]["Health"]["Status"] == "healthy",
            "existing Admin must be healthy")
    labels = old["Config"]["Labels"]
    project = labels.get("com.docker.compose.project", "")
    require(project == "karrygo-admin-backoffice-service" and labels.get("com.docker.compose.service") == "app",
            "unexpected Admin Compose identity")
    files = [Path(x) for x in labels.get("com.docker.compose.project.config_files", "").split(",") if x]
    cwd = Path(labels.get("com.docker.compose.project.working_dir", ""))
    require(bool(files) and cwd.is_dir() and all(f.is_file() and os.access(f, os.R_OK) and
            str(f.resolve()).startswith(("/opt/karrygo/", "/etc/cosmicforge/staging/")) for f in files),
            "existing Admin Compose files unavailable to deployment account")
    require(ports(old) and len(ports(old)) == 1, "unexpected Admin listener")
    target, ip, port = next(iter(ports(old)))
    addr = ipaddress.ip_address(ip)
    require(target == "8110/tcp" and port == "8110" and addr.version == 4 and
            (addr.is_loopback or any(addr in ipaddress.ip_network(n) for n in
             ("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10"))),
            "Admin listener is not approved private IPv4")
    networks = set(old["NetworkSettings"]["Networks"])
    require("karrygo-admin-backoffice-service_admin-private" in networks and
            "karrygo-net" not in networks, "unexpected Admin private topology")
    env = dict(os.environ, IMAGE=image, ADMIN_PRIVATE_BIND_IP=ip)
    ingress = "karrygo-admin-backoffice-service_admin-ingress"
    if ingress in networks:
        net = inspect(ingress, "network")
        ipam = net["IPAM"]["Config"]
        if len(ipam) == 1 and ipam[0].get("Subnet"):
            env["ADMIN_INGRESS_SUBNET"] = ipam[0]["Subnet"]
    cmd = ["docker", "compose", "-p", project, "--project-directory", str(cwd)]
    for file in files:
        cmd += ["-f", str(file)]
    config = json.loads(capture(cmd + ["config", "--format", "json"], env=env, cwd=cwd))
    app = config["services"]["app"]
    migrate = config["services"]["migrate"]
    require(app.get("image") == image and migrate.get("image") == image, "Admin Compose image not parameterized")
    require(app.get("container_name") == APP and str(app.get("user")) == "100:101" and
            app.get("read_only") and not app.get("privileged") and
            "ALL" in app.get("cap_drop", []) and not app.get("cap_add") and
            any(x.startswith("no-new-privileges") for x in app.get("security_opt", [])),
            "Admin Compose safety configuration mismatch")
    desired_ports = {(str(p["target"]) + "/" + p.get("protocol", "tcp"),
                     p.get("host_ip", ""), str(p["published"])) for p in app.get("ports", [])}
    require(desired_ports == ports(old), "Admin Compose would change listener")
    wanted_mounts = {(x["source"], x["target"]) for x in app.get("volumes", [])
                     if x.get("type") == "bind" and x.get("read_only")}
    require(wanted_mounts == private_mounts(old) and len(wanted_mounts) == len(app.get("volumes", [])),
            "Admin Compose would change secret mounts")
    desired = {config["networks"][key]["name"] for key in app.get("networks", {})}
    extras = networks - desired
    require(desired <= networks and extras <= {"cf-staging-karryam-bridge", "cf-staging-karryam-owner-readonly"},
            "Admin Compose would change private networks")
    current_env = envmap(old["Config"]["Env"])
    desired_env = app.get("environment", {})
    require(isinstance(desired_env, dict), "Admin Compose environment not resolved")
    current_admin = {k:v for k,v in current_env.items() if k.startswith("ADMIN_")}
    desired_admin = {k:str(v) for k,v in desired_env.items() if k.startswith("ADMIN_")}
    require(current_admin == desired_admin, "Admin Compose would change existing Admin configuration")
    return old, cmd, env, cwd, extras

def dburl(raw, source):
    u = urlsplit(raw)
    allowed = {DB}
    for network in source["NetworkSettings"]["Networks"].values():
        allowed.update(network.get("Aliases") or [])
        allowed.add(network.get("IPAddress"))
    require(u.scheme in ("postgres", "postgresql") and u.username and u.path and u.hostname in allowed,
            "Admin database URL does not identify existing private staging DB")
    require(not any(k.lower() in {"host", "hostaddr", "port", "service", "dbname"} for k, _ in parse_qsl(u.query)),
            "Admin database URL contains endpoint override")
    return urlunsplit((u.scheme, u.netloc.rsplit("@", 1)[0]+"@127.0.0.1:5432", u.path, u.query, ""))

def write_env(path, values):
    require(all("\n" not in v and "\r" not in v for v in values.values()), "multiline Admin env unsupported")
    path.write_text("".join(k+"="+v+"\n" for k,v in values.items()))

def backup(path):
    with gzip.open(path, "wb") as out:
        p = sp.Popen(["docker", "exec", DB, "sh", "-c",
                      'pg_dumpall --clean --if-exists -U "$POSTGRES_USER"'], stdout=sp.PIPE, stderr=sp.DEVNULL)
        try:
            shutil.copyfileobj(p.stdout, out)
        finally:
            p.stdout.close()
            require(p.wait() == 0, "Admin database backup failed")
    with gzip.open(path, "rb") as f:
        while f.read(1024*1024):
            pass

def rehearse(old, image, archive, log):
    source = inspect(DB)
    name = "karrygo-admin-rehearsal-" + uuid.uuid4().hex[:12]
    user = "restore_" + uuid.uuid4().hex[:12]
    appname = name + "-app"
    with tempfile.TemporaryDirectory(prefix="admin-rehearsal-") as td:
        td = Path(td)
        migr = dotenv(ROOT/"admin.migrate.env")
        write_env(td/"migrate.env", {"ADMIN_BACKOFFICE_DATABASE_URL": dburl(migr["ADMIN_BACKOFFICE_DATABASE_URL"], source)})
        runtime = {k:v for k,v in envmap(old["Config"]["Env"]).items() if k.startswith("ADMIN_") or k=="APP_ENV"}
        runtime["ADMIN_BACKOFFICE_DATABASE_URL"] = dburl(runtime["ADMIN_BACKOFFICE_DATABASE_URL"], source)
        write_env(td/"runtime.env", runtime)
        def call(args):
            sp.run(args, check=True, stdin=sp.DEVNULL, stdout=log, stderr=log)
        try:
            call(["docker", "run", "-d", "--name", name, "--network", "none", "--memory", "1g",
                  "-e", "POSTGRES_USER="+user, "-e", "POSTGRES_PASSWORD="+uuid.uuid4().hex,
                  "-e", "POSTGRES_DB=restore_bootstrap", source["Image"]])
            for _ in range(60):
                if sp.run(["docker", "exec", name, "pg_isready", "-h", "127.0.0.1", "-U", user, "-d", "restore_bootstrap"],
                          stdout=log, stderr=log).returncode == 0:
                    break
                time.sleep(1)
            else:
                raise ValueError("Admin rehearsal database readiness timeout")
            with gzip.open(archive, "rb") as f:
                p = sp.Popen(["docker", "exec", "-i", name, "psql", "-X", "-q", "-v", "ON_ERROR_STOP=1",
                              "-U", user, "-d", "restore_bootstrap"], stdin=sp.PIPE, stdout=log, stderr=log)
                try:
                    shutil.copyfileobj(f, p.stdin)
                finally:
                    p.stdin.close()
                    require(p.wait() == 0, "Admin backup restore failed")
            common = ["docker", "run", "--network", "container:"+name, "--read-only",
                      "--user", "100:101", "--cap-drop", "ALL", "--security-opt", "no-new-privileges"]
            call(common + ["--rm", "--env-file", str(td/"migrate.env"), image, "--migrate"])
            mounts = []
            for sourcepath, dest in private_mounts(old):
                mounts += ["--mount", "type=bind,src="+sourcepath+",dst="+dest+",readonly"]
            # Test both the candidate and the exact previous image against the
            # migrated isolated copy. No network path to live owner services.
            for binary in (image, old["Image"]):
                call(common + ["-d", "--name", appname, "--env-file", str(td/"runtime.env")] + mounts + [binary])
                try:
                    wait_probe(appname, log)
                finally:
                    call(["docker", "rm", "-f", appname])
        finally:
            sp.run(["docker", "rm", "-f", "-v", appname, name], stdout=log, stderr=log)

def verify_runtime(old, image, extras, log):
    actual = inspect(APP)
    require(actual["Image"] == inspect(image, "image")["Id"], "Admin runtime image mismatch")
    for network in extras - set(actual["NetworkSettings"]["Networks"]):
        args = ["docker", "network", "connect"]
        for alias in old["NetworkSettings"]["Networks"][network].get("Aliases") or []:
            if alias and alias not in {old["Id"], old["Id"][:12]}:
                args += ["--alias", alias]
        sp.run(args+[network, APP], check=True, stdout=log, stderr=log)
    wait_probe(APP, log)
    actual = inspect(APP)
    require(ports(actual) == ports(old) and private_mounts(actual) == private_mounts(old) and
            set(actual["NetworkSettings"]["Networks"]) == set(old["NetworkSettings"]["Networks"]),
            "Admin runtime topology mismatch")
    require(actual["HostConfig"]["ReadonlyRootfs"] and not actual["HostConfig"]["Privileged"],
            "Admin runtime isolation mismatch")
    # Negative mTLS test only, not a claim of authenticated HQ acceptance.
    _, ip, port = next(iter(ports(actual)))
    try:
        with socket.create_connection((ip, int(port)), timeout=5) as raw:
            with ssl._create_unverified_context().wrap_socket(raw, server_hostname="localhost") as tls:
                tls.sendall(b"GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n")
                response = tls.recv(256)
                require(not response.startswith(b"HTTP/"), "Admin accepted unauthenticated TLS client")
    except ssl.SSLError:
        pass

def main():
    image, mode = sys.argv[1:]
    require(re.fullmatch(r"ghcr.io/cosmicforgehealthnet/admin-backoffice-service:[0-9a-f]{40}", image), "invalid Admin staging image")
    old, cmd, env, cwd, extras = plan(image)
    if mode == "preflight":
        print("Admin existing deployment topology verified")
        return
    require(mode == "deploy", "unknown Admin operation")
    require(inspect(image, "image")["Config"]["Labels"].get("org.opencontainers.image.revision") == image.rsplit(":",1)[1],
            "Admin candidate revision mismatch")
    os.umask(0o077)
    directory = Path("/opt/karrygo/backups/predeploy")
    directory.mkdir(parents=True, exist_ok=True)
    base = directory/("admin-auto-"+uuid.uuid4().hex)
    previous = old["Config"]["Image"]
    require(inspect(previous, "image")["Id"] == old["Image"], "previous Admin image reference no longer immutable")
    with open(str(base)+".log", "w") as log:
        def compose(args, override=None):
            sp.run(cmd+args, env=dict(env, IMAGE=override or image), cwd=cwd, check=True,
                   stdin=sp.DEVNULL, stdout=log, stderr=log)
        archive = str(base)+".sql.gz"
        backup(archive)
        rehearse(old, image, archive, log)
        print("ADMIN_REHEARSAL_OK restore, migration, candidate and previous runtime probes")
        compose(["stop", "app"])
        try:
            backup(str(base)+".quiesced.sql.gz")
        except Exception:
            compose(["start", "app"], previous)
            raise
        # Migrations are run with the existing separate DDL credential; the
        # application never receives it. No approval marker is manufactured.
        try:
            compose(["run", "--rm", "--no-deps", "-T", "migrate"])
        except Exception:
            raise ValueError("Admin migration failed; app stopped for safe forward recovery")
        try:
            compose(["up", "-d", "--no-deps", "app"])
            verify_runtime(old, image, extras, log)
        except Exception:
            compose(["up", "-d", "--no-deps", "app"], previous)
            verify_runtime(old, previous, extras, log)
            raise ValueError("Admin candidate failed; rehearsed previous image restored")
        for name, value in (("release.previous.env", previous), ("release.env", image)):
            with tempfile.NamedTemporaryFile(mode="w", dir=ROOT, delete=False) as f:
                f.write("IMAGE="+value+"\n")
            os.replace(f.name, ROOT/name)
        evidence = {"image":image, "previous":previous, "runtime_image":inspect(APP)["Image"],
                    "migration_rehearsed":True, "previous_runtime_rehearsed":True, "private_topology_preserved":True,
                    "authenticated_hq_acceptance":False}
        (ROOT/"bridge.auto-certification.json").write_text(json.dumps(evidence, sort_keys=True)+"\n")
        print("ADMIN_AUTO_RUNTIME_OK exact image, isolated restore rehearsal and private topology")

if __name__ == "__main__":
    try:
        main()
    except ValueError as e:
        # Only explicitly constructed diagnostic messages, never subprocess
        # output or configuration values.
        print(str(e), file=sys.stderr)
        sys.exit(1)
    except Exception:
        print("Admin automatic deployment failed; see private VPS backup log", file=sys.stderr)
        sys.exit(1)
