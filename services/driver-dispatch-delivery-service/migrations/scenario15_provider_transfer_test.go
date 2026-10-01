package migrations_test

// Section 25 acceptance scenario 15: a Dispatch provider transfer after pickup
// moves custody to the replacement rider and keeps every piece of history --
// the parcel, its manifest binding, the carrier history and the transfer leg.
// It runs the real commercemission repository against the real migration
// chain. The shared multi-manifest custody move is certified separately by
// custodyops.TestPostgresPartialPickupReplacementTransfersExactInventory.

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercemission"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

func TestScenario15PostCustodyTransferPreservesParcelManifestAndCustodyHistory(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	f := seedOperationalFixture(t, pool)
	ctx := context.Background()
	secret := []byte("scenario-15-handover-secret-0123456789")
	now := time.Now().UTC()
	const lat, lng = 6.52, 3.41

	// Dispatch has already chosen a compatible replacement (its own matcher);
	// the original rider still holds the parcel.
	if _, err := pool.Exec(ctx, `UPDATE commerce_missions SET replacement_provider_id=$2, replacement_trip_id=$3,
		replacement_state='ASSIGNED' WHERE id=$1`, f.missionID, f.providerB, f.tripB); err != nil {
		t.Fatal(err)
	}
	var manifestRefBefore string
	var manifestVersionBefore int64
	if err := pool.QueryRow(ctx, `SELECT manifest_ref, manifest_version FROM commerce_mission_parcels
		WHERE commerce_mission_id=$1 AND parcel_ref=$2`, f.missionID, f.parcelRef).Scan(&manifestRefBefore, &manifestVersionBefore); err != nil {
		t.Fatal(err)
	}

	service := commercemission.NewOperationalService(commercemission.NewRepository(pool), secret, func() time.Time { return now }).
		WithMediaAttachmentAuthorizer(approveMissionMedia{})
	credential, err := service.StartProviderTransfer(ctx, f.providerA, f.missionRef, commercemission.StartTransferCommand{
		OperationRef: "s15-start", ParcelRefs: []string{f.parcelRef}, PackageCount: 1, Latitude: lat, Longitude: lng,
	})
	if err != nil {
		t.Fatalf("start transfer: %v", err)
	}
	complete := commercemission.CompleteTransferCommand{
		OperationRef: "s15-complete", Code: credential.ChallengeCode, ParcelRefs: []string{f.parcelRef}, PackageCount: 1,
		Latitude: lat, Longitude: lng,
		Evidence: dispatchcommerce.EvidenceInput{CapturedAt: now, MediaRefs: []string{"media-s15"}, Description: "handover to replacement"},
	}

	// Only the assigned replacement may accept custody.
	if _, err = service.CompleteProviderTransfer(ctx, f.providerA, f.missionRef, credential.TransferRef, complete); !isStatus(err, 403) {
		t.Fatalf("original rider completed its own transfer: %v", err)
	}
	if _, err = service.CompleteProviderTransfer(ctx, f.providerB, f.missionRef, credential.TransferRef, complete); err != nil {
		t.Fatalf("complete transfer: %v", err)
	}
	// A replayed completion changes nothing.
	if _, err = service.CompleteProviderTransfer(ctx, f.providerB, f.missionRef, credential.TransferRef, complete); err != nil {
		t.Fatalf("replayed completion: %v", err)
	}

	// Parcel: same row, same manifest binding, now held by the replacement.
	var custodian, status, manifestRef string
	var manifestVersion, parcels int64
	if err = pool.QueryRow(ctx, `SELECT custodian_ref, status, manifest_ref, manifest_version,
		(SELECT count(*) FROM commerce_mission_parcels WHERE commerce_mission_id=$1)
		FROM commerce_mission_parcels WHERE commerce_mission_id=$1 AND parcel_ref=$2`, f.missionID, f.parcelRef).
		Scan(&custodian, &status, &manifestRef, &manifestVersion, &parcels); err != nil {
		t.Fatal(err)
	}
	if custodian != f.providerB || status != "IN_TRANSIT" || parcels != 1 {
		t.Fatalf("parcel after transfer: custodian=%s status=%s rows=%d", custodian, status, parcels)
	}
	if manifestRef != manifestRefBefore || manifestVersion != manifestVersionBefore {
		t.Fatalf("manifest binding rewritten: %s/%d -> %s/%d", manifestRefBefore, manifestVersionBefore, manifestRef, manifestVersion)
	}

	// Carrier history: the original custody is closed, not erased.
	type carrier struct {
		provider string
		ended    bool
		reason   string
	}
	rows, err := pool.Query(ctx, `SELECT provider_id::text, custody_ended_at IS NOT NULL, COALESCE(end_reason,'')
		FROM commerce_mission_carrier_history WHERE commerce_mission_id=$1 ORDER BY custody_started_at, provider_id`, f.missionID)
	if err != nil {
		t.Fatal(err)
	}
	var history []carrier
	for rows.Next() {
		var c carrier
		if err = rows.Scan(&c.provider, &c.ended, &c.reason); err != nil {
			t.Fatal(err)
		}
		history = append(history, c)
	}
	rows.Close()
	if len(history) != 2 {
		t.Fatalf("carrier history has %d rows, want the original and the replacement: %+v", len(history), history)
	}
	byProvider := map[string]carrier{history[0].provider: history[0], history[1].provider: history[1]}
	if a := byProvider[f.providerA]; !a.ended || a.reason != "PROVIDER_TRANSFER" {
		t.Fatalf("original custody not closed as a transfer: %+v", a)
	}
	if b := byProvider[f.providerB]; b.ended {
		t.Fatalf("replacement custody already ended: %+v", b)
	}

	// Transfer leg and transfer record are kept, completed.
	var legState, transferState string
	if err = pool.QueryRow(ctx, `SELECT l.state, t.state FROM commerce_mission_legs l
		JOIN commerce_provider_transfers t ON t.transfer_ref=l.leg_ref
		WHERE l.commerce_mission_id=$1 AND l.leg_type='TRANSFER'`, f.missionID).Scan(&legState, &transferState); err != nil {
		t.Fatal(err)
	}
	if legState != "COMPLETED" || transferState != "COMPLETED" {
		t.Fatalf("leg=%s transfer=%s, want COMPLETED/COMPLETED", legState, transferState)
	}

	// The mission now runs on the replacement's trip; the stop history is untouched.
	var currentProvider, tripID, stopStatus string
	if err = pool.QueryRow(ctx, `SELECT m.current_provider_id::text, m.trip_id::text, s.status
		FROM commerce_missions m JOIN commerce_mission_stops s ON s.commerce_mission_id=m.id WHERE m.id=$1`, f.missionID).
		Scan(&currentProvider, &tripID, &stopStatus); err != nil {
		t.Fatal(err)
	}
	if currentProvider != f.providerB || tripID != f.tripB || stopStatus != "PICKED_UP" {
		t.Fatalf("mission after transfer: provider=%s trip=%s stop=%s", currentProvider, tripID, stopStatus)
	}
}

func isStatus(err error, status int) bool {
	var appErr *apperrors.Error
	return errors.As(err, &appErr) && appErr.Status == status
}

// approveMissionMedia stands in for Media's binding verdict; the Media
// authorization boundary itself is certified by TestProviderTransferUsesSameMediaAuthorizationBoundary.
type approveMissionMedia struct{}

func (approveMissionMedia) AuthorizeEvidence(context.Context, string, []string, []string) error { return nil }
