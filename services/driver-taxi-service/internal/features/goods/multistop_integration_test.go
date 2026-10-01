package goods

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/custodyops"
	"cosmicforge/logistics/shared/go/geo"
)

// Scenario D against real PostgreSQL. Requires TAXI_TEST_DATABASE_URL with
// every Taxi migration (including 011-014) applied to a disposable database.
func multiStopDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	db := goodsTestDB(t)
	var exists bool
	if err := db.QueryRow(context.Background(), `SELECT to_regclass('public.taxi_goods_mission_stops') IS NOT NULL AND to_regclass('public.custody_stop_manifests') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatalf("NOT EXECUTABLE: migrations 012-014 are not applied to TAXI_TEST_DATABASE_URL (%v)", err)
	}
	return db
}

type fixedCandidates struct{ ids []string }

func (f *fixedCandidates) NearbyProviders(_ context.Context, _, class string, _ geo.Point, _ float64, _ int) ([]availabilityrepositories.ProviderStatus, error) {
	if class != "economy" {
		return nil, nil
	}
	out := make([]availabilityrepositories.ProviderStatus, 0, len(f.ids))
	for _, id := range f.ids {
		out = append(out, availabilityrepositories.ProviderStatus{ProviderID: id})
	}
	return out, nil
}
func (*fixedCandidates) SetDiscoverable(context.Context, string, string, string, bool) error {
	return nil
}

type capturingNotifier struct{ delivery Credential }

func (c *capturingNotifier) NotifyDeliveryCredential(_ context.Context, _ string, _ string, credential Credential) error {
	c.delivery = credential
	return nil
}
func (c *capturingNotifier) NotifyPickupCredential(context.Context, string, string, Credential) error {
	return nil
}

func seedEligibleProvider(t *testing.T, db *pgxpool.Pool, maxGrams int64) (string, string) {
	t.Helper()
	ctx := context.Background()
	provider, vehicle := uuid.NewString(), uuid.NewString()
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO taxi_providers(id,phone,account_status,onboarding_status,identity_verification_status,vehicle_operational_status,vehicle_verification_status,document_compliance_status) VALUES($1,$2,'active','complete','verified','active','verified','compliant')`, []any{provider, "+234" + strings.ReplaceAll(provider, "-", "")[:10]}},
		{`INSERT INTO vehicles(id,provider_id,vehicle_class,plate_number,status,total_seats,passenger_capacity) VALUES($1,$2,'economy',$3,'active',5,4)`, []any{vehicle, provider, "MS" + vehicle[:8]}},
		{`INSERT INTO taxi_goods_provider_settings(provider_id,capability_status,goods_available) VALUES($1,'APPROVED',true)`, []any{provider}},
		{`INSERT INTO taxi_vehicle_capabilities(vehicle_id,goods_max_weight_grams,goods_max_volume_ml,goods_max_length_mm,goods_max_width_mm,goods_max_height_mm,goods_max_package_count,verified_at) VALUES($1,$2,500000,900,700,500,30,now())`, []any{vehicle, maxGrams}},
		{`INSERT INTO taxi_vehicle_product_eligibility(vehicle_id,product_code,status,policy_version,certification_ref,source_version) VALUES($1,'TAXI_GOODS_STANDARD','ELIGIBLE','test-1','cert-test',1)`, []any{vehicle}},
		{`INSERT INTO provider_operational_state(provider_id,state,version) VALUES($1,'AVAILABLE',1)`, []any{provider}},
	} {
		if _, err := db.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatalf("seed provider: %v", err)
		}
	}
	return provider, vehicle
}

