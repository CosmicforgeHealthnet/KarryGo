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
('vendor', 'refund-decisions'):('vendor-service', 'vendor.refund_decisions.read', 'R2', ['TestVendorSecureAdminOrderAllocationGraph']),
('vendor', 'parcel-returns'):('vendor-service', 'vendor.parcel_returns.read', 'R2', ['TestVendorSecureAdminOrderAllocationGraph']),
('vendor', 'merchants'):('vendor-service', 'vendor.merchants.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'merchant-capabilities'):('vendor-service', 'vendor.merchant_capabilities.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'brands'):('vendor-service', 'vendor.brands.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'branches'):('vendor-service', 'vendor.branches.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'branch-settings'):('vendor-service', 'vendor.branch_settings.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'branch-capabilities'):('vendor-service', 'vendor.branch_capabilities.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'verification-references'):('vendor-service', 'vendor.verification_references.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'food-items'):('vendor-service', 'vendor.food_items.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'food-variants'):('vendor-service', 'vendor.food_variants.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'shop-products'):('vendor-service', 'vendor.shop_products.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'shop-variants'):('vendor-service', 'vendor.shop_variants.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'shop-listings'):('vendor-service', 'vendor.shop_listings.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
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
('vendor', 'substitutions'):('vendor-service', 'vendor.substitutions.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'support-cases'):('vendor-service', 'vendor.support_cases.read', 'R2', ['TestVendorSecureAdminNativeRowsAndBranchAuthorization']),
('vendor', 'cases'):('vendor-service', 'vendor.cases.read', 'R2', ['TestVendorSecureAdminCasesAndNativeMutations']),
})

SOURCE_PATH = {"vendor":"/internal/vendor/v1/admin/resources","dispatch": "/internal/dispatch/v1/admin/resources", "taxi": "/internal/taxi/v1/admin/resources", "hauling": "/internal/hauling/v1/admin/resources", "fleet": "/internal/fleet-partner/v1/admin/resources"}

# Native operational rows rather than case records.
EVIDENCE = {key: "Owner-native operational row bound by an owner-signed context; immutable owner disclosure audit and bridge audit" for key in KINDS if key[1] in ("bookings", "delivery-missions", "commerce-missions", "partner-requests", "goods-missions", "corporate-rides")}
EVIDENCE.update({key:"Current Vendor-native row and canonical parent relationships; immutable owner disclosure audit and Bridge audit. A native status or historical result is not new operational authority." for key in KINDS if key[0]=="vendor" and key[1]!="cases"})


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
                "feature_flag": f"ADMIN_{'FLEET_PARTNER' if domain == 'fleet' else domain.upper()}_RESOURCE_READ_ENABLED; {'FLEET_PARTNER' if domain == 'fleet' else domain.upper()}_ADMIN_RESOURCE_READ_ENABLED",
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
    path.write_text(json.dumps(matrix, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(f"{len(matrix)} capability entries")
