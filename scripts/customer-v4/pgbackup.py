#!/usr/bin/env python3
"""Generic PostgreSQL backup / restore / verify for the Customer V4 services.

Nothing here is service-specific. The schema, the migration ledger and the
rows to compare are discovered from the database catalogue at run time, so
the same tool serves customer, taxi, dispatch, hauling, vendor, wallet and
media (and any other service-owned PostgreSQL database).

Credentials are never taken from the command line and never written out:

  * connection strings are read from environment variables whose NAMES are
    passed on the command line (defaults: BACKUP_DATABASE_URL for backup,
    RESTORE_TARGET_DATABASE_URL for restore);
  * the password reaches pg_dump/psql/pg_restore only through PGPASSWORD in
    the child environment (and `docker exec -e PGPASSWORD`, which forwards the
    caller's variable without putting the value on the command line);
  * logs and metadata name the database, never the user, password or host
    (a salted host fingerprint is kept so a restore onto the backup's own
    source can be refused).

PostgreSQL client tools either come from PATH or, with --docker-container
(or PGTOOLS_DOCKER_CONTAINER), run inside that container so their version
matches the server. In container mode the URL must be the one valid from
INSIDE the container.

Exit codes: 0 success, 2 usage or safety refusal, 3 dump/restore failure,
4 verification mismatch.
"""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import re
import subprocess
import sys
from pathlib import Path
from urllib.parse import unquote, urlsplit

TOOL_VERSION = "customer-v4-pgbackup/2"
EXIT_USAGE, EXIT_TOOL, EXIT_VERIFY = 2, 3, 4
# Row contents are digested for tables up to this size so migration-seeded and
# fixture rows are proven identical; larger tables are compared by count.
DEFAULT_DIGEST_ROW_LIMIT = 20000


class Refusal(Exception):
    """A safety rule refused the operation (exit 2)."""


class ToolFailure(Exception):
    """pg_dump / pg_restore / psql failed (exit 3)."""


def log(message: str) -> None:
    print(f"[pgbackup] {message}", flush=True)


# --------------------------------------------------------------------------
# connection handling (no credential ever leaves this block)
# --------------------------------------------------------------------------
class Conn:
    def __init__(self, url: str, container: str | None):
        if not url:
            raise Refusal("connection URL environment variable is empty")
        parts = urlsplit(url)
        if parts.scheme not in ("postgres", "postgresql"):
            raise Refusal("connection URL must be postgres:// or postgresql://")
        self.host = parts.hostname or "localhost"
        self.port = str(parts.port or 5432)
        self.user = unquote(parts.username or "")
        self.password = unquote(parts.password or "")
        self.database = unquote(parts.path.lstrip("/"))
        query = dict(p.split("=", 1) for p in parts.query.split("&") if "=" in p)
        self.sslmode = query.get("sslmode", "")
        if not self.database:
            raise Refusal("connection URL names no database")
        self.container = container

    def with_database(self, name: str) -> "Conn":
        clone = Conn.__new__(Conn)
        clone.__dict__.update(self.__dict__)
        clone.database = name
        return clone

    def url_for(self, database: str) -> str:
        """A URL for another database on this server, for handing to a child
        process's environment. Never logged."""
        from urllib.parse import quote
        auth = quote(self.user, safe="") + (":" + quote(self.password, safe="") if self.password else "")
        return f"postgres://{auth}@{self.host}:{self.port}/{quote(database, safe='')}?sslmode={self.sslmode or 'disable'}"

    def host_fingerprint(self) -> str:
        return hashlib.sha256(f"{self.host}:{self.port}".encode()).hexdigest()[:16]

    def _env(self) -> dict:
        env = os.environ.copy()
        env["PGPASSWORD"] = self.password
        env["PGCONNECT_TIMEOUT"] = "10"
        if self.sslmode:
            env["PGSSLMODE"] = self.sslmode
        return env

    def _argv(self, tool: str, extra: list[str], database: str | None = None) -> list[str]:
        base = [tool, "-h", self.host, "-p", self.port, "-U", self.user, "-d", database or self.database]
        if self.container:
            passthrough = ["-e", "PGPASSWORD", "-e", "PGCONNECT_TIMEOUT"] + (["-e", "PGSSLMODE"] if self.sslmode else [])
            return ["docker", "exec", "-i", *passthrough, self.container, *base, *extra]
        return base + extra

    def run(self, tool: str, extra: list[str], *, stdin=None, stdout=None, database: str | None = None,
            input_bytes: bytes | None = None) -> subprocess.CompletedProcess:
        argv = self._argv(tool, extra, database)
        try:
            return subprocess.run(argv, env=self._env(), stdin=None if input_bytes is not None else stdin, input=input_bytes,
                                  stdout=stdout if stdout is not None else subprocess.PIPE, stderr=subprocess.PIPE, check=False)
        except FileNotFoundError as exc:
            raise ToolFailure(f"{tool} is not available ({exc.filename}); install PostgreSQL client tools or use --docker-container") from exc
        except OSError as exc:
            raise ToolFailure(f"{tool} could not be started: {exc.strerror}") from exc

    def query(self, sql: str, database: str | None = None) -> list[list[str]]:
        # SQL goes through stdin: a catalogue query over hundreds of tables
        # exceeds the Windows command-line limit when passed with -c.
        result = self.run("psql", ["-X", "-A", "-t", "-q", "-v", "ON_ERROR_STOP=1", "-F", "\x1f", "-f", "-"],
                          database=database, input_bytes=sql.encode("utf-8"))
        if result.returncode != 0:
            raise ToolFailure(f"psql failed on {database or self.database}: {scrub(result.stderr)}")
        text = result.stdout.decode("utf-8", "replace")
        return [line.split("\x1f") for line in text.splitlines() if line != ""]

    def scalar(self, sql: str, database: str | None = None) -> str:
        rows = self.query(sql, database)
        return rows[0][0] if rows else ""


