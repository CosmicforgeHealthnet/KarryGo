"""Strict PostgreSQL backup/restore verification, always into a new sandbox.

Operator-owned SELECT assertions are mandatory. A table count alone is not
restore certification. Archive/validation errors propagate; evidence is written
only after all checks succeed. No existing database is ever a restore target.
"""
import argparse
import datetime as dt
import gzip
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import time
import uuid


def run(args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)


def sha256(path):
    with open(path, "rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


class Docker:
    def __init__(self, container, user):
        self.prefix = ["docker", "exec", "-i", container]
        self.user = user

    def sql(self, database, query):
        return subprocess.check_output([*self.prefix, "psql", "-X", "-q", "-A", "-t", "-v", "ON_ERROR_STOP=1",
                                        "-U", self.user, "-d", database], input=query, text=True)

    def restore(self, archive):
        # pg_dumpall --clean can drop postgres AND template1. The bootstrap DB
        # must not exist in the source archive, and is never a live target.
        with gzip.open(archive, "rb") as source:
            process = subprocess.Popen([*self.prefix, "psql", "-X", "-q", "-v", "ON_ERROR_STOP=1",
                                        "-U", self.user, "-d", "restore_bootstrap"], stdin=subprocess.PIPE,
                                       stdout=subprocess.DEVNULL)
            try:
                shutil.copyfileobj(source, process.stdin)
                process.stdin.close()
            except BaseException:
                process.stdin.close()
                process.terminate()
                process.wait()
                raise
            if process.wait() != 0:
                raise ValueError("restore command failed")


def assertions(client, specification):
    if not isinstance(specification, dict) or not specification:
        raise ValueError("nonempty database validation specification required")
    results = {}
    for database, queries in specification.items():
        if not queries:
            raise ValueError("missing database assertions")
        results[database] = []
        for query in queries:
            sql = query["sql"]
            if not query.get("name") or not re.match(r"\s*SELECT\b", sql, re.I) or "\\" in sql:
                raise ValueError("named SELECT assertion required; no psql commands")
            # Source assertions cannot mutate the original database.
            output = client.sql(database, "BEGIN READ ONLY;\n" + sql.rstrip(";\n ") + ";\nCOMMIT;\n")
            results[database].append({"name": query["name"], "sql": sql,
                                      "expected_sha256": hashlib.sha256(output.encode()).hexdigest()})
    return results


def record(client, archive, specification, output):
    archive = Path(archive)
    if not archive.is_file() or archive.stat().st_size == 0:
        raise ValueError("missing/empty backup artifact")
    with gzip.open(archive, "rb") as stream:
        while stream.read(1024 * 1024):
            pass  # Verify gzip CRC/truncation before certifying the archive.
    version = int(client.sql("template1", "SHOW server_version_num").strip()) // 10000
    checks = assertions(client, specification)
    # Require explicit validation for every non-template source database.
    databases = client.sql("template1", "SELECT datname FROM pg_database WHERE datallowconn AND NOT datistemplate ORDER BY datname").splitlines()
    if set(checks) != set(databases):
        raise ValueError("validation must cover every non-template source database")
    manifest = {"schema": 1, "archive_sha256": sha256(archive), "database_major": version,
                "checks": checks, "created_at": dt.datetime.now(dt.timezone.utc).isoformat()}
    Path(output).write_text(json.dumps(manifest, sort_keys=True, indent=2) + "\n")
    return manifest


def verify(client, archive, manifest):
    if manifest.get("schema") != 1 or sha256(archive) != manifest["archive_sha256"]:
        raise ValueError("backup artifact digest/schema mismatch")
    version = int(client.sql("template1", "SHOW server_version_num").strip()) // 10000
    if version != manifest["database_major"]:
        raise ValueError("restore database version mismatch")
    client.restore(archive)
    specification = {database: [{"name": q["name"], "sql": q["sql"]} for q in queries]
                     for database, queries in manifest["checks"].items()}
    actual = assertions(client, specification)
    if actual != manifest["checks"]:
        raise ValueError("restored database validation mismatch")
    return {"validated": True, "commands_succeeded": True, "backup_sha256": sha256(archive),
            "validation_sha256": hashlib.sha256(json.dumps(actual, sort_keys=True).encode()).hexdigest(),
            "source_database_version": manifest["database_major"], "restore_database_version": version,
            "validation_summary": {db: [q["name"] for q in checks] for db, checks in actual.items()},
            "completed_at": dt.datetime.now(dt.timezone.utc).isoformat()}


def sandbox(archive, manifest_path, output):
    manifest = json.loads(Path(manifest_path).read_text())
    # Validate inputs BEFORE creating any container. Digest also checked in verify.
    if sha256(archive) != manifest["archive_sha256"] or not isinstance(manifest["database_major"], int):
        raise ValueError("invalid backup manifest")
    name = "karrygo-restore-test-" + uuid.uuid4().hex
    user = "restore_bootstrap_" + uuid.uuid4().hex[:12]
    image = f"postgres:{manifest['database_major']}-alpine"
    created = False
    try:
        run(["docker", "run", "-d", "--name", name, "--network", "none", "--read-only",
             "--memory", "512m", "--cpus", "1", "--pids-limit", "256",
             "--tmpfs", "/var/lib/postgresql/data:rw", "--tmpfs", "/var/run/postgresql:rw", "--tmpfs", "/tmp:rw",
             "-e", "PGDATA=/var/lib/postgresql/data/data", "-e", "POSTGRES_USER=" + user,
             "-e", "POSTGRES_PASSWORD=" + uuid.uuid4().hex, "-e", "POSTGRES_DB=restore_bootstrap", image])
        created = True
        for attempt in range(60):
            # The image's temporary init server accepts Unix sockets before it
            # shuts down. Only TCP readiness proves the final server is running.
            if subprocess.run(["docker", "exec", name, "pg_isready", "-h", "127.0.0.1", "-U", user, "-d", "restore_bootstrap"],
                              stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
                break
            time.sleep(1)
        else:
            raise ValueError("restore database readiness timeout")
        evidence = verify(Docker(name, user), archive, manifest)
        evidence["artifact_location"] = str(Path(archive).resolve())
        # This evidence alone does not attest live runtime compatibility or grant
        # release approval. The separate reviewed certification is mandatory.
    finally:
        if created:
            run(["docker", "rm", "-f", "-v", name])  # Cleanup failure is a failure too.
    Path(output).write_text(json.dumps(evidence, sort_keys=True, indent=2) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    commands = parser.add_subparsers(dest="command", required=True)
    capture = commands.add_parser("record")
    capture.add_argument("--container", required=True)
    capture.add_argument("--user", required=True)
    capture.add_argument("--queries", required=True)
    for command in (capture, commands.add_parser("verify")):
        command.add_argument("--archive", required=True)
        command.add_argument("--manifest", required=True)
        if command.prog.endswith("verify"):
            command.add_argument("--output", required=True)
    args = parser.parse_args()
    if args.command == "record":
        record(Docker(args.container, args.user), args.archive, json.loads(Path(args.queries).read_text()), args.manifest)
    else:
        sandbox(args.archive, args.manifest, args.output)
