package migrations_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/securehandover"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type allowHandoverLimiter struct{}

func (allowHandoverLimiter) Allow(context.Context, string, int, time.Duration) error { return nil }

type handoverFixture struct {
	providerA string
	providerB string
	customer  string
	booking   string
	trip      string
}

func seedHandoverFixture(t *testing.T, policies ...securehandover.Policy) (*securehandover.Service, *pgxpool.Pool, handoverFixture, func()) {
	t.Helper()
	pool, cleanup := operationalTestPool(t)
	f := handoverFixture{providerA: uuid.NewString(), providerB: uuid.NewString(), customer: uuid.NewString(), booking: uuid.NewString(), trip: uuid.NewString()}
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO providers(id,phone,verification_status,is_active,onboarding_complete)
		VALUES($1,'+15550000001','verified',true,true),($2,'+15550000002','verified',true,true)`, f.providerA, f.providerB); err != nil {
		cleanup()
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dispatch_bookings(id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone)
		VALUES($1,$2,'accepted','pickup',6.5,3.4,'dropoff',6.6,3.5,1000,'receiver','+15551112222')`, f.booking, f.customer); err != nil {
		cleanup()
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO trips(id,booking_id,provider_id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone)
		VALUES($1,$2,$3,$4,'arrived_pickup','pickup',6.5,3.4,'dropoff',6.6,3.5,1000,'receiver','+15551112222')`, f.trip, f.booking, f.providerA, f.customer); err != nil {
		cleanup()
		t.Fatal(err)
	}
	secret := []byte("postgres-handover-test-secret")
	repository, err := securehandover.NewRepository(pool, secret)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	policy := securehandover.Policy{Enabled: true, QREnabled: true, MaxAttempts: 5}
	if len(policies) > 0 {
		policy = policies[0]
		policy.Enabled = true
	}
	service, err := securehandover.NewService(repository, secret, policy)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	service.WithRateLimiter(allowHandoverLimiter{})
	return service, pool, f, cleanup
}

func TestSecureHandoverOTPAndQRRaceConsumesOnce(t *testing.T) {
	service, _, f, cleanup := seedHandoverFixture(t)
	defer cleanup()
	ctx := context.Background()
	credential, err := service.IssuePickup(ctx, f.providerA, f.trip, "pickup", "issue-race")
	if err != nil {
		t.Fatal(err)
	}
	commands := []securehandover.VerifyCommand{
		{ChallengeRef: credential.ChallengeRef, Method: securehandover.MethodOTP, Credential: credential.OTP, ManifestRef: credential.ManifestRef, ManifestVersion: credential.ManifestVersion, OperationRef: "verify-otp"},
		{ChallengeRef: credential.ChallengeRef, Method: securehandover.MethodQR, Credential: credential.QRToken, ManifestRef: credential.ManifestRef, ManifestVersion: credential.ManifestVersion, OperationRef: "verify-qr"},
	}
	errs := make(chan error, len(commands))
	var wait sync.WaitGroup
	for _, command := range commands {
		wait.Add(1)
		go func(command securehandover.VerifyCommand) {
			defer wait.Done()
			_, verifyErr := service.VerifyPickupByCustomer(ctx, f.customer, f.booking, command)
			errs <- verifyErr
		}(command)
	}
	wait.Wait()
	close(errs)
	successes := 0
	for verifyErr := range errs {
		if verifyErr == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful consumers = %d, want 1", successes)
	}
}

func TestSecureHandoverIdentityChangeInvalidatesActiveChallenge(t *testing.T) {
	service, pool, f, cleanup := seedHandoverFixture(t)
	defer cleanup()
	ctx := context.Background()
	credential, err := service.IssuePickup(ctx, f.providerA, f.trip, "pickup", "issue-reassignment")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.VerifyPickupByCustomer(ctx, f.customer, f.booking, securehandover.VerifyCommand{
		ChallengeRef: credential.ChallengeRef, Method: securehandover.MethodOTP, Credential: "000000",
		ManifestRef: credential.ManifestRef, ManifestVersion: credential.ManifestVersion, OperationRef: "wrong-before-reassign",
	}); err == nil {
		t.Fatal("wrong credential unexpectedly verified")
	}
	// A provider reassignment is the authoritative identity mutation; migration
	// 047 invalidates any still-active challenge in the same database commit.
	if _, err = pool.Exec(ctx, `UPDATE trips SET provider_id=$2 WHERE id=$1`, f.trip, f.providerB); err != nil {
		t.Fatal(err)
	}
	if _, err = service.VerifyPickupByCustomer(ctx, f.customer, f.booking, securehandover.VerifyCommand{
		ChallengeRef: credential.ChallengeRef, Method: securehandover.MethodOTP, Credential: credential.OTP,
		ManifestRef: credential.ManifestRef, ManifestVersion: credential.ManifestVersion, OperationRef: "verify-after-reassign",
	}); err == nil {
		t.Fatal("old provider challenge verified after reassignment")
	}
}

func TestSecureHandoverMigrationApplyRollbackReapplyAndConstraints(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	down, err := os.ReadFile("000047_secure_handover_chain_of_custody.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile("000047_secure_handover_chain_of_custody.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("rollback 047: %v", err)
	}
	if _, err = pool.Exec(ctx, string(up)); err != nil {
		t.Fatalf("reapply 047: %v", err)
	}
	var definition string
	if err = pool.QueryRow(ctx, `SELECT pg_get_triggerdef(oid) FROM pg_trigger WHERE tgname='dispatch_trip_handover_identity_guard' AND NOT tgisinternal`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition, "provider_id") {
		t.Fatalf("identity trigger definition = %s", definition)
	}
}

type handoverSecretRecorder struct {
	intent securehandover.SecretDeliveryIntent
}

func (r *handoverSecretRecorder) Deliver(_ context.Context, intent securehandover.SecretDeliveryIntent) (string, string, []string, error) {
	r.intent = intent
	return "notification-1", "ACCEPTED", intent.Channels, nil
}

type allowEvidenceMedia struct{}

func (allowEvidenceMedia) AuthorizeEvidence(context.Context, string, []string, []string) error {
	return nil
}

func TestSecureDeliveryEvidenceGatesCompletionAndIsAppendOnly(t *testing.T) {
	policy := securehandover.Policy{Enabled: true, QREnabled: true, DeliveryEvidenceEnabled: true, InAppAllowed: true, MaxAttempts: 5}
	service, pool, f, cleanup := seedHandoverFixture(t, policy)
	defer cleanup()
	recorder := &handoverSecretRecorder{}
	service.WithNotifier(recorder).WithMediaAuthorizer(allowEvidenceMedia{})
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE trips SET status='in_progress' WHERE id=$1`, f.trip); err != nil {
		t.Fatal(err)
	}
	challenge, err := service.IssueDelivery(ctx, f.trip, "issue-delivery")
	if err != nil {
		t.Fatal(err)
	}
	if recorder.intent.OTP == "" || recorder.intent.Challenge.ChallengeRef != challenge.ChallengeRef {
		t.Fatal("receiver secret was not handed to notification authority")
	}
	_, err = service.VerifyDelivery(ctx, f.providerA, f.trip, securehandover.VerifyCommand{
		ChallengeRef: challenge.ChallengeRef, Method: securehandover.MethodOTP, Credential: recorder.intent.OTP,
		ManifestRef: challenge.ManifestRef, ManifestVersion: challenge.ManifestVersion, OperationRef: "verify-delivery",
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := service.DeliveryReady(ctx, f.trip)
	if err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("delivery became ready without required evidence")
	}
	session, err := service.CreateCaptureSession(ctx, f.providerA, f.trip, "delivery", securehandover.CaptureSessionCommand{
		Stage: securehandover.StageDelivery, EvidenceType: "PHOTO", ManifestRef: challenge.ManifestRef,
		ManifestVersion: challenge.ManifestVersion, OperationRef: "capture-delivery",
	})
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := service.SubmitEvidence(ctx, f.providerA, f.trip, "delivery", securehandover.EvidenceCommand{
		CaptureSessionRef: session.Ref, MediaRef: "media-private-1", CapturedAt: time.Now().UTC(), OperationRef: "evidence-delivery",
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err = service.DeliveryReady(ctx, f.trip)
	if err != nil || !ready {
		t.Fatalf("delivery ready = %v, error = %v", ready, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE dispatch_evidence_refs SET reason_code='tampered' WHERE evidence_ref=$1`, evidence.EvidenceRef); err == nil {
		t.Fatal("evidence UPDATE was not rejected")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM dispatch_evidence_refs WHERE evidence_ref=$1`, evidence.EvidenceRef); err == nil {
		t.Fatal("evidence DELETE was not rejected")
	}
}
