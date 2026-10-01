package custodyops

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type integrationMedia struct{}

func (integrationMedia) ValidateCustodyMedia(context.Context, EvidenceCommand) error { return nil }

func integrationRepository(t *testing.T) *Repository {
	t.Helper()
	dsn := os.Getenv("CUSTODY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CUSTODY_TEST_DATABASE_URL is required for live PostgreSQL custody certification")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var exists bool
	if err = pool.QueryRow(context.Background(), `SELECT to_regclass('public.custody_manifests') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatalf("custody migration is not applied: %v", err)
	}
	return NewRepository(pool, "CERTIFICATION")
}

func TestPostgresReturnCompletionIdempotencyAndEvidenceIsolation(t *testing.T) {
	repo := integrationRepository(t)
	id := uuid.NewString()
	manifest, err := repo.FreezeManifest(context.Background(), FreezeManifestCommand{OperationRef: "freeze-" + id, MissionRef: "mission-" + id, TenantRef: "tenant-" + id, BusinessRef: "business-" + id, BranchRef: "branch-" + id, ManifestRef: "manifest-" + id, ManifestVersion: 1, PickupStopRef: "stop-" + id, ReceiverChallengeRef: "receiver-old-" + id, Parcels: []Parcel{{ParcelRef: "parcel-" + id, StopRef: "stop-" + id, Count: 1}}, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	confirm := ConfirmParcelsCommand{OperationRef: "confirm-" + id, MissionRef: manifest.MissionRef, TenantRef: manifest.TenantRef, BusinessRef: manifest.BusinessRef, BranchRef: manifest.BranchRef, PickupStopRef: manifest.PickupStopRef, ManifestRef: manifest.ManifestRef, ManifestVersion: 1, ProviderRef: "provider-" + id, ParcelRefs: manifest.ParcelRefs, PackageCount: 1}
	if _, err = repo.ConfirmParcels(context.Background(), confirm); err != nil {
		t.Fatal(err)
	}
	evidence, err := repo.RecordEvidence(context.Background(), EvidenceCommand{OperationRef: "return-evidence-" + id, MissionRef: manifest.MissionRef, TenantRef: manifest.TenantRef, ManifestRef: manifest.ManifestRef, ManifestVersion: 1, ParcelRefs: manifest.ParcelRefs, EvidenceType: EvidenceReturn, MediaRef: "media-" + id, ProviderRef: confirm.ProviderRef, ActorRef: confirm.ProviderRef, CapturedAt: time.Now(), AccessPolicy: "MISSION_PARTICIPANTS"}, integrationMedia{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.StartReturn(context.Background(), ReturnCommand{OperationRef: "return-start-" + id, MissionRef: manifest.MissionRef, TenantRef: manifest.TenantRef, OriginalManifest: manifest.ManifestRef, OriginalVersion: 1, ReturnManifest: "return-manifest-" + id, ParcelRefs: manifest.ParcelRefs, ParcelCondition: "SEALED", DestinationRef: manifest.BranchRef, ReceiverRef: manifest.BusinessRef, Reason: "RECEIVER_UNAVAILABLE", CategoryPolicy: "RETAIL_RETURN_TO_VENDOR", EvidenceRef: evidence.ID}); err != nil {
		t.Fatal(err)
	}
	complete := ReturnCompletionCommand{OperationRef: "return-complete-" + id, MissionRef: manifest.MissionRef, TenantRef: manifest.TenantRef, ProviderRef: confirm.ProviderRef, ReturnManifest: "return-manifest-" + id, ParcelRefs: manifest.ParcelRefs, EvidenceRef: evidence.ID}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, e := repo.CompleteReturn(context.Background(), complete)
			errs <- e
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err = range errs {
		if err != nil {
			t.Fatalf("idempotent return completion: %v", err)
		}
	}
	wrong := complete
	wrong.OperationRef = "wrong-return-" + id
	wrong.EvidenceRef = "other-mission-evidence"
	if _, err = repo.CompleteReturn(context.Background(), wrong); !errors.Is(err, ErrConflict) {
		t.Fatalf("cross-mission evidence error=%v", err)
	}
}

func TestPostgresFinancialAllocationBoundsAndUnknownOutcome(t *testing.T) {
	repo := integrationRepository(t)
	id := uuid.NewString()
	manifest, err := repo.FreezeManifest(context.Background(), FreezeManifestCommand{OperationRef: "freeze-fin-" + id, MissionRef: "mission-fin-" + id, TenantRef: "tenant-fin-" + id, BusinessRef: "business-fin-" + id, BranchRef: "branch-fin-" + id, ManifestRef: "manifest-fin-" + id, ManifestVersion: 1, PickupStopRef: "stop-fin-" + id, ReceiverChallengeRef: "receiver-fin-" + id, Parcels: []Parcel{{ParcelRef: "parcel-fin-" + id, StopRef: "stop-fin-" + id, Count: 1}}, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	base := AllocationCommand{OperationRef: "allocation-" + id, MissionRef: manifest.MissionRef, TenantRef: manifest.TenantRef, PolicyRef: "milestone-policy-v1", ApprovedTotalMinor: 1000, Currency: "NGN", WalletOperationRef: "wallet-" + id, WalletStatus: "UNKNOWN", Allocations: []ProviderAllocation{{ProviderRef: "old-" + id, AmountMinor: 400}, {ProviderRef: "new-" + id, AmountMinor: 600}}}
	if _, err = repo.FinalizeAllocation(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.FinalizeAllocation(context.Background(), base); err != nil {
		t.Fatalf("unknown outcome retry duplicated: %v", err)
	}
	over := base
	over.OperationRef = "allocation-over-" + id
	over.WalletOperationRef = "wallet-over-" + id
	over.Allocations[1].AmountMinor = 601
	if _, err = repo.FinalizeAllocation(context.Background(), over); !errors.Is(err, ErrPolicy) {
		t.Fatalf("over-allocation error=%v", err)
	}
}
