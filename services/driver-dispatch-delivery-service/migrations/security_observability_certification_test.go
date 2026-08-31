package migrations_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/deliveryrecovery"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/securehandover"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/observability"
	"github.com/google/uuid"
)

func TestLegacyCredentialIsolationMigrationShape(t *testing.T) {
	up, err := os.ReadFile("000052_legacy_handover_secret_isolation.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range []string{"RAISE EXCEPTION", "pickup_code_expires_at > now()", "trips_pickup_reference_only", "trips_delivery_reference_only", "^handover_"} {
		if !strings.Contains(string(up), term) {
			t.Fatalf("missing guard %s", term)
		}
	}
	down, err := os.ReadFile("000052_legacy_handover_secret_isolation.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(down), "DELETE") || strings.Contains(string(down), "DROP TABLE") {
		t.Fatal("destructive rollback")
	}
}

func TestLegacyCredentialIsolationApplyRollbackReapplyPostgres(t *testing.T) {
	_, pool, f, cleanup := seedHandoverFixture(t)
	defer cleanup()
	ctx := context.Background()
	up, _ := os.ReadFile("000052_legacy_handover_secret_isolation.up.sql")
	down, _ := os.ReadFile("000052_legacy_handover_secret_isolation.down.sql")
	if _, err := pool.Exec(ctx, `UPDATE trips SET pickup_code='123456' WHERE id=$1`, f.trip); err == nil {
		t.Fatal("raw OTP persisted")
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE trips SET pickup_code='123456',pickup_code_expires_at=now()+interval '15 minutes' WHERE id=$1`, f.trip); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, string(up)); err == nil {
		t.Fatal("active legacy credential silently invalidated")
	}
	_ = tx.Rollback(ctx)
	if _, err = pool.Exec(ctx, `UPDATE trips SET pickup_code_expires_at=now()-interval '1 minute' WHERE id=$1`, f.trip); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err = pool.QueryRow(ctx, `SELECT pickup_code FROM trips WHERE id=$1`, f.trip).Scan(&stored); err != nil || stored != "" {
		t.Fatal("historical secret not purged")
	}
	if _, err = pool.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
}

func TestSecureHandoverResendVersusVerifyRacePostgres(t *testing.T) {
	s, pool, f, cleanup := seedHandoverFixture(t, securehandover.Policy{Enabled: true, QREnabled: true, ResendCooldown: time.Nanosecond})
	defer cleanup()
	ctx := context.Background()
	c, err := s.IssuePickup(ctx, f.providerA, f.trip, "pickup", "issue-resend-race")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	outcomes := make(chan error, 2)
	go func() {
		defer wg.Done()
		_, e := s.Resend(ctx, securehandover.Actor{Type: "PROVIDER", Ref: f.providerA}, securehandover.ResendCommand{ChallengeRef: c.ChallengeRef, OperationRef: "resend-race"})
		outcomes <- e
	}()
	go func() {
		defer wg.Done()
		_, e := s.VerifyPickupByCustomer(ctx, f.customer, f.booking, securehandover.VerifyCommand{ChallengeRef: c.ChallengeRef, Method: securehandover.MethodQR, Credential: c.QRToken, ManifestRef: c.ManifestRef, ManifestVersion: c.ManifestVersion, OperationRef: "verify-race"})
		outcomes <- e
	}()
	wg.Wait()
	close(outcomes)
	success := 0
	for e := range outcomes {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("competing transitions successes=%d", success)
	}
	var active, consumed int
	if err = pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE status='ACTIVE'),count(*) FILTER(WHERE status='CONSUMED') FROM dispatch_handover_challenges WHERE trip_id=$1`, f.trip).Scan(&active, &consumed); err != nil {
		t.Fatal(err)
	}
	if active > 1 || consumed > 1 || active+consumed != 1 {
		t.Fatal("challenge race invariant broken")
	}
}

