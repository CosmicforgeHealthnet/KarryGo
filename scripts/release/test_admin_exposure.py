"""Section 20/29 exposure invariants for the deployment artifacts.

Static, offline checks over the files operators install. Nothing here applies a
firewall rule, starts a container or contacts a host; the one optional
`nft --check` dry parse runs only where nft is installed and the test runs as
root (it never commits a ruleset).
"""
import importlib.util
import ipaddress
import os
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[2]
NGINX_CONFS = (
    ROOT / "deploy/platform/nginx/api-gateway.staging.karryam.com.conf",
    ROOT / "deploy/customer-public-ingress.locations.conf",
)
ADMIN_COMPOSE = ROOT / "deploy/platform/compose/admin-backoffice-service.yml"
CLOSE_PORTS = ROOT / "deploy/platform/bin/close-public-ports.sh"
EXPOSURE_SCRIPTS = sorted((ROOT / "scripts/admin-bridge/exposure").glob("*.sh"))
INTERNAL_PORTS = {8080, 8110, 5432, 6379, *range(8101, 8117), *range(5433, 5452), *range(6380, 6392)}
APPROVED_BINDS = {"127.0.0.1", "${ADMIN_PRIVATE_BIND_IP:-127.0.0.1}"}
NEVER_BLOCK = (22, 80, 443)

spec = importlib.util.spec_from_file_location("admin_policy", ROOT / "deploy/platform/network/admin_policy.py")
policy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(policy)


def _locations(text):
    """(start offset, modifier, pattern, body) for each location block."""
    found = []
    for match in re.finditer(r"location\s+(=|~\*|~|\^~)?\s*(\S+)\s*\{", text):
        depth, index = 1, match.end()
        while depth:
            depth += {"{": 1, "}": -1}.get(text[index], 0)
            index += 1
        found.append((match.start(), match.group(1) or "", match.group(2), text[match.end():index - 1]))
    return found


def _ranges(specs):
    for spec in specs:
        low, _, high = spec.partition(":")
        yield int(low), int(high or low)


def _covered(port, specs):
    return any(low <= port <= high for low, high in _ranges(specs))


def _compose_files():
    return sorted(set(ROOT.glob("deploy/**/*compose*.yml")) | set((ROOT / "deploy/platform/compose").glob("*.yml")))


class NginxDeniesBeforeProxy(unittest.TestCase):
    def test_admin_and_internal_denied_case_insensitively_before_any_proxy(self):
        for conf in NGINX_CONFS:
            text = re.sub(r"#[^\n]*", "", conf.read_text())
            locations = _locations(text)
            first_proxy = min(start for start, _, _, body in locations if "proxy_pass" in body)
            for required in (r"^/api/v1/admin(/|$)", r"^/internal(/|$)", r"(^|/)(internal|\.\.)(/|$)"):
                with self.subTest(conf=conf.name, deny=required):
                    match = [loc for loc in locations if loc[2] == required]
                    self.assertEqual(len(match), 1, "deny location missing")
                    start, modifier, _, body = match[0]
                    self.assertEqual(modifier, "~*", "deny must be case-insensitive")
                    self.assertEqual(body.strip(), "return 404;")
                    self.assertLess(start, first_proxy, "deny must precede every proxy location")
            # No prefix location may use ^~, which would skip the regex denies.
            self.assertFalse([loc for loc in locations if loc[1] == "^~"], conf.name)

    def test_deny_patterns_match_the_attack_spellings(self):
        admin = re.compile(r"^/api/v1/admin(/|$)", re.I)
        internal = re.compile(r"^/internal(/|$)", re.I)
        anywhere = re.compile(r"(^|/)(internal|\.\.)(/|$)", re.I)
        for path in ("/api/v1/admin", "/API/V1/ADMIN/x", "/api/v1/admin/", "/Api/V1/Admin"):
            self.assertTrue(admin.search(path), path)
        for path in ("/internal", "/INTERNAL/x", "/internal/"):
            self.assertTrue(internal.search(path), path)
        for path in ("/api/v1/taxi/internal/x", "/api/v1/../admin"):
            self.assertTrue(anywhere.search(path), path)
        self.assertFalse(admin.search("/api/v1/administrator"))


