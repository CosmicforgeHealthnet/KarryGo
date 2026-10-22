package adminresources

import (
	"context"
	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Owner = "driver-hauling-service"

type haulingReadSource struct{ *adminowner.SQLSource }

func Source() *haulingReadSource {
	return &haulingReadSource{adminowner.NewSQLSource(Owner, map[string]adminowner.SQLSpec{
		"product-eligibility": productEligibilitySource(),
		"fit-decisions":       fitDecisionSource(),
		"restrictions":        restrictionSource(),
		"custody-missions":    {Table: "custody_mission_state", BindingExpression: `jsonb_build_object('tenant',t.tenant_ref,'source_tenant',m.tenant_ref,'authority',m.authority,'custody_mode',m.custody_mode)`, VersionColumn: "version", ReferenceExpression: "t.mission_ref", JoinClause: "JOIN custody_missions m ON m.mission_ref=t.mission_ref", CreatedAtExpression: "m.created_at", StateColumn: "status", TenantColumn: "tenant_ref", Predicate: "t.tenant_ref=m.tenant_ref", BindingColumns: []string{"tenant_ref"}, Dimensions: map[string]string{"phase": "status", "handover": "", "return": ""}, References: map[string]string{"mission": "mission_ref"}, TimelineTable: "custody_operations", TimelineParentColumn: "mission_ref", TimelineTypeColumn: "operation_type", TimelineRefExpression: "e.operation_ref", TimelineTimeColumn: "created_at"},
		"providers":           {Table: "truck_providers", DimensionQuery: `SELECT jsonb_build_object('availability',(SELECT state FROM hauling_provider_availability WHERE provider_id::text=$1))`, StateColumn: "status", Dimensions: map[string]string{"account": "status", "onboarding": "onboarding_status", "identity_verification": "", "vehicle_verification": "", "document_compliance": "", "eligibility": "", "availability": "", "license_expiry": "license_expiry_date"}},
		"trucks":              {Table: "trucks", StateColumn: "status", BindingColumns: []string{"provider_id"}, Dimensions: map[string]string{"verification": "", "document_compliance": "", "eligibility": "", "license_expiry": "", "inspection": "", "enclosed": "enclosed", "weather_protected": "weather_protected", "upright_safe": "upright_safe", "fragile_safe": "fragile_safe", "refrigerated": "refrigerated", "tail_lift": "tail_lift", "crane": "crane", "hydraulic": "hydraulic", "loading_ramp": "loading_ramp", "trailer_capable": "trailer_capable", "insurance_valid": "insurance_valid"}, Counters: map[string]string{"capacity_kg": "capacity_kg", "max_payload_grams": "max_payload_grams", "internal_length_mm": "internal_length_mm", "internal_width_mm": "internal_width_mm", "internal_height_mm": "internal_height_mm", "usable_volume_ml": "usable_volume_ml", "opening_width_mm": "opening_width_mm", "opening_height_mm": "opening_height_mm", "axle_count": "axle_count", "capability_version": "capability_version", "capability_source_version": "capability_source_version"}, References: map[string]string{"provider": "provider_id"}},
		"recoveries":          {Table: "hauling_recovery_cases", StateColumn: "state", VersionColumn: "version", CreatedAtExpression: "t.detected_at", UpdatedAtExpression: "COALESCE((t.audit_json->-1->>'at')::timestamptz,t.resolved_at,t.started_at,t.detected_at)", JoinClause: "JOIN haulage_bookings b ON b.id=t.booking_id", Predicate: haulingRecoveryParentPredicate, CanonicalQuery: `SELECT '', '', COALESCE(NULLIF(b.corporate_organization_id,''),b.winner_organization_id::text,''), COALESCE(NULLIF(b.corporate_organization_id,''),b.winner_organization_id::text,'') FROM hauling_recovery_cases t JOIN haulage_bookings b ON b.id=t.booking_id WHERE t.id::text=$1 AND ` + haulingRecoveryParentPredicate, BindingExpression: `jsonb_build_object('booking',b.id,'customer',b.customer_id,'planning_organization',b.corporate_organization_id,'winner_organization',b.winner_organization_id,'execution_mode',b.execution_mode,'mission',t.mission_ref)`, BindingColumns: []string{"booking_id", "mission_ref"}, Dimensions: map[string]string{"trigger": "trigger_code", "custody_transfer_required": "custody_transfer_required"}, References: map[string]string{"booking": "booking_id", "mission": "mission_ref", "original_provider": "original_provider_id", "original_truck": "original_truck_id", "replacement_provider": "replacement_provider_id", "replacement_truck": "replacement_truck_id"}},

		"missions": {Table: "haulage_bookings", StateColumn: "status", CanonicalQuery: `SELECT '','',COALESCE(NULLIF(corporate_organization_id,''),winner_organization_id::text,''),COALESCE(NULLIF(corporate_organization_id,''),winner_organization_id::text,'') FROM haulage_bookings WHERE id::text=$1 AND (NULLIF(corporate_organization_id,'') IS NULL OR winner_organization_id IS NULL OR corporate_organization_id=winner_organization_id::text)`, Predicate: `(NULLIF(t.corporate_organization_id,'') IS NULL OR t.winner_organization_id IS NULL OR t.corporate_organization_id=t.winner_organization_id::text)`, BindingColumns: []string{"customer_id", "corporate_organization_id", "winner_organization_id", "execution_mode"}, Dimensions: map[string]string{"mode": "execution_mode", "source_context": "", "execution": "status", "custody": "", "handover": "", "return": ""}, Counters: map[string]string{"cargo_weight_kg": "cargo_weight_kg"}, References: map[string]string{"provider": "provider_id", "vehicle": "truck_id", "fleet_assignment": "fleet_assignment_id", "fleet_response": "provider_response_id"}, TimelineTable: "booking_events", TimelineParentColumn: "booking_id", TimelineTypeColumn: "event_type"},
	})}
}
func Runtime(ctx context.Context, db *pgxpool.Pool) (*adminowner.Handler, error) {
	profile, _ := adminowner.ForOwner(Owner)
	return adminowner.FromEnvironment(ctx, db, Source(), profile.Definitions, Owner, profile.Prefix, profile.Path)
}

// Native commerce identity and planning tenant must agree. Settlement identity
// is not authorization, and recovery snapshots cannot fabricate mission links.
const haulingRecoveryParentPredicate = `(NULLIF(b.corporate_organization_id,'') IS NULL OR b.winner_organization_id IS NULL OR b.corporate_organization_id=b.winner_organization_id::text) AND ((t.mission_ref='' AND NOT EXISTS(SELECT 1 FROM commerce_missions m WHERE m.booking_id=b.id)) OR (t.mission_ref<>'' AND EXISTS(SELECT 1 FROM commerce_missions m WHERE m.booking_id=b.id AND m.mission_ref=t.mission_ref) AND NOT EXISTS(SELECT 1 FROM commerce_missions m WHERE m.booking_id=b.id AND m.mission_ref<>t.mission_ref)))`
