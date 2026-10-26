"""Owner resource kinds added for Sections 8-10 case records.

Each is a read-only, source-scoped list/detail pair over a record the owner
itself writes through its administrative command contract. Running this file
idempotently (re)writes their capability matrix entries.
"""
KINDS = {
    ("taxi", "incidents"): ("driver-taxi-service", "taxi.incidents.read", "R2", ["TestTaxiIncidentReadsAndCaseEvidenceAuthority"]),
    ("fleet", "investigations"): ("fleet-partner-service", "fleet.investigations.read", "R2", ['TestPostgresFleetInvestigationLifecycleApprovalAndRealEffects', 'TestPostgresFleetInvestigationCaseAuthorityProof']),
    # Section 6 analytics populations: native owner rows anchored by
    # owner-signed contexts exactly like trips, riders and independent rides.
    ("dispatch", "bookings"): ("driver-dispatch-delivery-service", "dispatch.bookings.read", "R2", ["TestDispatchConsumerPopulationReadsBindOwnRowsWithoutInventingScope"]),
    ("dispatch", "delivery-missions"): ("driver-dispatch-delivery-service", "dispatch.delivery_missions.read", "R2", ["TestDispatchConsumerPopulationReadsBindOwnRowsWithoutInventingScope"]),
    ("dispatch", "commerce-missions"): ("driver-dispatch-delivery-service", "dispatch.commerce_missions.read", "R2", ["TestDispatchConsumerPopulationReadsBindOwnRowsWithoutInventingScope"]),
    ("dispatch", "partner-requests"): ("driver-dispatch-delivery-service", "dispatch.partner_requests.read", "R2", ["TestDispatchConsumerPopulationReadsBindOwnRowsWithoutInventingScope"]),
    ("taxi", "goods-missions"): ("driver-taxi-service", "taxi.goods_missions.read", "R2", ["TestTaxiGoodsAndCorporateRideReadsBindOwnRowsWithoutInventingScope"]),
    ("taxi", "corporate-rides"): ("driver-taxi-service", "taxi.corporate_rides.read", "R2", ["TestTaxiGoodsAndCorporateRideReadsBindOwnRowsWithoutInventingScope"]),
    ("hauling", "inspections"): ("driver-hauling-service", "hauling.inspections.read", "R2", ["TestPostgresInspectionLifecycleFailRestrictsTruckAndServesCaseAuthority", "TestPostgresInspectionCrossTenantAndRevokedOwnership"]),
}
# Secure Admin Section 11: finite native Vendor resources.
KINDS.update({
('vendor', 'fulfilment-units'):('vendor-service', 'vendor.fulfilment_units.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'delivery-mission-links'):('vendor-service', 'vendor.delivery_mission_links.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'vendor-delivery-status'):('vendor-service', 'vendor.vendor_delivery_status.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
("vendor","branch-hours"):("vendor-service","vendor.branch_hours.read","R2",["TestVendorSecureAdminNativeRowsAndBranchAuthorization"]),
('vendor', 'order-lines'):('vendor-service', 'vendor.order_lines.read', 'R2', ['TestVendorSecureAdminOrderAllocationGraph']),
('vendor', 'payment-allocations'):('vendor-service', 'vendor.payment_allocations.read', 'R2', ['TestVendorSecureAdminOrderAllocationGraph']),
('vendor', 'fulfilment-parcels'):('vendor-service', 'vendor.fulfilment_parcels.read', 'R2', ['TestVendorSecureAdminOrderAllocationGraph']),
('vendor', 'order-events'):('vendor-service', 'vendor.order_events.read', 'R2', ['TestVendorSecureAdminOrderAllocationGraph']),
('vendor', 'logistics-events'):('vendor-service', 'vendor.logistics_events.read', 'R2', ['TestVendorSecureAdminOrderAllocationGraph']),
('vendor', 'refund-decisions'):('vendor-service', 'vendor.refund_decisions.read', 'R2', ['TestVendorSecureAdminOrderAllocationGraph', 'TestSubstitution_PendingLiabilityRemainsDurableUntilWalletApplies']),
('vendor', 'parcel-returns'):('vendor-service', 'vendor.parcel_returns.read', 'R2', ['TestVendorSecureAdminOrderAllocationGraph']),
('vendor', 'merchants'):('vendor-service', 'vendor.merchants.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization', 'TestVendorCheckoutRejectsSuspendedMerchantAndClosedBranch']),
('vendor', 'merchant-capabilities'):('vendor-service', 'vendor.merchant_capabilities.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'brands'):('vendor-service', 'vendor.brands.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'branches'):('vendor-service', 'vendor.branches.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization', 'TestVendorCheckoutRejectsSuspendedMerchantAndClosedBranch']),
('vendor', 'branch-settings'):('vendor-service', 'vendor.branch_settings.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'branch-capabilities'):('vendor-service', 'vendor.branch_capabilities.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'verification-references'):('vendor-service', 'vendor.verification_references.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization', 'TestVendorCheckoutFailsClosedForExpiredVerificationOnApprovedQuote']),
('vendor', 'food-items'):('vendor-service', 'vendor.food_items.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'food-variants'):('vendor-service', 'vendor.food_variants.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'shop-products'):('vendor-service', 'vendor.shop_products.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'shop-variants'):('vendor-service', 'vendor.shop_variants.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'shop-listings'):('vendor-service', 'vendor.shop_listings.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization', 'TestVendorCheckoutRejectsTakenDownRealListing']),
('vendor', 'stock-balances'):('vendor-service', 'vendor.stock_balances.read', 'R2', ['TestVendorSecureAdminStockReadsUseCanonicalExpiryAwareAvailability']),
('vendor', 'stock-reservations'):('vendor-service', 'vendor.stock_reservations.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'stock-exceptions'):('vendor-service', 'vendor.stock_exceptions.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'production-jobs'):('vendor-service', 'vendor.production_jobs.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'production-batches'):('vendor-service', 'vendor.production_batches.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'capacity-pools'):('vendor-service', 'vendor.capacity_pools.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'capacity-reservations'):('vendor-service', 'vendor.capacity_reservations.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'production-exceptions'):('vendor-service', 'vendor.production_exceptions.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'kitchen-packages'):('vendor-service', 'vendor.kitchen_packages.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'pastry-commitments'):('vendor-service', 'vendor.pastry_commitments.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'vendor-orders'):('vendor-service', 'vendor.vendor_orders.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'substitutions'):('vendor-service', 'vendor.substitutions.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization', 'TestSubstitution_DeadlineRequiredAndExpiredApprovalPersistsTerminalState']),
('vendor', 'support-cases'):('vendor-service', 'vendor.support_cases.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'cases'):('vendor-service', 'vendor.cases.read', 'R2', ['TestVendorSecureAdminCasesAndNativeMutations']),
})

