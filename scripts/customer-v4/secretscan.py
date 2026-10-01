#!/usr/bin/env python3
"""Customer V4 §25.3 / §29.5 secret-hygiene gate over distributable paths.

Credential-SHAPED values (private keys, Google/Paystack keys, JWTs, service
account files, high-entropy secret assignments) fail the gate unless listed in
secret-scan.reviewed.txt. Only mobile Firebase/Maps client identifiers under
apps/ may be reviewed there: those ship inside every app binary by design and
are restricted by application, not kept secret. A credential-shaped value in
backend, deploy, docs, scripts or CI files can never be allowlisted.

Readable development literals (development-x-secret) are reported, not failed.
Exit 1 on any unreviewed finding. Output never prints a full value.
"""
import glob
import json
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", ".."))
PATTERNS = ("services/*/docs/openapi/*.json", "services/*/.env.example", "deploy/**/*", "apps/*/lib/**/*.dart",
            "apps/*/android/app/src/main/**/*.xml", "apps/*/ios/Runner/Info.plist", "apps/*/assets/**/*.json",
            "docs/**/*.md", "services/*/README.md", "scripts/**/*", ".github/workflows/*.yml")
ENV_LINE = re.compile(r"(?m)^\s*(?:export\s+)?([A-Z0-9_]*(?:SECRET|TOKEN|PASSWORD|API_KEY|PRIVATE_KEY)[A-Z0-9_]*)=(\S*)")
DART_LITERAL = re.compile(r"""(?i)\b\w*(?:secret|servicetoken|hmac|apikey|privatekey)\w*\s*[:=]\s*['"]([^'"$]{12,})['"]""")
SHAPES = {
    "private-key": re.compile(r"-----BEGIN [A-Z ]*PRIVATE KEY-----"),
    "google-api-key": re.compile(r"AIza[0-9A-Za-z_\-]{35}"),
    "paystack-live": re.compile(r"sk_live_[0-9a-zA-Z]{10,}"),
    "paystack-test": re.compile(r"sk_test_[0-9a-zA-Z]{10,}"),
    "jwt": re.compile(r"eyJ[A-Za-z0-9_\-]{10,}\.eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}"),
    "firebase-service-account": re.compile(r'"type"\s*:\s*"service_account"'),
}
PLACEHOLDER = re.compile(r"(?i)^$|change[-_]?me|example|placeholder|dummy|your[-_]|xxx|replace|<|\$\{|redacted|\*\*\*")
DEV_LITERAL = re.compile(r"^[a-z0-9]+(?:[-_.][a-z0-9]+)+$")
REVIEWABLE_KINDS = {"google-api-key", "dart-literal"}
SELF = {"scripts/customer-v4/secretscan.py"}


def high_entropy(value):
    return (len(value) >= 24 and re.fullmatch(r"[A-Za-z0-9+/=_\-]+", value) and not DEV_LITERAL.fullmatch(value)
            and sum(c.isdigit() for c in value) >= 3 and sum(c.isupper() for c in value) >= 3)


def scan():
    targets = sorted({p for pattern in PATTERNS for p in glob.glob(os.path.join(ROOT, pattern), recursive=True) if os.path.isfile(p)})
    findings, dev = [], set()
    for path in targets:
        rel = os.path.relpath(path, ROOT).replace("\\", "/")
        if rel in SELF or "/openapigen/" in rel and rel.endswith(".sum"):
            continue
        try:
            text = open(path, encoding="utf-8", errors="replace").read()
        except OSError:
            continue
        for kind, rx in SHAPES.items():
            for _ in rx.finditer(text):
                findings.append((rel, kind))
        for m in ENV_LINE.finditer(text):
            for value in ([v.split("=", 1)[-1] for v in m.group(2).split(",")] if m.group(2) else [""]):
                if PLACEHOLDER.search(value):
                    continue
                if high_entropy(value):
                    findings.append((rel, "high-entropy-assignment"))
                else:
                    dev.add(f"{m.group(1)}={value if DEV_LITERAL.fullmatch(value) else value[:4] + '...'}")
        for m in DART_LITERAL.finditer(text):
            if not PLACEHOLDER.search(m.group(1)) and high_entropy(m.group(1)):
                findings.append((rel, "dart-literal"))
    return len(targets), sorted(set(findings)), sorted(dev)


def main():
    reviewed_path = os.path.join(HERE, "secret-scan.reviewed.txt")
    reviewed = set()
    if os.path.exists(reviewed_path):
        for line in open(reviewed_path, encoding="utf-8"):
            line = line.strip()
            if line and not line.startswith("#"):
                path, kind = line.rsplit(" ", 1)
                reviewed.add((path, kind))
    count, findings, dev = scan()
    illegal_reviews = [r for r in reviewed if not r[0].startswith("apps/") or r[1] not in REVIEWABLE_KINDS]
    unreviewed = [f for f in findings if f not in reviewed]
    stale = sorted(reviewed - set(findings))
    report = {"scanned_files": count, "credential_shaped": len(findings), "reviewed_client_identifiers": len(findings) - len(unreviewed),
              "unreviewed": [f"{p} {k}" for p, k in unreviewed], "illegal_review_entries": [f"{p} {k}" for p, k in illegal_reviews],
              "stale_review_entries": [f"{p} {k}" for p, k in stale], "development_literals": len(dev)}
    print(json.dumps(report, indent=1))
    return 1 if unreviewed or illegal_reviews else 0


if __name__ == "__main__":
    sys.exit(main())
