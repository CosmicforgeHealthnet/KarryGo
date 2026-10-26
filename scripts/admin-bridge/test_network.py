import importlib.util
import unittest
from pathlib import Path

path = Path(__file__).resolve().parents[2] / "deploy/platform/network/admin_policy.py"
spec = importlib.util.spec_from_file_location("admin_policy", path)
policy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(policy)

class NetworkPolicyTests(unittest.TestCase):
    def test_bind_default_and_private_only(self):
        self.assertEqual(policy.private("127.0.0.1", True), "127.0.0.1")
        for address in ("0.0.0.0", "::", "8.8.8.8", "127.0.0.2", "169.254.1.2", "10.0.0.1:8110"):
            with self.assertRaises(ValueError):
                policy.private(address, True)

    def test_acl_exact_hq_port_and_deny_other_access(self):
        candidate = policy.render("wg-admin", "10.254.1.1", "10.254.1.2")
        self.assertIn('iifname "wg-admin" ip saddr 10.254.1.1 ip daddr 10.254.1.2 tcp dport 8110 counter accept', candidate)
        self.assertIn("ip daddr 10.254.1.2 tcp dport 8110 counter drop", candidate)
        self.assertIn('iifname "wg-admin" ip saddr 10.254.1.1 counter drop', candidate)
        for interface in ("wg-admin; flush ruleset", "../wg", ""):
            with self.assertRaises(ValueError):
                policy.render(interface, "10.254.1.1", "10.254.1.2")

    def test_public_guard_inbound_only_and_never_management(self):
        candidate = policy.render_public_guard("eth0")
        self.assertIn("8101-8116", candidate)  # 8110 merged into the service range
        self.assertIn('iifname "eth0"', candidate)
        self.assertNotIn("hook output", candidate)
        self.assertEqual(candidate.count("policy accept"), 2)
        self.assertNotIn("policy drop", candidate)
        for spec in (("22",), ("80",), ("443",), ("1:1024",), ("440:450",)):
            with self.assertRaises(ValueError):
                policy.render_public_guard("eth0", spec)
        for interface in ("eth0; flush ruleset", "", "a" * 16):
            with self.assertRaises(ValueError):
                policy.render_public_guard(interface)

if __name__ == "__main__":
    unittest.main()