func TestHandoverAdversarialObjectManifestAndReplayPostgres(t *testing.T) {
	s, _, f, cleanup := seedHandoverFixture(t)
	defer cleanup()
	ctx := context.Background()
	c, err := s.IssuePickup(ctx, f.providerA, f.trip, "pickup", "issue-negative")
	if err != nil {
		t.Fatal(err)
	}
	cmd := securehandover.VerifyCommand{ChallengeRef: c.ChallengeRef, Method: securehandover.MethodQR, Credential: c.QRToken, ManifestRef: c.ManifestRef, ManifestVersion: c.ManifestVersion, OperationRef: "verify-foreign-customer"}
	if _, err = s.VerifyPickupByCustomer(ctx, uuid.NewString(), f.booking, cmd); err == nil {
		t.Fatal("foreign customer accepted")
	}
	wrong := cmd
	wrong.OperationRef = "verify-wrong-manifest"
	wrong.ManifestVersion++
	if _, err = s.VerifyPickupByCustomer(ctx, f.customer, f.booking, wrong); err == nil {
		t.Fatal("wrong manifest accepted")
	}
	cmd.OperationRef = "verify-correct"
	if _, err = s.VerifyPickupByCustomer(ctx, f.customer, f.booking, cmd); err != nil {
		t.Fatal(err)
	}
	cmd.OperationRef = "replay-another-operation"
	if _, err = s.VerifyPickupByCustomer(ctx, f.customer, f.booking, cmd); err == nil {
		t.Fatal("credential consumed twice")
	}
}