# Secure Admin Section 12: Fleet corporate operations. Every entry is a
# fixed, minimized Fleet-owned projection. The two explicit exceptions to the
# general R2 risk are private manifests and immutable audit history, which use
# dedicated capabilities and R3 review.
for _kind, _permission, _risk in [
    ('organizations', 'fleet.organizations.read', 'R2'),
    ('onboarding-stages', 'fleet.onboarding_stages.read', 'R2'),
    ('organization-documents', 'fleet.organization_documents.read', 'R3'),
    ('branches', 'fleet.branches.read', 'R2'),
    ('departments', 'fleet.departments.read', 'R2'),
    ('organization-staff', 'fleet.organization_staff.read', 'R3'),
    ('role-bindings', 'fleet.role_bindings.read', 'R3'),
    ('provider-memberships', 'fleet.provider_memberships.read', 'R2'),
    ('assets', 'fleet.assets.read', 'R2'),
    ('asset-assignments', 'fleet.asset_assignments.read', 'R2'),
    ('asset-documents', 'fleet.asset_documents.read', 'R3'),
    ('contracts', 'fleet.contracts.read', 'R3'),
    ('crm-contacts', 'fleet.crm_contacts.read', 'R3'),
    ('quotations', 'fleet.quotations.read', 'R3'),
    ('approval-requests', 'fleet.approval_requests.read', 'R3'),
    ('wallet-transactions', 'fleet.wallet_transactions.read', 'R3'),
    ('private-orders', 'fleet.private_orders.read', 'R3'),
    ('recurring-orders', 'fleet.recurring_orders.read', 'R2'),
    ('manifests', 'fleet.private_manifests.read', 'R3'),
    ('route-plans', 'fleet.route_plans.read', 'R2'),
    ('route-optimizations', 'fleet.route_optimizations.read', 'R2'),
    ('operational-alerts', 'fleet.operational_alerts.read', 'R2'),
    ('incidents', 'fleet.incidents.read', 'R3'),
    ('sla-measurements', 'fleet.sla_measurements.read', 'R2'),
    ('audit-records', 'fleet.audit_records.read', 'R3'),
]:
    KINDS[('fleet', _kind)] = (
        'fleet-partner-service', _permission, _risk,
        ['TestCorporateAdminResourcesAreOwnerBackedAndMinimized'],
    )

