package goods

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/commercequote"
)

func seedLegacyQuote(t *testing.T, repo *Repository, grams int64) (string, commercemission.Request) {
	t.Helper()
	ctx := context.Background()
	quoteID := uuid.NewString()
	pickup := commercequote.Point{Latitude: 6.5, Longitude: 3.3}
	dropoff := commercequote.Point{Latitude: 6.6, Longitude: 3.4}
	quoted := commercequote.Request{QuoteReference: "q-" + quoteID, VehicleClass: commercequote.VehicleCar, Pickup: pickup, Dropoff: dropoff,
		Load: commercequote.Load{TotalWeightGrams: grams, PackageCount: 1}, Currency: "NGN"}
	payload, _ := json.Marshal(quoted)
	if _, err := repo.db.Exec(ctx, `INSERT INTO taxi_goods_quotes(id,source_service,quote_reference,revision,request_fingerprint,request_payload,serviceable,reason_code,distance_km,estimated_minutes,fee_minor,currency,available_providers,expires_at)
		VALUES($1,'vendor-service',$2,1,'fp',$3,true,'SERVICEABLE',1,5,1000,'NGN',1,now()+interval '15 minutes')`, quoteID, quoted.QuoteReference, payload); err != nil {
		t.Fatalf("seed quote: %v", err)
	}
	ref := "legacy-" + quoteID
	request := commercemission.Request{MissionRef: ref, ParentOrderRef: "parent-" + ref, VendorOrderRefs: []string{"vo-" + ref}, CustomerRef: "cust-" + ref,
		Service: commercemission.ServiceTaxi, VehicleClass: commercemission.VehicleCar, QuoteID: quoteID, QuoteReference: quoted.QuoteReference,
		Pickup: commercemission.Point{Latitude: pickup.Latitude, Longitude: pickup.Longitude}, Dropoff: commercemission.Point{Latitude: dropoff.Latitude, Longitude: dropoff.Longitude},
		Load: commercemission.Load{WeightGrams: grams, PackageCount: 1}, Currency: "NGN", AgreedFeeMinor: 1000, RequestedAt: time.Now().UTC().Truncate(time.Second)}
	t.Cleanup(func() {
		_, _ = repo.db.Exec(ctx, `DELETE FROM taxi_outbox WHERE aggregate_id IN (SELECT id::text FROM taxi_goods_missions WHERE quote_id=$1)`, quoteID)
		_, _ = repo.db.Exec(ctx, `DELETE FROM taxi_goods_missions WHERE quote_id=$1`, quoteID)
		_, _ = repo.db.Exec(ctx, `DELETE FROM taxi_goods_quotes WHERE id=$1`, quoteID)
	})
	return quoteID, request
}

// A legacy quote-only mission (no plan binding) used to fail with a NOT NULL
// violation on plan_ref. It is now stored explicitly as LEGACY_QUOTE.
func TestPostgresLegacyQuoteMissionWithoutPlanIsCreated(t *testing.T) {
	db := goodsTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	_, first := seedLegacyQuote(t, repo, 2000)
	_, second := seedLegacyQuote(t, repo, 2000)
	for _, request := range []commercemission.Request{first, second} {
		mission, err := repo.CreateMission(ctx, "vendor-service", request)
		if err != nil {
			t.Fatalf("legacy mission without plan: %v", err)
		}
		var mode, planRef string
		var version int64
		if err = db.QueryRow(ctx, `SELECT plan_binding_mode,plan_ref,plan_version FROM taxi_goods_missions WHERE id=$1`, mission.ID).Scan(&mode, &planRef, &version); err != nil {
			t.Fatal(err)
		}
		if mode != PlanBindingLegacyQuote || planRef != "" || version != 0 {
			t.Fatalf("stored binding = %s/%q/%d, want LEGACY_QUOTE with no invented plan identity", mode, planRef, version)
		}
		replay, err := repo.CreateMission(ctx, "vendor-service", request)
		if err != nil || replay.ID != mission.ID {
			t.Fatalf("idempotent replay = %v/%v", replay.ID, err)
		}
	}
}

func TestPostgresPlanBoundSingleMissionRejectsSlotReuseAndPartialBinding(t *testing.T) {
	db := goodsTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()
	planRef := "plan-" + uuid.NewString()
	_, first := seedLegacyQuote(t, repo, 1000)
	first.Plan = &commercemission.PlanBinding{PlanRef: planRef, PlanVersion: 1, PlanFingerprint: "fp", MissionSlotRef: "slot-1"}
	mission, err := repo.CreateMission(ctx, "vendor-service", first)
	if err != nil {
		t.Fatalf("plan-bound single mission: %v", err)
	}
	var mode string
	if err = db.QueryRow(ctx, `SELECT plan_binding_mode FROM taxi_goods_missions WHERE id=$1`, mission.ID).Scan(&mode); err != nil || mode != PlanBindingPlanBound {
		t.Fatalf("mode = %q (%v)", mode, err)
	}
	_, duplicate := seedLegacyQuote(t, repo, 1000)
	duplicate.Plan = &commercemission.PlanBinding{PlanRef: planRef, PlanVersion: 1, PlanFingerprint: "fp", MissionSlotRef: "slot-1"}
	if _, err = repo.CreateMission(ctx, "vendor-service", duplicate); !errors.Is(err, ErrConflict) {
		t.Fatalf("second mission on the same plan slot = %v, want conflict", err)
	}
	_, partial := seedLegacyQuote(t, repo, 1000)
	partial.Plan = &commercemission.PlanBinding{PlanRef: "plan-partial", MissionSlotRef: "slot-1"}
	if _, err = repo.CreateMission(ctx, "vendor-service", partial); !errors.Is(err, ErrInvalid) {
		t.Fatalf("partial plan binding = %v, want invalid", err)
	}
	if _, err = db.Exec(ctx, `UPDATE taxi_goods_missions SET plan_binding_mode='LEGACY_QUOTE' WHERE id=$1`, mission.ID); err == nil {
		t.Fatal("a LEGACY_QUOTE row must not carry a plan identity")
	}
}