def scrub(stderr: bytes) -> str:
    """Tool error text, with anything URL- or password-shaped removed."""
    text = stderr.decode("utf-8", "replace").strip()
    text = re.sub(r"postgres(?:ql)?://\S+", "postgres://***", text)
    text = re.sub(r"(?i)password[^\s]*\s*\S+", "password ***", text)
    return text[-2000:]


def conn_from_env(name: str, container: str | None) -> Conn:
    value = os.environ.get(name, "")
    if not value:
        raise Refusal(f"environment variable {name} is not set")
    return Conn(value, container)


# --------------------------------------------------------------------------
# catalogue snapshot: everything verification compares
# --------------------------------------------------------------------------
USER_SCHEMAS = "n.nspname NOT IN ('pg_catalog','information_schema','pg_toast') AND n.nspname NOT LIKE 'pg_temp%' AND n.nspname NOT LIKE 'pg_toast_temp%'"

CATALOG_SQL = {
    "tables": f"""SELECT n.nspname||'.'||c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
        WHERE c.relkind IN ('r','p') AND {USER_SCHEMAS} ORDER BY 1""",
    "columns": f"""SELECT n.nspname||'.'||c.relname||'.'||a.attname||':'||format_type(a.atttypid,a.atttypmod)||':'||a.attnotnull||':'||COALESCE(pg_get_expr(d.adbin,d.adrelid),'')
        FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace
        LEFT JOIN pg_attrdef d ON d.adrelid=a.attrelid AND d.adnum=a.attnum
        WHERE c.relkind IN ('r','p','v','m') AND a.attnum>0 AND NOT a.attisdropped AND {USER_SCHEMAS} ORDER BY 1""",
    "constraints": f"""SELECT n.nspname||'.'||c.relname||'.'||k.conname||':'||pg_get_constraintdef(k.oid,true)
        FROM pg_constraint k JOIN pg_class c ON c.oid=k.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace
        WHERE {USER_SCHEMAS} ORDER BY 1""",
    "indexes": """SELECT schemaname||'.'||indexname||':'||indexdef FROM pg_indexes
        WHERE schemaname NOT IN ('pg_catalog','information_schema') ORDER BY 1""",
    "triggers": f"""SELECT n.nspname||'.'||c.relname||'.'||t.tgname||':'||pg_get_triggerdef(t.oid,true)
        FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace
        WHERE NOT t.tgisinternal AND {USER_SCHEMAS} ORDER BY 1""",
    "functions": f"""SELECT n.nspname||'.'||p.proname||'('||pg_get_function_identity_arguments(p.oid)||'):'||md5(pg_get_functiondef(p.oid))
        FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
        LEFT JOIN pg_depend d ON d.objid=p.oid AND d.deptype='e'
        WHERE {USER_SCHEMAS} AND p.prokind IN ('f','p') AND d.objid IS NULL ORDER BY 1""",
    "views": f"""SELECT n.nspname||'.'||c.relname||':'||md5(pg_get_viewdef(c.oid,true)) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
        WHERE c.relkind IN ('v','m') AND {USER_SCHEMAS} ORDER BY 1""",
    "sequences": f"""SELECT n.nspname||'.'||c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
        WHERE c.relkind='S' AND {USER_SCHEMAS} ORDER BY 1""",
    "extensions": "SELECT extname||':'||extversion FROM pg_extension ORDER BY 1",
}


