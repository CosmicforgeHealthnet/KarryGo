"""Offline self-tests for the Customer V4 certification tooling.

Run: python -m unittest discover -s scripts/customer-v4 -p "test_*.py"
"""
import json
import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import monitoring_check as mc
import pgbackup


class PromQLSubsetParser(unittest.TestCase):
    def setUp(self):
        self.metrics = {"karry_x_total": {"authority", "outcome"}, "karry_gauge": set(), "up": set()}

    def problems(self, expr):
        out = []
        mc.check_expr(expr, self.metrics, "t", out)
        return out

    def test_accepts_the_rule_idioms(self):
        for expr in [
            'sum by (authority) (rate(karry_x_total{outcome="stale"}[15m])) / clamp_min(sum by (authority) (rate(karry_x_total[15m])), 0.001) > 0.2',
            "(sum(rate(karry_x_total[10m])) or vector(0)) / clamp_min(sum(rate(karry_x_total[10m])) or vector(0), 0.001) > 0.25 and (sum(rate(karry_x_total[10m])) or vector(0)) > 0.05",
            "sum(karry_gauge) < 0.3 * sum(karry_gauge offset 1h) and sum(karry_gauge offset 1h) > 50",
            'max by (authority) (karry_x_total{outcome=~"a|b"}) > 3600',
            "delta(karry_gauge[1h]) > 25",
        ]:
            self.assertEqual(self.problems(expr), [], expr)

    def test_rejects_malformed_and_unsafe_expressions(self):
        cases = {
            "sum(rate(karry_x_total[5m])": "parse error",
            "karry_x_total[5m] > 1": "range selector",
            "rate(karry_x_total) > 1": "range vector",
            "histogram_quantile(0.9, karry_x_total) > 1": "parse error",
            "karry_unknown_total > 1": "not emitted",
            'karry_x_total{customer_id="c"} > 1': "not a bounded vocabulary",
            'karry_x_total{family="f"} > 1': "carries no label family",
            "sum by (booking_ref) (karry_x_total) > 1": "grouping label",
            "1 > 0": "not a vector",
            "karry_x_total or 1": "vector operands",
        }
        for expr, expected in cases.items():
            found = " | ".join(self.problems(expr))
            self.assertIn(expected, found, expr)

    def test_repository_rules_and_dashboard_validate(self):
        with mock.patch("sys.stdout"):
            self.assertEqual(mc.main(), 0)


class BackupToolSafety(unittest.TestCase):
    def test_connection_parsing_never_exposes_the_password(self):
        conn = pgbackup.Conn("postgres://svc%40x:p%21ss@db.internal:6543/customer_service?sslmode=require", None)
        self.assertEqual((conn.user, conn.password, conn.host, conn.port, conn.database, conn.sslmode),
                         ("svc@x", "p!ss", "db.internal", "6543", "customer_service", "require"))
        argv = conn._argv("psql", ["-c", "SELECT 1"])
        self.assertNotIn("p!ss", " ".join(argv))
        docker = pgbackup.Conn("postgres://u:secretvalue@localhost:5432/db", "pg")._argv("pg_dump", [])
        self.assertNotIn("secretvalue", " ".join(docker))
        self.assertIn("PGPASSWORD", docker)  # forwarded by name, not value

    def test_error_text_is_scrubbed(self):
        text = pgbackup.scrub(b'connection to postgres://u:hunter2@h/db failed: password authentication failed for user "u" password=hunter2')
        self.assertNotIn("hunter2", text)

    def test_restore_refusals(self):
        with tempfile.TemporaryDirectory() as tmp:
            dump = Path(tmp, "svc.dump")
            dump.write_bytes(b"PGDMP-fake")
            meta = {"dump_file": "svc.dump", "dump_sha256": pgbackup.sha256_file(dump), "database": "customer_service",
                    "source_host_fingerprint": pgbackup.Conn("postgres://u:p@h:5432/customer_service", None).host_fingerprint(),
                    "service": "customer-service", "snapshot": {}}
            meta_path = Path(tmp, "svc.meta.json")
            meta_path.write_text(json.dumps(meta))

            def restore(url, *extra, env=None):
                with mock.patch.dict(os.environ, {"TARGET": url, **(env or {})}, clear=False):
                    return pgbackup.main(["restore", "--metadata", str(meta_path), "--url-env", "TARGET", *extra])

            # restoring onto the database the backup came from
            self.assertEqual(restore("postgres://u:p@h:5432/customer_service", "--replace", "--confirm-target", "customer_service"), 2)
            # production-looking target without an approval reference
            self.assertEqual(restore("postgres://u:p@h:5432/customer_prod"), 2)
            self.assertEqual(restore("postgres://u:p@other:5432/restore_drill", env={"APP_ENV": "production"}), 2)
            # --replace without repeating the target name
            self.assertEqual(restore("postgres://u:p@other:5432/restore_drill", "--replace"), 2)
            # an altered artefact
            dump.write_bytes(b"PGDMP-tampered")
            self.assertEqual(restore("postgres://u:p@other:5432/restore_drill"), 2)

    def test_missing_url_is_a_refusal(self):
        with mock.patch.dict(os.environ, {}, clear=True):
            self.assertEqual(pgbackup.main(["backup", "--service", "x", "--output-dir", tempfile.gettempdir(), "--url-env", "NOPE"]), 2)

    def test_check_constraint_deparse_is_normalized_but_real_changes_are_not(self):
        before = "public.t.c:CHECK (status::text = ANY (ARRAY['active'::character varying, 'suspended'::character varying]::text[]))"
        restored = "public.t.c:CHECK (status::text = ANY (ARRAY['active'::character varying::text, 'suspended'::character varying::text]))"
        changed = "public.t.c:CHECK (status::text = ANY (ARRAY['active'::character varying::text, 'closed'::character varying::text]))"
        self.assertEqual(pgbackup.canonical("constraints", before), pgbackup.canonical("constraints", restored))
        self.assertNotEqual(pgbackup.canonical("constraints", before), pgbackup.canonical("constraints", changed))
        # Only CHECK constraints are canonicalized; a foreign key is compared verbatim.
        fk = "public.t.fk:FOREIGN KEY (a) REFERENCES b(id)::text"
        self.assertEqual(pgbackup.canonical("constraints", fk), fk)

    def test_migrations_order_by_number_not_spelling(self):
        with tempfile.TemporaryDirectory() as tmp:
            for name in ("001_media_assets.sql", "000005_evidence.up.sql", "000005_evidence.down.sql", "003_auth_down.sql"):
                Path(tmp, name).write_text("")
            self.assertEqual(pgbackup.repository_tip_file(tmp), "000005_evidence.up.sql")
            self.assertEqual(pgbackup.migration_files_tip(tmp), "000005_evidence")

    def test_migration_ledgers_are_discovered_by_name(self):
        tables = ["public.taxi_schema_migrations", "public.vendor_service_schema_migrations", "public.schema_migrations",
                  "public.migration_notes", "public.bookings"]
        self.assertEqual(pgbackup.migration_tables(tables), tables[:3])


