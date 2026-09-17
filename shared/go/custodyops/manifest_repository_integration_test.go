package custodyops

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// These tests need an isolated, disposable PostgreSQL with the custody
// migrations and the shared multi-manifest migration applied. They never
// fall back to a default DSN.
func multiManifestRepository(t *testing.T) *Repository {
	t.Helper()
	repo := integrationRepository(t)
	var exists bool
	if err := repo.pool.QueryRow(context.Background(), `SELECT to_regclass('public.custody_stop_manifests') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatalf("multi-manifest custody migration is not applied: %v", err)
	}
	return repo
}

type abcFixture struct {
	mission, tenant string
}

// registerABC registers A(MA) -> B(MB1, MB2) -> C(MC) -> D.
func registerABC(t *testing.T, repo *Repository) abcFixture {
	t.Helper()
	id := uuid.NewString()
	f := abcFixture{mission: "mission-" + id, tenant: "tenant-" + id}
	manifest := func(ref, stop string, packages ...string) ManifestInput {
		m := ManifestInput{ManifestRef: ref + "-" + id, ManifestVersion: 1, PickupStopRef: stop, SourceAuthority: "VENDOR", SourceManifestChecksum: "sha-" + ref, VendorRef: "vendor-" + stop}
		for _, p := range packages {
			m.Packages = append(m.Packages, PackageInput{PackageRef: p + "-" + id, PackageCount: 1, WeightGrams: 500})
		}
		return m
	}
	_, err := repo.RegisterMission(context.Background(), RegisterMissionCommand{OperationRef: "register-" + id, MissionRef: f.mission, TenantRef: f.tenant, ReceiverChallengeRef: "rcv-" + id, SourceVersion: 1,
		Stops:     []StopInput{{StopRef: "A", StopType: StopPickup, Sequence: 1}, {StopRef: "B", StopType: StopPickup, Sequence: 2}, {StopRef: "C", StopType: StopPickup, Sequence: 3}, {StopRef: "D", StopType: StopDropoff, Sequence: 4}},
		Manifests: []ManifestInput{manifest("MA", "A", "a1", "a2"), manifest("MB1", "B", "b1"), manifest("MB2", "B", "b2"), manifest("MC", "C", "c1", "c2", "c3")}})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f abcFixture) key(ref string) ManifestKey {
	return ManifestKey{ManifestRef: ref + "-" + f.mission[len("mission-"):], ManifestVersion: 1}
}

func (f abcFixture) collect(op, stop, provider string, version int64, refs ...string) CollectManifestsCommand {
	keys := make([]ManifestKey, 0, len(refs))
	for _, ref := range refs {
		keys = append(keys, f.key(ref))
	}
	return CollectManifestsCommand{OperationRef: op + "-" + f.mission, MissionRef: f.mission, TenantRef: f.tenant, StopRef: stop, ProviderRef: provider,
		HandoverChallengeRef: "hc-" + stop, HandoverVerifiedByRef: "vendor-" + stop, SourceVersion: version, Manifests: keys}
}

func activeHoldings(t *testing.T, repo *Repository, mission string) map[string]string {
	t.Helper()
	rows, err := repo.pool.Query(context.Background(), `SELECT manifest_ref, custodian_ref FROM custody_manifest_holdings WHERE mission_ref=$1 AND released_at IS NULL`, mission)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var ref, custodian string
		if err := rows.Scan(&ref, &custodian); err != nil {
			t.Fatal(err)
		}
		out[ref] = custodian
	}
	return out
}

func TestPostgresMultiManifestCollectionIsStopScopedAndIdempotent(t *testing.T) {
	repo := multiManifestRepository(t)
	ctx := context.Background()
	f := registerABC(t, repo)

	if _, err := repo.CollectManifests(ctx, f.collect("pick-a", "A", "p1", 2, "MA")); err != nil {
		t.Fatal(err)
	}
	if held := activeHoldings(t, repo, f.mission); len(held) != 1 || held[f.key("MA").ManifestRef] != "p1" {
		t.Fatalf("after A holdings = %v", held)
	}
	// Same operation replayed, then the same handover with a new operation ref.
	if res, err := repo.CollectManifests(ctx, f.collect("pick-a", "A", "p1", 2, "MA")); err != nil || !res.Replayed {
		t.Fatalf("operation replay = %+v %v", res, err)
	}
	if res, err := repo.CollectManifests(ctx, f.collect("pick-a-retry", "A", "p1", 2, "MA")); err != nil || !res.Replayed {
		t.Fatalf("duplicate callback = %+v %v", res, err)
	}
	var holdingRows int
	if err := repo.pool.QueryRow(ctx, `SELECT count(*) FROM custody_manifest_holdings WHERE mission_ref=$1`, f.mission).Scan(&holdingRows); err != nil || holdingRows != 1 {
		t.Fatalf("duplicate pickup created %d holding rows (%v)", holdingRows, err)
	}
	if _, err := repo.CollectManifests(ctx, f.collect("wrong-stop", "A", "p1", 3, "MC")); !errors.Is(err, ErrConflict) {
		t.Fatalf("future-stop manifest at A: err = %v", err)
	}
	if _, err := repo.CollectManifests(ctx, f.collect("pick-b1", "B", "p1", 3, "MB1")); err != nil {
		t.Fatal(err)
	}
	var stopStatus string
	if err := repo.pool.QueryRow(ctx, `SELECT status FROM custody_mission_stops WHERE mission_ref=$1 AND stop_ref='B'`, f.mission).Scan(&stopStatus); err != nil || stopStatus != "PARTIALLY_COLLECTED" {
		t.Fatalf("stop B = %s %v", stopStatus, err)
	}
	if _, err := repo.CollectManifests(ctx, f.collect("stale", "B", "p1", 2, "MB2")); !errors.Is(err, ErrStaleSourceVersion) {
		t.Fatalf("stale source version: err = %v", err)
	}
}

func TestPostgresConcurrentPickupCannotCreateTwoCustodians(t *testing.T) {
	repo := multiManifestRepository(t)
	f := registerABC(t, repo)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, provider := range []string{"p1", "p2"} {
		wg.Add(1)
		go func(i int, provider string) {
			defer wg.Done()
			_, errs[i] = repo.CollectManifests(context.Background(), f.collect("race-"+provider, "A", provider, 2, "MA"))
		}(i, provider)
	}
	wg.Wait()
	if (errs[0] == nil) == (errs[1] == nil) {
		t.Fatalf("exactly one concurrent pickup must win: %v", errs)
	}
	if held := activeHoldings(t, repo, f.mission); len(held) != 1 {
		t.Fatalf("active custodians of MA = %v", held)
	}
}

func TestPostgresPartialPickupReplacementTransfersExactInventory(t *testing.T) {
	repo := multiManifestRepository(t)
	ctx := context.Background()
	f := registerABC(t, repo)
	for i, step := range []CollectManifestsCommand{f.collect("pa", "A", "p1", 2, "MA"), f.collect("pb", "B", "p1", 3, "MB1", "MB2")} {
		if _, err := repo.CollectManifests(ctx, step); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	if _, err := repo.ReportIncident(ctx, IncidentCommand{OperationRef: "incident-" + f.mission, MissionRef: f.mission, TenantRef: f.tenant, ProviderRef: "p1", Reason: IncidentBreakdown, OccurredAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AssignManifestReplacement(ctx, AssignReplacementCommand{OperationRef: "assign-small-" + f.mission, MissionRef: f.mission, TenantRef: f.tenant, OldProviderRef: "p1", NewProviderRef: "p2", VehicleClass: "CAR", CapacityCount: 3, ComplianceRef: "c", AvailabilityRef: "a"}); !errors.Is(err, ErrPolicy) {
		t.Fatalf("replacement below held+pending capacity: err = %v", err)
	}
	if _, err := repo.AssignManifestReplacement(ctx, AssignReplacementCommand{OperationRef: "assign-" + f.mission, MissionRef: f.mission, TenantRef: f.tenant, OldProviderRef: "p1", NewProviderRef: "p2", VehicleClass: "CAR", CapacityCount: 7, ComplianceRef: "c", AvailabilityRef: "a"}); err != nil {
		t.Fatal(err)
	}
	evidence, err := repo.RecordEvidence(ctx, EvidenceCommand{OperationRef: "ev-" + f.mission, MissionRef: f.mission, TenantRef: f.tenant, ManifestRef: f.key("MA").ManifestRef, ManifestVersion: 1, EvidenceType: EvidenceProviderTransfer, MediaRef: "media-" + f.mission, ProviderRef: "p1", ActorRef: "ops", CapturedAt: time.Now(), AccessPolicy: "OPS"}, integrationMedia{})
	if err != nil {
		t.Fatal(err)
	}
	transfer := func(op, newProvider string) TransferManifestsCommand {
		return TransferManifestsCommand{OperationRef: op + "-" + f.mission, MissionRef: f.mission, TenantRef: f.tenant, TransferRef: "tr-" + f.mission, OldProviderRef: "p1", NewProviderRef: newProvider,
			EvidenceRef: evidence.ID, OldProviderConfirm: true, NewProviderConfirm: true, NewReceiverChallengeRef: "rcv2", SourceVersion: 4,
			Manifests: []ManifestKey{f.key("MA"), f.key("MB1"), f.key("MB2")}}
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, provider := range []string{"p2", "p3"} {
		wg.Add(1)
		go func(i int, provider string) {
			defer wg.Done()
			_, errs[i] = repo.TransferManifests(ctx, transfer("transfer-"+provider, provider))
		}(i, provider)
	}
	wg.Wait()
	if errs[0] != nil || errs[1] == nil {
		t.Fatalf("only the assigned replacement may take custody: %v", errs)
	}
	held := activeHoldings(t, repo, f.mission)
	if len(held) != 3 || held[f.key("MA").ManifestRef] != "p2" || held[f.key("MB2").ManifestRef] != "p2" {
		t.Fatalf("after transfer holdings = %v", held)
	}
	var items int
	if err := repo.pool.QueryRow(ctx, `SELECT count(*) FROM custody_transfer_items WHERE mission_ref=$1 AND from_custodian_ref='p1' AND to_custodian_ref='p2'`, f.mission).Scan(&items); err != nil || items != 3 {
		t.Fatalf("transfer inventory rows = %d %v", items, err)
	}
	if res, err := repo.TransferManifests(ctx, transfer("transfer-p2", "p2")); err != nil || !res.Replayed {
		t.Fatalf("transfer replay = %+v %v", res, err)
	}
	var mcStatus string
	if err := repo.pool.QueryRow(ctx, `SELECT status FROM custody_stop_manifests WHERE mission_ref=$1 AND manifest_ref=$2`, f.mission, f.key("MC").ManifestRef).Scan(&mcStatus); err != nil || mcStatus != ManifestRegistered {
		t.Fatalf("MC must stay pending: %s %v", mcStatus, err)
	}
	if _, err := repo.CollectManifests(ctx, f.collect("pc", "C", "p2", 5, "MC")); err != nil {
		t.Fatal(err)
	}
	deliver := DeliverManifestsCommand{OperationRef: "deliver-part-" + f.mission, MissionRef: f.mission, TenantRef: f.tenant, DropoffStopRef: "D", ProviderRef: "p2", DeliveryChallengeRef: "rcv2", SourceVersion: 6, Manifests: []ManifestKey{f.key("MA")}}
	if res, err := repo.DeliverManifests(ctx, deliver); err != nil || res.State == "DELIVERED" {
		t.Fatalf("partial delivery = %+v %v", res, err)
	}
	if err := repo.AssertMissionCompletable(ctx, f.mission); !errors.Is(err, ErrManifestsUnaccounted) {
		t.Fatalf("completion with open manifests: err = %v", err)
	}
	deliver.OperationRef, deliver.Manifests = "deliver-rest-"+f.mission, []ManifestKey{f.key("MB1"), f.key("MB2"), f.key("MC")}
	if res, err := repo.DeliverManifests(ctx, deliver); err != nil || res.State != "DELIVERED" {
		t.Fatalf("final delivery = %+v %v", res, err)
	}
	if err := repo.AssertMissionCompletable(ctx, f.mission); err != nil {
		t.Fatalf("all manifests delivered, completion refused: %v", err)
	}
	if _, err := repo.pool.Exec(ctx, `UPDATE custody_stop_manifests SET status='COLLECTED', current_custodian_ref='p2' WHERE mission_ref=$1 AND manifest_ref=$2`, f.mission, f.key("MA").ManifestRef); err == nil {
		t.Fatal("database must refuse regressing a delivered manifest")
	}
}

func TestPostgresLegacySingleManifestMissionStillExecutes(t *testing.T) {
	repo := multiManifestRepository(t)
	ctx := context.Background()
	id := uuid.NewString()
	mission, tenant := "legacy-"+id, "tenant-"+id
	manifest, err := repo.FreezeManifest(ctx, FreezeManifestCommand{OperationRef: "freeze-" + id, MissionRef: mission, TenantRef: tenant, BusinessRef: "b", BranchRef: "br", ManifestRef: "man-" + id, ManifestVersion: 1, PickupStopRef: "stop", ReceiverChallengeRef: "rcv", Parcels: []Parcel{{ParcelRef: "p-" + id, StopRef: "stop", Count: 1}}, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ConfirmParcels(ctx, ConfirmParcelsCommand{OperationRef: "confirm-" + id, MissionRef: mission, TenantRef: tenant, BusinessRef: "b", BranchRef: "br", PickupStopRef: "stop", ManifestRef: manifest.ManifestRef, ManifestVersion: 1, ProviderRef: "p1", ParcelRefs: manifest.ParcelRefs, PackageCount: 1}); err != nil {
		t.Fatalf("legacy confirm: %v", err)
	}
	var mode string
	if err := repo.pool.QueryRow(ctx, `SELECT custody_mode FROM custody_missions WHERE mission_ref=$1`, mission).Scan(&mode); err != nil || mode != ModeLegacySingle {
		t.Fatalf("legacy anchor mode = %s %v", mode, err)
	}
	if err := repo.AssertMissionCompletable(ctx, mission); err != nil {
		t.Fatalf("legacy completion semantics changed: %v", err)
	}
	if _, err := repo.CollectManifests(ctx, CollectManifestsCommand{OperationRef: "multi-on-legacy-" + id, MissionRef: mission, TenantRef: tenant, StopRef: "stop", ProviderRef: "p1", HandoverChallengeRef: "c", HandoverVerifiedByRef: "v", SourceVersion: 1, Manifests: []ManifestKey{{ManifestRef: manifest.ManifestRef, ManifestVersion: 1}}}); err == nil {
		t.Fatal("multi-manifest operations must not run on legacy missions")
	}
}