def quote_ident(name: str) -> str:
    return '"' + name.replace('"', '""') + '"'


def qualified(table: str) -> str:
    schema, rel = table.split(".", 1)
    return f"{quote_ident(schema)}.{quote_ident(rel)}"


def migration_tables(tables: list[str]) -> list[str]:
    """Migration ledgers are discovered by name, not configured per service."""
    return [t for t in tables if re.search(r"(^|[._])(schema_migrations|migrations)$", t.split(".", 1)[1])
            or t.split(".", 1)[1].endswith("_schema_migrations")]


# PostgreSQL re-deparses some CHECK constraints differently after a dump and
# re-parse (e.g. `ARRAY['a'::character varying]::text[]` comes back as
# `ARRAY['a'::character varying::text]`). The constraint is identical; only
# the cast placement moved. For CHECK constraints alone, casts to this closed
# list of types are removed before fingerprinting, so a real change in
# values, columns or structure still fails verification.
_CHECK_CASTS = re.compile(r"::(?:character varying|character|text|integer|bigint|smallint|numeric|jsonb|json|uuid|boolean|"
                          r"timestamp with time zone|timestamp without time zone|date|double precision|real|bpchar|name)(?:\[\])?")


def canonical(kind: str, line: str) -> str:
    if kind == "constraints" and ":CHECK " in line:
        return _CHECK_CASTS.sub("", line)
    return line


def snapshot(conn: Conn, digest_row_limit: int) -> dict:
    catalog = {kind: [row[0] for row in conn.query(sql)] for kind, sql in CATALOG_SQL.items()}
    fingerprint = hashlib.sha256()
    for kind in sorted(catalog):
        for line in sorted(canonical(kind, line) for line in catalog[kind]):
            fingerprint.update(f"{kind}\t{line}\n".encode())
    tables = catalog["tables"]
    rows: dict[str, dict] = {}
    if tables:
        counts = conn.query(" UNION ALL ".join(f"SELECT '{t}', count(*) FROM {qualified(t)}" for t in tables))
        for name, count in counts:
            rows[name] = {"count": int(count)}
        small = [name for name in tables if rows[name]["count"] <= digest_row_limit]
        for start in range(0, len(small), 100):
            batch = small[start:start + 100]
            digests = conn.query(" UNION ALL ".join(
                f"SELECT '{name}', COALESCE(md5(string_agg(r::text, E'\\n' ORDER BY r::text)),'empty') FROM {qualified(name)} r"
                for name in batch))
            for name, value in digests:
                rows[name]["digest"] = value
    ledgers = {}
    for table in migration_tables(tables):
        entries = [row[0] for row in conn.query(f"SELECT r::text FROM {qualified(table)} r ORDER BY 1")]
        ledgers[table] = {"entries": len(entries), "digest": hashlib.sha256("\n".join(entries).encode()).hexdigest(),
                          "tip": max(entries, key=lambda e: (migration_number(e.lstrip("(")), e)) if entries else ""}
    deparse_normalized = sum(1 for line in catalog["constraints"] if canonical("constraints", line) != line)
    return {
        "server_version": conn.scalar("SHOW server_version"),
        "check_constraints_with_casts_normalized": deparse_normalized,
        "counts": {kind: len(values) for kind, values in catalog.items()},
        "schema_fingerprint": fingerprint.hexdigest(),
        "migration_ledgers": ledgers,
        "rows": rows,
        "digest_row_limit": digest_row_limit,
    }


