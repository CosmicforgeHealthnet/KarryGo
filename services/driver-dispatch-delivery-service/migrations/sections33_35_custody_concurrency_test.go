package migrations_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/custodyops"
)

func TestSections33To35MigrationRollbackReapply(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	down, err := os.ReadFile("000056_sections33_35_custody_recovery.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile("000056_sections33_35_custody_recovery.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(context.Background(), string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	if _, err = pool.Exec(context.Background(), string(up)); err != nil {
		t.Fatalf("up: %v", err)
	}
	var table string
	if err = pool.QueryRow(context.Background(), `SELECT 'custody_manifests'::regclass::text`).Scan(&table); err != nil {
		t.Fatal(err)
	}
}

type allowCustodyMedia struct{}

func (allowCustodyMedia) ValidateCustodyMedia(context.Context, custodyops.EvidenceCommand) error {
	return nil
}

func custodyManifest(id string, version int64) custodyops.FreezeManifestCommand {
	return custodyops.FreezeManifestCommand{OperationRef: "freeze-" + id, MissionRef: "mission-" + id, TenantRef: "tenant-" + id, BusinessRef: "business-" + id, BranchRef: "branch-" + id, ManifestRef: "manifest-" + id, ManifestVersion: version, PickupStopRef: "stop-" + id, ReceiverChallengeRef: "receiver-old-" + id, Parcels: []custodyops.Parcel{{ParcelRef: "parcel-a-" + id, StopRef: "stop-" + id, Count: 1}, {ParcelRef: "parcel-b-" + id, StopRef: "stop-" + id, Count: 1}}, CreatedAt: time.Now().UTC()}
}

func TestSections33To35ManifestFreezeAndTransferRaces(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	repo := custodyops.NewRepository(pool, "DISPATCH")
	id := uuid.NewString()
	first := custodyManifest(id, 1)
	stale := custodyManifest(id, 2)
	stale.OperationRef = "freeze-stale-" + id
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, command := range []custodyops.FreezeManifestCommand{first, stale} {
		wg.Add(1)
		go func(command custodyops.FreezeManifestCommand) {
			defer wg.Done()
			<-start
			_, err := repo.FreezeManifest(context.Background(), command)
			errs <- err
		}(command)
	}
	close(start)
	wg.Wait()
	close(errs)
	success := 0
	conflict := 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, custodyops.ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("freeze results success=%d conflict=%d", success, conflict)
	}
	manifest, err := repo.FreezeManifest(context.Background(), first)
	if err != nil {
		if !errors.Is(err, custodyops.ErrConflict) {
			t.Fatal(err)
		}
		first = stale
		manifest, err = repo.FreezeManifest(context.Background(), stale)
		if err != nil {
			t.Fatal(err)
		}
	}
	confirm := custodyops.ConfirmParcelsCommand{OperationRef: "confirm-" + id, MissionRef: manifest.MissionRef, TenantRef: manifest.TenantRef, BusinessRef: manifest.BusinessRef, BranchRef: manifest.BranchRef, PickupStopRef: manifest.PickupStopRef, ManifestRef: manifest.ManifestRef, ManifestVersion: manifest.ManifestVersion, ProviderRef: "provider-old-" + id, ParcelRefs: manifest.ParcelRefs, PackageCount: manifest.PackageCount}
	if _, err = repo.ConfirmParcels(context.Background(), confirm); err != nil {
		t.Fatal(err)
	}
	incident := custodyops.IncidentCommand{OperationRef: "incident-" + id, MissionRef: manifest.MissionRef, TenantRef: manifest.TenantRef, ProviderRef: confirm.ProviderRef, Reason: custodyops.IncidentBreakdown, OccurredAt: time.Now().UTC()}
	if _, err = repo.ReportIncident(context.Background(), incident); err != nil {
		t.Fatal(err)
	}
	replacement := "provider-new-" + id
	if _, err = repo.AssignReplacement(context.Background(), custodyops.AssignReplacementCommand{OperationRef: "replacement-" + id, MissionRef: manifest.MissionRef, TenantRef: manifest.TenantRef, OldProviderRef: confirm.ProviderRef, NewProviderRef: replacement, VehicleClass: "MOTORCYCLE", CapacityCount: manifest.PackageCount, ComplianceRef: "compliance-" + id, AvailabilityRef: "availability-" + id, ManifestRef: manifest.ManifestRef, ManifestVersion: manifest.ManifestVersion, ReceiverChallenge: manifest.ReceiverChallengeRef}); err != nil {
		t.Fatal(err)
	}
	evidence, err := repo.RecordEvidence(context.Background(), custodyops.EvidenceCommand{OperationRef: "evidence-" + id, MissionRef: manifest.MissionRef, TenantRef: manifest.TenantRef, ManifestRef: manifest.ManifestRef, ManifestVersion: manifest.ManifestVersion, ParcelRefs: manifest.ParcelRefs, EvidenceType: custodyops.EvidenceProviderTransfer, MediaRef: "media-" + id, ProviderRef: confirm.ProviderRef, ActorRef: confirm.ProviderRef, CapturedAt: time.Now().UTC(), AccessPolicy: "MISSION_PARTICIPANTS"}, allowCustodyMedia{})
	if err != nil {
		t.Fatal(err)
	}
	transfer := custodyops.TransferCommand{MissionRef: manifest.MissionRef, TenantRef: manifest.TenantRef, TransferRef: "transfer-" + id, OldProviderRef: confirm.ProviderRef, NewProviderRef: replacement, ManifestRef: manifest.ManifestRef, ManifestVersion: manifest.ManifestVersion, ParcelRefs: manifest.ParcelRefs, PackageCount: manifest.PackageCount, EvidenceRef: evidence.ID, Latitude: 1, Longitude: 1, OldProviderConfirm: true, NewProviderConfirm: true, NewReceiverChallengeRef: "receiver-new-" + id}
	start = make(chan struct{})
	errs = make(chan error, 2)
	wg = sync.WaitGroup{}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			command := transfer
			command.OperationRef = "transfer-op-" + uuid.NewString()
			<-start
			_, e := repo.CompleteTransfer(context.Background(), command)
			errs <- e
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	success, conflict = 0, 0
	for err = range errs {
		if err == nil {
			success++
		} else if errors.Is(err, custodyops.ErrConflict) || errors.Is(err, custodyops.ErrUnauthorized) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("transfer results success=%d conflict=%d", success, conflict)
	}
}

func TestSections33To35CrossScopeAndEvidenceTypeProtection(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	repo := custodyops.NewRepository(pool, "DISPATCH")
	id := uuid.NewString()
	command := custodyManifest(id, 1)
	manifest, err := repo.FreezeManifest(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	wrong := custodyops.ConfirmParcelsCommand{OperationRef: "wrong-" + id, MissionRef: manifest.MissionRef, TenantRef: manifest.TenantRef, BusinessRef: manifest.BusinessRef, BranchRef: "other-branch", PickupStopRef: manifest.PickupStopRef, ManifestRef: manifest.ManifestRef, ManifestVersion: manifest.ManifestVersion, ProviderRef: "provider", ParcelRefs: manifest.ParcelRefs, PackageCount: manifest.PackageCount}
	if _, err = repo.ConfirmParcels(context.Background(), wrong); !errors.Is(err, custodyops.ErrConflict) {
		t.Fatalf("cross-branch error=%v", err)
	}
	if _, err = repo.RecordEvidence(context.Background(), custodyops.EvidenceCommand{OperationRef: "media-" + id, MissionRef: manifest.MissionRef, TenantRef: "other-tenant", ManifestRef: manifest.ManifestRef, ManifestVersion: manifest.ManifestVersion, EvidenceType: custodyops.EvidenceReturn, MediaRef: "cross-mission", ActorRef: "actor", CapturedAt: time.Now(), AccessPolicy: "MISSION_PARTICIPANTS"}, allowCustodyMedia{}); !errors.Is(err, custodyops.ErrConflict) {
		t.Fatalf("cross-tenant evidence error=%v", err)
	}
}
