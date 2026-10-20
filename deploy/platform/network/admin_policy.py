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

def render(interface, hq, bridge):
    if not re.fullmatch(r"[A-Za-z0-9_.-]{1,15}", interface):
        raise ValueError("invalid VPN interface")
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
    args = parser.parse_args()
    try:
        if args.validate_bind:
            private(args.validate_bind, loopback=True)
            print("Admin bind address is loopback/private; transport identity still requires certification")
        else:
            print(render(args.vpn_interface or "", args.hq_ip or "", args.bridge_ip or ""), end="")
    except ValueError as error:
        parser.error(str(error))
