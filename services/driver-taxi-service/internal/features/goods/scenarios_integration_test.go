package goods

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/taxi-service/internal/movement"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/custodyops"
)

// End-to-end multi-pickup scenarios against real PostgreSQL. Scenario D
// (partial pickup, provider recovery, delivered once) lives beside these in
// multistop_integration_test.go.

func scenarioService(t *testing.T, db *pgxpool.Pool, notifier *capturingNotifier, nearby *fixedCandidates, maxPickups int) (*Service, *Repository) {
	t.Helper()
	repo := NewRepository(db).WithMovementEvents(&movement.Writer{Enabled: true})
	svc := NewService(Options{Repository: repo, Markets: fakeMarkets{}, Candidates: nearby, Enabled: true,
		HandoverSecret: []byte("01234567890123456789012345678901"), Notifier: notifier, Capabilities: allOn,
		Policies: staticPolicy{policyWith(maxPickups)}, Now: time.Now})
	return svc, repo
}

func scenarioRequest(t *testing.T, stops ...string) commercemission.Request {
	t.Helper()
	suffix := uuid.NewString()
	request := commitRequest(stops...)
	request.MissionRef, request.OperationRef, request.ParentOrderRef = "mission-"+suffix, "op-"+suffix, "order-"+suffix
	request.Plan.PlanRef, request.Plan.MissionSlotRef = "plan-"+suffix, "slot-1"
	for i := range request.Parcels {
		request.Parcels[i].ParcelRef += "-" + suffix
	}
	return request
}

