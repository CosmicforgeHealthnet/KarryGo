package integration_test

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	financefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/finance"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

type unknownWallet struct {
	submits, reads    int
	projectionVersion int64
	projectionID      string
	// verifications is the authoritative job payment answer keyed by reference.
	// A reference that is absent behaves exactly as Wallet would: not found.
	verifications map[string]platformclients.JobPaymentVerification
	verifyCalls   int
	verifyErr     error
}

func (w *unknownWallet) Configured() bool { return true }
func (w *unknownWallet) EnsureOrganizationWallet(context.Context, platformclients.CallMeta, platformclients.WalletProvisionRequest) (platformclients.WalletAccount, error) {
	return platformclients.WalletAccount{}, nil
}
func (w *unknownWallet) EnsureProviderSubWallet(context.Context, platformclients.CallMeta, platformclients.WalletProvisionRequest) (platformclients.WalletAccount, error) {
	return platformclients.WalletAccount{}, nil
}
func (w *unknownWallet) ReadAccount(context.Context, platformclients.CallMeta, string) (platformclients.WalletAccount, error) {
	return platformclients.WalletAccount{}, nil
}
func (w *unknownWallet) DisableForClosure(context.Context, platformclients.CallMeta, string, string) error {
	return nil
}
func (w *unknownWallet) SubmitOperation(context.Context, platformclients.CallMeta, platformclients.WalletOperationRequest) (platformclients.WalletOperation, error) {
	w.submits++
	return platformclients.WalletOperation{}, &platformclients.Error{Service: "payment-wallet-service", Outcome: platformclients.OutcomeUnknown}
}
func (w *unknownWallet) ReadOperation(context.Context, platformclients.CallMeta, string, string, string) (platformclients.WalletOperation, error) {
	return platformclients.WalletOperation{}, nil
}
func (w *unknownWallet) ReadOperationByKey(context.Context, platformclients.CallMeta, string, string, string) (platformclients.WalletOperation, error) {
	w.reads++
	return platformclients.WalletOperation{ID: "hold-wallet-1", Status: "ACTIVE", Version: 1}, nil
}

func (w *unknownWallet) ReadProjection(context.Context, platformclients.CallMeta, string, string, string) (platformclients.WalletOperation, error) {
	w.reads++
	version := w.projectionVersion
	if version == 0 {
		version = 1
	}
	id := w.projectionID
	if id == "" {
		id = "wallet-projection"
	}
	return platformclients.WalletOperation{ID: id, Status: "ACTIVE", Version: version}, nil
}

func (w *unknownWallet) VerifyJobPayment(_ context.Context, _ platformclients.CallMeta,
	organizationID, _, reference string) (platformclients.JobPaymentVerification, error) {
	w.verifyCalls++
	if w.verifyErr != nil {
		return platformclients.JobPaymentVerification{}, w.verifyErr
	}
	verification, ok := w.verifications[reference]
	if !ok {
		return platformclients.JobPaymentVerification{}, &platformclients.Error{
			Service: "payment-wallet-service", Outcome: platformclients.OutcomeNotFound}
	}
	if verification.OrganizationID == "" {
		verification.OrganizationID = organizationID
	}
	return verification, nil
}

func (w *unknownWallet) ReadReversal(context.Context, platformclients.CallMeta, string, string) (platformclients.WalletReversal, error) {
	return platformclients.WalletReversal{}, nil
}

// settledPayment builds an authoritative verification for a paid job.
func settledPayment(id string, amountMinor int64, currency string) platformclients.JobPaymentVerification {
	return platformclients.JobPaymentVerification{PaymentVerificationID: id,
		AmountMinor: amountMinor, Currency: currency, PaymentStatus: "PAID",
		SourceVersion: 1, RefundState: "NONE", ReversalState: "NONE", Settled: true}
}

