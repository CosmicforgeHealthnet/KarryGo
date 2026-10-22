"""Owner resource kinds added for Sections 8-10 case records.

Each is a read-only, source-scoped list/detail pair over a record the owner
itself writes through its administrative command contract. Running this file
idempotently (re)writes their capability matrix entries.
"""
KINDS = {
    ("taxi", "incidents"): ("driver-taxi-service", "taxi.incidents.read", "R2", ["TestTaxiIncidentReadsAndCaseEvidenceAuthority"]),
    ("fleet", "investigations"): ("fleet-partner-service", "fleet.investigations.read", "R2", ['TestPostgresFleetInvestigationLifecycleApprovalAndRealEffects', 'TestPostgresFleetInvestigationCaseAuthorityProof']),
    ("hauling", "inspections"): ("driver-hauling-service", "hauling.inspections.read", "R2", ["TestPostgresInspectionLifecycleFailRestrictsTruckAndServesCaseAuthority", "TestPostgresInspectionCrossTenantAndRevokedOwnership"]),
}
SOURCE_PATH = {"taxi": "/internal/taxi/v1/admin/resources", "hauling": "/internal/hauling/v1/admin/resources", "fleet": "/internal/fleet-partner/v1/admin/resources"}


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
                "evidence": "Owner-written case record and its immutable decision history; immutable owner disclosure audit and bridge audit",
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