def compare(expected: dict, actual: dict) -> list[str]:
    problems = []
    if expected["schema_fingerprint"] != actual["schema_fingerprint"]:
        problems.append("schema fingerprint differs")
    for kind, count in expected["counts"].items():
        if actual["counts"].get(kind) != count:
            problems.append(f"{kind}: expected {count}, restored {actual['counts'].get(kind)}")
    if expected["migration_ledgers"] != actual["migration_ledgers"]:
        problems.append("migration ledger differs")
    for table, want in expected["rows"].items():
        got = actual["rows"].get(table)
        if got is None:
            problems.append(f"table missing after restore: {table}")
        elif got.get("count") != want.get("count"):
            problems.append(f"row count {table}: expected {want['count']}, restored {got.get('count')}")
        elif "digest" in want and got.get("digest") != want["digest"]:
            problems.append(f"row content differs: {table}")
    return problems


def migration_number(name: str) -> int:
    """Services mix 3- and 6-digit prefixes (001_x.sql beside 000066_y.up.sql),
    so migrations are ordered by their leading number, never by spelling."""
    match = re.match(r"\(?\s*\"?(\d+)", name)
    return int(match.group(1)) if match else -1


def repository_tip_file(directory) -> str:
    files = [p.name for p in Path(directory).iterdir()
             if p.is_file() and p.name.endswith(".sql") and not p.name.endswith(".down.sql")]
    if not files:
        raise Refusal(f"no migration files in {directory}")
    # Highest number wins; an .up.sql beats a same-numbered companion file.
    return max(files, key=lambda n: (migration_number(n), n.endswith(".up.sql"), n))


def migration_files_tip(directory: str) -> str:
    return re.sub(r"(\.up)?\.sql$", "", repository_tip_file(directory))


def check_migration_tip(meta: dict, directory: str | None) -> list[str]:
    if not directory:
        return []
    tip = migration_files_tip(directory)
    ledgers = meta["migration_ledgers"]
    if not ledgers:
        # Services whose runner re-applies every file keep no ledger; their
        # schema fingerprint is the version evidence.
        return []
    if not any(tip in ledger["tip"] for ledger in ledgers.values()):
        return [f"repository migration tip {tip} is not the applied tip ({', '.join(l['tip'][:60] for l in ledgers.values())})"]
    return []


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1 << 20), b""):
            digest.update(chunk)
    return digest.hexdigest()


def build_identity(explicit: str | None) -> str:
    if explicit:
        return explicit
    if os.environ.get("BUILD_ID"):
        return os.environ["BUILD_ID"]
    try:
        head = subprocess.run(["git", "rev-parse", "HEAD"], capture_output=True, text=True, check=True).stdout.strip()
        dirty = subprocess.run(["git", "status", "--porcelain"], capture_output=True, text=True, check=True).stdout.strip()
        return head + ("+dirty" if dirty else "")
    except Exception:
        return "unknown"