func TestPostgresScenarioDPartialPickupRecoveryDeliversOnce(t *testing.T) {
	db := multiStopDB(t)
	ctx := context.Background()
	p1, v1 := seedEligibleProvider(t, db, 60_000)
	p2, _ := seedEligibleProvider(t, db, 60_000)
	notifier := &capturingNotifier{}
	repo := NewRepository(db)
	secret := []byte("01234567890123456789012345678901")
	// No provider is nearby at commit, so the test controls the first assignment;
	// provider 2 appears later as the only replacement candidate.
	nearby := &fixedCandidates{}
	svc := NewService(Options{Repository: repo, Markets: fakeMarkets{}, Candidates: nearby, Enabled: true,
		HandoverSecret: secret, Notifier: notifier, Capabilities: allOn, Policies: staticPolicy{policyWith(4)}, Now: time.Now})

	suffix := uuid.NewString()
	request := commitRequest("A", "B", "C")
	request.MissionRef, request.OperationRef, request.ParentOrderRef = "mission-"+suffix, "op-"+suffix, "order-"+suffix
	request.Plan.PlanRef, request.Plan.MissionSlotRef = "plan-"+suffix, "slot-1"
	for i := range request.Parcels {
		request.Parcels[i].ParcelRef += "-" + suffix
	}
	mission, err := svc.CreateMission(ctx, commercemission.SignatureCaller, request)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if replay, err := svc.CreateMission(ctx, commercemission.SignatureCaller, request); err != nil || replay.ID != mission.ID {
		t.Fatalf("duplicate Vendor commit must return the same mission: %+v %v", replay, err)
	}
	stale := request
	stale.MissionRef, stale.OperationRef, stale.Plan = "stale-"+suffix, "stale-op-"+suffix, &commercemission.PlanBinding{PlanRef: request.Plan.PlanRef, PlanVersion: 30, PlanFingerprint: "old", MissionSlotRef: "slot-9"}
	if _, err := svc.CreateMission(ctx, commercemission.SignatureCaller, stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale plan version commit: %v", err)
	}
	duplicateSlot := request
	duplicateSlot.MissionRef, duplicateSlot.OperationRef = "dup-"+suffix, "dup-op-"+suffix
	if _, err := svc.CreateMission(ctx, commercemission.SignatureCaller, duplicateSlot); !errors.Is(err, ErrConflict) {
		t.Fatalf("second mission for the same plan slot: %v", err)
	}

	// Taxi assigns provider 1 (Vendor never names a provider).
	if err = repo.Offer(ctx, mission.ID, p1, v1); err != nil {
		t.Fatalf("offer: %v", err)
	}
	if _, err = svc.Accept(ctx, p1, mission.MissionRef); err != nil {
		t.Fatalf("accept: %v", err)
	}
	pickup := func(provider, stop string) {
		t.Helper()
		if _, err := svc.StopEnRoute(ctx, provider, mission.MissionRef, stop); err != nil {
			t.Fatalf("en route %s: %v", stop, err)
		}
		if _, err := svc.StopArrived(ctx, provider, mission.MissionRef, stop); err != nil {
			t.Fatalf("arrived %s: %v", stop, err)
		}
		credential, err := svc.StopHandover(ctx, provider, mission.MissionRef, stop)
		if err != nil {
			t.Fatalf("handover %s: %v", stop, err)
		}
		verify := VerifyCredential{ChallengeRef: credential.ChallengeRef, Method: "OTP", Credential: credential.OTP, OperationRef: "verify-" + stop + "-" + suffix}
		if _, err := svc.VerifyPickup(ctx, commercemission.SignatureCaller, mission.MissionRef, verify); err != nil {
			t.Fatalf("verify %s: %v", stop, err)
		}
		if _, err := svc.VerifyPickup(ctx, commercemission.SignatureCaller, mission.MissionRef, verify); err != nil {
			t.Fatalf("replayed verify %s must be idempotent: %v", stop, err)
		}
	}
	pickup(p1, "A")
	pickup(p1, "B")
	view, err := repo.custody.ReadMissionCustody(ctx, mission.MissionRef, request.ParentOrderRef)
	if err != nil {
		t.Fatal(err)
	}
	if held := view.HeldBy(p1); len(held) != 2 {
		t.Fatalf("provider 1 custody after A+B = %v", held)
	}
	var holdingRows int
	_ = db.QueryRow(ctx, `SELECT count(*) FROM custody_manifest_holdings WHERE mission_ref=$1`, mission.MissionRef).Scan(&holdingRows)
	if holdingRows != 2 {
		t.Fatalf("duplicate verification duplicated custody: %d holding rows", holdingRows)
	}
	if notifier.delivery.OTP != "" {
		t.Fatal("delivery credential must not be issued before the final pickup")
	}

	// Provider 1 breaks down before C.
	nearby.ids = []string{p2}
	if _, err = svc.StartRecovery(ctx, "operations-service", mission.MissionRef, StartRecoveryCommand{OperationRef: "recover-" + suffix, TriggerCode: "PROVIDER_BREAKDOWN"}); err != nil {
		t.Fatalf("start recovery: %v", err)
	}
	recovery, err := svc.SelectReplacement(ctx, "operations-service", mission.MissionRef, "select-"+suffix)
	if err != nil || recovery.State != RecoveryTransferRequired || recovery.ReplacementProviderID != p2 {
		t.Fatalf("replacement = %+v %v", recovery, err)
	}
	evidence, err := repo.custody.RecordEvidence(ctx, custodyops.EvidenceCommand{OperationRef: "evidence-" + suffix, MissionRef: mission.MissionRef, TenantRef: request.ParentOrderRef,
		ManifestRef: "manifest-A", ManifestVersion: 1, EvidenceType: custodyops.EvidenceProviderTransfer, MediaRef: "media-" + suffix, ProviderRef: p1,
		ActorRef: "operations-service", CapturedAt: time.Now(), AccessPolicy: "OPS"}, allowAllMedia{})
	if err != nil {
		t.Fatalf("transfer evidence: %v", err)
	}
	transfer := CompleteTransferCommand{OperationRef: "transfer-" + suffix, TransferRef: "handoff-" + suffix, EvidenceRef: evidence.ID, OldProviderConfirmed: true, NewProviderConfirmed: true}
	if _, err = svc.CompleteRecoveryTransfer(ctx, "operations-service", mission.MissionRef, transfer); err != nil {
		t.Fatalf("custody transfer: %v", err)
	}
	var items int
	_ = db.QueryRow(ctx, `SELECT count(*) FROM custody_transfer_items WHERE mission_ref=$1 AND from_custodian_ref=$2 AND to_custodian_ref=$3`, mission.MissionRef, p1, p2).Scan(&items)
	if items != 2 {
		t.Fatalf("transfer inventory = %d, want A and B", items)
	}

	// Provider 2 continues to C, then delivers everything.
	pickup(p2, "C")
	if notifier.delivery.OTP == "" {
		t.Fatal("delivery credential must be issued after the final pickup")
	}
	if _, err = svc.InTransit(ctx, p2, mission.MissionRef); err != nil {
		t.Fatalf("in transit: %v", err)
	}
	if _, err = svc.ArriveDropoff(ctx, p2, mission.MissionRef); err != nil {
		t.Fatalf("arrive dropoff: %v", err)
	}
	if _, err = svc.VerifyDelivery(ctx, p2, mission.MissionRef, VerifyCredential{ChallengeRef: notifier.delivery.ChallengeRef, Method: "OTP", Credential: notifier.delivery.OTP,
		OperationRef: "deliver-" + suffix, ReceiverRef: request.CustomerRef}); err != nil {
		t.Fatalf("verify delivery: %v", err)
	}
	if err = repo.custody.AssertMissionCompletable(ctx, mission.MissionRef); err != nil {
		t.Fatalf("all manifests must be accounted for: %v", err)
	}
	if _, err = db.Exec(ctx, `UPDATE taxi_goods_missions SET delivery_evidence_ref='delivery-evidence' WHERE id=$1`, mission.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Complete(ctx, p2, mission.MissionRef); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err = svc.Complete(ctx, p2, mission.MissionRef); err == nil {
		t.Fatal("repeated completion must not trigger a second settlement")
	}
	var missions, replacements int
	_ = db.QueryRow(ctx, `SELECT count(*), max(provider_replacement_count) FROM taxi_goods_missions WHERE parent_order_ref=$1`, request.ParentOrderRef).Scan(&missions, &replacements)
	if missions != 1 || replacements != 1 {
		t.Fatalf("logical missions = %d, replacements = %d; want one mission replaced once", missions, replacements)
	}
}

type allowAllMedia struct{}

func (allowAllMedia) ValidateCustodyMedia(context.Context, custodyops.EvidenceCommand) error {
	return nil
}