class AdminComposeHardening(unittest.TestCase):
    def setUp(self):
        self.compose = yaml.safe_load(ADMIN_COMPOSE.read_text())
        self.services = self.compose["services"]

    def test_private_listener_topology(self):
        app, networks = self.services["app"], self.compose["networks"]
        self.assertEqual(app["ports"], ["${ADMIN_PRIVATE_BIND_IP:-127.0.0.1}:8110:8110"])
        self.assertEqual(app["networks"], ["admin-ingress", "admin-private"])
        self.assertIs(networks["admin-private"]["internal"], True)
        ingress = networks["admin-ingress"]
        # Docker refuses to publish from an internal-only container; the
        # ingress network is non-internal but inert.
        self.assertIs(ingress["internal"], False)
        opts = ingress["driver_opts"]
        self.assertEqual(opts["com.docker.network.bridge.enable_ip_masquerade"], "false")
        self.assertEqual(opts["com.docker.network.bridge.enable_icc"], "false")
        self.assertEqual(opts["com.docker.network.bridge.host_binding_ipv4"], "${ADMIN_PRIVATE_BIND_IP:-127.0.0.1}")
        for name in ("db", "migrate"):
            self.assertEqual(self.services[name]["networks"], ["admin-private"], name)
            self.assertNotIn("ports", self.services[name], name)
            self.assertNotIn("expose", self.services[name], name)
        self.assertNotIn("karrygo-net", str(self.compose))

    def test_every_service_is_hardened(self):
        for name, service in self.services.items():
            with self.subTest(service=name):
                self.assertIs(service["read_only"], True)
                self.assertEqual(service["cap_drop"], ["ALL"])
                self.assertIn("no-new-privileges:true", service["security_opt"])
                self.assertIn("apparmor=docker-default", service["security_opt"])
                self.assertFalse([o for o in service["security_opt"] if "unconfined" in o])
                self.assertNotIn("privileged", service)
                self.assertNotIn("network_mode", service)
                self.assertNotIn("pid", service)
                for key in ("pids_limit", "mem_limit", "cpus"):
                    self.assertIn(key, service)
                self.assertFalse([v for v in service.get("volumes", []) if "docker.sock" in str(v)])

    def test_app_identity_health_and_restart(self):
        app = self.services["app"]
        uid, _, gid = app["user"].partition(":")
        self.assertTrue(uid.isdigit() and gid.isdigit() and int(uid) > 0 and int(gid) > 0)
        self.assertEqual(self.services["migrate"]["user"], app["user"])
        self.assertEqual(app["healthcheck"]["test"], ["CMD", "/bin/cosmicforge-logistics-service", "--probe"])
        self.assertEqual(app["restart"], "unless-stopped")
        self.assertTrue(all(v.endswith(":ro") for v in app["volumes"]))
        # The image's user is created by the shared Dockerfile; keep them aligned.
        dockerfile = (ROOT / "services/Dockerfile").read_text()
        self.assertIn("adduser -S cosmicforge -G cosmicforge", dockerfile)
        self.assertIn("USER cosmicforge", dockerfile)

    def test_db_capabilities_are_minimal(self):
        db = self.services["db"]
        self.assertEqual(sorted(db["cap_add"]), ["CHOWN", "DAC_OVERRIDE", "FOWNER", "SETGID", "SETUID"])
        self.assertEqual(db["restart"], "unless-stopped")
        self.assertIn("healthcheck", db)


class NoComposePublishesInternalPortsPublicly(unittest.TestCase):
    def test_every_published_port_has_an_approved_private_host_address(self):
        files = _compose_files()
        self.assertIn(ADMIN_COMPOSE, files)
        for path in files:
            document = yaml.safe_load(path.read_text()) or {}
            for name, service in (document.get("services") or {}).items():
                self.assertNotEqual(service.get("network_mode"), "host", f"{path.name}:{name}")
                for entry in service.get("ports", []):
                    with self.subTest(file=path.name, service=name, port=entry):
                        if isinstance(entry, dict):
                            host_ip = entry.get("host_ip", "")
                            published = str(entry.get("published", ""))
                        else:
                            # "ip:host:container" with ${VAR:-default} IPs kept whole.
                            parts = re.findall(r"\$\{[^}]*\}|[^:]+", str(entry))
                            self.assertEqual(len(parts), 3, "host address must be explicit")
                            host_ip, published = parts[0], parts[1]
                        self.assertIn(host_ip, APPROVED_BINDS)
                        default = re.search(r":-([^}]*)\}$", published)
                        port = int(default.group(1) if default else published)
                        if port in INTERNAL_PORTS:
                            self.assertEqual(host_ip.replace("${ADMIN_PRIVATE_BIND_IP:-127.0.0.1}", "127.0.0.1"), "127.0.0.1")

    def test_wildcard_forms_are_rejected_by_the_parser(self):
        for entry in ("8110:8110", "0.0.0.0:8110:8110", "[::]:5432:5432", "5432"):
            parts = re.findall(r"\$\{[^}]*\}|[^:]+", entry)
            self.assertFalse(len(parts) == 3 and parts[0] in APPROVED_BINDS, entry)


