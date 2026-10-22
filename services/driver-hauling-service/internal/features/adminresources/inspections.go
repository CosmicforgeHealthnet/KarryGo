package adminresources

import "cosmicforge/logistics/shared/go/adminowner"

// Hauling-owned inspection cases. The case is visible only while the truck is
// still owned by the recorded provider and still bound to the recorded
// organization through ACTIVE truck and provider links; a revoked link or a
// truck transferred elsewhere withdraws the case (and its evidence) at once.
// Dates, kind and conclusion are bounded tokens; no free text, no Media URL.
func inspectionSource() adminowner.SQLSpec {
	const binding = `SELECT jsonb_build_object('truck',v.id,'provider',v.provider_id,'organization',l.organization_id,'truck_link_version',l.version,'provider_link_version',p.version,'membership',p.membership_id)
 FROM hauling_organization_truck_links l
 JOIN hauling_organization_provider_links p ON p.provider_id=v.provider_id AND p.organization_id=l.organization_id AND p.state='ACTIVE'
 WHERE l.truck_id=v.id AND l.organization_id=t.organization_id AND l.state='ACTIVE'`
	return adminowner.SQLSpec{Table: "hauling_admin_inspections", JoinClause: `JOIN trucks v ON v.id=t.truck_id AND v.provider_id=t.provider_id`,
		StateColumn: "state", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true,
		Predicate:         `(` + binding + `) IS NOT NULL`,
		BindingExpression: `jsonb_build_object('inspection',t.id,'truck',t.truck_id,'provider',t.provider_id,'organization',t.organization_id,'kind',t.inspection_kind,'native_subject_link',(` + binding + `))`,
		Dimensions:        map[string]string{"inspection_kind": "inspection_kind", "conclusion": "conclusion", "performed_on": "performed_on", "valid_until": "valid_until"},
		Counters:          map[string]string{"evidence_count": "evidence_count"},
		References:        map[string]string{"truck": "truck_id", "provider": "provider_id"},
		TimelineTable:     "hauling_admin_inspection_history", TimelineParentColumn: "inspection_id", TimelineTypeColumn: "action", TimelineTimeColumn: "recorded_at",
	}
}
