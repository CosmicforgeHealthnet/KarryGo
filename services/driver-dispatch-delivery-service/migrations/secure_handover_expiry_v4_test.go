package migrations_test

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/securehandover"
)

// Customer V4 §26.6: an expired challenge is refused (no custody transfer, no
// automatic bypass), the expiry sweep is idempotent, and re-issuing a
// credential leaves the old one unusable.
func TestSecureHandoverExpiryRefusesAndSweepsIdempotently(t *testing.T) {
	service, pool, f, cleanup := seedHandoverFixture(t)
	defer cleanup()
	ctx := context.Background()
	repository, err := securehandover.NewRepository(pool, []byte("postgres-handover-test-secret"))
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.IssuePickup(ctx, f.providerA, f.trip, "pickup", "issue-expiry-1")
	if err != nil {
		t.Fatal(err)
	}
	// Age the challenge past its expiry (created_at moves too, keeping the
	// table's own expires_at > created_at invariant).
	if _, err = pool.Exec(ctx, `UPDATE dispatch_handover_challenges SET created_at=now()-interval '2 hours',
		expires_at=now()-interval '1 minute' WHERE challenge_ref=$1`, first.ChallengeRef); err != nil {
		t.Fatal(err)
	}
	command := securehandover.VerifyCommand{ChallengeRef: first.ChallengeRef, Method: securehandover.MethodOTP, Credential: first.OTP,
		ManifestRef: first.ManifestRef, ManifestVersion: first.ManifestVersion, OperationRef: "verify-expired"}
	if _, err = service.VerifyPickupByCustomer(ctx, f.customer, f.booking, command); err == nil {
		t.Fatal("an expired credential was accepted")
	}
	var tripStatus string
	if err = pool.QueryRow(ctx, `SELECT status FROM trips WHERE id=$1`, f.trip).Scan(&tripStatus); err != nil || tripStatus != "arrived_pickup" {
		t.Fatalf("expiry moved the trip: status=%s err=%v", tripStatus, err)
	}

	// A second, still-active challenge is aged and swept; the sweep is
	// idempotent on repeat.
	second, err := service.IssuePickup(ctx, f.providerA, f.trip, "pickup", "issue-expiry-2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE dispatch_handover_challenges SET created_at=now()-interval '2 hours',
		expires_at=now()-interval '1 minute' WHERE challenge_ref=$1 AND status='ACTIVE'`, second.ChallengeRef); err != nil {
		t.Fatal(err)
	}
	swept, err := repository.Expire(ctx, time.Now().UTC(), 100)
	if err != nil || swept < 1 {
		t.Fatalf("first sweep expired %d (%v)", swept, err)
	}
	again, err := repository.Expire(ctx, time.Now().UTC(), 100)
	if err != nil || again != 0 {
		t.Fatalf("repeat sweep expired %d more (%v)", again, err)
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM dispatch_handover_challenges WHERE challenge_ref=$1`, second.ChallengeRef).Scan(&status); err != nil || status != "EXPIRED" {
		t.Fatalf("swept challenge status=%s err=%v", status, err)
	}

	// Re-issue after expiry: the new credential works, the expired one
	// never does, and custody moves exactly once.
	third, err := service.IssuePickup(ctx, f.providerA, f.trip, "pickup", "issue-expiry-3")
	if err != nil {
		t.Fatal(err)
	}
	stale := securehandover.VerifyCommand{ChallengeRef: second.ChallengeRef, Method: securehandover.MethodOTP, Credential: second.OTP,
		ManifestRef: second.ManifestRef, ManifestVersion: second.ManifestVersion, OperationRef: "verify-stale"}
	if _, err = service.VerifyPickupByCustomer(ctx, f.customer, f.booking, stale); err == nil {
		t.Fatal("a stale credential was accepted after re-issue")
	}
	fresh := securehandover.VerifyCommand{ChallengeRef: third.ChallengeRef, Method: securehandover.MethodOTP, Credential: third.OTP,
		ManifestRef: third.ManifestRef, ManifestVersion: third.ManifestVersion, OperationRef: "verify-fresh"}
	if _, err = service.VerifyPickupByCustomer(ctx, f.customer, f.booking, fresh); err != nil {
		t.Fatalf("the re-issued credential was refused: %v", err)
	}
}
