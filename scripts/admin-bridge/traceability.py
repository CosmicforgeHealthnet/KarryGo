"""Generate docs/admin-bridge/a-z-final-capability-traceability.json (Section 36).

One row per private Bridge capability (from the pinned capability matrix) plus
explicit rows for capabilities that are deliberately NOT shipped because a
domain authority, policy or infrastructure dependency is missing. Status labels
are conservative: nothing is ever marked STAGING_CERTIFIED or
PRODUCTION_CERTIFIED from repository evidence alone.

Usage: python traceability.py --sha <code commit> [--check]
"""
import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
MATRIX = ROOT / "services/admin-backoffice-service/docs/openapi/KARRYGO_ADMIN_CAPABILITY_MATRIX.json"
OUT = ROOT / "docs/admin-bridge/a-z-final-capability-traceability.json"

SOURCE_PATHS = {
    "admin-backoffice-service": "services/admin-backoffice-service/internal",
    "analytics-service": "services/analytics-service/internal/features/adminprivacy",
    "customer-service": "services/customer-service/internal/features/{adminresources,admincommands,adminreview,marketing_consent}",
    "driver-taxi-service": "services/driver-taxi-service/internal/features/{adminread,adminresources,admininterventions}",
    "driver-dispatch-delivery-service": "services/driver-dispatch-delivery-service/internal/features/{adminresources,adminintervention}",
    "driver-hauling-service": "services/driver-hauling-service/internal/features/{adminresources,admincommands}",
    "fleet-partner-service": "services/fleet-partner-service/internal/features/{adminresources,admininvestigations}",
    "vendor-service": "services/vendor-service/internal/features/admin*",
    "payment-wallet-service": "services/payment-wallet-service/internal/features/{adminresources,admincommands,financereports}",
    "pricing-service": "services/pricing-service/internal",
    "referral-service": "services/referral-service/internal/features/{adminresources,admincommands}",
    "support-dispute-service": "services/support-dispute-service/internal/features/{adminresources,admincommands,adminfeed}",
    "verification-compliance-service": "services/verification-compliance-service/internal/features/{adminresources,admincommands,adminsnapshot,vendors}",
    "notification-service": "services/notification-service/internal/features/campaigns",
    "media-file-service": "services/media-file-service/internal/features/mediaobjects",
}

# Integration-tested families: PostgreSQL-backed owner or Bridge suites exist and
# are enforced zero-skip by a mandatory workflow.
INTEGRATION_PREFIXES = ("customer.", "taxi.", "dispatch.", "hauling.", "fleet.", "vendor.", "wallet.", "referral.", "support.", "verification.", "hq.", "media.", "analytics.scoped.", "pricing.", "bridge.")

BLOCKED = [
    {"CAPABILITY_ID": "pricing.quote.issue_bind_reconcile", "OWNER": "pricing-service", "STATUS": "BLOCKED_DOMAIN", "REASON": "Pricing quote issuance/bind/reconcile endpoints are not built by the owner; production cutover stays disabled.", "SAFE_FALLBACK": "Existing draft/approve/activate/freeze policy capabilities only."},
    {"CAPABILITY_ID": "wallet.event.settlement_reversed.v1", "OWNER": "payment-wallet-service", "STATUS": "BLOCKED_DOMAIN", "REASON": "Wallet does not yet publish finance.job.settlement_reversed.v1 after a committed reversal; Referral cannot claim post-settlement reversal propagation.", "SAFE_FALLBACK": "Referral reversal requests remain pending until Wallet posts."},
    {"CAPABILITY_ID": "commerce.market.global_closure", "OWNER": "vendor-service", "STATUS": "BLOCKED_DOMAIN", "REASON": "No owner state machine defines global cross-service market closure semantics.", "SAFE_FALLBACK": "Per-branch pause/reopen only."},
    {"CAPABILITY_ID": "referral.appeals_campaigns_rankings", "OWNER": "referral-service", "STATUS": "BLOCKED_DOMAIN", "REASON": "No Referral state machine for appeals, incentive campaigns or ranking publication.", "SAFE_FALLBACK": "Flag resolution and account/referral review only."},
    {"CAPABILITY_ID": "notification.channel.sms_whatsapp", "OWNER": "notification-service", "STATUS": "BLOCKED_INFRA", "REASON": "No contracted SMS/WhatsApp provider or channel implementation.", "SAFE_FALLBACK": "Campaign channels limited to in_app, push, email."},
    {"CAPABILITY_ID": "notification.delivery_receipts", "OWNER": "notification-service", "STATUS": "BLOCKED_INFRA", "REASON": "Providers expose acceptance ids only; no delivery/viewed/conversion/billing receipts are integrated.", "SAFE_FALLBACK": "delivered_count=null, delivery_confirmation=UNAVAILABLE; logging providers counted as LOGGED_ONLY."},
    {"CAPABILITY_ID": "notification.emergency_broadcast", "OWNER": "notification-service", "STATUS": "BLOCKED_POLICY", "REASON": "Emergency broadcasts need a separate higher authorization and staffing policy; refused on the marketing contract.", "SAFE_FALLBACK": "CATEGORY_NOT_PERMITTED rejection."},
    {"CAPABILITY_ID": "verification.external_kyc_authority", "OWNER": "verification-compliance-service", "STATUS": "BLOCKED_POLICY", "REASON": "No lawful-basis approval or contracted government/KYC provider; DisabledAuthority only.", "SAFE_FALLBACK": "SOURCE_UNVERIFIED recommendation; human review required; AUTO_ELIGIBLE unreachable."},
    {"CAPABILITY_ID": "verification.address_consistency_check", "OWNER": "verification-compliance-service", "STATUS": "BLOCKED_DOMAIN", "REASON": "Verification holds no address data for vendor cases.", "SAFE_FALLBACK": "Recorded NOT_EVALUATED."},
    {"CAPABILITY_ID": "media.malware_scanner_certification", "OWNER": "media-file-service", "STATUS": "BLOCKED_INFRA", "REASON": "ClamAV production deployment/certification is external.", "SAFE_FALLBACK": "High-risk purposes fail closed without a SAFE verdict."},
    {"CAPABILITY_ID": "media.legacy_mapping.document_signature", "OWNER": "media-file-service", "STATUS": "BLOCKED_DOMAIN", "REASON": "Legacy document_file/signature assets have no authoritative owner association; inference from names/URLs is refused.", "SAFE_FALLBACK": "Not found; never publicly served."},
    {"CAPABILITY_ID": "support.sla_policy", "OWNER": "support-dispute-service", "STATUS": "BLOCKED_POLICY", "REASON": "No approved SLA/staffing/escalation policy for Support or SOS.", "SAFE_FALLBACK": "sla_state=POLICY_UNAVAILABLE."},
    {"CAPABILITY_ID": "finance.revenue_recognition", "OWNER": "payment-wallet-service", "STATUS": "BLOCKED_POLICY", "REASON": "No approved revenue recognition policy.", "SAFE_FALLBACK": "Fee figures labelled SOURCE_FIGURE_RECOGNITION_POLICY_PENDING; GMV/escrow NOT_REVENUE."},
    {"CAPABILITY_ID": "privacy.cross_service_erasure", "OWNER": "all owners", "STATUS": "BLOCKED_POLICY", "REASON": "Only customer-service has an approved owner privacy command; retention/erasure schedules for other holders are unapproved.", "SAFE_FALLBACK": "Disposition register records POLICY_REQUIRED; nothing deleted."},
    {"CAPABILITY_ID": "production.activation", "OWNER": "platform", "STATUS": "BLOCKED_INFRA", "REASON": "Private network, certificates, keys, signed policies, staff grants, owner contexts and deployment approvals are not supplied.", "SAFE_FALLBACK": "Every remote read/write and HQ feature defaults disabled."},
]


