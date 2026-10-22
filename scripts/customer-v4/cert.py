#!/usr/bin/env python3
"""Customer V4 backend certification runner (B21, B22, B24).

One entrypoint for local runs and CI. It reuses the services' OWN migration
runners and test suites and the route/secret/monitoring tools beside it; it
adds no second copy of their logic.

  migrate        apply each V4 service's migrations with its own runner onto a
                 fresh template database (migration verification)
  backup-drill   pgbackup.py backup -> refused unsafe restores -> replace ->
                 restore -> verify, for every V4 service at its migration tip
  gates          targeted V4 Go suites, go test -json; any FAIL, any SKIP, or a
                 required package that ran no test fails the gate
  routes         routeaudit.py all (capture, exposure, drift, register freshness)
  secrets        secretscan.py
  monitoring     monitoring_check.py + tooling self-tests
  manifest       write docs/customer-v4-release-manifest.json
  all            every step above, in order

Environment (no production secret is ever needed; use a disposable server):
  CERT_PG_URL         maintenance URL as seen from this host (…/postgres)
  CERT_PG_TOOLS_URL   same server as seen by the PostgreSQL client tools
                      (defaults to CERT_PG_URL)
  PGTOOLS_DOCKER_CONTAINER  run pg tools inside this container (optional)
  CERT_REDIS_ADDR     disposable Redis host:port
  CERT_OUT_DIR        evidence directory (default tmp/customer-v4-cert)
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import subprocess
import sys
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
sys.path.insert(0, str(HERE))
import pgbackup  # noqa: E402

OUT = Path(os.environ.get("CERT_OUT_DIR", ROOT / "tmp" / "customer-v4-cert"))
PREFIX = "v4cert_test"
MANIFEST = ROOT / "docs" / "customer-v4-release-manifest.json"

# Each service's own migration runner, exactly as its test suite uses it.
RUNNERS = {
    "customer-service": ("CLOSURE_MIGRATION_DATABASE_URL", ["go", "test", "./internal/database", "-run", "^TestClosureApplyOwningMigrations$", "-count=1", "-v"]),
    "driver-taxi-service": ("TAXI_TEST_DATABASE_URL", ["go", "test", "./internal/database", "-run", "^TestApplyMigrationsCleanAndReplay$", "-count=1", "-v"]),
    "driver-dispatch-delivery-service": ("DISPATCH_TEST_DATABASE_URL", ["go", "run", "./cmd/test-migrate"]),
    "driver-hauling-service": ("HAULING_TEST_DATABASE_URL", ["go", "test", "./internal/database", "-run", "^TestApplyMigrationsTracksAndReplays$", "-count=1", "-v"]),
    "vendor-service": ("CLOSURE_MIGRATION_DATABASE_URL", ["go", "test", "./internal/database", "-run", "^TestClosureApplyOwningMigrations$", "-count=1", "-v"]),
    "payment-wallet-service": ("PAYMENT_WALLET_TEST_DATABASE_URL", ["go", "test", "./internal/integration", "-run", "^TestMigrationsReplayAndRollback$", "-count=1", "-v"]),
    "media-file-service": ("MEDIA_FILE_TEST_DATABASE_URL", ["go", "test", "./internal/integration", "-run", "^TestMigrationsReplayAndRollback$", "-count=1", "-v"]),
    "support-dispute-service": ("SUPPORT_DISPUTE_TEST_DATABASE_URL", ["go", "test", "./internal/database", "-run", "^TestOrganizationCaseMigration$", "-count=1", "-v"]),
}
# Backed up and restored in the drill (Support is migrated only as a gate template).
DRILL_SERVICES = ["customer-service", "driver-taxi-service", "driver-dispatch-delivery-service", "driver-hauling-service",
                  "vendor-service", "payment-wallet-service", "media-file-service"]
MIGRATION_DIRS = {s: ROOT / "services" / s / "migrations" for s in RUNNERS}
# Files a service's deployable schema needs BEFORE its runner, in the order
# the service's own bootstrap applies them (dispatch-local-bootstrap.sh
# applies the legacy rider-auth files, then the 0000*.up.sql chain;
# cmd/test-migrate alone runs only the .up.sql chain).
PRE_SQL = {"driver-dispatch-delivery-service": ["001_dispatch_rider_auth.sql", "002_dispatch_rider_auth_alter.sql",
                                               "003_dispatch_rider_auth_signup.sql"]}
# Fixture rows proved identical after restore, beyond migration-seeded rows.
SEEDS = {"driver-dispatch-delivery-service": "seeds/dev_seed.sql", "driver-hauling-service": "seeds/dev_seed.sql"}

VENDOR_E2E = ("^(TestCommerceLifecycleE2E_|TestCustomerV4E2E07|TestCartDeliveryPreview|TestCustomerOrderMovement|"
              "TestCustomerCommerce14|TestSection17FWallet_|TestSection17FPayment_|TestSection17DQuote_MaterialCustomerChange|"
              "TestSection17FChange_AddressChange|TestSection17CLogistics_|TestPhysicalReplan_|TestRequirementEvidence_|"
              "TestCustomerDomainGate|TestDispatchAdapterRecovers|TestHaulingMissionUnknownCreate)")
VENDOR_ENV = {"VENDOR_SERVICE_TEST_DATABASE_URL": "tip:vendor-service", "VENDOR_SERVICE_TEST_REDIS_ADDR": "redis",
              "JOURNEY_TEST_REDIS_ADDR": "redis", "CUSTOMER14_TEST_REDIS_ADDR": "redis"}
# Targeted V4 gates. db: "tip:<service>" clones that service's migrated
# template, "empty" is a fresh database the suite migrates itself.
GATES = [
    {"name": "shared-go", "dir": "shared/go", "packages": "list:./...:!stacke2e",
     "env": {"JOURNEY_TEST_REDIS_ADDR": "redis", "MOVEMENT_TEST_DATABASE_URL": "empty", "TEST_DATABASE_URL": "empty",
             "POLICY_INGEST_TEST_DATABASE_URL": "empty", "CUSTODY_TEST_DATABASE_URL": "tip:driver-dispatch-delivery-service",
             "OUTBOX_TEST_DATABASE_URL": "empty", "ADMIN_CONTEXT_TEST_DATABASE_URL": "empty", "ADMIN_OWNER_TEST_DATABASE_URL": "empty",
             "ANALYTICS_ADMIN_TEST_DATABASE_URL": "empty"}},
    {"name": "customer", "dir": "services/customer-service", "packages": ["./..."],
     "env": {"CUSTOMER_TEST_DATABASE_URL": "tip:customer-service", "CLOSURE_MIGRATION_DATABASE_URL": "empty",
             "JOURNEY_TEST_REDIS_ADDR": "redis", "ADMIN_OWNER_TEST_DATABASE_URL": "empty"}},
    {"name": "taxi", "dir": "services/driver-taxi-service",
     "packages": ["./internal/features/booking/...", "./internal/features/customertracking/...", "./internal/capability/...",
                  "./internal/movement/...", "./internal/config/...", "./internal/features/adminresources"],
     "env": {"TAXI_TEST_DATABASE_URL": "tip:driver-taxi-service", "TAXI_TEST_REDIS_ADDR": "redis", "JOURNEY_TEST_REDIS_ADDR": "redis", "ADMIN_OWNER_TEST_DATABASE_URL": "empty"}},
    {"name": "dispatch", "dir": "services/driver-dispatch-delivery-service",
     "packages": ["./internal/features/booking/...", "./internal/features/customershare/...", "./internal/features/securehandover/...",
                  "./internal/features/movement/...", "./internal/features/supportcontext/...", "./migrations", "./internal/features/adminresources"],
     "env": {k: "tip:driver-dispatch-delivery-service" for k in (
         "DISPATCH_TEST_DATABASE_URL", "TEST_DATABASE_URL", "DISPATCH_TRIP_TEST_DATABASE_URL", "DISPATCH_REQUEST_TEST_DATABASE_URL",
         "DISPATCH_AVAILABILITY_TEST_DATABASE_URL", "DISPATCH_PROFILE_TEST_DATABASE_URL", "DISPATCH_VEHICLE_TEST_DATABASE_URL",
         "DISPATCH_VERIFICATION_TEST_DATABASE_URL", "DISPATCH_VERIFICATION_EVENT_TEST_DATABASE_URL")} | {"JOURNEY_TEST_REDIS_ADDR": "redis", "DISPATCH_ADMIN_TEST_REDIS_ADDR": "redis", "ADMIN_OWNER_TEST_DATABASE_URL": "empty"}},
    {"name": "hauling", "dir": "services/driver-hauling-service",
     "packages": ["./internal/features/booking/...", "./internal/features/customershare/...", "./internal/features/movement/...", "./internal/features/adminresources"],
     "env": {"HAULING_TEST_DATABASE_URL": "empty", "JOURNEY_TEST_REDIS_ADDR": "redis", "ADMIN_OWNER_TEST_DATABASE_URL": "empty"},
     # Hauling's booking suites migrate an empty database themselves; its
     # share adapter reads an already-migrated schema.
     "package_env": {"/internal/features/customershare": {"HAULING_TEST_DATABASE_URL": "tip:driver-hauling-service"}}},
    {"name": "vendor-e2e", "dir": "services/vendor-service", "packages": ["./cmd"], "run": VENDOR_E2E,
     "env": {"VENDOR_SERVICE_TEST_DATABASE_URL": "tip:vendor-service", "VENDOR_SERVICE_TEST_REDIS_ADDR": "redis", "JOURNEY_TEST_REDIS_ADDR": "redis", "CUSTOMER14_TEST_REDIS_ADDR": "redis"}},
    {"name": "vendor-packages", "dir": "services/vendor-service",
     "packages": ["./internal/features/cart", "./internal/features/customershare", "./internal/features/fulfilment",
                  "./internal/integrations/logistics", "./internal/crmcontract", "./internal/config"],
     "env": VENDOR_ENV},
    # Its two running-stack tests need all five services running and are
    # certified by the §§9-11 running-stack evidence, not by this per-build
    # gate. They are excluded by name, never counted as passing.
    {"name": "vendor-fulfilmentgroup", "dir": "services/vendor-service", "packages": ["./internal/features/fulfilmentgroup"],
     "env": VENDOR_ENV, "exclude": {"pattern": "^TestStack", "reason": "running-stack E2E (SECTIONS911_STACK_E2E=1 and five running services)"}},
    # These tests deliberately read a canonical paid order and a reserved
    # Dispatch mission reference instead of fabricating one; the real
    # commerce lifecycle E2E produces exactly that fixture first.
    {"name": "vendor-orders", "dir": "services/vendor-service", "packages": ["./internal/features/orders"], "env": VENDOR_ENV,
     "fixture": {"dir": "services/vendor-service", "command": ["go", "test", "./cmd", "-run", "^TestCommerceLifecycleE2E_ShopPerExecutor$", "-count=1"]}},
    # Bounded Wallet load tests are opt-in per build; the certification runs them.
    {"name": "wallet", "dir": "services/payment-wallet-service", "packages": ["./internal/..."],
     "env": {"PAYMENT_WALLET_TEST_DATABASE_URL": "tip:payment-wallet-service", "WALLET_TEST_DATABASE_URL": "tip:payment-wallet-service",
             "WALLET_LOAD_TEST": "value:1"}},
    {"name": "media", "dir": "services/media-file-service", "packages": ["./..."], "env": {"MEDIA_FILE_TEST_DATABASE_URL": "empty"}},
    {"name": "notification", "dir": "services/notification-service", "packages": ["./..."], "env": {"NOTIFICATION_TEST_DATABASE_URL": "empty"}},
    {"name": "support", "dir": "services/support-dispute-service", "packages": ["./internal/features/support/...", "./internal/database/..."],
     "env": {"SUPPORT_DISPUTE_TEST_DATABASE_URL": "tip:support-dispute-service"}},
]


def log(message: str) -> None:
    print(f"[cert] {message}", flush=True)


class Server:
    """Creates and drops disposable databases; never logs a credential."""

    def __init__(self):
        url = os.environ.get("CERT_PG_URL")
        if not url:
            raise SystemExit("CERT_PG_URL (maintenance URL of a DISPOSABLE PostgreSQL server) is required")
        self.host = pgbackup.Conn(url, None)
        self.tools = pgbackup.Conn(os.environ.get("CERT_PG_TOOLS_URL") or url, os.environ.get("PGTOOLS_DOCKER_CONTAINER") or None)
        self.redis = os.environ.get("CERT_REDIS_ADDR", "127.0.0.1:6379")
        self.created: list[str] = []

    def sql(self, statement: str, database: str | None = None) -> None:
        result = self.tools.run("psql", ["-X", "-q", "-v", "ON_ERROR_STOP=1", "-c", statement], database=database)
        if result.returncode:
            raise SystemExit(f"psql failed: {pgbackup.scrub(result.stderr)}")

    def create(self, name: str, template: str | None = None) -> str:
        self.drop(name)
        self.sql(f'CREATE DATABASE "{name}"' + (f' TEMPLATE "{template}"' if template else ""))
        self.created.append(name)
        return name

    def drop(self, name: str) -> None:
        self.tools.run("psql", ["-X", "-q", "-c", f"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='{name}' AND pid<>pg_backend_pid()"])
        self.sql(f'DROP DATABASE IF EXISTS "{name}"')

    def url(self, database: str) -> str:
        return self.host.url_for(database)

    def tools_url(self, database: str) -> str:
        return self.tools.url_for(database)

    def cleanup(self, keep: set[str] = frozenset()) -> None:
        for name in reversed(self.created):
            if name not in keep:
                self.drop(name)
        self.created = [n for n in self.created if n in keep]


def go_env(extra: dict) -> dict:
    env = os.environ.copy()
    env.update(extra)
    env.setdefault("GOFLAGS", "")
    return env


def template(service: str) -> str:
    return f"{PREFIX}_tip_{service.replace('-', '_')}"


# --------------------------------------------------------------------------
def step_migrate(server: Server, services=None) -> dict:
    results = {}
    for service in services or RUNNERS:
        env_key, command = RUNNERS[service]
        name = template(service)
        server.create(name)
        started = time.time()
        for file in PRE_SQL.get(service, []):
            with open(MIGRATION_DIRS[service] / file, "rb") as sql:
                applied = server.tools.run("psql", ["-X", "-q", "-v", "ON_ERROR_STOP=1"], stdin=sql, database=name)
            if applied.returncode:
                raise SystemExit(f"{service}: pre-runner migration {file} failed: {pgbackup.scrub(applied.stderr)}")
        run = subprocess.run(command, cwd=ROOT / "services" / service, env=go_env({env_key: server.url(name)}),
                             capture_output=True, text=True, encoding="utf-8", errors="replace")
        output = run.stdout + run.stderr
        (OUT / "migrate").mkdir(parents=True, exist_ok=True)
        (OUT / "migrate" / f"{service}.log").write_text(output, encoding="utf-8")
        skipped = "--- SKIP" in output or "no tests to run" in output
        if run.returncode or skipped:
            raise SystemExit(f"migration runner for {service} failed or skipped; see migrate/{service}.log")
        files = [p.name for p in MIGRATION_DIRS[service].iterdir() if p.name.endswith(".sql") and not p.name.endswith(".down.sql")]
        tip = pgbackup.repository_tip_file(MIGRATION_DIRS[service])
        results[service] = {"runner": " ".join(command[:3]), "repository_tip": tip, "files": len(files),
                            "seconds": round(time.time() - started, 1)}
        log(f"migrated {service} to {tip} ({len(files)} files)")
    return results


def step_backup_drill(server: Server, services=None) -> dict:
    results, backups = {}, OUT / "backups"
    backups.mkdir(parents=True, exist_ok=True)
    tool = [sys.executable, str(HERE / "pgbackup.py")]
    for service in services or DRILL_SERVICES:
        source = server.create(f"{PREFIX}_drill_src_{service.replace('-', '_')}", template(service))
        if service in SEEDS:
            with open(ROOT / "services" / service / SEEDS[service], "rb") as seed:
                applied = server.tools.run("psql", ["-X", "-q", "-v", "ON_ERROR_STOP=1"], stdin=seed, database=source)
            if applied.returncode:
                raise SystemExit(f"fixture seed failed for {service}: {pgbackup.scrub(applied.stderr)}")
        target = server.create(f"{PREFIX}_drill_dst_{service.replace('-', '_')}")
        env = dict(os.environ, BACKUP_DATABASE_URL=server.tools_url(source), RESTORE_TARGET_DATABASE_URL=server.tools_url(target),
                   PGTOOLS_DOCKER_CONTAINER=os.environ.get("PGTOOLS_DOCKER_CONTAINER", ""))

        def run(*args):
            return subprocess.run(tool + list(args), env=env, capture_output=True, text=True)

        record = {}
        backup = run("backup", "--service", service, "--output-dir", str(backups / service), "--require-quiet",
                     "--expect-migrations-dir", str(MIGRATION_DIRS[service]))
        if backup.returncode:
            raise SystemExit(f"{service}: backup failed:\n{backup.stdout[-1500:]}")
        paths = json.loads([line for line in backup.stdout.splitlines() if line.startswith("{")][-1])
        meta = json.loads(Path(paths["metadata"]).read_text(encoding="utf-8"))
        # 1. unsafe target: occupied, no --replace -> refused (exit 2)
        server.sql("CREATE TABLE occupied_marker(x int)", database=target)
        record["refuses_occupied_target"] = run("restore", "--metadata", paths["metadata"]).returncode == 2
        # 2. restore onto the backup's own source -> refused
        env_source = dict(env, RESTORE_TARGET_DATABASE_URL=server.tools_url(source))
        record["refuses_source_overwrite"] = subprocess.run(tool + ["restore", "--metadata", paths["metadata"], "--replace",
                                                                    "--confirm-target", source], env=env_source,
                                                           capture_output=True, text=True).returncode == 2
        # 3. replace the disposable target, restore, verify
        restore = run("restore", "--metadata", paths["metadata"], "--replace", "--confirm-target", target,
                      "--expect-migrations-dir", str(MIGRATION_DIRS[service]))
        reports = [line for line in restore.stdout.splitlines() if line.startswith("{")]
        verified = json.loads(reports[-1]) if reports else {}
        if restore.returncode:
            (OUT / "backups" / f"{service}.restore.log").write_text(restore.stdout + restore.stderr, encoding="utf-8")
        record["restore_exit"] = restore.returncode
        record["verified"] = restore.returncode == 0 and verified.get("verified") is True
        # 4. an altered artefact is refused
        dump = Path(paths["dump"])
        original = dump.read_bytes()
        dump.write_bytes(original[:-1] + bytes([original[-1] ^ 1]))
        record["refuses_tampered_dump"] = run("restore", "--metadata", paths["metadata"], "--replace", "--confirm-target", target).returncode == 2
        dump.write_bytes(original)
        # 5. a drifted database fails verification (exit 4)
        server.sql("CREATE TABLE drift_marker(x int)", database=target)
        record["detects_drift"] = run("verify", "--metadata", paths["metadata"]).returncode == 4
        record.update({k: verified.get(k) for k in ("tables", "constraints", "indexes", "triggers", "rows_digested", "migration_ledgers")})
        record["schema_fingerprint"] = meta["snapshot"]["schema_fingerprint"][:16]
        record["rows"] = sum(r["count"] for r in meta["snapshot"]["rows"].values())
        record["dump_bytes"] = meta["dump_bytes"]
        record["passed"] = all(record[k] for k in ("refuses_occupied_target", "refuses_source_overwrite", "verified",
                                                    "refuses_tampered_dump", "detects_drift"))
        results[service] = record
        log(f"drill {service}: {'PASS' if record['passed'] else 'FAIL'} tables={record['tables']} rows={record['rows']} "
            f"digested={record['rows_digested']} ledgers={record['migration_ledgers']}")
        server.drop(source)
        server.drop(target)
    return results


def resolve_env(spec: dict, server: Server, label: str) -> dict:
    """Build one package's environment. "redis" -> the disposable Redis,
    "value:X" -> the literal X, "empty" -> a fresh empty database,
    "tip:<service>" -> a fresh clone of that service's migrated template.
    Keys naming the same source share one database."""
    env, cache = {}, {}
    for key, source in spec.items():
        if source == "redis":
            env[key] = server.redis
            continue
        if source.startswith("value:"):
            env[key] = source.split(":", 1)[1]
            continue
        if source not in cache:
            name = f"{PREFIX}_gate_{re.sub(r'[^a-z0-9]+', '_', label.lower())[-40:]}_{len(cache)}"
            cache[source] = server.create(name, template(source.split(":", 1)[1]) if source.startswith("tip:") else None)
        env[key] = server.url(cache[source])
    return env


def packages_for(gate: dict) -> list[str]:
    """Expand a gate's patterns to import paths, so every package can run
    against its own fresh database (earlier packages' rows never leak into a
    later package's assertions)."""
    spec = gate["packages"]
    patterns, exclude = (spec, None) if isinstance(spec, list) else (spec.split(":")[1:2], spec.split(":")[2].lstrip("!"))
    listed = subprocess.run(["go", "list", *patterns], cwd=ROOT / gate["dir"], capture_output=True, text=True, check=True).stdout.split()
    return [p for p in listed if not exclude or exclude not in p]


def parse_events(stdout: str) -> dict:
    counts = {"passed": 0, "failed": [], "skipped": [], "package_failures": [], "packages": set()}
    for line in stdout.splitlines():
        try:
            event = json.loads(line)
        except ValueError:
            continue
        action, test, package = event.get("Action"), event.get("Test"), event.get("Package")
        if test and action == "pass" and "/" not in test:
            counts["passed"] += 1
            counts["packages"].add(package)
        elif test and action == "fail":
            counts["failed"].append(f"{package}.{test}")
        elif test and action == "skip":
            counts["skipped"].append(f"{package}.{test}")
        elif not test and action == "fail":
            counts["package_failures"].append(package)
    return counts


def step_gates(server: Server, only=None) -> dict:
    results = {}
    (OUT / "gates").mkdir(parents=True, exist_ok=True)
    for gate in GATES:
        if only and gate["name"] not in only:
            continue
        started, log_lines = time.time(), []
        totals = {"passed": 0, "failed": [], "skipped": [], "package_failures": [], "packages": set()}
        exit_codes = []
        for package in packages_for(gate):
            spec = dict(gate["env"])
            for suffix, override in gate.get("package_env", {}).items():
                if package.endswith(suffix):
                    spec.update(override)
            env = resolve_env(spec, server, f"{gate['name']}_{package.rsplit('/', 1)[-1]}")
            fixture = gate.get("fixture")
            if fixture:
                prepared = subprocess.run(fixture["command"], cwd=ROOT / fixture["dir"], env=go_env(env), capture_output=True,
                                          text=True, encoding="utf-8", errors="replace")
                if prepared.returncode:
                    totals["package_failures"].append(f"{package} (fixture: {' '.join(fixture['command'][2:5])})")
                    log_lines.append(prepared.stdout + prepared.stderr)
                    server.cleanup(keep={template(s) for s in RUNNERS})
                    continue
            command = ["go", "test", "-json", "-count=1", "-timeout", "40m"]
            if gate.get("run"):
                command += ["-run", gate["run"]]
            if gate.get("exclude"):
                command += ["-skip", gate["exclude"]["pattern"]]
            run = subprocess.run(command + [package], cwd=ROOT / gate["dir"], env=go_env(env), capture_output=True,
                                 text=True, encoding="utf-8", errors="replace")
            exit_codes.append(run.returncode)
            log_lines.append(run.stdout)
            counts = parse_events(run.stdout)
            for key in ("failed", "skipped", "package_failures"):
                totals[key] += counts[key]
            totals["passed"] += counts["passed"]
            totals["packages"] |= counts["packages"]
            if run.returncode and not counts["failed"] and not counts["package_failures"]:
                totals["package_failures"].append(package)
                log_lines.append(run.stderr)
            server.cleanup(keep={template(s) for s in RUNNERS})
        (OUT / "gates" / f"{gate['name']}.jsonl").write_text("\n".join(log_lines), encoding="utf-8")
        empty_run = bool(gate.get("run")) and totals["passed"] == 0
        ok = not totals["failed"] and not totals["skipped"] and not totals["package_failures"] and totals["passed"] > 0 and not empty_run
        results[gate["name"]] = {"passed": totals["passed"], "failed": totals["failed"][:20], "skipped": totals["skipped"][:40],
                                 "package_failures": totals["package_failures"][:10], "packages_with_tests": len(totals["packages"]),
                                 "excluded": gate.get("exclude"), "seconds": round(time.time() - started, 1), "ok": ok}
        log(f"gate {gate['name']}: {'PASS' if ok else 'FAIL'} passed={totals['passed']} failed={len(totals['failed'])} "
            f"skipped={len(totals['skipped'])} packages={len(totals['packages'])} ({results[gate['name']]['seconds']}s)")
        for name in (totals["failed"] + totals["skipped"] + totals["package_failures"])[:10]:
            log(f"   {name}")
    return results


def run_tool(script: str, *args) -> dict:
    run = subprocess.run([sys.executable, str(HERE / script), *args], capture_output=True, text=True, encoding="utf-8", errors="replace")
    (OUT / f"{Path(script).stem}.log").write_text(run.stdout + run.stderr, encoding="utf-8")
    return {"exit": run.returncode, "ok": run.returncode == 0}


def step_monitoring() -> dict:
    result = run_tool("monitoring_check.py")
    tests = subprocess.run([sys.executable, "-m", "unittest", "discover", "-s", str(HERE), "-p", "test_*.py"],
                           cwd=HERE, capture_output=True, text=True)
    (OUT / "tooling-tests.log").write_text(tests.stdout + tests.stderr, encoding="utf-8")
    result["tooling_tests_ok"] = tests.returncode == 0
    result["ok"] = result["ok"] and result["tooling_tests_ok"]
    return result


# --------------------------------------------------------------------------
# release manifest (B24)
# --------------------------------------------------------------------------
RELEASE_ROOTS = ("services/", "shared/go/", "deploy/", "scripts/customer-v4/", ".github/workflows/customer-v4-certification.yml",
                 "go.work", "go.work.sum")
DIGEST_EXCLUDED = {"docs/customer-v4-release-manifest.json"}
N_CONTRACTS = [
    ("N01", "customer-service", "POST", "/api/v1/customer/journey-shares", "USER_BEARER"),
    ("N02", "customer-service", "GET", "/api/v1/customer/journey-shares", "USER_BEARER"),
    ("N03", "customer-service", "GET", "/api/v1/customer/journey-shares/:shareId", "USER_BEARER"),
    ("N04", "customer-service", "POST", "/api/v1/customer/journey-shares/:shareId/revoke", "USER_BEARER"),
    ("N05", "customer-service", "POST", "/api/v1/customer/journey-shares/:shareId/rotate", "USER_BEARER"),
    ("N06", "customer-service", "GET", "/api/v1/customer/public/journey-shares/:token", "NO_AUTH_MIDDLEWARE"),
    ("N07", "driver-taxi-service", "GET", "/api/v1/taxi/customer/bookings/:id/location", "USER_BEARER"),
    ("N08", "driver-taxi-service", "GET", "/internal/taxi/v1/customer-share/bookings/:sourceRef", "SERVICE_HMAC"),
    ("N09", "driver-dispatch-delivery-service", "GET", "/internal/dispatch/v1/customer-share/trips/:sourceRef", "SERVICE_HMAC"),
    ("N10", "driver-hauling-service", "GET", "/internal/hauling/v1/customer-share/bookings/:sourceRef", "SERVICE_HMAC"),
    ("N11", "vendor-service", "GET", "/internal/vendor/v1/customer-share/missions/:sourceRef", "SERVICE_HMAC"),
    ("N12", "customer-service", "POST", "/api/v1/customer/media-upload-sessions", "USER_BEARER"),
]
FLAGS = ["CUSTOMER_SHARE_JOURNEY_ENABLED", "TAXI_CUSTOMER_LIVE_LOCATION_ENABLED", "CUSTOMER_MEDIA_UPLOAD_SESSION_ENABLED",
         "CUSTOMER_FOOD_ENABLED", "CUSTOMER_SHOP_ENABLED", "EXTERNAL_REFERENCE_REPRICING_ENABLED", "CUSTOMER_UNIFIED_MOVEMENT_HISTORY_ENABLED",
         "CUSTOMER_MOVEMENT_EVENTS_CONSUMER_ENABLED", "CUSTOMER_MOBILITY_PREFERENCES_ENABLED", "CUSTOMER_GUEST_TRAVELLERS_ENABLED",
         "CUSTOMER_TRUSTED_CONTACTS_ENABLED", "VENDOR_CUSTOMER_MOVEMENT_GROUPING_ENABLED", "TAXI_SERVICE_PRODUCTS_ENABLED",
         "TAXI_CUSTOMER_CONTEXT_ENABLED", "COMMERCE_BULK_RFQ_ENABLED", "DISPATCH_HANDOVER_CHALLENGE_ENABLED"]
B_STATUS = {
    **{f"B{n:02d}": "PASS_LOCAL" for n in range(0, 21)},
    "B21": "PASS_LOCAL", "B22": "PASS_LOCAL_HOSTED_CI_PENDING", "B23": "BLOCKED_EXTERNAL",
    "B24": "PREPARED_LOCALLY_FINAL_FREEZE_PENDING_IMMUTABLE_BUILD_AND_B23",
}


def git(*args) -> str:
    return subprocess.run(["git", *args], cwd=ROOT, capture_output=True, text=True, check=True).stdout


def sha256_normalized(path: Path) -> str:
    data = path.read_bytes()
    if b"\0" not in data[:8000]:
        data = data.replace(b"\r\n", b"\n")
    return hashlib.sha256(data).hexdigest()


def release_files() -> list[str]:
    listed = git("ls-files", "-z", "--cached", "--others", "--exclude-standard").split("\0")
    return sorted({p for p in listed if p and p.startswith(RELEASE_ROOTS) and p not in DIGEST_EXCLUDED and (ROOT / p).is_file()})


def flag_defaults() -> dict:
    sources = "\n".join(p.read_text(encoding="utf-8", errors="replace") for p in ROOT.glob("services/*/internal/config/*.go")
                        if not p.name.endswith("_test.go"))
    out = {}
    for flag in FLAGS:
        m = re.search(r'(?:getEnvBool|boolValue|envBool|getBool)\(\s*"' + flag + r'"\s*,\s*(true|false)\s*\)', sources)
        out[flag] = m.group(1) if m else "not-found"
    return out


def step_manifest(evidence: dict) -> dict:
    files = release_files()
    digest = hashlib.sha256()
    for path in files:
        digest.update(f"{path}\0{sha256_normalized(ROOT / path)}\n".encode())
    status = git("status", "--porcelain")
    routes_path = OUT / "routes" / "classification.json"
    routes = json.loads(routes_path.read_text(encoding="utf-8")) if routes_path.exists() else []
    mounted = {(r["service"], r["method"], r["path"]): r["cls"] for r in routes}
    n_contracts = []
    for ident, service, method, path, cls in N_CONTRACTS:
        found = mounted.get((service, method, path))
        n_contracts.append({"id": ident, "service": service, "method": method, "path": path, "auth": cls,
                            "mounted": found is not None, "auth_matches": found == cls})
    inventory = {}
    for r in routes:
        entry = inventory.setdefault(r["service"], {"routes": 0, "public": 0, "internal_service_hmac": 0})
        entry["routes"] += 1
        entry["public"] += int(r["exposed"])
        entry["internal_service_hmac"] += int(r["cls"] == "SERVICE_HMAC")
    openapi = {p: sha256_normalized(ROOT / p) for p in files if re.match(r"services/[^/]+/docs/openapi/.+\.json$", p)}
    route_intent = (ROOT / "shared/go/notifications/route_intent.go").read_text(encoding="utf-8")
    notifications = (ROOT / "shared/go/notifications/notifications.go").read_text(encoding="utf-8")
    sys.path.insert(0, str(HERE))
    import monitoring_check

    tips = {}
    for directory in sorted(ROOT.glob("services/*/migrations")):
        names = [p.name for p in directory.iterdir() if p.name.endswith(".sql") and not p.name.endswith(".down.sql")]
        if names:
            tips[directory.parent.name] = {"tip": pgbackup.repository_tip_file(directory), "files": len(names)}
    manifest = {
        "kind": "karryam-customer-v4-backend-release-manifest",
        "manifest_version": 1,
        "statement": "LOCAL RELEASE CANDIDATE. Not a production build ID, not a commit, not a deployed immutable build.",
        "local_head": git("rev-parse", "HEAD").strip(),
        "branch": git("rev-parse", "--abbrev-ref", "HEAD").strip(),
        "worktree": {"dirty": bool(status.strip()), "modified": sum(1 for l in status.splitlines() if not l.startswith("??")),
                     "untracked": sum(1 for l in status.splitlines() if l.startswith("??"))},
        "LOCAL_RELEASE_CANDIDATE_DIGEST": {
            "sha256": digest.hexdigest(), "files": len(files), "roots": list(RELEASE_ROOTS), "excluded": sorted(DIGEST_EXCLUDED),
            "method": "sha256 over sorted 'path\\0sha256(content with CRLF normalized to LF)\\n' for tracked + untracked, "
                      "non-ignored files under the roots; recompute with scripts/customer-v4/cert.py manifest"},
        "migration_tips": tips,
        "route_inventory": inventory,
        "route_evidence": {"classification": "tmp/customer-v4-cert/routes/classification.json (regenerated by routeaudit.py)",
                           "public_unauthenticated_reviewed": "scripts/customer-v4/public-unauthenticated-routes.reviewed.txt"},
        "openapi_fingerprints": openapi,
        "n_contracts": n_contracts,
        "feature_flags": {flag: {"default": default} for flag, default in flag_defaults().items()},
        "event_catalogue": {
            "movement_envelope": "shared/go/events.Envelope + MovementSummary (authorities TAXI, DISPATCH, HAULING, VENDOR_COMMERCE)",
            "vendor_domain_events": ["commerce.customer_order.movement", "commerce.scheduled_order.accepted", "commerce.rfq.quote_submitted", "commerce.rfq.accepted"],
            "notification_event_constants": sorted(set(re.findall(r'=\s*"([a-z_]+\.[a-z_.]+)"', notifications))),
        },
        "deep_link_intents": sorted(set(re.findall(r'target, reference = "([A-Z_]+)"', route_intent))),
        "service_allowlists": {
            "customer_context_caller_scopes_default": re.search(r'CUSTOMER_CONTEXT_CALLER_SCOPES", "([^"]+)"', (ROOT / "services/customer-service/internal/config/config.go").read_text(encoding="utf-8")).group(1),
            "metrics_scrape_callers": ["admin-backoffice-service", "analytics-service"],
            "movement_ingress": "customer-service caller->authority map (taxi, dispatch, hauling, vendor)",
            "share_authorities": "CUSTOMER_SHARE_{TAXI,DISPATCH,HAULING,VENDOR}_{URL,SECRET}",
        },
        "metrics_catalogue": sorted(monitoring_check.catalogue()),
        "workers_and_reconciliation": [
            "customer: share expiry sweep, share notification outbox", "taxi/dispatch/hauling: customer movement reconcilers",
            "vendor: purchase-row movement worker + reconciler, wallet/refund unknown-outcome read-back, mission read-back",
            "vendor (COMMERCE_BULK_RFQ_ENABLED): scheduled cutoff notifications, RFQ expiry", "notification: retry with backoff then one dead letter",
            "dispatch: handover challenge expiry", "media: orphan and grant maintenance"],
        "backup_restore_tooling": {"tool": pgbackup.TOOL_VERSION, "sha256": sha256_normalized(HERE / "pgbackup.py"),
                                   "doc": "scripts/customer-v4/README.md"},
        "monitoring_definitions": {p: sha256_normalized(ROOT / p) for p in ("deploy/monitoring/customer-v4/alerts.rules.yml",
                                                                           "deploy/monitoring/customer-v4/dashboard.json")},
        "rollback_notes": [
            "Roll back by feature flag, never by destructive schema rollback during an incident (V4 §31.2).",
            "Food/Shop off: new carts refused; existing carts, orders, payments, refunds and support stay readable.",
            "Share Journey off: creation, public read and revoke return typed 503; nothing leaks.",
            "Reconcile UNKNOWN money and mission states through read-back before any manual compensation.",
            "Restore a database only into a new database and cut over (pgbackup.py refuses in-place restores)."],
        "external_dependencies": ["staging hosts, credentials and allowlists (B23)", "immutable image build and digest", "Prometheus/Alertmanager/Grafana deployment",
                                  "staging PostgreSQL backups", "Paystack (Wallet)", "FCM and email providers (Notification)", "Firebase Storage (Media)",
                                  "External Reference System / Repricing adapter (absent, gated off)", "Google Maps/Firebase key restriction review"],
        "b_status": B_STATUS,
        "evidence": evidence,
    }
    MANIFEST.write_text(json.dumps(manifest, indent=1, sort_keys=False) + "\n", encoding="utf-8")
    problems = [c["id"] for c in n_contracts if routes and not (c["mounted"] and c["auth_matches"])]
    problems += [f for f, d in manifest["feature_flags"].items() if d["default"] != "false"]
    log(f"manifest written: digest {digest.hexdigest()[:16]}... over {len(files)} files; problems: {problems or 'none'}")
    return {"ok": not problems, "digest": digest.hexdigest(), "files": len(files), "problems": problems}


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("step", choices=["migrate", "backup-drill", "gates", "routes", "secrets", "monitoring", "manifest", "all"])
    parser.add_argument("--only", nargs="*", help="gates: run only these gate names")
    parser.add_argument("--keep-templates", action="store_true")
    args = parser.parse_args(argv)
    OUT.mkdir(parents=True, exist_ok=True)
    summary_path = OUT / "summary.json"
    summary = json.loads(summary_path.read_text(encoding="utf-8")) if summary_path.exists() else {}
    server = None
    try:
        steps = ["monitoring", "secrets", "migrate", "backup-drill", "gates", "routes", "manifest"] if args.step == "all" else [args.step]
        for step in steps:
            if step in ("migrate", "backup-drill", "gates") and server is None:
                server = Server()
                if step != "migrate" and "migrate" not in steps:
                    summary["migrate"] = step_migrate(server)
            if step == "migrate":
                summary["migrate"] = step_migrate(server)
            elif step == "backup-drill":
                summary["backup_drill"] = step_backup_drill(server)
            elif step == "gates":
                # --only reruns merge into the recorded results for the other gates.
                summary["gates"] = {**(summary.get("gates") if args.only else {}), **step_gates(server, args.only)}
            elif step == "routes":
                summary["routes"] = run_tool("routeaudit.py", "all")
            elif step == "secrets":
                summary["secrets"] = run_tool("secretscan.py")
            elif step == "monitoring":
                summary["monitoring"] = step_monitoring()
            elif step == "manifest":
                summary["manifest"] = step_manifest({k: v for k, v in summary.items() if k != "manifest"})
            summary_path.write_text(json.dumps(summary, indent=1, sort_keys=True), encoding="utf-8")
    finally:
        if server is not None:
            server.cleanup(keep={template(s) for s in RUNNERS} if args.keep_templates else set())
    failures = []
    for name, value in summary.items():
        if name == "backup_drill":
            failures += [f"backup-drill:{s}" for s, r in value.items() if not r.get("passed")]
        elif name == "gates":
            failures += [f"gate:{g}" for g, r in value.items() if not r.get("ok")]
        elif isinstance(value, dict) and value.get("ok") is False:
            failures.append(name)
    log("FAILURES: " + ", ".join(failures) if failures else "all requested steps passed")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
