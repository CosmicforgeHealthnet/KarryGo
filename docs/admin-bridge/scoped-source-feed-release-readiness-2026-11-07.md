# Verification and Notification staging release readiness

The signed, read-only source feeds are implemented for Verification/Compliance Service and Notification Service. They remain disabled on the VPS. This is not confirmation that all Karryam services are connected to Central Admin.

## Checked candidate

- Notification Service: 69 tests passed; no failed or skipped tests.
- Verification/Compliance Service: 60 tests passed; no failed or skipped tests.
- Product Integration Core and Karryam Admin were not changed in this source batch.
- Karryam Backoffice Bridge, Notification Service and Verification/Compliance Service binaries compile with the corrected dependency versions. Trivy 0.76.0 reports zero HIGH/CRITICAL vulnerabilities and zero secret findings.
- golang.org/x/net is now v0.62.0 in the three release modules, fixing CVE-2026-78661. No scanner exceptions were added.
- Detailed test, binary and restore evidence is in `scoped-source-feed-acceptance-2026-11-07.json`.

## Activation still requires

1. All required GitHub checks must pass on the final staging commit.
2. Actual backup restore and runtime compatibility must be certified for that exact release. Database migration previews are isolated checks, not permission to migrate a live database.
3. The existing deployment gate requires an independent reviewer for these services. GitHub staging currently has no required reviewer configured. The explicit source-controlled delegation covers only Support/Dispute Service (`scripts/release/gate.py`, `release_authorization`); it does not cover this release. No review was fabricated and this boundary was not changed.
4. Install signed policies and native owner context allocations for the additional source scopes, then configure distinct transport credentials. Existing Taxi/Support policy does not authorize Verification or Notification exports.
5. Deploy only the approved services, verify scope rejection/replay/recovery, enable the approved HQ dataset subscriptions and confirm real data with a genuinely MFA-authenticated authorized staff account.

Dispatch/Delivery Service remains excluded from deployment. No production changes, staff grants, fake MFA, domain writes or direct HQ database access are part of this batch.
