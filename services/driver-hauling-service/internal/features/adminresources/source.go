package adminresources

import (
	"context"
	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Owner = "driver-hauling-service"

func Source() *adminowner.SQLSource {
	return adminowner.NewSQLSource(Owner, map[string]adminowner.SQLSpec{
		"providers": {Table: "truck_providers", DimensionQuery: `SELECT jsonb_build_object('availability',(SELECT state FROM hauling_provider_availability WHERE provider_id::text=$1))`, StateColumn: "status", Dimensions: map[string]string{"account": "status", "onboarding": "onboarding_status", "identity_verification": "", "vehicle_verification": "", "document_compliance": "", "eligibility": "", "availability": "", "license_expiry": "license_expiry_date"}},
		"trucks":    {Table: "trucks", StateColumn: "status", BindingColumns: []string{"provider_id"}, Dimensions: map[string]string{"verification": "", "document_compliance": "", "eligibility": "", "license_expiry": "", "inspection": ""}, Counters: map[string]string{"capacity_kg": "capacity_kg"}, References: map[string]string{"provider": "provider_id"}},
		"missions":  {Table: "haulage_bookings", StateColumn: "status", CanonicalQuery: `SELECT '','',COALESCE(NULLIF(corporate_organization_id,''),winner_organization_id::text,''),COALESCE(NULLIF(corporate_organization_id,''),winner_organization_id::text,'') FROM haulage_bookings WHERE id::text=$1 AND (NULLIF(corporate_organization_id,'') IS NULL OR winner_organization_id IS NULL OR corporate_organization_id=winner_organization_id::text)`, Predicate: `(NULLIF(t.corporate_organization_id,'') IS NULL OR t.winner_organization_id IS NULL OR t.corporate_organization_id=t.winner_organization_id::text)`, BindingColumns: []string{"customer_id", "corporate_organization_id", "winner_organization_id", "execution_mode"}, Dimensions: map[string]string{"mode": "execution_mode", "source_context": "", "execution": "status", "custody": "", "handover": "", "return": ""}, Counters: map[string]string{"cargo_weight_kg": "cargo_weight_kg"}, References: map[string]string{"provider": "provider_id", "vehicle": "truck_id", "fleet_assignment": "fleet_assignment_id", "fleet_response": "provider_response_id"}, TimelineTable: "booking_events", TimelineParentColumn: "booking_id", TimelineTypeColumn: "event_type"},
	})
}
func Runtime(ctx context.Context, db *pgxpool.Pool) (*adminowner.Handler, error) {
	profile, _ := adminowner.ForOwner(Owner)
	return adminowner.FromEnvironment(ctx, db, Source(), profile.Definitions, Owner, profile.Prefix, profile.Path)
}