func TestHandoverReassignmentVersusVerificationRacePostgres(t *testing.T) {
	s, pool, f, cleanup := seedHandoverFixture(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := s.IssuePickup(ctx, f.providerA, f.trip, "pickup", "issue-provider-race")
	if err != nil {
		t.Fatal(err)
	}
	command := securehandover.VerifyCommand{ChallengeRef: c.ChallengeRef, Method: securehandover.MethodOTP,
		Credential: c.OTP, ManifestRef: c.ManifestRef, ManifestVersion: c.ManifestVersion, OperationRef: "verify-provider-race"}
	start := make(chan struct{})
	reassigned, verified := make(chan error, 1), make(chan error, 1)
	go func() {
		<-start
		_, e := pool.Exec(ctx, `UPDATE trips SET provider_id=$2 WHERE id=$1`, f.trip, f.providerB)
		reassigned <- e
	}()
	go func() {
		<-start
		_, e := s.VerifyPickupByCustomer(ctx, f.customer, f.booking, command)
		verified <- e
	}()
	close(start)
	if err = <-reassigned; err != nil {
		t.Fatal(err)
	}
	verificationErr := <-verified
	// Verification may win before reassignment, but never after its commit.
	// A different operation identity deliberately rules out an idempotent read-back.
	command.OperationRef = "verify-after-provider-race"
	if _, err = s.VerifyPickupByCustomer(ctx, f.customer, f.booking, command); err == nil {
		t.Fatal("old provider credential authorized after reassignment commit")
	}
	if _, err = s.IssuePickup(ctx, f.providerA, f.trip, "pickup", "old-provider-reissue"); err == nil {
		t.Fatal("old provider issued another credential")
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM dispatch_handover_challenges WHERE challenge_ref=$1`, c.ChallengeRef).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if verificationErr == nil && status != "CONSUMED" || verificationErr != nil && status != "INVALIDATED" {
		t.Fatalf("reassignment/verification did not serialize: status=%s verification=%v", status, verificationErr)
	}
}

func TestEmergencyRecoveryVersionAuditDoesNotVerifyHandoverPostgres(t *testing.T) {
	s, pool, f, cleanup := seedHandoverFixture(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := s.IssuePickup(ctx, f.providerA, f.trip, "pickup", "emergency-challenge"); err != nil {
		t.Fatal(err)
	}
	r := deliveryrecovery.New(deliveryrecovery.NewRepository(pool), deliveryrecovery.Policy{Enabled: true, ReplacementEnabled: true})
	if _, err := r.ReportIncident(ctx, deliveryrecovery.IncidentCommand{TripID: f.trip, ProviderID: f.providerA, ReasonCode: "BREAKDOWN", IdempotencyKey: "incident-emergency"}); err != nil {
		t.Fatal(err)
	}
	command := deliveryrecovery.EmergencyRecoveryCommand{TripID: f.trip, ActorRef: "admin-backoffice-service:operator-a", ReasonCode: "CUSTODY_SAFETY", ExpectedTripVersion: 999, IdempotencyKey: "emergency-a"}
	if _, err := r.RecordEmergencyRecovery(ctx, command); err == nil {
		t.Fatal("stale emergency version accepted")
	}
	if err := pool.QueryRow(ctx, `SELECT source_version FROM trips WHERE id=$1`, f.trip).Scan(&command.ExpectedTripVersion); err != nil {
		t.Fatal(err)
	}
	item, err := r.RecordEmergencyRecovery(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := r.RecordEmergencyRecovery(ctx, command)
	if err != nil || !replay.Replayed || replay.ID != item.ID {
		t.Fatal("emergency replay changed result")
	}
	var verified int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_handover_challenges WHERE trip_id=$1 AND status='CONSUMED'`, f.trip).Scan(&verified); err != nil || verified != 0 {
		t.Fatal("emergency forged verification")
	}
	if _, err = pool.Exec(ctx, `DELETE FROM dispatch_emergency_recovery_actions WHERE id=$1`, item.ID); err == nil {
		t.Fatal("emergency audit deleted")
	}
}

func TestDurableTelemetryEmptyBacklogsPostgres(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	if err := observability.Collect(ctx, pool); err != nil {
		t.Fatal(err)
	}
	rendered := observability.Metrics.Render()
	for _, metric := range []string{"dispatch_queue_depth", "dispatch_event_outbox_depth", "dispatch_reconciliation_backlog", "dispatch_booking_notification_backlog"} {
		if !strings.Contains(rendered, metric) {
			t.Fatalf("empty metric missing: %s", metric)
		}
	}
}

func TestHandoverExpiryAndAttemptCeilingPostgres(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "attempt_limit", true: "expiry"}[expired], func(t *testing.T) {
			policy := securehandover.Policy{Enabled: true, QREnabled: true, MaxAttempts: 3}
			if expired {
				policy.TTL = 2 * time.Millisecond
			}
			s, pool, f, cleanup := seedHandoverFixture(t, policy)
			defer cleanup()
			ctx := context.Background()
			c, err := s.IssuePickup(ctx, f.providerA, f.trip, "pickup", "issue-limits")
			if err != nil {
				t.Fatal(err)
			}
			cmd := securehandover.VerifyCommand{ChallengeRef: c.ChallengeRef, Method: securehandover.MethodOTP, Credential: c.OTP, ManifestRef: c.ManifestRef, ManifestVersion: c.ManifestVersion, OperationRef: "correct-after-limit"}
			if expired {
				time.Sleep(3 * time.Millisecond)
			} else {
				for _, operation := range []string{"wrong-a", "wrong-b", "wrong-c"} {
					wrong := cmd
					wrong.OperationRef = operation
					wrong.Credential = "not-a-code"
					if _, err = s.VerifyPickupByCustomer(ctx, f.customer, f.booking, wrong); err == nil {
						t.Fatal("wrong code accepted")
					}
				}
			}
			if _, err = s.VerifyPickupByCustomer(ctx, f.customer, f.booking, cmd); err == nil {
				t.Fatal("expired/locked credential accepted")
			}
			var status string
			if err = pool.QueryRow(ctx, `SELECT status FROM dispatch_handover_challenges WHERE challenge_ref=$1`, c.ChallengeRef).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if expired && status != "EXPIRED" || !expired && status != "LOCKED" {
				t.Fatalf("terminal status %s", status)
			}
		})
	}
}
