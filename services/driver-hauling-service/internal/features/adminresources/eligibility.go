package adminresources

import (
	"context"
	"cosmicforge/logistics/services/hauling-service/internal/features/truckcapability"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5"
	"time"
)

// This is a recorded product-admission observation, never a fresh assignment
// authorization. An asset's global admission can be investigated only through
// a currently active, matching native provider AND truck organization link.
func productEligibilitySource() adminowner.SQLSpec {
	const joins = `JOIN trucks v ON v.id=t.truck_id AND v.provider_id=t.provider_id
 JOIN truck_providers p ON p.id=t.provider_id
 JOIN hauling_organization_truck_links l ON l.truck_id=v.id AND l.state='ACTIVE'
 JOIN hauling_organization_provider_links m ON m.provider_id=p.id AND m.organization_id=l.organization_id AND m.state='ACTIVE'`
	const ref = `t.id::text||':'||l.organization_id::text`
	return adminowner.SQLSpec{Table: "hauling_truck_product_eligibility", JoinClause: joins, ReferenceExpression: ref, StateColumn: "status",
		CanonicalQuery:    `SELECT '','',l.organization_id::text,l.organization_id::text FROM hauling_truck_product_eligibility t ` + joins + ` WHERE ` + ref + `=$1`,
		BindingExpression: `jsonb_build_object('eligibility',t.id,'product',t.product_code,'truck',v.id,'truck_owner',v.provider_id,'provider',p.id,'organization',l.organization_id,'truck_link_revision',l.version,'provider_link_revision',m.version,'membership',m.membership_id)`,
		DimensionQuery: `SELECT COALESCE((SELECT jsonb_build_object('effective_at_utc',to_char(t.effective_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
   'expires_at_utc',to_char(t.expires_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
   'inspection_reference_recorded',NULLIF(v.inspection_ref,'')IS NOT NULL,
   'certification_reference_recorded',NULLIF(t.certification_ref,'')IS NOT NULL)
	   FROM hauling_truck_product_eligibility t ` + joins + ` WHERE ` + ref + `=$1),'{}'::jsonb)`,
		Dimensions: map[string]string{"product": "product_code", "policy_revision": "policy_version", "effective_window": "", "effective_at_utc": "", "expires_at_utc": "", "record_window_usable": "", "inspection_reference_recorded": "", "certification_reference_recorded": ""},
		Counters:   map[string]string{"authority_source_revision": "source_version"}, References: map[string]string{"provider": "provider_id", "truck": "truck_id"},
	}
}

func (s *haulingReadSource) Read(ctx context.Context, tx pgx.Tx, kind, ref string, detail bool) (adminowner.Resource, error) {
	resource, err := s.SQLSource.Read(ctx, tx, kind, ref, detail)
	if err == nil && kind == "booking-cargo-inputs" {
		return enrichBookingCargoInputs(resource)
	}
	if err != nil || kind != "product-eligibility" {
		return resource, err
	}
	spec := s.Specs[kind]
	var native truckcapability.ProductEligibility
	if err := tx.QueryRow(ctx, `SELECT t.status,t.effective_at,t.expires_at FROM `+spec.Table+` t `+spec.JoinClause+` WHERE `+spec.ReferenceExpression+`=$1`, ref).Scan(&native.Status, &native.EffectiveAt, &native.ExpiresAt); err != nil {
		return adminowner.Resource{}, err
	}
	now := time.Now().UTC()
	usable := native.UsableAt(now)
	window := "WITHIN_RECORDED_WINDOW"
	switch {
	case native.ExpiresAt != nil && !now.Before(*native.ExpiresAt):
		window = "EXPIRED"
	case native.EffectiveAt != nil && now.Before(*native.EffectiveAt):
		window = "NOT_YET_EFFECTIVE"
	case native.EffectiveAt == nil && native.ExpiresAt == nil:
		window = "UNBOUNDED_RECORDED_WINDOW"
	case native.EffectiveAt == nil:
		window = "NO_RECORDED_START"
	case native.ExpiresAt == nil:
		window = "NO_RECORDED_EXPIRY"
	}
	for i := range resource.Dimensions {
		switch resource.Dimensions[i].Name {
		case "record_window_usable":
			resource.Dimensions[i].Flag = &usable
		case "effective_window":
			resource.Dimensions[i].Value = &window
		}
	}
	// The SQL resource hash precedes native temporal enrichment. Recompute from
	// the complete allowlist so expiry and callback changes cannot retain a tag.
	resource.SnapshotETag = ""
	resource.SnapshotETag = admincontext.Fingerprint(resource)
	return resource, nil
}
