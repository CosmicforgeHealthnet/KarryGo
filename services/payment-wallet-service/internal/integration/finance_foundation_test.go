package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"cosmicforge/logistics/services/payment-wallet-service/internal/events"
	financehttp "cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/http"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/repositories"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/usecases"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/audit"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/financeauth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type financeFixture struct {
	pool        *pgxpool.Pool
	service     *usecases.Service
	ledger      *repositories.LedgerRepository
	billing     *repositories.BillingRepository
	marketplace *repositories.MarketplaceRepository
	auth        *financeauth.Authorizer
}

type unknownThenSuccessGateway struct{}

type recordingPublisher struct {
	ids  []string
	fail bool
}

func (p *recordingPublisher) Publish(_ context.Context, record events.OutboxRecord) error {
	if p.fail {
		return errors.New("temporary delivery failure")
	}
	p.ids = append(p.ids, record.ID)
	return nil
}

func (unknownThenSuccessGateway) Configured() bool { return true }
func (unknownThenSuccessGateway) ExecuteSettlement(_ context.Context, _ models.SettlementBatch, item models.SettlementItem) (usecases.ExternalOutcome, error) {
	return usecases.ExternalOutcome{Kind: usecases.OutcomeUnknown, ExternalReference: "payout-" + item.ID, SafeErrorCode: "timeout"}, nil
}
func (unknownThenSuccessGateway) ReadSettlement(_ context.Context, item models.SettlementItem) (usecases.ExternalOutcome, error) {
	return usecases.ExternalOutcome{Kind: usecases.OutcomeSucceeded, ExternalReference: item.ExternalReference}, nil
}
func (unknownThenSuccessGateway) ExecuteRefund(_ context.Context, refund models.Refund) (usecases.ExternalOutcome, error) {
	return usecases.ExternalOutcome{Kind: usecases.OutcomeUnknown, ExternalReference: "refund-" + refund.ID, SafeErrorCode: "timeout"}, nil
}
func (unknownThenSuccessGateway) ReadRefund(_ context.Context, refund models.Refund) (usecases.ExternalOutcome, error) {
	return usecases.ExternalOutcome{Kind: usecases.OutcomeSucceeded, ExternalReference: refund.ExternalReference}, nil
}
func (unknownThenSuccessGateway) ExecuteReversal(_ context.Context, reversal models.Reversal) (usecases.ExternalOutcome, error) {
	return usecases.ExternalOutcome{Kind: usecases.OutcomeUnknown, ExternalReference: "reversal-" + reversal.ID, SafeErrorCode: "timeout"}, nil
}
func (unknownThenSuccessGateway) ReadReversal(_ context.Context, reversal models.Reversal) (usecases.ExternalOutcome, error) {
	return usecases.ExternalOutcome{Kind: usecases.OutcomeSucceeded, ExternalReference: reversal.ExternalOperationReference}, nil
}

func newFinanceFixture(t *testing.T) financeFixture {
	t.Helper()
	pool := testPool(t)
	ledger := repositories.NewLedgerRepository(pool)
	authz := financeauth.NewAuthorizer(pool)
	return financeFixture{pool: pool, ledger: ledger, billing: repositories.NewBillingRepository(pool),
		marketplace: repositories.NewMarketplaceRepository(pool), auth: authz,
		service: usecases.NewService(usecases.Options{Pool: pool, Ledger: ledger,
			Accounts: repositories.NewAccountRepository(pool, ledger), Billing: repositories.NewBillingRepository(pool),
			Marketplace: repositories.NewMarketplaceRepository(pool), Holds: repositories.NewHoldRepository(pool),
			Operations: repositories.NewOperationsRepository(pool), Authorizer: authz, Audit: audit.NewRecorder(),
			Events: events.NewDomainPublisher(events.NewOutboxRepository(pool))})}
}

