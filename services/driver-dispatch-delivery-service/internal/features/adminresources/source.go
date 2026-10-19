package adminresources

import (
	"context"
	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Owner = "driver-dispatch-delivery-service"

func Source() *adminowner.SQLSource {
	return adminowner.NewSQLSource(Owner, map[string]adminowner.SQLSpec{
		"incidents":           tripScoped(adminowner.SQLSpec{Table: "dispatch_delivery_incidents", StateColumn: "state", Dimensions: map[string]string{"reason": "reason_code", "replacement_strategy": "replacement_strategy", "recovery_result": "recovery_result"}, Counters: map[string]string{"recovery_attempts": "recovery_attempts"}, References: map[string]string{"trip": "trip_id", "original_provider": "provider_id", "replacement_provider": "replacement_provider_id"}, BindingColumns: []string{"trip_id", "provider_id"}}, "trip_id", ""),
		"delivery-recoveries": tripScoped(adminowner.SQLSpec{Table: "dispatch_delivery_recovery_actions", StateColumn: "state", Dimensions: map[string]string{"action": "action"}, References: map[string]string{"trip": "trip_id", "delivery_attempt": "delivery_attempt_id"}, BindingColumns: []string{"trip_id", "delivery_attempt_id", "booking_id", "customer_id"}}, "trip_id", "t.booking_id=p.booking_id AND t.customer_id=p.customer_id AND EXISTS (SELECT 1 FROM dispatch_delivery_attempts a WHERE a.id=t.delivery_attempt_id AND a.trip_id=p.id)"),
		"returns":             tripScoped(adminowner.SQLSpec{Table: "dispatch_return_workflows", StateColumn: "state", References: map[string]string{"trip": "original_trip_id", "recovery_action": "recovery_action_id", "return_mission": "return_mission_id", "return_booking": "return_booking_id"}, BindingColumns: []string{"original_trip_id", "recovery_action_id", "return_mission_id", "return_booking_id"}}, "original_trip_id", "EXISTS (SELECT 1 FROM dispatch_delivery_recovery_actions a WHERE a.id=t.recovery_action_id AND a.trip_id=p.id AND a.booking_id=p.booking_id AND a.customer_id=p.customer_id AND EXISTS (SELECT 1 FROM dispatch_delivery_attempts d WHERE d.id=a.delivery_attempt_id AND d.trip_id=p.id))"),
		"riders":              {Table: "providers", DimensionQuery: `SELECT jsonb_build_object('availability',(SELECT status FROM provider_availability WHERE provider_id::text=$1),'vehicle_verification',(SELECT status FROM verification_steps WHERE provider_id::text=$1 AND step='vehicle'),'license_expiry',(SELECT licence_expiry_date::text FROM verification_steps WHERE provider_id::text=$1 AND step='licence'))`, StateColumn: "verification_status", Dimensions: map[string]string{"account": "is_active", "onboarding": "onboarding_complete", "identity_verification": "verification_status", "vehicle_verification": "", "document_compliance": "", "eligibility": "", "availability": "", "license_expiry": ""}},
		"motorcycles":         {Table: "bikes", StateColumn: "verification_status", BindingColumns: []string{"provider_id"}, Dimensions: map[string]string{"verification": "verification_status", "document_compliance": "", "eligibility": "", "license_expiry": "", "inspection": ""}, Counters: map[string]string{"capacity_kg": ""}, References: map[string]string{"provider": "provider_id"}},
		"missions":            {Table: "trips", StateColumn: "status", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"customer_id", "organization_id", "execution_source", "execution_mode"}, Dimensions: map[string]string{"mode": "execution_mode", "source_context": "execution_source", "execution": "status", "custody": "", "handover": "", "return": ""}, References: map[string]string{"provider": "provider_id", "vehicle": "", "fleet_assignment": "fleet_assignment_id", "fleet_response": "provider_response_id"}, TimelineTable: "trip_state_log", TimelineParentColumn: "trip_id", TimelineTypeColumn: "to_status", TimelineTimeColumn: "changed_at"},
		"custody-missions":    {Table: "custody_mission_state", BindingExpression: `jsonb_build_object('tenant',t.tenant_ref,'source_tenant',m.tenant_ref,'authority',m.authority,'custody_mode',m.custody_mode)`, VersionColumn: "version", ReferenceExpression: "t.mission_ref", JoinClause: "JOIN custody_missions m ON m.mission_ref=t.mission_ref", CreatedAtExpression: "m.created_at", StateColumn: "status", TenantColumn: "tenant_ref", Predicate: "t.tenant_ref=m.tenant_ref", BindingColumns: []string{"tenant_ref"}, Dimensions: map[string]string{"phase": "status", "handover": "", "return": ""}, References: map[string]string{"mission": "mission_ref"}, TimelineTable: "custody_operations", TimelineParentColumn: "mission_ref", TimelineTypeColumn: "operation_type", TimelineRefExpression: "e.operation_ref", TimelineTimeColumn: "created_at"},
	})
}
func Runtime(ctx context.Context, db *pgxpool.Pool) (*adminowner.Handler, error) {
	profile, _ := adminowner.ForOwner(Owner)
	return adminowner.FromEnvironment(ctx, db, Source(), profile.Definitions, Owner, profile.Prefix, profile.Path)
}

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
