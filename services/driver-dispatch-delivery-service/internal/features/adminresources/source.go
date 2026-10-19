package adminresources

import (
	"context"
	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Owner = "driver-dispatch-delivery-service"

func Source() *adminowner.SQLSource {
	return adminowner.NewSQLSource(Owner, map[string]adminowner.SQLSpec{
		"riders":           {Table: "providers", DimensionQuery: `SELECT jsonb_build_object('availability',(SELECT status FROM provider_availability WHERE provider_id::text=$1),'vehicle_verification',(SELECT status FROM verification_steps WHERE provider_id::text=$1 AND step='vehicle'),'license_expiry',(SELECT licence_expiry_date::text FROM verification_steps WHERE provider_id::text=$1 AND step='licence'))`, StateColumn: "verification_status", Dimensions: map[string]string{"account": "is_active", "onboarding": "onboarding_complete", "identity_verification": "verification_status", "vehicle_verification": "", "document_compliance": "", "eligibility": "", "availability": "", "license_expiry": ""}},
		"motorcycles":      {Table: "bikes", StateColumn: "verification_status", BindingColumns: []string{"provider_id"}, Dimensions: map[string]string{"verification": "verification_status", "document_compliance": "", "eligibility": "", "license_expiry": "", "inspection": ""}, Counters: map[string]string{"capacity_kg": ""}, References: map[string]string{"provider": "provider_id"}},
		"missions":         {Table: "trips", StateColumn: "status", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"customer_id", "organization_id", "execution_source", "execution_mode"}, Dimensions: map[string]string{"mode": "execution_mode", "source_context": "execution_source", "execution": "status", "custody": "", "handover": "", "return": ""}, References: map[string]string{"provider": "provider_id", "vehicle": "", "fleet_assignment": "fleet_assignment_id", "fleet_response": "provider_response_id"}, TimelineTable: "trip_state_log", TimelineParentColumn: "trip_id", TimelineTypeColumn: "to_status", TimelineTimeColumn: "changed_at"},
		"custody-missions": {Table: "custody_mission_state", VersionColumn: "version", ReferenceExpression: "t.mission_ref", JoinClause: "JOIN custody_missions m ON m.mission_ref=t.mission_ref", CreatedAtExpression: "m.created_at", StateColumn: "status", TenantColumn: "tenant_ref", Predicate: "t.tenant_ref=m.tenant_ref", BindingColumns: []string{"tenant_ref"}, Dimensions: map[string]string{"phase": "status", "handover": "", "return": ""}, References: map[string]string{"mission": "mission_ref"}, TimelineTable: "custody_operations", TimelineParentColumn: "mission_ref", TimelineTypeColumn: "operation_type", TimelineRefExpression: "e.operation_ref", TimelineTimeColumn: "created_at"},
	})
}
func Runtime(ctx context.Context, db *pgxpool.Pool) (*adminowner.Handler, error) {
	profile, _ := adminowner.ForOwner(Owner)
	return adminowner.FromEnvironment(ctx, db, Source(), profile.Definitions, Owner, profile.Prefix, profile.Path)
}
