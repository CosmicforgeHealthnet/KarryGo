"""Acceptance tests for the reviewed Dispatch 070 transition.

scripts/platform/dispatch-070-transition.sh runs for real here, against a
stubbed `docker` that keeps simulated server state (containers, migration
state, relay rows) and records every mutating call. The tests prove the
ordering the procedure requires and that every failure either changes nothing
or holds without restarting the old image.

The workflow and request file are checked statically: the run is gated by a
separate reviewer in its own environment, refuses outside its window, and
carries the image by digest.
"""

import json
import os
import pathlib
import re
import shutil
import subprocess
import tempfile
import unittest

import yaml

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "platform" / "dispatch-070-transition.sh"
WORKFLOW = ROOT / ".github" / "workflows" / "dispatch-070-transition.yml"
REQUEST = ROOT / "deploy" / "platform" / "dispatch-070-request.txt"
PACKAGE = ROOT / "docs" / "deployment" / "dispatch-070-transition-package.md"
BASH = os.environ.get("TEST_BASH", "bash")

DIGEST = "sha256:" + "a" * 64
IMAGE = "ghcr.io/cosmicforgehealthnet/driver-dispatch-delivery-service@" + DIGEST
REVISION = "b" * 40
APP = "karrygo-dispatch-delivery-service"
DB = "karrygo-dispatch-delivery-db"
PENDING = [
    "000070_relay_uncertain_outcome_fence.up.sql", "000071_relay_readback_audit.up.sql",
    "000072_admin_authorization_context.up.sql", "000073_admin_command_ledger.up.sql",
    "000074_admin_interventions.up.sql", "000075_analytics_coverage_producer.up.sql",
    "000076_dispatch_unmatched_demand_fact.up.sql", "000077_dispatch_recovery_outcome_facts.up.sql",
    "000078_dispatch_return_is_part_of_original_booking.up.sql", "000079_organization_provider_links.up.sql",
]

# A stand-in for the docker CLI. State lives in $STUB_STATE (JSON); every
# mutating call is appended to state["events"].
STUB = r'''#!/usr/bin/env python3
import json, os, sys
path = os.environ["STUB_STATE"]
st = json.load(open(path))
args = sys.argv[1:]
out = []
def save():
    json.dump(st, open(path, "w"))
def done(code=0, text=None, err=None):
    if text is not None:
        sys.stdout.write(text if text.endswith("\n") or text == "" else text + "\n")
    if err:
        sys.stderr.write(err + "\n")
    save(); sys.exit(code)
def ev(*e):
    st["events"].append(" ".join(e))
C = st["containers"]
def field(c, fmt):
    if "if .State.Health" in fmt:
        return c.get("health", "healthy") if c["running"] else "exited"
    if "State.Running" in fmt: return "true" if c["running"] else "false"
    if "Config.Image" in fmt: return c["image"]
    if "RestartPolicy" in fmt: return c["restart"]
    if "State.Status" in fmt: return "running" if c["running"] else "exited"
    if "State.Health.Status" in fmt: return c.get("health", "healthy")
    if fmt == "{{.Image}}": return c["image_id"]
    return ""
def sql_answer(sql, restore=False):
    m = st["migrated"]
    if "karrygo_deploy_migrations) ||" in sql:
        return st["restore_counts"] if restore else st["source_counts"]
    if restore:
        return ""
    if "UPDATE dispatch_fleet_relay_readback_audit" in sql:
        if m and not st.get("audit_mutable"):
            done(3, "", "ERROR:  Relay read-back audit is append-only")
        done(0, "")
    if "device_command_ref" in sql: return "1"
    if "column_name = 'readback_required'" in sql: return "1" if m else "0"
    if "WHERE readback_required" in sql: return "0"
    if "dispatch_fleet_relay_readback_audit') IS NOT NULL" in sql: return "t" if m else "f"
    if "total=" in sql: return st["relay"]
    return ""
a = args
if a[:2] == ["compose", "version"]:
    done(0, "Docker Compose version v2")
if a and a[0] == "compose":
    envfile = a[a.index("--env-file") + 1]
    image = open(envfile).read().strip().split("=", 1)[1]
    if "run" in a:
        if "--status" in a:
            pend = [] if st["migrated"] else st["pending"]
            done(0, "".join("  PENDING  %s\n" % p for p in pend) + "migrate.sh: status")
        ev("migrate", image)
        if st.get("migrate_fails"):
            done(1, "", "migrate.sh: failed")
        st["migrated"] = True
        done(0, "migrate.sh: database is up to date")
    if "up" in a:
        ev("up", image)
        if APP_NAME in C:
            C.pop(APP_NAME)
        C[APP_NAME] = {"image": image, "image_id": st["images"].get(image, {}).get("id", "?"),
                       "running": True, "restart": "unless-stopped",
                       "health": "unhealthy" if st.get("new_unhealthy") else "healthy"}
        done(0, "")
if a[0] == "ps":
    done(0, "\n".join(C))
if a[0] == "container" and a[1] == "inspect":
    done(0 if a[2] in C else 1, "")
if a[0] == "inspect":
    name = a[-1]
    if name not in C: done(1, "", "no such container")
    done(0, field(C[name], a[2]))
if a[0] == "image" and a[1] == "inspect":
    img = st["images"].get(a[-1])
    if img is None:
        img = next((v for v in st["images"].values() if v["id"] == a[-1]), None)
    if img is None: done(1, "", "no such image")
    done(0, img["revision"] if "revision" in a[3] else img["id"])
if a[0] == "pull":
    done(0 if a[-1] in st["images"] else 1, "")
if a[0] == "update":
    pol = a[1].split("=", 1)[1]; name = a[2]
    ev("update", name, pol); C[name]["restart"] = pol; done(0, "")
if a[0] == "stop":
    name = a[-1]; ev("stop", name); C[name]["running"] = False; done(0, "")
if a[0] == "start":
    name = a[-1]; ev("start", name); C[name]["running"] = True; done(0, "")
if a[0] == "rename":
    ev("rename", a[1], a[2]); C[a[2]] = C.pop(a[1]); done(0, "")
if a[0] == "run":
    name = a[a.index("--name") + 1]; ev("restore-check-start", name)
    st["restore_container"] = name; done(0, "cid")
if a[0] == "rm":
    ev("rm", a[-1]); done(0, "")
if a[0] == "exec":
    target = a[2] if a[1] == "-i" else a[1]
    rest = " ".join(a)
    if "pg_dumpall" in rest:
        ev("dump"); done(0, "-- dump\n")
    if "echo" in rest and "POSTGRES_DB" in rest:
        done(0, "dispatch_delivery")
    if "pg_isready" in rest:
        done(0, "")
    sql = sys.stdin.read() if a[1] == "-i" else ""
    if target == st.get("restore_container"):
        done(0, sql_answer(sql, restore=True))
    done(0, sql_answer(sql))
done(2, "", "stub: unhandled " + " ".join(a))
'''