func TestFinanceOutboxWorkerDeliveryRetryAndDeadLetter(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := events.NewOutboxRepository(pool)
	envelope, err := events.NewEnvelope("wallet.test.v1", 1, "TEST", "test-1", 1, "org-outbox", "corr", "", map[string]any{"safe": true}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = database.WithinTx(ctx, pool, func(tx pgx.Tx) error { return repo.Append(ctx, tx, envelope) }); err != nil {
		t.Fatal(err)
	}
	failing := &recordingPublisher{fail: true}
	worker := events.NewWorker(repo, failing, events.WorkerConfig{Owner: "test", BatchSize: 1, Lease: time.Second, MaxAttempts: 1})
	if err = worker.RunPass(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM finance_outbox WHERE id=$1`, envelope.EventID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != events.OutboxStatusDeadLettered {
		t.Fatalf("outbox status=%s", status)
	}
	if err = repo.Replay(ctx, envelope.EventID); err != nil {
		t.Fatal(err)
	}
	success := &recordingPublisher{}
	worker = events.NewWorker(repo, success, events.WorkerConfig{Owner: "test", BatchSize: 1, Lease: time.Second})
	if err = worker.RunPass(ctx); err != nil {
		t.Fatal(err)
	}
	if len(success.ids) != 1 || success.ids[0] != envelope.EventID {
		t.Fatalf("published ids=%v", success.ids)
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM finance_outbox WHERE id=$1`, envelope.EventID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != events.OutboxStatusPublished {
		t.Fatalf("outbox status=%s", status)
	}
}

func TestFinanceConcurrentWalletCreationIsSingleAndIdempotent(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	const count = 8
	ids := make(chan string, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wallet, err := f.service.EnsureOrganizationWallet(ctx, serviceMeta("fleet-partner-service", "wallet-concurrent", "org-concurrent"), "org-concurrent", "NGN")
			if err != nil {
				errs <- err
				return
			}
			ids <- wallet.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent ensure: %v", err)
	}
	var first string
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatalf("wallet ids differ: %s vs %s", id, first)
		}
	}
	var wallets, accounts, history, outbox int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM finance_wallets WHERE organization_id='org-concurrent'`).Scan(&wallets); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM finance_ledger_accounts WHERE organization_id='org-concurrent'`).Scan(&accounts); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM finance_state_history WHERE organization_id='org-concurrent' AND entity_type='WALLET'`).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM finance_outbox WHERE organization_id='org-concurrent'`).Scan(&outbox); err != nil {
		t.Fatal(err)
	}
	if wallets != 1 || accounts != 1 || history != 1 || outbox != 1 {
		t.Fatalf("wallets=%d accounts=%d history=%d outbox=%d", wallets, accounts, history, outbox)
	}
}

func serviceMeta(actor, key, organizationID string) usecases.Meta {
	return usecases.Meta{Actor: financeauth.Actor{Type: financeauth.ActorService, ID: actor, OrganizationID: organizationID}, IdempotencyKey: key, CorrelationID: key, RequestID: key}
}

