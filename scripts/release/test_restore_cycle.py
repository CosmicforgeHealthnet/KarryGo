"""Real pg_dumpall/gzip/strict psql cycle. Only disposable clusters/containers.

Linux CI uses Docker. Windows uses isolated native PostgreSQL 18 clusters on
random loopback ports, never the workstation's installed database service.
"""
import copy
import gzip
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import time
import unittest
import uuid

import restore


class Native:
    def __init__(self, directory, user):
        self.bin = Path(os.environ.get("RELEASE_TEST_PG_BIN", "C:/Program Files/PostgreSQL/18/bin"))
        self.directory = Path(directory)
        self.user = user
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0)); self.port = sock.getsockname()[1]
        self.flags = {"creationflags": subprocess.CREATE_NO_WINDOW} if os.name == "nt" else {}
        self.call([self.bin / "initdb", "-D", directory, "-U", user, "--auth=trust", "--encoding=UTF8", "--no-locale"])
        # A Windows postmaster can inherit captured pipe handles from pg_ctl;
        # do not wait for pipe EOF from the long-running disposable server.
        subprocess.run([str(self.bin / "pg_ctl"), "-D", directory, "-l", str(self.directory / "test.log"),
                        "-o", f"-h 127.0.0.1 -p {self.port}", "-w", "start"], check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, **self.flags)
        if user == "restore_bootstrap":
            self.sql("postgres", "CREATE DATABASE restore_bootstrap")

    def call(self, args, **kwargs):
        try:
            return subprocess.run([str(a) for a in args], check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, **self.flags, **kwargs)
        except subprocess.CalledProcessError as error:
            error.add_note(error.stderr.decode(errors="replace") if isinstance(error.stderr, bytes) else error.stderr)
            raise

    def sql(self, database, query):
        return self.call([self.bin / "psql", "-X", "-q", "-A", "-t", "-v", "ON_ERROR_STOP=1",
                          "-h", "127.0.0.1", "-p", str(self.port), "-U", self.user, "-d", database], input=query, text=True).stdout

    def dump(self):
        return self.call([self.bin / "pg_dumpall", "--clean", "--if-exists", "-h", "127.0.0.1", "-p", str(self.port), "-U", self.user]).stdout

    def restore(self, archive):
        with gzip.open(archive, "rb") as stream:
            self.sql("restore_bootstrap", stream.read().decode())

    def close(self):
        self.call([self.bin / "pg_ctl", "-D", self.directory, "-m", "fast", "-w", "stop"])


class Container(restore.Docker):
    def __init__(self, user):
        self.name = "karrygo-release-cycle-" + uuid.uuid4().hex
        self.user = user
        subprocess.run(["docker", "run", "-d", "--name", self.name, "--network", "none",
                        "-e", "POSTGRES_USER=" + user, "-e", "POSTGRES_PASSWORD=disposable_only",
                        "-e", "POSTGRES_DB=" + ("restore_bootstrap" if user == "restore_bootstrap" else "postgres"),
                        "postgres:16-alpine"], check=True, capture_output=True)
        super().__init__(self.name, user)
        for _ in range(60):
            if subprocess.run(["docker", "exec", self.name, "pg_isready", "-h", "127.0.0.1", "-U", user, "-d", "postgres"], capture_output=True).returncode == 0:
                return
            time.sleep(1)
        self.close()
        raise RuntimeError("disposable source readiness timeout")

    def dump(self):
        return subprocess.check_output([*self.prefix, "pg_dumpall", "--clean", "--if-exists", "-U", self.user])

    def close(self):
        subprocess.run(["docker", "rm", "-f", "-v", self.name], check=True, capture_output=True)