class Dispatch070TransitionScriptTest(unittest.TestCase):
    def setUp(self):
        self.tmp = pathlib.Path(tempfile.mkdtemp(prefix="dispatch070-"))
        self.addCleanup(shutil.rmtree, self.tmp, True)
        platform = self.tmp / "platform"
        shutil.copytree(ROOT / "deploy" / "platform" / "bin", platform / "bin")
        shutil.copytree(ROOT / "deploy" / "platform" / "compose", platform / "compose")
        shutil.copy(ROOT / "deploy" / "platform" / "services.tsv", platform / "services.tsv")
        self.platform = platform
        self.root = self.tmp / "opt"
        self.home = self.root / "dispatch-delivery-service"
        self.home.mkdir(parents=True)
        env = self.home / "dispatch.env"
        env.write_text("DISPATCH_DELIVERY_DATABASE_URL=postgres://x\n")
        env.chmod(0o600)
        (self.home / "release.env").write_text("IMAGE=ghcr.io/cosmicforgehealthnet/driver-dispatch-delivery-service:old\n")
        bindir = self.tmp / "stubbin"
        bindir.mkdir()
        stub = bindir / "docker"
        stub.write_text(STUB.replace("APP_NAME", repr(APP)))
        stub.chmod(0o755)
        self.path = f"{bindir}{os.pathsep}{os.environ.get('PATH', '')}"
        self.state_file = self.tmp / "state.json"
        self.state = {
            "events": [], "migrated": False, "pending": list(PENDING),
            "relay": "total=0 uncertain=0 live_leases=0",
            "source_counts": "72 0 150", "restore_counts": "72 0 150",
            "images": {IMAGE: {"id": "sha256:new", "revision": REVISION},
                       "ghcr.io/cosmicforgehealthnet/driver-dispatch-delivery-service:old": {"id": "sha256:old", "revision": "c" * 40}},
            "containers": {
                APP: {"image": "ghcr.io/cosmicforgehealthnet/driver-dispatch-delivery-service:old", "image_id": "sha256:old",
                      "running": True, "restart": "unless-stopped"},
                "karrygo-dispatch-delivery-service-before-email-20261016-090932": {
                    "image": "ghcr.io/cosmicforgehealthnet/driver-dispatch-delivery-service:latest", "image_id": "sha256:older",
                    "running": False, "restart": "unless-stopped"},
                "karrygo-dispatch-email-diagnostic": {
                    "image": "karrygo/driver-dispatch-delivery-service:dc7b", "image_id": "sha256:diag",
                    "running": False, "restart": "no"},
                DB: {"image": "postgres:16-alpine", "image_id": "sha256:pg", "running": True, "restart": "unless-stopped"},
            },
        }

    def run_transition(self, image=IMAGE, revision=REVISION, **overrides):
        self.state.update(overrides)
        self.state_file.write_text(json.dumps(self.state))
        env = dict(os.environ, PATH=self.path, STUB_STATE=str(self.state_file), KARRYGO_ROOT=str(self.root))
        result = subprocess.run([BASH, str(SCRIPT), str(self.platform), image, revision],
                                capture_output=True, text=True, env=env, timeout=300)
        self.final = json.loads(self.state_file.read_text())
        return result

    def events(self):
        return self.final["events"]

    def index(self, prefix):
        for i, e in enumerate(self.events()):
            if e.startswith(prefix):
                return i
        self.fail(f"no event starting {prefix!r} in {self.events()}")

    def mutations(self):
        return [e for e in self.events() if e.split()[0] in {"update", "stop", "start", "migrate", "up", "rename", "dump"}]

    def test_completes_in_the_required_order(self):
        r = self.run_transition()
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("DISPATCH_070 result=COMPLETED", r.stdout)
        # Every restart path is removed before the running consumer stops.
        self.assertLess(max(i for i, e in enumerate(self.events()) if e.startswith("update ")), self.index(f"stop {APP}"))
        self.assertLess(self.index(f"stop {APP}"), self.index("dump"))
        self.assertLess(self.index("dump"), self.index("restore-check-start"))
        self.assertLess(self.index("rm "), self.index("migrate "))
        self.assertLess(self.index("migrate "), self.index("up "))
        self.assertNotIn("start", [e.split()[0] for e in self.events()], "the old container must never be started")
        self.assertTrue(self.final["migrated"])
        c = self.final["containers"]
        self.assertEqual(c[APP]["image"], IMAGE)
        self.assertTrue(c[APP]["running"])
        others = [n for n in c if n not in (APP, DB)]
        self.assertTrue(any(n.startswith(APP + "-pre070-") for n in others))
        for n in others:
            self.assertFalse(c[n]["running"], n)
            self.assertEqual(c[n]["restart"], "no", n)
        self.assertEqual((self.home / "release.env").read_text().strip(), "IMAGE=" + IMAGE)
        self.assertIn(":old", (self.home / "release.previous.env").read_text())
        self.assertFalse((self.home / "release.next.env").exists())
        self.assertIn("readback_audit=append_only", r.stdout)
        backups = list((self.root / "backups" / "dispatch-070").glob("dispatch-*.sql.gz"))
        self.assertEqual(len(backups), 1)
        self.assertEqual(oct(backups[0].stat().st_mode & 0o777), "0o600")

    def assert_refused_unchanged(self, r):
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("result=REFUSED phase=preflight changed=nothing", r.stdout)
        self.assertEqual(self.mutations(), [])
        self.assertFalse((self.home / "release.next.env").exists())
        self.assertTrue(self.final["containers"][APP]["running"])

    def test_relay_rows_refuse_before_any_change(self):
        self.assert_refused_unchanged(self.run_transition(relay="total=1 uncertain=1 live_leases=0"))

    def test_unexpected_pending_set_refuses(self):
        self.assert_refused_unchanged(self.run_transition(pending=["000069_offline_location_atomic.up.sql"] + PENDING))

    def test_wrong_revision_refuses(self):
        self.assert_refused_unchanged(self.run_transition(revision="d" * 40))

    def test_unknown_running_consumer_refuses(self):
        self.state["containers"]["karrygo-dispatch-email-diagnostic"]["running"] = True
        self.assert_refused_unchanged(self.run_transition())

    def test_tag_instead_of_digest_refuses(self):
        r = self.run_transition(image="ghcr.io/cosmicforgehealthnet/driver-dispatch-delivery-service:8f992eee")
        self.assertNotEqual(r.returncode, 0)
        self.assertEqual(self.mutations(), [])

    def test_failed_restore_check_restores_previous_container(self):
        r = self.run_transition(restore_counts="70 0 149")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("previous_container=restored", r.stdout)
        self.assertNotIn("migrate", [e.split()[0] for e in self.events()])
        c = self.final["containers"][APP]
        self.assertTrue(c["running"])
        self.assertEqual(c["restart"], "unless-stopped")
        self.assertEqual(c["image_id"], "sha256:old")
        self.assertIn(f"rm karrygo-dispatch-070-restore-check-", " ".join(self.events()))

    def test_failed_migration_holds_without_old_image(self):
        r = self.run_transition(migrate_fails=True)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("result=HOLD phase=migrate", r.stdout)
        kinds = [e.split()[0] for e in self.events()]
        self.assertNotIn("start", kinds)
        self.assertNotIn("up", kinds)
        self.assertFalse(self.final["containers"][APP]["running"])

    def test_mutable_audit_holds(self):
        r = self.run_transition(audit_mutable=True)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("result=HOLD phase=migrate", r.stdout)
        self.assertNotIn("start", [e.split()[0] for e in self.events()])

    def test_unhealthy_new_version_holds_and_stops_it(self):
        r = self.run_transition(new_unhealthy=True)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("result=HOLD phase=start", r.stdout)
        self.assertNotIn("start", [e.split()[0] for e in self.events()])
        c = self.final["containers"][APP]
        self.assertEqual(c["image"], IMAGE)
        self.assertFalse(c["running"])
        self.assertEqual(c["restart"], "no")


