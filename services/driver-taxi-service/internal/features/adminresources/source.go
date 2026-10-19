package adminresources

import (
	"context"
	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Owner = "driver-taxi-service"

func Source() *adminowner.SQLSource {
	return adminowner.NewSQLSource(Owner, map[string]adminowner.SQLSpec{
		"providers":         {Table: "taxi_providers", DimensionQuery: `SELECT jsonb_build_object('availability',(SELECT state FROM provider_operational_state WHERE provider_id::text=$1))`, StateColumn: "account_status", Dimensions: map[string]string{"account": "account_status", "onboarding": "onboarding_status", "identity_verification": "identity_verification_status", "vehicle_verification": "vehicle_verification_status", "document_compliance": "document_compliance_status", "eligibility": "", "availability": "", "license_expiry": "license_expiry_date"}},
		"vehicles":          {Table: "vehicles", StateColumn: "status", BindingColumns: []string{"provider_id"}, Dimensions: map[string]string{"verification": "", "document_compliance": "", "eligibility": "", "license_expiry": "", "inspection": ""}, Counters: map[string]string{"capacity_kg": "", "seat_capacity": "seats"}, References: map[string]string{"provider": "provider_id"}},
		"independent-rides": {Table: "taxi_bookings", StateColumn: "status", Predicate: "NOT t.is_corporate_managed AND t.organization_id IS NULL", BindingColumns: []string{"customer_id", "market_id", "operating_zone_id", "organization_id", "source_context"}, CanonicalQuery: `SELECT m.code,z.code,'','' FROM taxi_bookings b JOIN markets m ON m.id=b.market_id JOIN operating_zones z ON z.id=b.operating_zone_id AND z.market_id=b.market_id WHERE b.id::text=$1 AND NOT b.is_corporate_managed AND b.organization_id IS NULL`, Dimensions: map[string]string{"mode": "mode", "source_context": "source_context", "execution": "status", "custody": "", "handover": "", "return": ""}, References: map[string]string{"provider": "provider_id", "vehicle": "vehicle_id"}, TimelineTable: "booking_events", TimelineParentColumn: "booking_id", TimelineTypeColumn: "event_type"},
	})
}
func Runtime(ctx context.Context, db *pgxpool.Pool) (*adminowner.Handler, error) {
	profile, _ := adminowner.ForOwner(Owner)
	return adminowner.FromEnvironment(ctx, db, Source(), profile.Definitions, Owner, profile.Prefix, profile.Path)
}