def status(row):
    if row["state"] in ("DISABLED", "BLOCKED_DEPENDENCY", "UNAVAILABLE"):
        return "NOT_IMPLEMENTED" if row["state"] == "UNAVAILABLE" else "BLOCKED_DOMAIN"
    if row["id"].startswith(INTEGRATION_PREFIXES):
        return "INTEGRATION_TESTED"
    return "CONTRACT_TESTED"


def build(sha):
    matrix = json.loads(MATRIX.read_text(encoding="utf-8"))
    rows = []
    for c in sorted(matrix, key=lambda r: r["id"]):
        rows.append({
            "CAPABILITY_ID": c["id"], "OWNER": c["owner"], "FEATURE": c["id"].split(".", 1)[0],
            "SOURCE_PATH": SOURCE_PATHS.get(c["owner"], "services/" + c["owner"]), "SOURCE_SHA": sha,
            "SOURCE_ROUTE": c["source_endpoint"], "BRIDGE_ROUTE": c["method"] + " /internal/karryam-admin/v1" + c["bridge_endpoint"],
            "SCOPE": c["scope"], "PRIVACY": c["privacy"], "PERMISSIONS": c["permission"],
            "IDEMPOTENCY": c["idempotency_required"], "SOURCE_VERSION": c["source_version_required"],
            "TESTS": c["tests"], "INTEGRATION_EVIDENCE": "Mandatory GitHub workflows on SOURCE_SHA (see sections-16-38-final-report.md); disposable PostgreSQL 16/Redis 7, zero skipped tests",
            "FLAG": c["feature_flag"], "ROLLBACK": "Disable the listed flags and ADMIN_REMOTE_WRITES_ENABLED; durable operations reconcile by original identity; evidence tables are never deleted.",
            "APPROVAL": c["approval"], "STATUS": status(c), "BLOCKED_POLICY": True,
            "BLOCKER": c["blocker"], "RISK": c["risk"],
        })
    return {"schema": "karryam.capability-traceability.v1", "source_sha": sha, "capability_count": len(rows),
            "status_legend": ["SOURCE_VERIFIED", "IMPLEMENTED_LOCAL", "CONTRACT_TESTED", "INTEGRATION_TESTED", "STAGING_CERTIFIED", "PRODUCTION_CERTIFIED", "BLOCKED_DOMAIN", "BLOCKED_INFRA", "BLOCKED_POLICY", "NOT_IMPLEMENTED"],
            "note": "BLOCKED_POLICY=true on every shipped capability: no signed production policy, owner context, staff grant or key is supplied, so all remain default-disabled. No row is STAGING_CERTIFIED or PRODUCTION_CERTIFIED.",
            "capabilities": rows, "not_shipped": BLOCKED}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--sha", required=True)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    content = (json.dumps(build(args.sha), indent=2) + "\n").encode()
    if args.check:
        assert OUT.read_bytes() == content, "traceability register drift"
    else:
        OUT.write_bytes(content)
    print(f"{json.loads(content)['capability_count']} capabilities, {len(BLOCKED)} explicit not-shipped rows")


if __name__ == "__main__":
    main()
