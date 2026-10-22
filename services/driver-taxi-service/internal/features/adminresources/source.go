package adminresources

import (
	"context"
	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5/pgxpool"
)

const Owner = "driver-taxi-service"

func Source() *adminowner.SQLSource {
	return adminowner.NewSQLSource(Owner, map[string]adminowner.SQLSpec{
		"recoveries":        {Table: "taxi_recovery_cases", StateColumn: "state", VersionColumn: "version", CreatedAtExpression: "t.detected_at", UpdatedAtExpression: "GREATEST(t.detected_at,t.resolved_at,(t.audit_json->-1->>'at')::timestamptz)", JoinClause: taxiRecoveryJoin, Predicate: taxiRecoveryParentPredicate, CanonicalQuery: `SELECT COALESCE(m.code,''),COALESCE(z.code,''),COALESCE(b.organization_id,''),COALESCE(b.organization_id,'') FROM taxi_recovery_cases t ` + taxiRecoveryJoin + ` WHERE t.id::text=$1 AND ` + taxiRecoveryParentPredicate, BindingExpression: `jsonb_build_object('subject_type',t.subject_type,'booking',b.id,'booking_customer',b.customer_id,'market',b.market_id,'zone',b.operating_zone_id,'organization',b.organization_id,'corporate',b.is_corporate_managed,'booking_source',b.source_context,'goods',g.id,'goods_customer',g.customer_ref,'goods_source',g.source_service,'mission_ref',g.mission_ref,'quote',g.quote_id,'plan_ref',g.plan_ref)`, Dimensions: map[string]string{"subject_type": "subject_type", "trigger": "trigger_code", "custody_transfer_required": "custody_transfer_required"}, References: map[string]string{"booking": "booking_id", "goods_mission": "goods_mission_id", "original_provider": "original_provider_id", "original_vehicle": "original_vehicle_id", "replacement_provider": "replacement_provider_id", "replacement_vehicle": "replacement_vehicle_id"}},
		"providers":         {Table: "taxi_providers", DimensionQuery: `SELECT jsonb_build_object('availability',(SELECT state FROM provider_operational_state WHERE provider_id::text=$1))`, StateColumn: "account_status", Dimensions: map[string]string{"account": "account_status", "onboarding": "onboarding_status", "identity_verification": "identity_verification_status", "vehicle_verification": "vehicle_verification_status", "document_compliance": "document_compliance_status", "eligibility": "", "availability": "", "license_expiry": "license_expiry_date"}},
		"vehicles":          {Table: "vehicles", StateColumn: "status", BindingColumns: []string{"provider_id"}, Dimensions: map[string]string{"verification": "", "document_compliance": "", "eligibility": "", "license_expiry": "", "inspection": ""}, Counters: map[string]string{"capacity_kg": "", "seat_capacity": "seats"}, References: map[string]string{"provider": "provider_id"}},
		"incidents":         {Table: "taxi_admin_incidents", ReferenceExpression: "t.case_ref", StateColumn: "state", VersionColumn: "version", CreatedAtExpression: "t.opened_at", UpdatedAtExpression: "t.updated_at", Predicate: taxiIncidentPredicate, CanonicalQuery: `SELECT COALESCE(m.code,''),COALESCE(z.code,''),'','' FROM taxi_admin_incidents t LEFT JOIN taxi_bookings b ON b.id::text=t.booking_id LEFT JOIN markets m ON m.id=b.market_id LEFT JOIN operating_zones z ON z.id=b.operating_zone_id AND z.market_id=b.market_id WHERE t.case_ref=$1 AND ` + taxiIncidentPredicate, BindingColumns: []string{"subject_kind", "subject_id"}, Dimensions: map[string]string{"subject_type": "subject_kind", "category": "category", "severity": "severity"}, References: map[string]string{"provider": "provider_id", "vehicle": "vehicle_id", "booking": "booking_id"}},
		"independent-rides": {Table: "taxi_bookings", StateColumn: "status", Predicate: "NOT t.is_corporate_managed AND t.organization_id IS NULL", BindingColumns: []string{"customer_id", "market_id", "operating_zone_id", "organization_id", "source_context"}, CanonicalQuery: `SELECT m.code,z.code,'','' FROM taxi_bookings b JOIN markets m ON m.id=b.market_id JOIN operating_zones z ON z.id=b.operating_zone_id AND z.market_id=b.market_id WHERE b.id::text=$1 AND NOT b.is_corporate_managed AND b.organization_id IS NULL`, Dimensions: map[string]string{"mode": "mode", "source_context": "source_context", "execution": "status", "custody": "", "handover": "", "return": ""}, References: map[string]string{"provider": "provider_id", "vehicle": "vehicle_id"}, TimelineTable: "booking_events", TimelineParentColumn: "booking_id", TimelineTypeColumn: "event_type"},
	})
}

// Passenger scope follows the native booking and actual zone-to-market FK.
// Goods missions have no canonical organization/tenant/geography columns; quote
// locations, route snapshots and vendor/customer strings are not authorization.
// A separate owner-approved allocation must resolve every unknown dimension.
const taxiRecoveryJoin = `LEFT JOIN taxi_bookings b ON b.id=t.booking_id LEFT JOIN markets m ON m.id=b.market_id LEFT JOIN operating_zones z ON z.id=b.operating_zone_id AND z.market_id=b.market_id LEFT JOIN taxi_goods_missions g ON g.id=t.goods_mission_id`
const taxiRecoveryParentPredicate = `((t.subject_type='BOOKING' AND b.id IS NOT NULL AND t.goods_mission_id IS NULL AND m.id IS NOT NULL AND z.id IS NOT NULL) OR (t.subject_type='GOODS_MISSION' AND g.id IS NOT NULL AND t.booking_id IS NULL))`

// Administrative incidents (Section 8) take passenger scope only from an
// independent ride subject's native booking (same market/zone FK join as
// independent-rides); provider and vehicle incidents carry no inferred
// geography or tenant and need an owner-approved allocation.
const taxiIncidentPredicate = `(t.booking_id IS NULL OR EXISTS(SELECT 1 FROM taxi_bookings ib JOIN markets im ON im.id=ib.market_id JOIN operating_zones iz ON iz.id=ib.operating_zone_id AND iz.market_id=ib.market_id WHERE ib.id::text=t.booking_id AND NOT ib.is_corporate_managed AND ib.organization_id IS NULL))`

func Runtime(ctx context.Context, db *pgxpool.Pool) (*adminowner.Handler, error) {
	profile, _ := adminowner.ForOwner(Owner)
	return adminowner.FromEnvironment(ctx, db, Source(), profile.Definitions, Owner, profile.Prefix, profile.Path)
}