class ClosePublicPorts(unittest.TestCase):
    def run_script(self, *args, env=None):
        return subprocess.run(["bash", str(CLOSE_PORTS), *args], capture_output=True, text=True, env=env, timeout=30)

    def test_covers_admin_and_internal_ports_but_never_management(self):
        listed = self.run_script("--list-ports").stdout.split()
        for port in (8110, 5432, 6379, 4100, 4101, 4103, *range(8101, 8117)):
            self.assertTrue(_covered(port, listed), port)
        for port in NEVER_BLOCK:
            self.assertFalse(_covered(port, listed), port)
        self.assertEqual(tuple(listed), policy.PROTECTED_PORTS, "shell and nft candidate lists diverged")

    def test_default_is_a_dry_run_that_touches_no_firewall(self):
        with tempfile.TemporaryDirectory() as stub:
            marker = Path(stub) / "called"
            for tool in ("iptables", "ip6tables", "systemctl", "nft", "ufw"):
                fake = Path(stub) / tool
                fake.write_text(f"#!/bin/sh\necho {tool} >> {marker}\n")
                fake.chmod(0o755)
            env = {**os.environ, "PATH": f"{stub}:{os.environ.get('PATH', '/usr/bin:/bin')}"}
            result = self.run_script(env=env)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertTrue(result.stdout.startswith("DRY RUN"))
            self.assertIn("--ctdir ORIGINAL", result.stdout)
            self.assertFalse(marker.exists(), "dry run invoked a firewall tool")
            self.assertEqual(self.run_script("--bogus").returncode, 2)

    def test_rules_are_inbound_only_and_idempotent(self):
        text = CLOSE_PORTS.read_text()
        self.assertIn('-i $ext_if -p tcp -m conntrack --ctorigdstport $1 --ctdir ORIGINAL', text)
        self.assertNotRegex(text, r"-[AI] (OUTPUT|INPUT)\b")
        self.assertIn("-C DOCKER-USER", text)  # checked before inserting
        self.assertIn("--apply-only", text)  # boot unit path


class FirewallCandidate(unittest.TestCase):
    def test_public_guard_never_drops_management_or_outbound(self):
        candidate = policy.render_public_guard("eth0")
        self.assertNotIn("hook output", candidate)
        self.assertNotIn("policy drop", candidate)
        self.assertNotIn("flush", candidate)
        elements = re.search(r"elements = \{ ([^}]*) \}", candidate).group(1)
        specs = [e.strip().replace("-", ":") for e in elements.split(",")]
        for port in NEVER_BLOCK:
            self.assertFalse(_covered(port, specs), port)
        for port in (8110, 5432, 6379, 8101, 8116):
            self.assertTrue(_covered(port, specs), port)
        # Every drop is scoped to the public interface and to original-direction traffic.
        for line in candidate.splitlines():
            if line.strip().endswith("drop"):
                self.assertIn('iifname "eth0"', line)
                self.assertIn("ct direction original", line)

    def test_admin_candidate_never_drops_management_or_outbound(self):
        candidate = policy.render("wg-admin", "10.254.1.1", "10.254.1.2")
        self.assertNotIn("hook output", candidate)
        self.assertNotIn("policy drop", candidate)
        self.assertNotRegex(candidate, r"dport (22|80|443)\b")

    def test_unsafe_renderings_are_refused(self):
        for specs in (("22",), ("443",), ("80:8080",)):
            with self.assertRaises(ValueError):
                policy.render_public_guard("eth0", specs)
        with self.assertRaises(ValueError):
            policy.private("0.0.0.0", True)

    @unittest.skipUnless(shutil.which("nft") and hasattr(os, "geteuid") and os.geteuid() == 0,
                         "nft dry parse needs nft and root; syntax still covered structurally")
    def test_candidates_parse_with_nft_check(self):
        for candidate in (policy.render_public_guard("eth0"), policy.render("wg-admin", "10.254.1.1", "10.254.1.2")):
            result = subprocess.run(["nft", "--check", "-f", "-"], input=candidate, capture_output=True, text=True, timeout=30)
            self.assertEqual(result.returncode, 0, result.stderr)


class ExposureScripts(unittest.TestCase):
    def test_scripts_parse_and_refuse_without_authorization(self):
        self.assertEqual({p.name for p in EXPOSURE_SCRIPTS}, {"external_scan.sh", "capture_host_evidence.sh"})
        for script in [*EXPOSURE_SCRIPTS, CLOSE_PORTS]:
            self.assertEqual(subprocess.run(["bash", "-n", str(script)]).returncode, 0, script)
            if script in EXPOSURE_SCRIPTS:
                self.assertIn("NOT EXECUTED", script.read_text())
        scan = str(ROOT / "scripts/admin-bridge/exposure/external_scan.sh")
        for args in ([], ["--base-url", "https://example.invalid", "--host", "example.invalid"],
                     ["--i-am-authorized"], ["--i-am-authorized", "--base-url", "https://u:p@example.invalid", "--host", "x"]):
            result = subprocess.run(["bash", scan, *args], capture_output=True, text=True, timeout=30)
            self.assertEqual(result.returncode, 2, args)
            self.assertIn("refusing", result.stderr)

    def test_host_capture_is_read_only(self):
        text = (ROOT / "scripts/admin-bridge/exposure/capture_host_evidence.sh").read_text()
        for forbidden in (r"\biptables -[ADIRFNXP]\b", r"\bnft (add|delete|flush|-f)\b", r"\bufw (allow|deny|enable|disable|reset)\b",
                          r"\bdocker (rm|stop|restart|run|inspect|exec)\b", r"systemctl (start|stop|restart)"):
            self.assertNotRegex(text, forbidden)


if __name__ == "__main__":
    unittest.main()