func countRows(t *testing.T, db *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := db.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// Scenario A: the pre-checkout preview answers serviceability for a
// multi-vendor cluster and changes NOTHING, and the commit that follows is
// bound to the plan it was quoted for.
func TestPostgresScenarioAPreviewIsInertThenCommitBindsThePlan(t *testing.T) {
	db := multiStopDB(t)
	ctx := context.Background()
	provider, vehicleID := seedEligibleProvider(t, db, 60_000)
	svc, _ := scenarioService(t, db, &capturingNotifier{}, &fixedCandidates{}, 4)

	before := countRows(t, db, `SELECT count(*) FROM taxi_goods_missions`)
	beforeCustody := countRows(t, db, `SELECT count(*) FROM custody_mission_state`)
	beforeOutbox := countRows(t, db, `SELECT count(*) FROM taxi_outbox`)

	preview := ServiceabilityRequest{PreviewRef: "cart-" + uuid.NewString(), Purpose: PurposeCommercePreview, CartVersion: 9,
		Dropoff: commercequote.Point{Latitude: 6.5, Longitude: 3.4}}
	for _, stop := range []string{"A", "B"} {
		preview.Pickups = append(preview.Pickups, PreviewPickup{StopRef: stop, Location: commercequote.Point{Latitude: 6.4, Longitude: 3.3},
			Cargo: previewRequest(12_000).Pickups[0].Cargo})
	}
	answer, err := svc.PreviewServiceability(ctx, preview)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	raw, _ := json.Marshal(answer)
	if strings.Contains(string(raw), provider) || strings.Contains(string(raw), vehicleID) {
		t.Fatalf("preview exposed execution identity: %s", raw)
	}
	if countRows(t, db, `SELECT count(*) FROM taxi_goods_missions`) != before ||
		countRows(t, db, `SELECT count(*) FROM custody_mission_state`) != beforeCustody ||
		countRows(t, db, `SELECT count(*) FROM taxi_outbox`) != beforeOutbox {
		t.Fatal("the preview wrote execution state")
	}

	request := scenarioRequest(t, "A", "B")
	mission, err := svc.CreateMission(ctx, commercemission.SignatureCaller, request)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	var planRef, mode string
	var planVersion int64
	if err = db.QueryRow(ctx, `SELECT plan_ref, plan_version, plan_binding_mode FROM taxi_goods_missions WHERE id=$1`, mission.ID).Scan(&planRef, &planVersion, &mode); err != nil {
		t.Fatal(err)
	}
	if planRef != request.Plan.PlanRef || planVersion != request.Plan.PlanVersion || mode != PlanBindingPlanBound {
		t.Fatalf("commit binding = %s/%d/%s", planRef, planVersion, mode)
	}
	if custody := countRows(t, db, `SELECT count(*) FROM custody_stop_manifests WHERE mission_ref=$1`, mission.MissionRef); custody != 2 {
		t.Fatalf("committed custody manifests = %d, want one per vendor stop", custody)
	}
}

// Scenario B: the ordinary multi-pickup delivery. Every stop is collected in
// turn, the delivery credential exists only after the LAST pickup, custody
// accounts for every manifest, and the mission delivers exactly once.
func TestPostgresScenarioBEveryStopCollectedThenDeliveredOnce(t *testing.T) {
	db := multiStopDB(t)
	ctx := context.Background()
	provider, vehicleID := seedEligibleProvider(t, db, 60_000)
	notifier := &capturingNotifier{}
	svc, repo := scenarioService(t, db, notifier, &fixedCandidates{}, 4)

	request := scenarioRequest(t, "A", "B", "C")
	mission, err := svc.CreateMission(ctx, commercemission.SignatureCaller, request)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err = repo.Offer(ctx, mission.ID, provider, vehicleID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Accept(ctx, provider, mission.MissionRef); err != nil {
		t.Fatal(err)
	}
	collect := func(stop string) {
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
		if _, err = svc.VerifyPickup(ctx, commercemission.SignatureCaller, mission.MissionRef, VerifyCredential{
			ChallengeRef: credential.ChallengeRef, Method: "OTP", Credential: credential.OTP,
			OperationRef: "verify-" + stop + "-" + mission.MissionRef}); err != nil {
			t.Fatalf("verify %s: %v", stop, err)
		}
	}
	collect("A")
	collect("B")
	if notifier.delivery.OTP != "" {
		t.Fatal("delivery credential issued before the final pickup")
	}
	collect("C")
	if notifier.delivery.OTP == "" {
		t.Fatal("delivery credential missing after the final pickup")
	}
	view, err := repo.custody.ReadMissionCustody(ctx, mission.MissionRef, request.ParentOrderRef)
	if err != nil {
		t.Fatal(err)
	}
	if held := view.HeldBy(provider); len(held) != 3 {
		t.Fatalf("custody after three pickups = %v", held)
	}
	if _, err = svc.InTransit(ctx, provider, mission.MissionRef); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ArriveDropoff(ctx, provider, mission.MissionRef); err != nil {
		t.Fatal(err)
	}
	deliver := VerifyCredential{ChallengeRef: notifier.delivery.ChallengeRef, Method: "OTP", Credential: notifier.delivery.OTP,
		OperationRef: "deliver-" + mission.MissionRef, ReceiverRef: request.CustomerRef}
	if _, err = svc.VerifyDelivery(ctx, provider, mission.MissionRef, deliver); err != nil {
		t.Fatalf("verify delivery: %v", err)
	}
	if err = repo.custody.AssertMissionCompletable(ctx, mission.MissionRef); err != nil {
		t.Fatalf("manifests unaccounted after delivery: %v", err)
	}
	if _, err = db.Exec(ctx, `UPDATE taxi_goods_missions SET delivery_evidence_ref='delivery-evidence' WHERE id=$1`, mission.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Complete(ctx, provider, mission.MissionRef); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err = svc.Complete(ctx, provider, mission.MissionRef); err == nil {
		t.Fatal("a second completion must not settle again")
	}
	// The customer's movement history is fed from the same transaction.
	if queued := countRows(t, db, `SELECT count(*) FROM taxi_outbox WHERE aggregate_id=$1 AND destination='CUSTOMER_MOVEMENT'`, mission.ID); queued == 0 {
		t.Fatal("no customer movement event was queued for the mission")
	}
}

// Scenario C: a delayed vendor is replanned rather than abandoned. The route
// order changes, custody already taken is untouched, and both a stale replan
// and one that drops a stop are refused.
func TestPostgresScenarioCDelayedPickupReplansWithoutLosingCustody(t *testing.T) {
	db := multiStopDB(t)
	ctx := context.Background()
	provider, vehicleID := seedEligibleProvider(t, db, 60_000)
	notifier := &capturingNotifier{}
	svc, repo := scenarioService(t, db, notifier, &fixedCandidates{}, 4)

	request := scenarioRequest(t, "A", "B", "C")
	mission, err := svc.CreateMission(ctx, commercemission.SignatureCaller, request)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err = repo.Offer(ctx, mission.ID, provider, vehicleID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Accept(ctx, provider, mission.MissionRef); err != nil {
		t.Fatal(err)
	}
	// A is collected before anything is replanned.
	if _, err = svc.StopEnRoute(ctx, provider, mission.MissionRef, "A"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.StopArrived(ctx, provider, mission.MissionRef, "A"); err != nil {
		t.Fatal(err)
	}
	credential, err := svc.StopHandover(ctx, provider, mission.MissionRef, "A")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.VerifyPickup(ctx, commercemission.SignatureCaller, mission.MissionRef, VerifyCredential{
		ChallengeRef: credential.ChallengeRef, Method: "OTP", Credential: credential.OTP, OperationRef: "verify-A-" + mission.MissionRef}); err != nil {
		t.Fatal(err)
	}

	// B reports a delay, so the provider takes C first.
	if _, err = svc.ReportPickupDelay(ctx, commercemission.SignatureCaller, mission.MissionRef, "B", time.Now().Add(45*time.Minute)); err != nil {
		t.Fatalf("report delay: %v", err)
	}
	dropoff := dropoffStopRef(t, db, mission.ID)
	version, err := svc.ReplanRoute(ctx, commercemission.SignatureCaller, mission.MissionRef, RouteReplanCommand{
		OperationRef: "replan-" + mission.MissionRef, ReasonCode: "VENDOR_DELAY", StopOrder: []string{"A", "C", "B", dropoff}, ExpectedRouteVersion: 1})
	if err != nil {
		t.Fatalf("replan: %v", err)
	}
	if version < 2 {
		t.Fatalf("replan version = %d, want a new route version", version)
	}
	if _, err = svc.ReplanRoute(ctx, commercemission.SignatureCaller, mission.MissionRef, RouteReplanCommand{
		OperationRef: "replan-stale-" + mission.MissionRef, ReasonCode: "VENDOR_DELAY", StopOrder: []string{"A", "B", "C", dropoff}, ExpectedRouteVersion: 1}); err == nil {
		t.Fatal("a replan against a stale route version must be refused")
	}
	// A dropped stop would strand a vendor's consignment, so it is refused.
	if _, err = svc.ReplanRoute(ctx, commercemission.SignatureCaller, mission.MissionRef, RouteReplanCommand{
		OperationRef: "replan-drop-" + mission.MissionRef, ReasonCode: "VENDOR_DELAY", StopOrder: []string{"A", "C", dropoff}, ExpectedRouteVersion: version}); err == nil {
		t.Fatal("a replan that drops a stop must be refused")
	}
	view, err := repo.custody.ReadMissionCustody(ctx, mission.MissionRef, request.ParentOrderRef)
	if err != nil {
		t.Fatal(err)
	}
	if held := view.HeldBy(provider); len(held) != 1 {
		t.Fatalf("custody after replan = %v, want the manifest collected at A", held)
	}
	// Every manifest is still unaccounted for: A is held by the provider and
	// B and C have not been collected. None of them has reached a receiver.
	if len(view.Pending) != 3 {
		t.Fatalf("unaccounted manifests after replan = %v, want all three still outstanding", view.Pending)
	}
	if err = repo.custody.AssertMissionCompletable(ctx, mission.MissionRef); !errors.Is(err, custodyops.ErrManifestsUnaccounted) {
		t.Fatalf("a replanned mission with pending manifests must not be completable: %v", err)
	}
}

// dropoffStopRef reads the mission's own final stop reference: a replan must
// name every stop the mission holds, and the dropoff stays last.
func dropoffStopRef(t *testing.T, db *pgxpool.Pool, missionID string) string {
	t.Helper()
	var ref string
	if err := db.QueryRow(context.Background(), `SELECT external_stop_ref FROM taxi_goods_mission_stops WHERE mission_id=$1 AND stop_type='DROPOFF'`, missionID).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	return ref
}
