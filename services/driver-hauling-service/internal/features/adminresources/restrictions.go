package adminresources

import "cosmicforge/logistics/shared/go/adminowner"

// Only native provider/asset subjects with live authoritative organization
// links are supported. The ledger's free-text subjects for other authorities
// are not enough to invent a local owner relationship or a Fleet tenant.
func restrictionSource() adminowner.SQLSpec {
	const provider = `SELECT jsonb_build_object('provider',p.id,'membership',l.membership_id,'organization',l.organization_id,'state',l.state,'version',l.version)
 FROM truck_providers p JOIN hauling_organization_provider_links l ON l.provider_id=p.id
 WHERE p.id::text=t.subject_reference AND l.organization_id::text=t.organization_id AND l.state='ACTIVE'`
	const truck = `SELECT jsonb_build_object('truck',v.id,'provider',v.provider_id,'membership',p.membership_id,'organization',l.organization_id,'truck_link_version',l.version,'provider_link_version',p.version)
 FROM trucks v JOIN truck_providers owner ON owner.id=v.provider_id
 JOIN hauling_organization_truck_links l ON l.truck_id=v.id
 JOIN hauling_organization_provider_links p ON p.provider_id=v.provider_id AND p.organization_id=l.organization_id
 WHERE v.id::text=t.subject_reference AND l.organization_id::text=t.organization_id AND l.state='ACTIVE' AND p.state='ACTIVE'`
	binding := `CASE WHEN t.subject_type='PROVIDER' THEN (` + provider + `) WHEN t.subject_type='ASSET' THEN (` + truck + `) ELSE NULL END`
	return adminowner.SQLSpec{Table: "hauling_authority_restrictions", StateColumn: "state", VersionColumn: "version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true,
		Predicate: `(` + binding + `) IS NOT NULL`, BindingExpression: `jsonb_build_object('subject_type',t.subject_type,'subject_reference',t.subject_reference,'organization',t.organization_id,'native_subject_link',` + binding + `)`,
		Dimensions: map[string]string{"subject_type": "subject_type", "reason": "reason_code"}, Counters: map[string]string{"source_revision": "source_version"}, References: map[string]string{"subject": "subject_reference"},
	}
}