# --------------------------------------------------------------------------
# commands
# --------------------------------------------------------------------------
def cmd_backup(args) -> int:
    source = conn_from_env(args.url_env, args.docker_container)
    out_dir = Path(args.output_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    try:
        os.chmod(out_dir, 0o700)
    except OSError:
        pass
    stamp = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    base = out_dir / f"{args.service}-{stamp}"
    dump, meta_path = base.with_suffix(".dump"), base.with_suffix(".meta.json")
    log(f"backing up database '{source.database}' for {args.service}")
    before = snapshot(source, args.digest_row_limit)
    with dump.open("wb") as handle:
        result = source.run("pg_dump", ["-Fc", "--no-owner", "--no-privileges"], stdout=handle)
    if result.returncode != 0:
        dump.unlink(missing_ok=True)
        raise ToolFailure(f"pg_dump failed: {scrub(result.stderr)}")
    after = snapshot(source, args.digest_row_limit)
    if before["schema_fingerprint"] != after["schema_fingerprint"] or before["migration_ledgers"] != after["migration_ledgers"]:
        dump.unlink(missing_ok=True)
        raise ToolFailure("schema or migration ledger changed while the backup ran; retry in a quiet window")
    # Row counts may legitimately move on a live database; the drill runs on a
    # quiet source, where the stricter check below applies.
    if args.require_quiet and before["rows"] != after["rows"]:
        dump.unlink(missing_ok=True)
        raise ToolFailure("rows changed during the backup of a source declared quiet")
    meta = {
        "tool": TOOL_VERSION,
        "service": args.service,
        "database": source.database,
        "source_host_fingerprint": source.host_fingerprint(),
        "created_at_utc": stamp,
        "build": build_identity(args.build_id),
        "format": "postgresql-custom",
        "dump_file": dump.name,
        "dump_sha256": sha256_file(dump),
        "dump_bytes": dump.stat().st_size,
        "snapshot": before,
    }
    violations = check_migration_tip(before, args.expect_migrations_dir)
    if violations:
        dump.unlink(missing_ok=True)
        raise Refusal("; ".join(violations))
    meta_path.write_text(json.dumps(meta, indent=1, sort_keys=True) + "\n", encoding="utf-8")
    log(f"backup written: {dump.name} ({meta['dump_bytes']} bytes) sha256={meta['dump_sha256'][:16]}...")
    log(f"schema fingerprint {before['schema_fingerprint'][:16]}..., {before['counts']['tables']} tables, ledgers {list(before['migration_ledgers'])}")
    print(json.dumps({"dump": str(dump), "metadata": str(meta_path)}))
    return 0


PRODUCTION_HINT = re.compile(r"(?i)(^|[_-])(prod|production|live)([_-]|$)")


def cmd_restore(args) -> int:
    meta_path = Path(args.metadata)
    meta = json.loads(meta_path.read_text(encoding="utf-8"))
    dump = meta_path.parent / meta["dump_file"]
    target = conn_from_env(args.url_env, args.docker_container)
    if not dump.is_file():
        raise Refusal(f"dump file {dump.name} is missing next to the metadata")
    if sha256_file(dump) != meta["dump_sha256"]:
        raise Refusal("dump checksum does not match its metadata; refusing to restore a corrupted or altered artefact")
    environment = (os.environ.get("APP_ENV", "") + " " + os.environ.get("KARRY_ENVIRONMENT", "")).lower()
    if PRODUCTION_HINT.search(target.database) or "production" in environment:
        if not (args.allow_production and os.environ.get("RESTORE_CHANGE_APPROVAL_REF")):
            raise Refusal("target looks like production; production restores need --allow-production and an approved "
                          "RESTORE_CHANGE_APPROVAL_REF, and are an operator procedure, not an automated one")
    if target.host_fingerprint() == meta["source_host_fingerprint"] and target.database == meta["database"]:
        raise Refusal("target is the database this backup was taken from; restore into a new database and cut over instead")
    if args.replace:
        if args.confirm_target != target.database:
            raise Refusal("--replace drops the target database; repeat its name with --confirm-target")
        maintenance = args.maintenance_database
        target.run("psql", ["-X", "-q", "-v", "ON_ERROR_STOP=1", "-c",
                            f"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='{target.database}' AND pid<>pg_backend_pid()"],
                   database=maintenance)
        for sql in (f"DROP DATABASE IF EXISTS {quote_ident(target.database)}", f"CREATE DATABASE {quote_ident(target.database)}"):
            result = target.run("psql", ["-X", "-q", "-v", "ON_ERROR_STOP=1", "-c", sql], database=maintenance)
            if result.returncode != 0:
                raise ToolFailure(f"could not replace target database: {scrub(result.stderr)}")
        log(f"target database '{target.database}' recreated")
    else:
        exists = target.run("psql", ["-X", "-A", "-t", "-c", "SELECT 1"])
        if exists.returncode != 0:
            raise Refusal(f"target database '{target.database}' is not reachable or does not exist: {scrub(exists.stderr)}")
        occupied = int(target.scalar(f"SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind IN ('r','p') AND {USER_SCHEMAS}") or 0)
        if occupied:
            raise Refusal(f"target database '{target.database}' already holds {occupied} tables; use an empty database or --replace")
    log(f"restoring {dump.name} into '{target.database}'")
    with dump.open("rb") as handle:
        result = target.run("pg_restore", ["--no-owner", "--no-privileges", "--exit-on-error", "--single-transaction"], stdin=handle)
    if result.returncode != 0:
        raise ToolFailure(f"pg_restore failed: {scrub(result.stderr)}")
    return verify(target, meta, args)


def verify(target: Conn, meta: dict, args) -> int:
    actual = snapshot(target, meta["snapshot"].get("digest_row_limit", DEFAULT_DIGEST_ROW_LIMIT))
    problems = compare(meta["snapshot"], actual) + check_migration_tip(actual, getattr(args, "expect_migrations_dir", None))
    report = {"service": meta["service"], "target_database": target.database, "verified": not problems,
              "tables": actual["counts"]["tables"], "constraints": actual["counts"]["constraints"],
              "indexes": actual["counts"]["indexes"], "triggers": actual["counts"]["triggers"],
              "migration_ledgers": {k: v["tip"] for k, v in actual["migration_ledgers"].items()},
              "rows_digested": sum(1 for r in actual["rows"].values() if "digest" in r),
              "problems": problems}
    print(json.dumps(report, sort_keys=True))
    if problems:
        log("VERIFY FAIL: " + "; ".join(problems[:20]))
        return EXIT_VERIFY
    log("VERIFY PASS")
    return 0


def cmd_verify(args) -> int:
    meta = json.loads(Path(args.metadata).read_text(encoding="utf-8"))
    return verify(conn_from_env(args.url_env, args.docker_container), meta, args)


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--docker-container", default=os.environ.get("PGTOOLS_DOCKER_CONTAINER") or None,
                        help="run PostgreSQL client tools inside this container")
    sub = parser.add_subparsers(dest="command", required=True)

    backup = sub.add_parser("backup", help="dump a database with metadata and checksum")
    backup.add_argument("--service", required=True)
    backup.add_argument("--output-dir", required=True)
    backup.add_argument("--url-env", default="BACKUP_DATABASE_URL", help="name of the env var holding the source URL")
    backup.add_argument("--build-id", help="release/build identifier recorded in metadata (default: BUILD_ID or git HEAD)")
    backup.add_argument("--expect-migrations-dir", help="refuse unless the applied ledger tip matches this directory's newest migration")
    backup.add_argument("--digest-row-limit", type=int, default=DEFAULT_DIGEST_ROW_LIMIT)
    backup.add_argument("--require-quiet", action="store_true", help="fail if any row changes while dumping")
    backup.set_defaults(func=cmd_backup)

    restore = sub.add_parser("restore", help="restore into an explicitly named target and verify it")
    restore.add_argument("--metadata", required=True, help="the .meta.json written by backup")
    restore.add_argument("--url-env", default="RESTORE_TARGET_DATABASE_URL", help="name of the env var holding the target URL")
    restore.add_argument("--replace", action="store_true", help="drop and recreate the target database first")
    restore.add_argument("--confirm-target", default="", help="required with --replace: the target database name")
    restore.add_argument("--maintenance-database", default="postgres")
    restore.add_argument("--allow-production", action="store_true")
    restore.add_argument("--expect-migrations-dir")
    restore.set_defaults(func=cmd_restore)

    check = sub.add_parser("verify", help="compare an existing database with a backup's metadata")
    check.add_argument("--metadata", required=True)
    check.add_argument("--url-env", default="RESTORE_TARGET_DATABASE_URL")
    check.add_argument("--expect-migrations-dir")
    check.set_defaults(func=cmd_verify)

    args = parser.parse_args(argv)
    try:
        return args.func(args)
    except Refusal as exc:
        log(f"REFUSED: {exc}")
        return EXIT_USAGE
    except ToolFailure as exc:
        log(f"FAILED: {exc}")
        return EXIT_TOOL


if __name__ == "__main__":
    sys.exit(main())