# Secure Admin Section 13: Wallet-native financial projections. Amounts are
# minor-unit integers and bank/payment-provider credentials are never exposed.
for _kind, _permission, _risk in [
    ('customer-accounts', 'wallet.customer_accounts.read', 'R3'),
    ('payment-intents', 'wallet.payment_intents.read', 'R3'),
    ('customer-refunds', 'wallet.refunds.read', 'R3'),
    ('provider-payouts', 'wallet.provider_withdrawals.read', 'R3'),
    ('provider-webhook-events', 'wallet.reconciliation.read', 'R3'),
    ('commerce-payment-intents', 'wallet.commerce_payments.read', 'R3'),
    ('commerce-allocations', 'wallet.commerce_allocations.read', 'R3'),
    ('commerce-operations', 'wallet.commerce_operations.read', 'R3'),
    ('organization-wallets', 'wallet.accounts.read', 'R3'),
    ('provider-accounts', 'wallet.provider_accounts.read', 'R3'),
    ('ledger-accounts', 'wallet.ledger_accounts.read', 'R3'),
    ('ledger-balances', 'wallet.ledger_balances.read', 'R3'),
    ('journals', 'wallet.transactions.read', 'R3'),
    ('journal-lines', 'wallet.transactions.read', 'R3'),
    ('holds', 'wallet.holds.read', 'R3'),
    ('subscriptions', 'wallet.subscriptions.read', 'R3'),
    ('invoices', 'wallet.invoices.read', 'R3'),
    ('payments', 'wallet.payments.read', 'R3'),
    ('commission-rules', 'wallet.commissions.read', 'R3'),
    ('earning-allocations', 'wallet.allocations.read', 'R3'),
    ('settlements', 'wallet.settlements.read', 'R3'),
    ('settlement-items', 'wallet.settlements.read', 'R3'),
    ('refunds', 'wallet.refunds.read', 'R3'),
    ('reversals', 'wallet.reversals.read', 'R3'),
    ('provider-allocations', 'wallet.provider_allocations.read', 'R3'),
    ('provider-withdrawals', 'wallet.provider_withdrawals.read', 'R3'),
    ('job-payments', 'wallet.job_payments.read', 'R3'),
    ('reconciliation-runs', 'wallet.reconciliation.read', 'R3'),
    ('reconciliation-findings', 'wallet.reconciliation.read', 'R3'),
    ('statements', 'wallet.statements.read', 'R3'),
    ('financial-audit', 'wallet.audit.read', 'R3'),
    ('financial-cases', 'wallet.cases.read', 'R3'),
]:
    KINDS[('wallet', _kind)] = (
        'payment-wallet-service', _permission, _risk,
        ['TestPostgresWalletAdminResourcesAndNativeFinancialCommands'],
    )

# Secure Admin Section 15: person-scoped Referral programme projections.
# Referral stores no canonical market/zone/organization/tenant relationship;
# every row therefore remains unavailable until a separately signed owner
# context binds that exact subject to all five delegated scope dimensions.
for _kind, _permission, _risk in [
    ('persons', 'referral.persons.read', 'R3'),
    ('accounts', 'referral.accounts.read', 'R3'),
    ('referrals', 'referral.referrals.read', 'R3'),
    ('tracks', 'referral.tracks.read', 'R2'),
    ('rewards', 'referral.rewards.read', 'R3'),
    ('fraud-flags', 'referral.fraud_flags.read', 'R3'),
    ('milestones', 'referral.milestones.read', 'R2'),
    ('reconciliation-mismatches', 'referral.reconciliation.read', 'R3'),
    ('admin-actions', 'referral.admin_actions.read', 'R3'),
]:
    KINDS[('referral', _kind)] = (
        'referral-service', _permission, _risk,
        ['TestReferralSecureAdminOwnerBoundary'],
    )

# Secure Admin Section 16: Support cases, SOS, disputes, message metadata and
# evidence references. Chat bodies, identity snapshots and coordinates are never
# disclosed; message/case labels are credential-redacted bounded previews.
for _kind, _permission, _risk in [
    ('cases', 'support.cases.read', 'R2'),
    ('sos-cases', 'support.sos.read', 'R3'),
    ('disputes', 'support.disputes.read', 'R3'),
    ('case-messages', 'support.messages.read', 'R3'),
    ('case-evidence', 'support.evidence.read', 'R3'),
]:
    KINDS[('support', _kind)] = ('support-dispute-service', _permission, _risk, ['TestSupportSecureAdminOwnerBoundary'])