class GenuineRestore(unittest.TestCase):
    def test_backup_restore_and_failure_paths(self):
        with tempfile.TemporaryDirectory(prefix="karrygo-release-test-") as directory:
            root = Path(directory)
            source = Native(str(root / "source"), "source_admin") if os.name == "nt" else Container("source_admin")
            target = None
            try:
                target = Native(str(root / "target"), "restore_bootstrap") if os.name == "nt" else Container("restore_bootstrap")
                source.sql("postgres", "CREATE DATABASE release_fixture;")
                source.sql("release_fixture", """
                    CREATE TABLE schema_migrations(filename text PRIMARY KEY);
                    INSERT INTO schema_migrations VALUES('070_uncertainty_fence');
                    CREATE TABLE decisions(id int PRIMARY KEY, original_key text NOT NULL, readback_required bool NOT NULL);
                    INSERT INTO decisions VALUES(1,'original-identity',true);
                    CREATE TABLE audit(id int PRIMARY KEY, digest text NOT NULL);
                    INSERT INTO audit VALUES(1,'immutable-receipt');
                    CREATE FUNCTION no_audit_change() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'immutable'; END $$;
                    CREATE TRIGGER immutable BEFORE UPDATE OR DELETE ON audit FOR EACH ROW EXECUTE FUNCTION no_audit_change();
                """)
                specification = {"postgres": [{"name": "source database owner", "sql": "SELECT current_database()"}],
                                 "release_fixture": [
                                     {"name": "migration ledger", "sql": "SELECT filename FROM schema_migrations ORDER BY filename"},
                                     {"name": "original identity and fence", "sql": "SELECT id,original_key,readback_required FROM decisions ORDER BY id"},
                                     {"name": "immutable receipt rows", "sql": "SELECT id,digest FROM audit ORDER BY id"},
                                     {"name": "audit trigger exists", "sql": "SELECT tgname FROM pg_trigger WHERE tgname='immutable'"}]}
                archive = root / "backup.sql.gz"
                with gzip.open(archive, "wb") as stream: stream.write(source.dump())
                manifest = restore.record(source, archive, specification, root / "manifest.json")
                evidence = restore.verify(target, archive, manifest)
                self.assertTrue(evidence["validated"])
                if os.name != "nt":
                    # Exercise the exact production-facing isolated-container
                    # verifier too, not only the shared record/verify core.
                    restore.sandbox(archive, root / "manifest.json", root / "sandbox-evidence.json")
                    self.assertTrue(json.loads((root / "sandbox-evidence.json").read_text())["validated"])
                with self.assertRaises(subprocess.CalledProcessError): target.sql("release_fixture", "DELETE FROM audit")
                target.close()
                target = Native(str(root / "target-invalid"), "restore_bootstrap") if os.name == "nt" else Container("restore_bootstrap")
                # Genuine missing relation during strict restore must fail.
                invalid = root / "invalid.sql.gz"
                with gzip.open(archive, "rb") as stream: sql = stream.read()
                with gzip.open(invalid, "wb") as stream: stream.write(sql + b'\nSELECT missing_restore_relation;\n')
                bad = copy.deepcopy(manifest); bad["archive_sha256"] = restore.sha256(invalid)
                with self.assertRaises((ValueError, subprocess.CalledProcessError)): restore.verify(target, invalid, bad)
                target.close()
                target = Native(str(root / "target-semantic"), "restore_bootstrap") if os.name == "nt" else Container("restore_bootstrap")
                # Successful psql is not enough: mismatched semantic evidence fails.
                bad = copy.deepcopy(manifest); bad["checks"]["release_fixture"][0]["expected_sha256"] = "0" * 64
                with self.assertRaises(ValueError): restore.verify(target, archive, bad)
                bad = copy.deepcopy(manifest); bad["database_major"] += 1
                with self.assertRaises(ValueError): restore.verify(target, archive, bad)
                with self.assertRaises(FileNotFoundError): restore.verify(target, root / "missing", manifest)
                truncated = root / "truncated.sql.gz"; truncated.write_bytes(archive.read_bytes()[:-20])
                with self.assertRaises((EOFError, OSError)): restore.record(source, truncated, specification, root / "must-not-exist.json")
                self.assertFalse((root / "must-not-exist.json").exists())
                with self.assertRaises(ValueError): restore.record(source, archive, {}, root / "must-not-exist.json")
                with self.assertRaises(ValueError): restore.verify(target, invalid, manifest)
                # Write only synthetic/disposable evidence for inspection; the
                # production gate explicitly rejects this environment label.
                output = Path(os.environ.get("RELEASE_TEST_EVIDENCE", ".codex-cache/release-restore-cycle.json"))
                output.parent.mkdir(parents=True, exist_ok=True)
                output.write_text(json.dumps({"environment": "disposable", "genuine_backup_restore": evidence,
                                             "negative_paths": ["strict SQL failure", "semantic mismatch", "version mismatch",
                                                                "missing archive", "truncated gzip", "missing validation", "digest mismatch"],
                                             "immutable_audit_preserved": True}, indent=2) + "\n")
            finally:
                if target: target.close()
                source.close()


if __name__ == "__main__":
    unittest.main()
