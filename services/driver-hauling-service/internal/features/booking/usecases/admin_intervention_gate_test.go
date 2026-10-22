package bookingusecases

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
	"cosmicforge/logistics/services/hauling-service/internal/features/admincommands"
	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/services/hauling-service/internal/features/restrictions"
	"cosmicforge/logistics/services/hauling-service/internal/features/truckcapability"
	"cosmicforge/logistics/shared/go/admincommand"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"github.com/google/uuid"
)

// realCapability is the matcher's capability authority over the real store.
type realCapability struct{ service *truckcapability.Service }

func (c realCapability) LoadProviderTrucks(ctx context.Context, ids []string, r cargofit.Requirement) (map[string][]cargofit.TruckCapability, error) {
	return c.service.LoadProviderTruckFits(ctx, ids, r)
}
func (realCapability) RecordDecisions(context.Context, []CapabilityDecision) error { return nil }
func (realCapability) PolicyVersion(context.Context) string                        { return "test" }
func (c realCapability) ResolveProduct(r cargofit.Requirement) (string, bool) {
	return c.service.ProductForRequirement(r)
}

// Admin Section 10 decisions are consumed by the REAL matcher chokepoint: an
// inspection FAIL restriction removes the truck, an UNSUITABLE decision or a
// pending remediation removes it for that booking, and a historical SUITABLE
// never keeps a truck that no longer fits the CURRENT booking inputs.
func TestPostgresAdminInterventionsGateRealMatching(t *testing.T) {
	db := fixture.Database(t, "../../../../migrations")
	ctx := context.Background()
	org := uuid.NewString()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(sql, err)
		}
	}
	seed := func() (string, string) {
		p, v := uuid.NewString(), uuid.NewString()
		exec(`INSERT INTO truck_providers(id,first_name,status) VALUES($1,'TEST_ONLY','active')`, p)
		exec(`INSERT INTO trucks(id,provider_id,truck_type,capacity_kg,plate_number,status,max_payload_grams) VALUES($1,$2,'flatbed',20000,$3,'active',20000000)`, v, p, "G-"+v[:8])
		exec(`INSERT INTO hauling_organization_provider_links(organization_id,provider_id,membership_id,state) VALUES($1,$2,$3,'ACTIVE')`, org, p, uuid.NewString())
		exec(`INSERT INTO hauling_organization_truck_links(organization_id,truck_id,state) VALUES($1,$2,'ACTIVE')`, org, v)
		return p, v
	}
	p1, t1 := seed()
	p2, t2 := seed()
	p3, t3 := seed()
	bookingID := uuid.NewString()
	exec(`INSERT INTO haulage_bookings(id,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,cargo_type,cargo_weight_kg,status,preferred_truck_type,corporate_organization_id) VALUES($1,gen_random_uuid(),'A',6.52,3.37,'B',6.43,3.42,'general',5000,'pending_match','flatbed',$2)`, bookingID, org)

	capability := truckcapability.New(truckcapability.Options{Repository: truckcapability.NewRepository(db)})
	executor := admincommands.New(capability, nil)
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: org, Tenant: org}
	apply := func(kind, id, action, caseRef string, expected int64, params map[string]string) {
		t.Helper()
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		in := admincommand.Execution{Command: admincommand.Command{SubjectKind: kind, SubjectID: id, CaseRef: caseRef, Action: action, ExpectedVersion: expected, Params: params}, DecisionID: uuid.NewString(), Scope: scope, Now: time.Now()}
		if _, err := executor.Apply(ctx, tx, in); err != nil {
			t.Fatal(action, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	service := &BookingService{
		availability:       rejectionStore{rejected: map[string]bool{}},
		capability:         realCapability{service: capability},
		capabilityMatching: CapabilityMatchingConfig{Enabled: true, FailClosed: true},
		maxRadiusKm:        25,
		restrictions:       restrictions.New(db),
		suitability:        admincommands.SuitabilityGate{DB: db},
	}
	candidates := []availabilityrepositories.ProviderStatus{
		{ProviderID: p1, TruckID: t1, Lat: 6.5244, Lng: 3.3792},
		{ProviderID: p2, TruckID: t2, Lat: 6.5244, Lng: 3.3792},
		{ProviderID: p3, TruckID: t3, Lat: 6.5244, Lng: 3.3792},
	}
	match := func(weight int) map[string]bool {
		exec(`UPDATE haulage_bookings SET cargo_weight_kg=$2 WHERE id=$1`, bookingID, weight)
		out := map[string]bool{}
		for _, p := range service.eligibleProviders(ctx, candidates, bookingmodels.Booking{ID: bookingID, CargoWeightKg: weight, PreferredTruckType: "flatbed"}, 6.5244, 3.3792) {
			out[p.ProviderID] = true
		}
		return out
	}
	if got := match(5000); !got[p1] || !got[p2] || !got[p3] {
		t.Fatal("baseline", got)
	}
	inspection := "insp-" + uuid.NewString()
	apply("trucks", t1, "RECORD_INSPECTION", inspection, 0, map[string]string{"inspection_kind": "ROADWORTHINESS", "performed_on": "2026-10-01"})
	apply("trucks", t1, "CONCLUDE_INSPECTION", inspection, 1, map[string]string{"conclusion": "FAIL"})
	apply("booking-cargo-inputs", bookingID, "RECORD_CARGO_SUITABILITY", "", 0, map[string]string{"decision": "UNSUITABLE", "truck_id": t2})
	apply("booking-cargo-inputs", bookingID, "RECORD_CARGO_SUITABILITY", "", 1, map[string]string{"decision": "SUITABLE", "truck_id": t3})
	got := match(5000)
	if got[p1] || got[p2] || !got[p3] {
		t.Fatal("inspection FAIL / UNSUITABLE not enforced by matching", got)
	}
	// Inputs change past the truck's payload: the earlier SUITABLE authorizes
	// nothing; current fit removes the truck.
	if got := match(30000); got[p3] {
		t.Fatal("historical SUITABLE authorized a truck that no longer fits")
	}
	// A pending remediation blocks until completed.
	apply("booking-cargo-inputs", bookingID, "RECORD_CARGO_SUITABILITY", "", 2, map[string]string{"decision": "SUITABLE_WITH_REMEDIATION", "truck_id": t3})
	if got := match(5000); got[p3] {
		t.Fatal("pending remediation did not block")
	}
	apply("booking-cargo-inputs", bookingID, "COMPLETE_CARGO_REMEDIATION", "", 3, map[string]string{"evidence_ref": "strapping-ok"})
	if got := match(5000); !got[p3] {
		t.Fatal("completed remediation still blocks")
	}
}
