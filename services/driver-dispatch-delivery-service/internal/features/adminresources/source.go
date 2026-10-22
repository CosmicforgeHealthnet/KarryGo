package adminresources

import (
	"context"
	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Owner = "driver-dispatch-delivery-service"

func Source() *adminowner.SQLSource {
	return adminowner.NewSQLSource(Owner, map[string]adminowner.SQLSpec{
		"incident-recovery-status": incidentRecoveryStatusSource(),
		"return-execution-status":  returnExecutionStatusSource(),
		"handover-states":          handoverStates(),
		"incidents":                tripScoped(adminowner.SQLSpec{Table: "dispatch_delivery_incidents", StateColumn: "state", Dimensions: map[string]string{"reason": "reason_code", "replacement_strategy": "replacement_strategy", "recovery_result": "recovery_result"}, Counters: map[string]string{"recovery_attempts": "recovery_attempts"}, References: map[string]string{"trip": "trip_id", "original_provider": "provider_id", "replacement_provider": "replacement_provider_id"}, BindingColumns: []string{"trip_id", "provider_id"}}, "trip_id", ""),
		"delivery-recoveries":      tripScoped(adminowner.SQLSpec{Table: "dispatch_delivery_recovery_actions", StateColumn: "state", Dimensions: map[string]string{"action": "action"}, References: map[string]string{"trip": "trip_id", "delivery_attempt": "delivery_attempt_id"}, BindingColumns: []string{"trip_id", "delivery_attempt_id", "booking_id", "customer_id"}}, "trip_id", "t.booking_id=p.booking_id AND t.customer_id=p.customer_id AND EXISTS (SELECT 1 FROM dispatch_delivery_attempts a WHERE a.id=t.delivery_attempt_id AND a.trip_id=p.id)"),
		"returns":                  tripScoped(adminowner.SQLSpec{Table: "dispatch_return_workflows", StateColumn: "state", References: map[string]string{"trip": "original_trip_id", "recovery_action": "recovery_action_id", "return_mission": "return_mission_id", "return_booking": "return_booking_id"}, BindingColumns: []string{"original_trip_id", "recovery_action_id", "return_mission_id", "return_booking_id"}}, "original_trip_id", returnParentPredicate),
		"riders":                   {Table: "providers", DimensionQuery: `SELECT jsonb_build_object('availability',(SELECT status FROM provider_availability WHERE provider_id::text=$1),'vehicle_verification',(SELECT status FROM verification_steps WHERE provider_id::text=$1 AND step='vehicle'),'license_expiry',(SELECT licence_expiry_date::text FROM verification_steps WHERE provider_id::text=$1 AND step='licence'))`, StateColumn: "verification_status", Dimensions: map[string]string{"account": "is_active", "onboarding": "onboarding_complete", "identity_verification": "verification_status", "vehicle_verification": "", "document_compliance": "", "eligibility": "", "availability": "", "license_expiry": ""}},
		"motorcycles":              {Table: "bikes", StateColumn: "verification_status", BindingColumns: []string{"provider_id"}, Dimensions: map[string]string{"verification": "verification_status", "document_compliance": "", "eligibility": "", "license_expiry": "", "inspection": ""}, Counters: map[string]string{"capacity_kg": ""}, References: map[string]string{"provider": "provider_id"}},
		"missions":                 {Table: "trips", StateColumn: "status", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"customer_id", "organization_id", "execution_source", "execution_mode"}, Dimensions: map[string]string{"mode": "execution_mode", "source_context": "execution_source", "execution": "status", "custody": "", "handover": "", "return": ""}, References: map[string]string{"provider": "provider_id", "vehicle": "", "fleet_assignment": "fleet_assignment_id", "fleet_response": "provider_response_id"}, TimelineTable: "trip_state_log", TimelineParentColumn: "trip_id", TimelineTypeColumn: "to_status", TimelineTimeColumn: "changed_at"},
		// Customer bookings, including unmatched ones that never get a trip.
		// The row records no geography or tenant, exactly like a consumer trip
		// without an organization: every dimension comes from an owner-signed
		// context bound to the booking identity and customer.
		"bookings": {Table: "dispatch_bookings", StateColumn: "status", BindingColumns: []string{"customer_id"}, Dimensions: map[string]string{"execution": "status", "service_tier": "service_tier", "package_size": "package_size"}, References: map[string]string{"provider": "provider_id"}},
		// Assignment-queue delivery missions. Organization/tenant only where
		// the mission's own delivery group records one.
		"delivery-missions": {Table: "dispatch_delivery_missions", JoinClause: deliveryGroupJoin, StateColumn: "state", CanonicalQuery: `SELECT '','',COALESCE(g.organization_id::text,''),COALESCE(g.organization_id::text,'') FROM dispatch_delivery_missions t ` + deliveryGroupJoin + ` WHERE t.id::text=$1`, BindingExpression: `jsonb_build_object('group',g.id,'group_source_type',g.source_type,'group_source_ref',g.source_ref,'group_customer',g.customer_id,'group_partner',g.partner_id,'group_organization',g.organization_id,'native_links',(SELECT jsonb_object_agg(k,to_jsonb(t)->k) FROM unnest($7::text[]) k))`, BindingColumns: []string{"delivery_group_id", "source_type", "source_ref", "source_version"}, Dimensions: map[string]string{"execution": "state", "source_type": "source_type"}, References: map[string]string{"booking": "booking_id", "trip": "trip_id"}},
		// Vendor commerce missions, identified by their canonical reference
		// (the aggregate id of their own events). Vendor order references are
		// Vendor-owned and never become tenant authority here.
		"commerce-missions": {Table: "commerce_missions", ReferenceExpression: "t.canonical_ref", StateColumn: "runtime_state", BindingColumns: []string{"customer_id", "caller_service", "operation_ref", "mission_ref", "parent_order_ref"}, Dimensions: map[string]string{"decision": "status", "runtime": "runtime_state"}, References: map[string]string{"booking": "booking_id", "trip": "trip_id"}},
		// Partner-API requests: the broadcast Dispatch records for a request
		// with no dispatch_bookings row, identified by the partner's booking
		// id (the analytics subject). The partner payload is never projected.
		"partner-requests": {Table: "request_broadcasts", ReferenceExpression: "t.booking_id::text", StateColumn: "status", Predicate: "NOT EXISTS (SELECT 1 FROM dispatch_bookings b WHERE b.id=t.booking_id)", BindingColumns: []string{"booking_id", "service_type"}, Dimensions: map[string]string{"execution": "status", "service_type": "service_type"}, References: map[string]string{"provider": "accepted_by_provider_id"}},
		"custody-missions": {Table: "custody_mission_state", BindingExpression: `jsonb_build_object('tenant',t.tenant_ref,'source_tenant',m.tenant_ref,'authority',m.authority,'custody_mode',m.custody_mode)`, VersionColumn: "version", ReferenceExpression: "t.mission_ref", JoinClause: "JOIN custody_missions m ON m.mission_ref=t.mission_ref", CreatedAtExpression: "m.created_at", StateColumn: "status", TenantColumn: "tenant_ref", Predicate: "t.tenant_ref=m.tenant_ref", BindingColumns: []string{"tenant_ref"}, Dimensions: map[string]string{"phase": "status", "handover": "", "return": ""}, References: map[string]string{"mission": "mission_ref"}, TimelineTable: "custody_operations", TimelineParentColumn: "mission_ref", TimelineTypeColumn: "operation_type", TimelineRefExpression: "e.operation_ref", TimelineTimeColumn: "created_at"},
	})
}
func Runtime(ctx context.Context, db *pgxpool.Pool) (*adminowner.Handler, error) {
	profile, _ := adminowner.ForOwner(Owner)
	return adminowner.FromEnvironment(ctx, db, Source(), profile.Definitions, Owner, profile.Prefix, profile.Path)
}

const deliveryGroupJoin = "JOIN dispatch_delivery_groups g ON g.id=t.delivery_group_id"

// Scope follows the actual immutable workflow parent, never a supplied tenant.
// Historical source_version/expected_trip_version fields describe a captured
// Trip version, not every workflow mutation, so SourceVersion remains unknown.
func tripScoped(spec adminowner.SQLSpec, parentColumn, predicate string) adminowner.SQLSpec {
	spec.JoinClause = "JOIN trips p ON p.id=t." + parentColumn
	spec.Predicate = predicate
	spec.CanonicalQuery = "SELECT '', '', COALESCE(p.organization_id::text,''), COALESCE(p.organization_id::text,'') FROM " + spec.Table + " t " + spec.JoinClause + " WHERE t.id::text=$1"
	if predicate != "" {
		spec.CanonicalQuery += " AND (" + predicate + ")"
	}
	spec.BindingExpression = "jsonb_build_object('parent_trip',p.id,'parent_customer',p.customer_id,'parent_organization',p.organization_id,'parent_source',p.execution_source,'native_links',(SELECT jsonb_object_agg(k,to_jsonb(t)->k) FROM unnest($7::text[]) k))"
	return spec
}
