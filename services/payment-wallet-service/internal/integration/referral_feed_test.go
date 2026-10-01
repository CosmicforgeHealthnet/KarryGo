package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
	"cosmicforge/logistics/shared/go/outbox"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// heldJobPayment funds a customer wallet through a real top-up and holds a
// job payment from it, exactly as a booking service would.
func heldJobPayment(t *testing.T, repo *walletrepositories.PostgresWalletRepository, amount int64, fee int64) walletmodels.PaymentIntent {
	t.Helper()
	ctx := context.Background()
	customerID := "cust_" + uuid.NewString()
	paystackRef := "ps_" + uuid.NewString()
	if _, _, err := repo.CreatePaymentIntent(ctx, walletrepositories.CreatePaymentIntentInput{
		SourceService: "payment-wallet-service", SourceReference: "topup_" + uuid.NewString(),
		CustomerID: customerID, AmountKobo: amount, Currency: "NGN",
		PaymentMethod: walletmodels.PaymentMethodPaystack, Status: walletmodels.PaymentStatusPending, PaystackRef: paystackRef,
	}); err != nil {
		t.Fatalf("create top-up intent: %v", err)
	}
	if _, err := repo.ApplyPaystackChargeSuccess(ctx, paystackRef); err != nil {
		t.Fatalf("fund wallet: %v", err)
	}
	intent, _, err := repo.CreatePaymentIntent(ctx, walletrepositories.CreatePaymentIntentInput{
		SourceService: "driver-taxi-service", SourceReference: "booking_" + uuid.NewString(),
		CustomerID: customerID, ProviderID: "drv_" + uuid.NewString(), ProviderType: "taxi",
		AmountKobo: amount, PlatformFeeKobo: fee, Currency: "NGN", PaymentMethod: walletmodels.PaymentMethodWallet,
	})
	if err != nil {
		t.Fatalf("create job intent: %v", err)
	}
	intent, err = repo.HoldPaymentFromWallet(ctx, intent.ID, "hold_"+intent.ID)
	if err != nil {
		t.Fatalf("hold job payment: %v", err)
	}
	return intent
}

func referralEvents(t *testing.T, pool *pgxpool.Pool, intentID string) []outbox.Envelope {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT payload FROM referral_outbox WHERE aggregate_type = 'payment_intent' AND aggregate_id = $1 ORDER BY created_at`, intentID)
	if err != nil {
		t.Fatalf("read referral outbox: %v", err)
	}
	defer rows.Close()
	var envelopes []outbox.Envelope
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var envelope outbox.Envelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("decode envelope: %v", err)
		}
		envelopes = append(envelopes, envelope)
	}
	return envelopes
}

// TestCompleteJobEmitsExactlyOneReferralEvent proves the settlement and its
// referral event commit together, the idempotent re-call emits nothing, and
// the payload carries the commission actually posted.
func TestCompleteJobEmitsExactlyOneReferralEvent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	store, err := outbox.NewStore(pool, referralcontract.OutboxTable)
	if err != nil {
		t.Fatal(err)
	}
	repo.SetReferralOutbox(store)

	intent := heldJobPayment(t, repo, 1_000_000, 150_000)
	// The job finishes after its payment was held, as in every real booking.
	completedAt := time.Now().UTC().Truncate(time.Millisecond)
	if _, err := repo.CompleteJob(ctx, intent.SourceService, intent.SourceReference, completedAt); err != nil {
		t.Fatalf("CompleteJob() error = %v", err)
	}
	// The retry a booking service performs after an ambiguous timeout.
	if _, err := repo.CompleteJob(ctx, intent.SourceService, intent.SourceReference, completedAt); err != nil {
		t.Fatalf("idempotent CompleteJob() error = %v", err)
	}

	events := referralEvents(t, pool, intent.ID)
	if len(events) != 1 {
		t.Fatalf("got %d referral events, want exactly 1", len(events))
	}
	if events[0].EventType != referralcontract.EventJobSettled || events[0].Producer != "payment-wallet-service" {
		t.Fatalf("unexpected envelope %+v", events[0])
	}
	var payload referralcontract.JobSettled
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if err := payload.Validate(); err != nil {
		t.Fatalf("payload invalid: %v", err)
	}
	if payload.CustomerID != intent.CustomerID || payload.ProviderID != intent.ProviderID || payload.ProviderType != "taxi" ||
		payload.AmountKobo != 1_000_000 || payload.PlatformFeeKobo != 150_000 || payload.ProviderNetKobo != 850_000 {
		t.Fatalf("payload does not match the settlement: %+v", payload)
	}
	if !payload.JobCompletedAt.Equal(completedAt) || payload.JobCompletedAtSource != referralcontract.CompletedAtFromProvider {
		t.Fatalf("completion time = %s (%s), want %s from provider", payload.JobCompletedAt, payload.JobCompletedAtSource, completedAt)
	}

	var stored time.Time
	if err := pool.QueryRow(ctx, `SELECT job_completed_at FROM payment_intents WHERE id = $1`, intent.ID).Scan(&stored); err != nil {
		t.Fatalf("read job_completed_at: %v", err)
	}
	if !stored.Equal(completedAt) {
		t.Fatalf("stored job_completed_at = %s, want %s", stored, completedAt)
	}
}

// TestCompleteJobWithoutReportedTimeFlagsSettlementFallback proves a caller
// that reports nothing still produces an event, clearly marked as degraded.
func TestCompleteJobWithoutReportedTimeFlagsSettlementFallback(t *testing.T) {
	pool := testPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	store, _ := outbox.NewStore(pool, referralcontract.OutboxTable)
	repo.SetReferralOutbox(store)

	intent := heldJobPayment(t, repo, 500_000, 75_000)
	if _, err := repo.CompleteJob(context.Background(), intent.SourceService, intent.SourceReference, time.Time{}); err != nil {
		t.Fatalf("CompleteJob() error = %v", err)
	}
	events := referralEvents(t, pool, intent.ID)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	var payload referralcontract.JobSettled
	_ = json.Unmarshal(events[0].Payload, &payload)
	if payload.JobCompletedAtSource != referralcontract.CompletedAtFromSettlement || payload.JobCompletedAt.IsZero() {
		t.Fatalf("expected a flagged settlement-time fallback, got %+v", payload)
	}
}

// TestCompleteJobWithFeedOffEmitsNothing proves the feed is genuinely off by
// default: no rows accumulate with nobody relaying them.
func TestCompleteJobWithFeedOffEmitsNothing(t *testing.T) {
	pool := testPool(t)
	repo := walletrepositories.NewPostgresWalletRepository(pool)
	intent := heldJobPayment(t, repo, 200_000, 30_000)
	if _, err := repo.CompleteJob(context.Background(), intent.SourceService, intent.SourceReference, time.Now()); err != nil {
		t.Fatalf("CompleteJob() error = %v", err)
	}
	if events := referralEvents(t, pool, intent.ID); len(events) != 0 {
		t.Fatalf("feed is off but %d events were written", len(events))
	}
}
