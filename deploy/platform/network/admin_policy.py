"""Render a reviewable nftables fragment; never executes firewall commands."""
import argparse
import ipaddress
import re

PRIVATE = [ipaddress.ip_network(value) for value in ("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10")]

def private(value, loopback=False):
    address = ipaddress.ip_address(value)
    if address.version != 4 or not (any(address in n for n in PRIVATE) or (loopback and str(address) == "127.0.0.1")):
        raise ValueError("only an approved private IPv4 address is supported")
    return str(address)

# Section 20/29: host ports that must never answer on the public interface.
# Kept identical to PORTS in deploy/platform/bin/close-public-ports.sh (a test
# compares the two). "a:b" is an inclusive range.
PROTECTED_PORTS = ("4100", "4101", "4103", "8080", "8110", "8101:8116", "5432:5451", "6379:6391")
# Management and web ingress. A rendering that would drop any of these is refused.
NEVER_BLOCK = (22, 80, 443)

def _interface(interface, what):
    if not re.fullmatch(r"[A-Za-z0-9_.-]{1,15}", interface or ""):
        raise ValueError(f"invalid {what} interface")
    return interface

def _port_ranges(specs):
    ranges = []
    for spec in specs:
        low, _, high = spec.partition(":")
        low, high = int(low), int(high or low)
        if not (0 < low <= high < 65536):
            raise ValueError(f"invalid port spec {spec}")
        for port in NEVER_BLOCK:
            if low <= port <= high:
                raise ValueError(f"refusing to block management/web port {port}")
        ranges.append((low, high))
    return ranges

def render_public_guard(public_interface, specs=PROTECTED_PORTS):
    """Inbound-only drop of protected ports on the public interface.

    Two base chains, both policy accept: `forward` covers Docker-published
    ports (matched on the ORIGINAL destination port, i.e. before Docker's
    DNAT, the nft twin of close-public-ports.sh), `input` covers host-native
    listeners. There is no output chain, so outbound traffic is never touched,
    and replies (ct direction reply) are never matched.
    """
    interface = _interface(public_interface, "public")
    # nft interval sets reject overlapping elements: merge first.
    merged = []
    for low, high in sorted(_port_ranges(specs)):
        if merged and low <= merged[-1][1] + 1:
            merged[-1] = (merged[-1][0], max(merged[-1][1], high))
        else:
            merged.append((low, high))
    elements = ", ".join(str(low) if low == high else f"{low}-{high}" for low, high in merged)
    return f'''# Section 29 candidate only; the generator never applies it. Review with the
# existing ufw/Docker rules, run `nft --check --file <candidate>`, keep a rollback.
table inet karryam_exposure_guard {{
  set protected_tcp {{
    type inet_service; flags interval;
    elements = {{ {elements} }}
  }}
  chain public_forward {{
    type filter hook forward priority -10; policy accept;
    iifname "{interface}" meta l4proto tcp ct direction original ct original proto-dst @protected_tcp counter drop
  }}
  chain public_input {{
    type filter hook input priority -10; policy accept;
    iifname "{interface}" tcp dport @protected_tcp ct direction original counter drop
  }}
}}
'''

def render(interface, hq, bridge):
    interface = _interface(interface, "VPN")
    hq, bridge = private(hq), private(bridge)
    if hq == bridge:
        raise ValueError("HQ and bridge addresses must differ")
    return f'''# Review with existing ingress, Docker NAT and management access before approval.
# This file is a candidate only; the generator never applies it.
table inet karryam_admin_guard {{
  chain private_ingress {{
    type filter hook prerouting priority -110; policy accept;
    iifname "{interface}" ip saddr {hq} ip daddr {bridge} tcp dport 8110 counter accept
    ip daddr {bridge} tcp dport 8110 counter drop
    iifname "{interface}" ip saddr {hq} counter drop
  }}
}}
'''

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--validate-bind")
    parser.add_argument("--vpn-interface")
    parser.add_argument("--hq-ip")
    parser.add_argument("--bridge-ip")
    parser.add_argument("--public-interface", help="render the Section 29 public exposure guard instead")
    args = parser.parse_args()
    try:
        if args.public_interface:
            print(render_public_guard(args.public_interface), end="")
        elif args.validate_bind:
            private(args.validate_bind, loopback=True)
            print("Admin bind address is loopback/private; transport identity still requires certification")
        else:
            print(render(args.vpn_interface or "", args.hq_ip or "", args.bridge_ip or ""), end="")
    except ValueError as error:
        parser.error(str(error))