func TestFinanceLedgerBalancedImmutableAndReversible(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	var journalID string
	err := database.WithinTx(ctx, f.pool, func(tx pgx.Tx) error {
		debit, err := f.ledger.EnsureAccount(ctx, tx, "org-ledger", models.OwnerPlatform, "debit", models.AccountPlatformReceivable, "NGN", models.SideDebit)
		if err != nil {
			return err
		}
		credit, err := f.ledger.EnsureAccount(ctx, tx, "org-ledger", models.OwnerPlatform, "credit", models.AccountPlatformClearing, "NGN", models.SideCredit)
		if err != nil {
			return err
		}
		if _, err := f.ledger.Post(ctx, tx, repositories.Posting{OrganizationID: "org-ledger", JournalType: models.JournalWalletTopUp, Currency: "NGN", IdempotencyKey: "ledger-balanced", Lines: []repositories.PostingLine{{LedgerAccountID: debit.ID, Side: models.SideDebit, AmountMinor: 1000}, {LedgerAccountID: credit.ID, Side: models.SideCredit, AmountMinor: 1000}}}); err != nil {
			return err
		}
		journal, err := f.ledger.FindJournalByIdempotencyKey(ctx, tx, "ledger-balanced")
		if err != nil {
			return err
		}
		journalID = journal.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = database.WithinTx(ctx, f.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE finance_journals SET memo='tampered' WHERE id=$1`, journalID)
		return err
	})
	if err == nil {
		t.Fatal("posted journal was mutable")
	}
	err = database.WithinTx(ctx, f.pool, func(tx pgx.Tx) error {
		_, err := f.ledger.Reverse(ctx, tx, journalID, models.JournalReversal, "correction", "SERVICE", "admin", "ledger-reverse", "corr")
		return err
	})
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	err = database.WithinTx(ctx, f.pool, func(tx pgx.Tx) error {
		account, _, err := repositories.NewAccountRepository(f.pool, f.ledger).EnsureWallet(ctx, tx, "org-x", "NGN")
		if err != nil {
			return err
		}
		_, err = f.ledger.Post(ctx, tx, repositories.Posting{OrganizationID: "org-x", JournalType: models.JournalWalletTopUp, Currency: "NGN", Lines: []repositories.PostingLine{{LedgerAccountID: account.LedgerAccountID, Side: models.SideDebit, AmountMinor: 9}, {LedgerAccountID: account.LedgerAccountID, Side: models.SideCredit, AmountMinor: 8}}})
		return err
	})
	if err == nil {
		t.Fatal("unbalanced journal was accepted")
	}
}

func TestFinanceWalletSubscriptionAllocationHoldAndStatement(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	org := "org-flow"
	fleet := serviceMeta("fleet-partner-service", "wallet-1", org)
	wallet, err := f.service.EnsureOrganizationWallet(ctx, fleet, org, "NGN")
	if err != nil {
		t.Fatalf("wallet: %v", err)
	}
	earning, err := f.service.EnsureEarningAccount(ctx, serviceMeta("fleet-partner-service", "earning-1", org), "DISPATCH", "provider-1", org, "NGN")
	if err != nil {
		t.Fatalf("earning: %v", err)
	}
	price := int64(250000)
	plan := &models.SubscriptionPlan{Code: "FLEET-PRO", Name: "Fleet Pro", Currency: "NGN", PriceMinor: &price, BillingInterval: models.IntervalMonth, Status: models.PlanActive}
	if err := f.billing.CreatePlan(ctx, nil, plan); err != nil {
		t.Fatalf("plan: %v", err)
	}
	sub, invoice, err := f.service.CreateSubscriptionInvoice(ctx, usecases.SubscribeInput{Meta: serviceMeta("fleet-partner-service", "subscribe-1", org), OrganizationID: org, PlanCode: plan.Code, DueDays: 7})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if sub.LatestInvoiceID != invoice.ID || invoice.TotalMinor != price {
		t.Fatalf("bad invoice/subscription: %#v %#v", sub, invoice)
	}
	if _, err := f.service.ConfirmInvoicePayment(ctx, usecases.InvoicePaymentInput{Meta: serviceMeta("fleet-partner-service", "payment-denied", org), OrganizationID: org, InvoiceID: invoice.ID, AmountMinor: price, Currency: "NGN", Method: "PAYSTACK", ExternalReference: "pay-denied"}); err == nil {
		t.Fatal("fleet confirmed its own payment")
	}
	payment, err := f.service.ConfirmInvoicePayment(ctx, usecases.InvoicePaymentInput{Meta: serviceMeta("payment-wallet-service", "payment-1", org), OrganizationID: org, InvoiceID: invoice.ID, AmountMinor: price, Currency: "NGN", Method: "PAYSTACK", ExternalReference: "pay-verified-1"})
	if err != nil {
		t.Fatalf("payment: %v", err)
	}
	if payment.Status != models.PaymentSucceeded {
		t.Fatalf("payment status=%s", payment.Status)
	}
	sub, err = f.service.Billing().GetSubscription(ctx, nil, org, sub.ID)
	if err != nil || sub.Status != models.SubscriptionActive {
		t.Fatalf("subscription not active: %#v %v", sub, err)
	}
	noPrice := &models.SubscriptionPlan{Code: "NO-PRICE", Name: "No Price", Currency: "NGN", BillingInterval: models.IntervalMonth, Status: models.PlanActive}
	if err := f.billing.CreatePlan(ctx, nil, noPrice); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.service.CreateSubscriptionInvoice(ctx, usecases.SubscribeInput{Meta: serviceMeta("fleet-partner-service", "subscribe-no-price", "org-other"), OrganizationID: "org-other", PlanCode: noPrice.Code}); !errors.Is(err, usecases.ErrPricingMissing) {
		t.Fatalf("missing price err=%v", err)
	}
	rule := &models.CommissionRule{OrganizationID: org, ServiceType: "DELIVERY", Currency: "NGN", CommissionBPS: 1000, FleetFeeBPS: 500, ProviderShareBPS: 6000, OrganizationShareBPS: 4000, Status: "ACTIVE"}
	if err := f.marketplace.CreateCommissionRule(ctx, nil, rule); err != nil {
		t.Fatalf("rule: %v", err)
	}
	input := usecases.AllocationInput{Meta: serviceMeta("fleet-partner-service", "allocation-1", org), OrganizationID: org, SourceService: "driver-dispatch-delivery-service", JobReference: "trip-1", ServiceType: "DELIVERY", ProviderReference: "provider-1", ProviderService: "DISPATCH", Currency: "NGN", GrossMinor: 100000}
	allocation, err := f.service.PostEarningAllocation(ctx, input)
	if err != nil {
		t.Fatalf("allocation: %v", err)
	}
	if allocation.GrossMinor != allocation.CommissionMinor+allocation.FleetFeeMinor+allocation.ProviderEarningMinor+allocation.OrganizationEarningMinor {
		t.Fatal("split does not reproduce gross")
	}
	replayed, err := f.service.PostEarningAllocation(ctx, input)
	if err != nil || replayed.ID != allocation.ID {
		t.Fatalf("allocation replay: %#v %v", replayed, err)
	}
	input.GrossMinor++
	if _, err := f.service.PostEarningAllocation(ctx, input); !errors.Is(err, usecases.ErrIdempotencyReuse) {
		t.Fatalf("conflicting replay err=%v", err)
	}
	wallet, err = f.service.Accounts().GetWalletByID(ctx, nil, org, wallet.ID)
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().UTC().Add(time.Hour)
	hold, err := f.service.CreateHold(ctx, usecases.HoldInput{Meta: serviceMeta("fleet-partner-service", "hold-1", org), OrganizationID: org, SubjectType: models.HoldSubjectWallet, SubjectID: wallet.ID, ReasonCode: "DISPUTE", AmountMinor: 1000, Currency: "NGN", AuthorityService: "support-dispute-service", AuthorityReference: "dispute-1", ReferenceVerified: false, ExpiresAt: &expiry, ExpectedVersion: wallet.Version})
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	if _, err := f.service.ReleaseHold(ctx, serviceMeta("fleet-partner-service", "hold-release-1", org), org, hold.ID, hold.Version, models.HoldReleased); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := f.service.CreateHold(ctx, usecases.HoldInput{Meta: serviceMeta("fleet-partner-service", "hold-indefinite", org), OrganizationID: org, SubjectType: models.HoldSubjectEarningAccount, SubjectID: earning.ID, ReasonCode: "INCIDENT", AmountMinor: 1, Currency: "NGN", AuthorityReference: "unverified", ReferenceVerified: false, ExpectedVersion: earning.Version}); err == nil {
		t.Fatal("unverified indefinite hold was accepted")
	}
	statement, err := f.service.GenerateStatement(ctx, serviceMeta("fleet-partner-service", "statement-1", org), org, models.OwnerOrganizationWallet, wallet.ID, "NGN", models.StatementTransaction, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("statement: %v", err)
	}
	if statement.LineCount == 0 || statement.ContentHash == "" {
		t.Fatalf("empty statement: %#v", statement)
	}
}

func TestFinanceSettlementRefundSeparationAndReconciliation(t *testing.T) {
	f := newFinanceFixture(t)
	ctx := context.Background()
	org := "org-sensitive"
	rule := &models.CommissionRule{OrganizationID: org, ServiceType: "DELIVERY", Currency: "NGN", ProviderShareBPS: 10000, Status: "ACTIVE"}
	if err := f.marketplace.CreateCommissionRule(ctx, nil, rule); err != nil {
		t.Fatal(err)
	}
	allocation, err := f.service.PostEarningAllocation(ctx, usecases.AllocationInput{Meta: serviceMeta("fleet-partner-service", "alloc-sensitive", org), OrganizationID: org, SourceService: "dispatch", JobReference: "trip-sensitive", ServiceType: "DELIVERY", ProviderReference: "provider-sensitive", ProviderService: "DISPATCH", Currency: "NGN", GrossMinor: 5000})
	if err != nil {
		t.Fatal(err)
	}
	account, err := f.service.Accounts().GetEarningAccountByID(ctx, nil, allocation.EarningAccountID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.Accounts().SetPayoutDestination(ctx, nil, account.ID, account.Version, "recipient-opaque"); err != nil {
		t.Fatal(err)
	}
	account, _ = f.service.Accounts().GetEarningAccountByID(ctx, nil, account.ID)
	batch, err := f.service.RequestSettlement(ctx, usecases.SettlementInput{Meta: serviceMeta("fleet-partner-service", "settlement-1", org), OrganizationID: org, BeneficiaryType: models.BeneficiaryProvider, BeneficiaryReference: "recipient-opaque", EarningAccountID: account.ID, Currency: "NGN", ExpectedVersion: account.Version})
	if err != nil {
		t.Fatalf("settlement: %v", err)
	}
	if _, err := f.service.ApproveSettlement(ctx, serviceMeta("fleet-partner-service", "settlement-self", org), org, batch.ID, batch.Version); err == nil {
		t.Fatal("initiator approved settlement")
	}
	approved, err := f.service.ApproveSettlement(ctx, serviceMeta("admin-backoffice-service", "settlement-approve", org), org, batch.ID, batch.Version)
	if err != nil || approved.Status != models.BatchApproved {
		t.Fatalf("approve settlement: %#v %v", approved, err)
	}
	refund, err := f.service.RequestRefund(ctx, usecases.RefundInput{Meta: serviceMeta("fleet-partner-service", "refund-1", org), OrganizationID: org, OriginalJournalID: allocation.JournalID, AmountMinor: 1000, ReasonCode: "CUSTOMER_REQUEST"})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if _, err := f.service.RequestRefund(ctx, usecases.RefundInput{Meta: serviceMeta("fleet-partner-service", "refund-too-large", org), OrganizationID: org, OriginalJournalID: allocation.JournalID, AmountMinor: 5000, ReasonCode: "EXCESS"}); err == nil {
		t.Fatal("cumulative over-refund accepted")
	}
	if _, err := f.service.ApproveRefund(ctx, serviceMeta("admin-backoffice-service", "refund-approve", org), org, refund.ID, refund.Version); err != nil {
		t.Fatalf("approve refund: %v", err)
	}
	workerConfig := usecases.WorkerConfig{Owner: "worker-test", BatchSize: 10, Lease: time.Minute, MaxAttempts: 3, BaseBackoff: time.Nanosecond}
	if err := f.service.RunFinancialPass(ctx, unknownThenSuccessGateway{}, workerConfig); err != nil {
		t.Fatalf("unknown worker pass: %v", err)
	}
	var settlementStatus, refundStatus string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM finance_settlement_items WHERE batch_id=$1`, batch.ID).Scan(&settlementStatus); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT status FROM finance_refunds WHERE id=$1`, refund.ID).Scan(&refundStatus); err != nil {
		t.Fatal(err)
	}
	if settlementStatus != models.SettlementUnknown || refundStatus != models.RefundUnknown {
		t.Fatalf("timeout did not become unknown: %s %s", settlementStatus, refundStatus)
	}
	if err := f.service.RunFinancialPass(ctx, unknownThenSuccessGateway{}, workerConfig); err != nil {
		t.Fatalf("readback worker pass: %v", err)
	}
	var settlementJournal, refundJournal string
	if err := f.pool.QueryRow(ctx, `SELECT status,COALESCE(journal_id,'') FROM finance_settlement_items WHERE batch_id=$1`, batch.ID).Scan(&settlementStatus, &settlementJournal); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT status,COALESCE(journal_id,'') FROM finance_refunds WHERE id=$1`, refund.ID).Scan(&refundStatus, &refundJournal); err != nil {
		t.Fatal(err)
	}
	if settlementStatus != models.SettlementSucceeded || refundStatus != models.RefundCompleted || settlementJournal == "" || refundJournal == "" {
		t.Fatalf("readback not posted: %s %s %q %q", settlementStatus, refundStatus, settlementJournal, refundJournal)
	}
	run, err := f.service.Reconcile(ctx, serviceMeta("payment-wallet-service", "reconcile-1", ""), "ALL")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if run.Status != "COMPLETED" || run.FindingsCount != 0 {
		t.Fatalf("unexpected reconciliation: %#v", run)
	}
}

func TestFinancePrivateHMACAndReplayProtection(t *testing.T) {
	f := newFinanceFixture(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	secret := []byte("wallet-private-test-secret")
	financehttp.RegisterPrivateRoutes(router.Group("/api/v1/payment-wallet/internal/finance"), f.service, serviceauth.NewVerifier(serviceauth.Secrets{"fleet-partner-service": secret}, 5*time.Minute), []string{"fleet-partner-service"})
	body := []byte(`{"payloadVersion":1,"organizationId":"org-http","currency":"NGN","idempotencyKey":"wallet-http-1","expectedVersion":0}`)
	makeRequest := func(nonce string) *http.Request {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/payment-wallet/internal/finance/wallets/ensure", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Service-Nonce", nonce)
		if err := serviceauth.SignRequest(request, "fleet-partner-service", secret, body, time.Now()); err != nil {
			t.Fatal(err)
		}
		return request
	}
	first := httptest.NewRecorder()
	router.ServeHTTP(first, makeRequest("nonce-one"))
	if first.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	second := httptest.NewRecorder()
	router.ServeHTTP(second, makeRequest("nonce-one"))
	if second.Code != http.StatusConflict {
		t.Fatalf("replay status=%d body=%s", second.Code, second.Body.String())
	}
	unsigned := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/payment-wallet/internal/finance/wallets/ensure", bytes.NewReader(body))
	request.Header.Set("X-Service-Nonce", "nonce-two")
	router.ServeHTTP(unsigned, request)
	if unsigned.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned status=%d", unsigned.Code)
	}
	var response map[string]any
	if err := json.Unmarshal(first.Body.Bytes(), &response); err != nil || response["success"] != true {
		t.Fatalf("bad response: %s", first.Body.String())
	}
}