func TestFinanceOperationsIdempotencyLeaseRecoveryAndReplay(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Finance Integration Ltd", "FIN-001")
	ctx := context.Background()
	repo := financefeature.NewRepository(pool)
	original := &financefeature.Operation{OrganizationID: org.ID, SubjectType: "TRIP", SubjectID: "trip-1", OperationType: "EARNING_ALLOCATION", Payload: json.RawMessage(`{"grossMinor":1000,"currency":"NGN"}`), PayloadVersion: 1, IdempotencyKey: "earn-trip-1", RequestFingerprint: "fingerprint-a", CorrelationID: "corr-1"}
	if err := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		created, err := repo.Create(ctx, tx, original)
		if err != nil || !created {
			t.Fatalf("create=%v err=%v", created, err)
		}
		duplicate := *original
		duplicate.ID = ""
		created, err = repo.Create(ctx, tx, &duplicate)
		if err != nil || created || duplicate.ID != original.ID {
			t.Fatalf("duplicate=%+v created=%v err=%v", duplicate, created, err)
		}
		conflict := duplicate
		conflict.RequestFingerprint = "fingerprint-b"
		_, err = repo.Create(ctx, tx, &conflict)
		if !errors.Is(err, financefeature.ErrFingerprintConflict) {
			t.Fatalf("conflict err=%v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.Claim(ctx, "worker-a", time.Millisecond, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	time.Sleep(2 * time.Millisecond)
	recovered, err := repo.Recover(ctx)
	if err != nil || recovered != 1 {
		t.Fatalf("recovered=%d err=%v", recovered, err)
	}
	claimed, err = repo.Claim(ctx, "worker-b", time.Second, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("reclaim=%v err=%v", claimed, err)
	}
	if err = repo.Outcome(ctx, nil, claimed[0], financefeature.StatusRetryable, "", "temporary", time.Now(), 1); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM fleet_partner_finance_operations WHERE id=$1`, original.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != financefeature.StatusDeadLetter {
		t.Fatalf("status=%s", status)
	}
	if err = repo.Replay(ctx, original.ID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM fleet_partner_finance_operations WHERE id=$1`, original.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != financefeature.StatusPending {
		t.Fatalf("replayed status=%s", status)
	}
}

func TestFinanceEventDuplicateConflictAndGap(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Finance Events Ltd", "FIN-002")
	ctx := context.Background()
	service := financefeature.NewService(pool, financefeature.NewRepository(pool), nil, events.NewDomainPublisher(events.NewOutboxRepository(pool)))
	event := events.Envelope{EventID: "wallet-event-1", EventType: "wallet.invoice.created.v1", SchemaVersion: 1, AggregateType: "INVOICE", AggregateID: "inv-1", AggregateVersion: 1, OrganizationID: org.ID, CorrelationID: "corr", OccurredAt: time.Now(), Producer: "payment-wallet-service", Payload: json.RawMessage(`{"invoiceId":"inv-1","status":"OPEN","amountMinor":1000,"currency":"NGN"}`)}
	raw, _ := json.Marshal(event)
	status, err := service.IngestWalletEvent(ctx, raw)
	if err != nil || status != "PROCESSED" {
		t.Fatalf("first status=%s err=%v", status, err)
	}
	status, err = service.IngestWalletEvent(ctx, raw)
	if err != nil || status != "DUPLICATE" {
		t.Fatalf("duplicate status=%s err=%v", status, err)
	}
	event.Payload = json.RawMessage(`{"invoiceId":"inv-1","status":"PAID"}`)
	changed, _ := json.Marshal(event)
	if _, err = service.IngestWalletEvent(ctx, changed); !errors.Is(err, events.ErrPayloadMismatch) {
		t.Fatalf("changed duplicate err=%v", err)
	}
	event.EventID = "wallet-event-3"
	event.AggregateVersion = 3
	event.Payload = json.RawMessage(`{"invoiceId":"inv-1","status":"PAID","amountMinor":1000,"currency":"NGN"}`)
	gap, _ := json.Marshal(event)
	status, err = service.IngestWalletEvent(ctx, gap)
	if err != nil || status != "GAP_DETECTED" {
		t.Fatalf("gap status=%s err=%v", status, err)
	}
	var freshness, safeStatus string
	var version int64
	if err = pool.QueryRow(ctx, `SELECT freshness_state,safe_status,source_version FROM fleet_partner_finance_projections WHERE organization_id=$1 AND projection_type='INVOICE' AND wallet_reference='inv-1'`, org.ID).Scan(&freshness, &safeStatus, &version); err != nil {
		t.Fatal(err)
	}
	if freshness != "GAP_DETECTED" || safeStatus != "PAID" || version != 3 {
		t.Fatalf("projection freshness=%s status=%s version=%d", freshness, safeStatus, version)
	}
}

func TestFinanceWorkerUnknownOutcomeUsesReadback(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Finance Worker Ltd", "FIN-003")
	ctx := context.Background()
	wallet := &unknownWallet{}
	service := financefeature.NewService(pool, financefeature.NewRepository(pool), wallet, events.NewDomainPublisher(events.NewOutboxRepository(pool)))
	op, err := service.Request(ctx, financefeature.Request{OrganizationID: org.ID, SubjectType: "INCIDENT", SubjectID: "incident-1", OperationType: "HOLD_CREATE", IdempotencyKey: "hold-1", CorrelationID: "corr", ActorType: "USER", ActorID: "operator", Payload: json.RawMessage(`{"subjectType":"WALLET","subjectId":"wallet-1","amountMinor":100,"currency":"NGN"}`)})
	if err != nil {
		t.Fatal(err)
	}
	cfg := financefeature.WorkerConfig{Owner: "worker", BatchSize: 1, Lease: time.Second, BaseBackoff: time.Nanosecond, MaxAttempts: 3}
	if err = service.RunPass(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM fleet_partner_finance_operations WHERE id=$1`, op.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != financefeature.StatusUnknown {
		t.Fatalf("first status=%s", status)
	}
	time.Sleep(time.Millisecond)
	if err = service.RunPass(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM fleet_partner_finance_operations WHERE id=$1`, op.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != financefeature.StatusSucceeded || wallet.submits != 1 || wallet.reads != 1 {
		t.Fatalf("status=%s submits=%d reads=%d", status, wallet.submits, wallet.reads)
	}
}

func TestFinanceProjectionGapReconciliation(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Finance Reconciliation Ltd", "FIN-004")
	ctx := context.Background()
	wallet := &unknownWallet{projectionVersion: 3, projectionID: "inv-reconcile"}
	service := financefeature.NewService(pool, financefeature.NewRepository(pool), wallet, events.NewDomainPublisher(events.NewOutboxRepository(pool)))

	for _, version := range []int64{1, 3} {
		event := events.Envelope{EventID: fmt.Sprintf("wallet-reconcile-%d", version), EventType: "wallet.invoice.paid.v1", SchemaVersion: 1, AggregateType: "INVOICE", AggregateID: "inv-reconcile", AggregateVersion: version, OrganizationID: org.ID, CorrelationID: "corr-reconcile", OccurredAt: time.Now(), Producer: "payment-wallet-service", Payload: json.RawMessage(`{"status":"PAID","amountMinor":1000,"currency":"NGN"}`)}
		raw, _ := json.Marshal(event)
		if _, err := service.IngestWalletEvent(ctx, raw); err != nil {
			t.Fatal(err)
		}
	}

	if err := service.RunPass(ctx, financefeature.WorkerConfig{Owner: "reconciler", BatchSize: 5, Lease: time.Second, BaseBackoff: time.Millisecond, MaxAttempts: 3, ProjectionStaleAfter: time.Hour}); err != nil {
		t.Fatal(err)
	}
	var freshness, reconciliation, status string
	var version int64
	if err := pool.QueryRow(ctx, `SELECT freshness_state,reconciliation_status,safe_status,source_version FROM fleet_partner_finance_projections WHERE organization_id=$1 AND wallet_reference='inv-reconcile'`, org.ID).Scan(&freshness, &reconciliation, &status, &version); err != nil {
		t.Fatal(err)
	}
	if freshness != "CURRENT" || reconciliation != "IDLE" || status != "ACTIVE" || version != 3 || wallet.reads != 1 {
		t.Fatalf("freshness=%s reconciliation=%s status=%s version=%d reads=%d", freshness, reconciliation, status, version, wallet.reads)
	}
	var history int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_finance_projection_history WHERE organization_id=$1`, org.ID).Scan(&history); err != nil || history != 1 {
		t.Fatalf("history=%d err=%v", history, err)
	}
}

func TestFinanceSubscriptionAndJobGatesFailClosed(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Finance Gates Ltd", "FIN-005")
	ctx := context.Background()
	// The job gate consults Wallet's authoritative payment lookup, so the fake
	// must answer as the authority — a Fleet projection alone is not evidence.
	wallet := &unknownWallet{verifications: map[string]platformclients.JobPaymentVerification{
		"payment-job-1": settledPayment("payment-job-1", 2500, "NGN")}}
	service := financefeature.NewService(pool, financefeature.NewRepository(pool), wallet, events.NewDomainPublisher(events.NewOutboxRepository(pool)))

	subscription := events.Envelope{EventID: "wallet-subscription-active", EventType: "wallet.subscription.status_changed.v1", SchemaVersion: 1, AggregateType: "SUBSCRIPTION", AggregateID: "sub-1", AggregateVersion: 1, OrganizationID: org.ID, CorrelationID: "corr-gate", OccurredAt: time.Now(), Producer: "payment-wallet-service", Payload: json.RawMessage(`{"status":"ACTIVE"}`)}
	raw, _ := json.Marshal(subscription)
	if _, err := service.IngestWalletEvent(ctx, raw); err != nil {
		t.Fatal(err)
	}
	if err := service.RequirePaidSubscription(ctx, org.ID); err != nil {
		t.Fatalf("active subscription blocked: %v", err)
	}

	payment := events.Envelope{EventID: "wallet-payment-succeeded", EventType: "wallet.invoice.paid.v1", SchemaVersion: 1, AggregateType: "INVOICE", AggregateID: "invoice-job-1", AggregateVersion: 1, OrganizationID: org.ID, CorrelationID: "corr-gate", OccurredAt: time.Now(), Producer: "payment-wallet-service", Payload: json.RawMessage(`{"status":"PAID","paymentId":"payment-job-1","amountPaidMinor":2500,"currency":"NGN"}`)}
	raw, _ = json.Marshal(payment)
	if _, err := service.IngestWalletEvent(ctx, raw); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, `INSERT INTO fleet_partner_trip_projections
		(id,organization_id,source_service,platform_trip_id,executing_provider_id,service_type,trip_status,source_version,last_event_id,last_event_at,freshness_state,completed_at)
		VALUES('trip-projection-finance',$1,'DISPATCH','trip-finance-1','provider-opaque','DELIVERY','COMPLETED',4,'trip-completed',now(),'CURRENT',now())`, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	op, err := service.RequestJobEarning(ctx, financefeature.JobEarningRequest{OrganizationID: org.ID, TripID: "trip-finance-1", PaymentReference: "payment-job-1", IdempotencyKey: "earning-job-1", CorrelationID: "corr-gate", ActorType: "SYSTEM", ActorID: "finance-gate", Payload: json.RawMessage(`{"grossMinor":2500,"currency":"NGN"}`)})
	if err != nil || op.OperationType != "EARNING_ALLOCATION" {
		t.Fatalf("earning op=%+v err=%v", op, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE fleet_partner_trip_projections SET failed_at=now(),failure_reason_code='delivery_failed' WHERE organization_id=$1 AND platform_trip_id='trip-finance-1'`, org.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.RequestJobEarning(ctx, financefeature.JobEarningRequest{OrganizationID: org.ID, TripID: "trip-finance-1", PaymentReference: "payment-job-1", IdempotencyKey: "earning-job-2", ActorType: "SYSTEM", ActorID: "finance-gate", Payload: json.RawMessage(`{"grossMinor":2500,"currency":"NGN"}`)}); !errors.Is(err, financefeature.ErrFinancialStateBlocked) {
		t.Fatalf("failed delivery authorized earning: %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE fleet_partner_finance_projections SET freshness_state='STALE' WHERE organization_id=$1 AND projection_type='SUBSCRIPTION'`, org.ID); err != nil {
		t.Fatal(err)
	}
	if err = service.RequirePaidSubscription(ctx, org.ID); !errors.Is(err, financefeature.ErrFinancialStateBlocked) {
		t.Fatalf("stale subscription authorized paid feature: %v", err)
	}
}
