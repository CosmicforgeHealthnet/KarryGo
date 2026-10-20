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

if __name__ == "__main__":
    unittest.main()