class Dispatch070WorkflowTest(unittest.TestCase):
    def setUp(self):
        self.text = WORKFLOW.read_text()
        self.data = yaml.load(self.text, Loader=yaml.BaseLoader)
        self.request = dict(line.split("=", 1) for line in REQUEST.read_text().splitlines()
                            if line and not line.startswith("#"))

    def test_triggers_only_on_the_request_file_on_staging(self):
        self.assertEqual(set(self.data["on"]), {"push", "workflow_dispatch"})
        self.assertEqual(self.data["on"]["push"]["branches"], ["staging"])
        self.assertEqual(self.data["on"]["push"]["paths"], ["deploy/platform/dispatch-070-request.txt"])
        self.assertEqual(self.data["permissions"], {"contents": "read"})

    def test_separate_reviewer_gates_the_server_job(self):
        jobs = self.data["jobs"]
        self.assertNotIn("environment", jobs["plan"])
        self.assertEqual(jobs["review"]["environment"], "dispatch-070-transition")
        self.assertEqual(jobs["review"]["needs"], "plan")
        self.assertNotIn("secrets.", json.dumps(jobs["plan"]) + json.dumps(jobs["review"]))
        t = jobs["transition"]
        self.assertEqual(t["needs"], ["plan", "review"])
        self.assertEqual(t["environment"], "staging")
        self.assertEqual(t["concurrency"]["group"], "staging-release-runtime")
        body = json.dumps(t)
        self.assertIn("/approvals", body)
        self.assertIn("dispatch-070-transition", body)
        self.assertIn("STAGING_RELEASE_INITIATORS", body)
        self.assertIn("dispatch-070-transition.sh", body)
        for job in jobs.values():
            self.assertIn("refs/heads/staging", job["if"])

    def test_never_uses_the_routine_deploy_path(self):
        for forbidden in ("deploy-service.sh", "rollback.sh", ":latest", "gate.py"):
            self.assertNotIn(forbidden, self.text)

    def test_request_is_complete_and_pins_the_built_digest(self):
        r = self.request
        self.assertRegex(r["DIGEST"], r"^sha256:[0-9a-f]{64}$")
        self.assertRegex(r["REVISION"], r"^[0-9a-f]{40}$")
        self.assertIn(r["DIGEST"], PACKAGE.read_text(), "the requested digest must be the one recorded in the package")
        reviewers = r["REVIEWERS"].split(",")
        self.assertTrue(reviewers)
        self.assertNotIn(r["OPERATOR"], reviewers, "the operator cannot review their own transition")
        self.assertRegex(r["WINDOW_START"], r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$")
        self.assertRegex(r["WINDOW_END"], r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$")
        self.assertLess(r["WINDOW_START"], r["WINDOW_END"])

    def test_script_keeps_the_fence(self):
        s = SCRIPT.read_text()
        self.assertNotIn("down.sql", s.replace("no down migration", ""))
        self.assertNotRegex(s, re.compile(r"readback_required\s*=\s*false", re.I))
        self.assertNotIn("--baseline", s)


if __name__ == "__main__":
    unittest.main()