# Secure Admin Section 18: formal Verification cases and automation findings.
# Document references, identity numbers and bank data are never disclosed.
for _kind, _permission, _risk in [
    ('vendor-cases', 'verification.vendor_cases.read', 'R3'),
    ('vendor-case-findings', 'verification.findings.read', 'R3'),
    ('asset-capability-cases', 'verification.asset_cases.read', 'R3'),
    ('organization-submissions', 'verification.organization_submissions.read', 'R3'),
]:
    KINDS[('verification', _kind)] = ('verification-compliance-service', _permission, _risk, ['TestVerificationSecureAdminOwnerBoundary'])

SOURCE_PATH = {"support": "/internal/support/v1/admin/resources", "verification": "/internal/verification/v1/admin/resources", "vendor":"/internal/vendor/v1/admin/resources","dispatch": "/internal/dispatch/v1/admin/resources", "taxi": "/internal/taxi/v1/admin/resources", "hauling": "/internal/hauling/v1/admin/resources", "fleet": "/internal/fleet-partner/v1/admin/resources", "wallet": "/internal/payment-wallet/v1/admin/resources", "referral": "/internal/referral/v1/admin-owner/resources"}

# Native operational rows rather than case records.
EVIDENCE = {key: "Owner-native operational row bound by an owner-signed context; immutable owner disclosure audit and bridge audit" for key in KINDS if key[1] in ("bookings", "delivery-missions", "commerce-missions", "partner-requests", "goods-missions", "corporate-rides")}
EVIDENCE.update({key:"Current Vendor-native row and canonical parent relationships; immutable owner disclosure audit and Bridge audit. A native status or historical result is not new operational authority." for key in KINDS if key[0]=="vendor" and key[1]!="cases"})


PREFIX = {"fleet": "FLEET_PARTNER", "wallet": "PAYMENT_WALLET", "support": "SUPPORT_DISPUTE", "verification": "VERIFICATION_COMPLIANCE"}


def entries():
    rows = []
    for (domain, kind), (owner, permission, risk, tests) in KINDS.items():
        for op, suffix in (("list", ""), ("detail", "/{id}")):
            rows.append({
                "id": f"{domain}.resources.{kind}.{op}", "version": "1.0", "owner": owner,
                "source_endpoint": f"{SOURCE_PATH[domain]}/{kind}{suffix}", "bridge_endpoint": f"/{domain}/resources/{kind}{suffix}",
                "method": "GET", "state": "READ_ONLY", "permission": permission,
                "scope": "karryam + exact market + zone + organization + tenant; delegated staff and current signed source-owner context",
                "risk": risk, "privacy": "MINIMIZED_OPERATIONAL_METADATA", "source_version_required": False, "idempotency_required": False,
                "approval": "BLOCKED_POLICY until independently signed owner context and approved governance policy exist",
                "evidence": EVIDENCE.get((domain, kind), "Owner-written case record and its immutable decision history; immutable owner disclosure audit and bridge audit"),
                "feature_flag": f"ADMIN_{PREFIX.get(domain, domain.upper())}_RESOURCE_READ_ENABLED; {PREFIX.get(domain, domain.upper())}_ADMIN_RESOURCE_READ_ENABLED",
                "tests": ["TestActualOwnerMigrationsAndSourceDictionary", "TestOwnerResourcesDelegation", *tests],
                "blocker": "Disabled by default; draft policy documents provide no access grants. Missing or conflicting source authority denies access.",
                "fallback": "Fail closed; no privileged mutations, raw owner proxy or tenant fallback.",
            })
    return rows


def kinds_by_domain():
    out = {}
    for domain, kind in KINDS:
        out.setdefault(domain, []).append(kind)
    return out


if __name__ == "__main__":
    import json
    from pathlib import Path
    path = Path(__file__).resolve().parents[2] / "services/admin-backoffice-service/docs/openapi/KARRYGO_ADMIN_CAPABILITY_MATRIX.json"
    matrix = json.loads(path.read_text(encoding="utf-8"))
    ids = {row["id"] for row in entries()}
    matrix = [row for row in matrix if row["id"] not in ids] + entries()
    path.write_bytes((json.dumps(matrix, indent=2, ensure_ascii=False) + "\n").encode("utf-8"))
    print(f"{len(matrix)} capability entries")
