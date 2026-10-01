package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CUST-10 / CFC-018: when the provider changes before pickup, the old
// provider must lose the credential.
//
// This is the case where a documentation-level answer is not good enough. The
// guarantee is a database trigger, and a trigger is exactly the kind of thing
// that survives a refactor in name only — the function can still exist while
// no longer firing, or fire on a column list that no longer includes the one
// that changed. So this certifies it by reassigning a real trip on the real
// migration chain and reading back what happened to the challenge.
func seedChallengeFixture(t *testing.T, pool *pgxpool.Pool) (tripID, bookingID, providerA, providerB, challengeRef string) {
	t.Helper()
	ctx := context.Background()
	tripID, bookingID = uuid.NewString(), uuid.NewString()
	providerA, providerB = uuid.NewString(), uuid.NewString()
	customer := uuid.NewString()
	challengeRef = "chg_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	if _, err := pool.Exec(ctx, `INSERT INTO providers(id,phone,verification_status,is_active,onboarding_complete)
		VALUES($1,$2,'verified',true,true),($3,$4,'verified',true,true)`,
		providerA, "+23482"+providerA[:8], providerB, "+23483"+providerB[:8]); err != nil {
		t.Fatalf("seed providers: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_bookings(id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone)
		VALUES($1,$2,'accepted','pickup',6.5,3.4,'dropoff',6.6,3.5,1000,'receiver','+2348000000000')`,
		bookingID, customer); err != nil {
		t.Fatalf("seed booking: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO trips(id,booking_id,provider_id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone)
		VALUES($1,$2,$3,$4,'assigned','pickup',6.5,3.4,'dropoff',6.6,3.5,1000,'receiver','+2348000000000')`,
		tripID, bookingID, providerA, customer); err != nil {
		t.Fatalf("seed trip: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_handover_challenges(
			challenge_ref, context_key, version, stage, mission_ref, stop_ref, trip_id, booking_id,
			manifest_ref, manifest_version, assigned_provider_id, intended_recipient_ref,
			recipient_type, otp_hash, qr_token_hash, expires_at, max_attempts,
			created_by_source, source_version)
		VALUES($1,$2,1,'PICKUP','mission-ref','stop-ref',$3,$4,'manifest-ref',1,$5,$6,
			'PROVIDER','otp-hash','qr-hash', now() + interval '15 minutes', 5, 'dispatch', 1)`,
		challengeRef, "ctx_"+challengeRef, tripID, bookingID, providerA, providerA); err != nil {
		t.Fatalf("seed challenge: %v", err)
	}
	return tripID, bookingID, providerA, providerB, challengeRef
}

func challengeState(t *testing.T, pool *pgxpool.Pool, challengeRef string) (status, reason string) {
	t.Helper()
	if err := pool.QueryRow(context.Background(),
		`SELECT status, invalidated_reason FROM dispatch_handover_challenges WHERE challenge_ref=$1`,
		challengeRef).Scan(&status, &reason); err != nil {
		t.Fatalf("read challenge: %v", err)
	}
	return status, reason
}

func TestProviderReassignmentBeforePickupRevokesTheOldCredential(t *testing.T) {
	pool, done := operationalTestPool(t)
	defer done()
	ctx := context.Background()

	tripID, _, _, providerB, challengeRef := seedChallengeFixture(t, pool)

	if status, _ := challengeState(t, pool, challengeRef); status != "ACTIVE" {
		t.Fatalf("the credential did not start ACTIVE: %s", status)
	}

	if _, err := pool.Exec(ctx,
		`UPDATE trips SET provider_id=$2 WHERE id=$1`, tripID, providerB); err != nil {
		t.Fatalf("reassign provider: %v", err)
	}

	status, reason := challengeState(t, pool, challengeRef)
	if status != "INVALIDATED" {
		t.Fatalf("the old provider's credential is still %s after reassignment", status)
	}
	if reason != "PROVIDER_REASSIGNED" {
		t.Fatalf("invalidated_reason = %q; the audit trail must say why", reason)
	}

	// CFC-016: the revoked credential is not deleted. A custody question
	// asked later has to be answerable, and a row that was removed answers
	// nothing.
	var remaining int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM dispatch_handover_challenges WHERE challenge_ref=$1`,
		challengeRef).Scan(&remaining); err != nil {
		t.Fatalf("count challenge: %v", err)
	}
	if remaining != 1 {
		t.Fatal("the revoked credential was deleted rather than invalidated")
	}
}

// A terminal credential can never be reactivated, so an invalidated one
// cannot be quietly switched back on for the old provider after the fact.
func TestRevokedCredentialCannotBeReactivated(t *testing.T) {
	pool, done := operationalTestPool(t)
	defer done()
	ctx := context.Background()

	tripID, _, _, providerB, challengeRef := seedChallengeFixture(t, pool)
	if _, err := pool.Exec(ctx, `UPDATE trips SET provider_id=$2 WHERE id=$1`, tripID, providerB); err != nil {
		t.Fatalf("reassign provider: %v", err)
	}

	_, err := pool.Exec(ctx,
		`UPDATE dispatch_handover_challenges SET status='ACTIVE', invalidated_at=NULL WHERE challenge_ref=$1`,
		challengeRef)
	if err == nil {
		t.Fatal("a terminal credential was reactivated")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "cannot be reactivated") {
		t.Fatalf("reactivation failed for an unrelated reason: %v", err)
	}
}

// Changing the receiver revokes the credential too: a delivery credential
// bound to one receiver must not survive being pointed at another.
func TestReceiverChangeRevokesTheCredential(t *testing.T) {
	pool, done := operationalTestPool(t)
	defer done()
	ctx := context.Background()

	tripID, _, _, _, challengeRef := seedChallengeFixture(t, pool)
	if _, err := pool.Exec(ctx,
		`UPDATE trips SET receiver_phone='+2348099999999' WHERE id=$1`, tripID); err != nil {
		t.Fatalf("change receiver: %v", err)
	}

	status, reason := challengeState(t, pool, challengeRef)
	if status != "INVALIDATED" || reason != "RECEIVER_CHANGED" {
		t.Fatalf("status=%s reason=%q after a receiver change", status, reason)
	}

}