class CertificationWorkflow(unittest.TestCase):
    """B22: the CI definition is validated locally; hosted execution needs a push."""

    ROOT = Path(__file__).resolve().parents[2]
    PATH = ROOT / ".github" / "workflows" / "customer-v4-certification.yml"

    def setUp(self):
        import yaml

        self.text = self.PATH.read_text(encoding="utf-8")
        self.doc = yaml.safe_load(self.text)

    def test_structure_and_least_privilege(self):
        triggers = self.doc.get("on", self.doc.get(True))  # PyYAML reads the bare key `on` as True
        self.assertIn("pull_request", triggers)
        self.assertEqual(self.doc["permissions"], {"contents": "read"})
        job = self.doc["jobs"]["customer-v4-certification"]
        self.assertEqual(job["services"]["postgres"]["image"], "postgres:16")
        self.assertIn("redis", job["services"])
        self.assertTrue(any(s.get("if") == "always()" and "upload-artifact" in s.get("uses", "") for s in job["steps"]))

    def test_no_secrets_deploys_or_pushes(self):
        lowered = self.text.lower()
        for forbidden in ("secrets.", "vps_", "ssh ", "scp ", "docker push", "git push", "kubectl", "ghcr.io", "deploy-"):
            self.assertNotIn(forbidden, lowered, forbidden)

    def test_actions_are_version_pinned_and_steps_exist(self):
        import cert

        steps = self.doc["jobs"]["customer-v4-certification"]["steps"]
        for step in steps:
            if "uses" in step:
                self.assertRegex(step["uses"], r"^[\w.-]+/[\w.-]+@v\d+$")
        choices = {"migrate", "backup-drill", "gates", "routes", "secrets", "monitoring", "manifest", "all"}
        called = set()
        for step in steps:
            for match in __import__("re").finditer(r"cert\.py (\S+)", step.get("run", "")):
                called.add(match.group(1))
        self.assertTrue(called <= choices, called - choices)
        self.assertTrue({"monitoring", "secrets", "backup-drill", "gates", "routes", "manifest"} <= called)
        for path in ("scripts/customer-v4/cert.py", "deploy/monitoring/customer-v4/alerts.rules.yml"):
            self.assertTrue((self.ROOT / path).exists(), path)
        self.assertTrue(cert.GATES)

    def test_run_blocks_are_valid_bash(self):
        import shutil
        import subprocess

        bash = shutil.which("bash")
        if not bash:
            self.fail("bash is required to validate the workflow's run blocks")
        for step in self.doc["jobs"]["customer-v4-certification"]["steps"]:
            if "run" in step:
                result = subprocess.run([bash, "-n"], input=step["run"], capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, f"{step.get('name')}: {result.stderr}")


if __name__ == "__main__":
    unittest.main()
