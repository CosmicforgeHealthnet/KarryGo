package adminresources

import "cosmicforge/logistics/shared/go/adminowner"

// Match decisions are historical operator evidence. Captured capability and
// policy revisions are not a full row mutation version or current eligibility.
func fitDecisionSource() adminowner.SQLSpec {
	const parent = `(NULLIF(b.corporate_organization_id,'') IS NULL OR b.winner_organization_id IS NULL OR b.corporate_organization_id=b.winner_organization_id::text)
 AND (t.truck_id IS NULL OR v.provider_id=t.provider_id)
 AND (t.mission_ref='' OR EXISTS(SELECT 1 FROM commerce_missions m WHERE m.booking_id=b.id AND m.mission_ref=t.mission_ref))`
	const joins = `JOIN haulage_bookings b ON b.id=t.booking_id JOIN truck_providers p ON p.id=t.provider_id LEFT JOIN trucks v ON v.id=t.truck_id`
	return adminowner.SQLSpec{Table: "hauling_match_decisions", StateColumn: "decision", UpdatedAtExpression: "t.created_at", JoinClause: joins, Predicate: parent,
		CanonicalQuery:    `SELECT '', '', COALESCE(NULLIF(b.corporate_organization_id,''),b.winner_organization_id::text,''), COALESCE(NULLIF(b.corporate_organization_id,''),b.winner_organization_id::text,'') FROM hauling_match_decisions t ` + joins + ` WHERE t.id::text=$1 AND ` + parent,
		BindingExpression: `jsonb_build_object('booking',b.id,'customer',b.customer_id,'planning_organization',b.corporate_organization_id,'winner_organization',b.winner_organization_id,'execution_mode',b.execution_mode,'provider',p.id,'truck',v.id,'truck_owner',v.provider_id,'mission',t.mission_ref)`,
		Dimensions:        map[string]string{"shadow": "shadow", "policy_revision": "policy_version"}, Counters: map[string]string{"captured_capability_revision": "capability_version"}, References: map[string]string{"booking": "booking_id", "provider": "provider_id", "truck": "truck_id", "mission": "mission_ref"},
	}
}
