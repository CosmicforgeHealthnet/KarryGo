// Package usecases implements the authoritative financial workflows.
package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"cosmicforge/logistics/services/payment-wallet-service/internal/events"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/repositories"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/audit"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/financeauth"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/money"
)

const (
	EventWalletCreated          = "wallet.organization.created.v1"
	EventEarningAccountCreated  = "wallet.earning_account.created.v1"
	EventInvoiceCreated         = "wallet.invoice.created.v1"
	EventInvoicePaid            = "wallet.invoice.paid.v1"
	EventSubscriptionChanged    = "wallet.subscription.status_changed.v1"
	EventAllocationPosted       = "wallet.earning.posted.v1"
	EventHoldCreated            = "wallet.hold.created.v1"
	EventHoldReleased           = "wallet.hold.released.v1"
	EventSettlementChanged      = "wallet.settlement.status_changed.v1"
	EventRefundChanged          = "wallet.refund.status_changed.v1"
	EventReversalCompleted      = "wallet.reversal.completed.v1"
	EventReconciliationRequired = "wallet.reconciliation.required.v1"
)

var (
	ErrInvalid           = errors.New("finance: invalid request")
	ErrIdempotencyReuse  = errors.New("finance: idempotency key reused with different inputs")
	ErrInsufficientFunds = errors.New("finance: insufficient funds")
	ErrPricingMissing    = errors.New("finance: plan pricing is not configured")
)

type Service struct {
	pool        *pgxpool.Pool
	accounts    *repositories.AccountRepository
	ledger      *repositories.LedgerRepository
	billing     *repositories.BillingRepository
	marketplace *repositories.MarketplaceRepository
	holds       *repositories.HoldRepository
	operations  *repositories.OperationsRepository
	reversals   *repositories.ReversalRepository
	jobPayments *repositories.JobPaymentRepository
	providerOps *repositories.ProviderOperationsRepository
	authorizer  *financeauth.Authorizer
	audit       *audit.Recorder
	events      *events.DomainPublisher
	now         func() time.Time
}

type Options struct {
	Pool        *pgxpool.Pool
	Accounts    *repositories.AccountRepository
	Ledger      *repositories.LedgerRepository
	Billing     *repositories.BillingRepository
	Marketplace *repositories.MarketplaceRepository
	Holds       *repositories.HoldRepository
	Operations  *repositories.OperationsRepository
	Reversals   *repositories.ReversalRepository
	JobPayments *repositories.JobPaymentRepository
	ProviderOps *repositories.ProviderOperationsRepository
	Authorizer  *financeauth.Authorizer
	Audit       *audit.Recorder
	Events      *events.DomainPublisher
}

func NewService(opts Options) *Service {
	// The reversal and job-payment repositories are wired from the same pool
	// when the caller did not supply them, so no construction site can end up
	// with a service whose reversal contract silently does not exist.
	if opts.Reversals == nil && opts.Pool != nil {
		opts.Reversals = repositories.NewReversalRepository(opts.Pool)
	}
	if opts.JobPayments == nil && opts.Pool != nil {
		opts.JobPayments = repositories.NewJobPaymentRepository(opts.Pool)
	}
	// Section 13 provider operations are wired the same way: no construction
	// site should be able to produce a service whose provider allocation and
	// withdrawal surface silently does not exist.
	if opts.ProviderOps == nil && opts.Pool != nil {
		opts.ProviderOps = repositories.NewProviderOperationsRepository(opts.Pool)
	}
	return &Service{pool: opts.Pool, accounts: opts.Accounts, ledger: opts.Ledger,
		billing: opts.Billing, marketplace: opts.Marketplace, holds: opts.Holds,
		operations: opts.Operations, reversals: opts.Reversals, jobPayments: opts.JobPayments,
		providerOps: opts.ProviderOps, authorizer: opts.Authorizer, audit: opts.Audit,
		events: opts.Events, now: func() time.Time { return time.Now().UTC() }}
}

type Meta struct {
	Actor          financeauth.Actor
	IdempotencyKey string
	CorrelationID  string
	CausationID    string
	RequestID      string
}

func (m Meta) validate() error {
	if err := m.Actor.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(m.IdempotencyKey) == "" {
		return fmt.Errorf("%w: idempotency key is required", ErrInvalid)
	}
	return nil
}

func fingerprint(fields ...any) string {
	h := sha256.New()
	for _, field := range fields {
		_, _ = fmt.Fprintf(h, "%v\x00", field)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Service) require(ctx context.Context, meta Meta, permissions ...string) error {
	if s.authorizer == nil {
		return financeauth.ErrForbidden
	}
	return s.authorizer.Require(ctx, meta.Actor, permissions...)
}

// AuthorizeRead applies finance RBAC to authenticated private read paths. Read
// handlers deliberately go through the use-case layer so repository access can
// never become an authorization bypass.
func (s *Service) AuthorizeRead(ctx context.Context, meta Meta, permissions ...string) error {
	if err := meta.Actor.Validate(); err != nil {
		return err
	}
	return s.require(ctx, meta, permissions...)
}

func (s *Service) EnsureOrganizationWallet(ctx context.Context, meta Meta,
	organizationID, currency string) (*models.Wallet, error) {
	if err := meta.validate(); err != nil {
		return nil, err
	}
	if _, err := money.New(0, currency); err != nil {
		return nil, err
	}
	if meta.Actor.OrganizationID != "" && meta.Actor.OrganizationID != organizationID {
		return nil, financeauth.ErrForbidden
	}
	if err := s.require(ctx, meta, financeauth.PermWalletView); err != nil {
		return nil, err
	}
	var result *models.Wallet
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		wallet, created, err := s.accounts.EnsureWallet(ctx, tx, organizationID, currency)
		if err != nil {
			return err
		}
		result = wallet
		if !created {
			return nil
		}
		if err := repositories.AppendHistory(ctx, tx, organizationID, "WALLET", wallet.ID, "", "ACTIVE", meta.Actor.Type, meta.Actor.ID, "created", nil, meta.CorrelationID, meta.CausationID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: organizationID, ActorType: meta.Actor.Type, ActorID: meta.Actor.ID, Action: "wallet.created", ResourceType: "wallet", ResourceID: wallet.ID, NewValues: map[string]any{"currency": currency, "status": wallet.Status}, RequestID: meta.RequestID, CorrelationID: meta.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventWalletCreated, "WALLET", wallet.ID, wallet.Version, organizationID, meta.CorrelationID, meta.CausationID, map[string]any{"walletId": wallet.ID, "organizationId": organizationID, "currency": currency, "status": wallet.Status})
	})
	return result, err
}

func (s *Service) EnsureEarningAccount(ctx context.Context, meta Meta, providerService,
	providerReference, organizationID, currency string) (*models.EarningAccount, error) {
	if err := meta.validate(); err != nil {
		return nil, err
	}
	if _, err := money.New(0, currency); err != nil {
		return nil, err
	}
	if err := s.require(ctx, meta, financeauth.PermWalletView); err != nil {
		return nil, err
	}
	var result *models.EarningAccount
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		account, created, err := s.accounts.EnsureEarningAccount(ctx, tx, providerService, providerReference, organizationID, currency)
		if err != nil {
			return err
		}
		result = account
		if !created {
			return nil
		}
		if err := repositories.AppendHistory(ctx, tx, organizationID, "EARNING_ACCOUNT", account.ID, "", "ACTIVE", meta.Actor.Type, meta.Actor.ID, "created", nil, meta.CorrelationID, meta.CausationID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: organizationID, ActorType: meta.Actor.Type, ActorID: meta.Actor.ID, Action: "earning_account.created", ResourceType: "earning_account", ResourceID: account.ID, NewValues: map[string]any{"providerService": providerService, "currency": currency}, RequestID: meta.RequestID, CorrelationID: meta.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventEarningAccountCreated, "EARNING_ACCOUNT", account.ID, account.Version, organizationID, meta.CorrelationID, meta.CausationID, map[string]any{"earningAccountId": account.ID, "providerReference": providerReference, "providerService": providerService, "currency": currency, "status": account.Status})
	})
	return result, err
}

func (s *Service) CreatePlan(ctx context.Context, meta Meta, plan *models.SubscriptionPlan) (*models.SubscriptionPlan, error) {
	if err := meta.validate(); err != nil {
		return nil, err
	}
	if err := s.require(ctx, meta, financeauth.PermSubscriptionManage); err != nil {
		return nil, err
	}
	if _, err := money.New(0, plan.Currency); err != nil {
		return nil, err
	}
	if plan.PriceMinor != nil && *plan.PriceMinor < 0 {
		return nil, ErrInvalid
	}
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.billing.CreatePlan(ctx, tx, plan); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, "", "PLAN", plan.ID, "", plan.Status, meta.Actor.Type, meta.Actor.ID, "created", map[string]any{"code": plan.Code}, meta.CorrelationID, meta.CausationID); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Entry{ActorType: meta.Actor.Type, ActorID: meta.Actor.ID, Action: "plan.created", ResourceType: "subscription_plan", ResourceID: plan.ID, NewValues: map[string]any{"code": plan.Code, "currency": plan.Currency, "status": plan.Status}, RequestID: meta.RequestID, CorrelationID: meta.CorrelationID})
	})
	return plan, err
}

func (s *Service) CreateCommissionRule(ctx context.Context, meta Meta, rule *models.CommissionRule) (*models.CommissionRule, error) {
	if err := meta.validate(); err != nil {
		return nil, err
	}
	if err := s.require(ctx, meta, financeauth.PermCommissionManage); err != nil {
		return nil, err
	}
	if rule.CommissionBPS+rule.FleetFeeBPS > 10000 || rule.ProviderShareBPS+rule.OrganizationShareBPS != 10000 {
		return nil, ErrInvalid
	}
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.marketplace.CreateCommissionRule(ctx, tx, rule); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, rule.OrganizationID, "COMMISSION_RULE", rule.ID, "", rule.Status, meta.Actor.Type, meta.Actor.ID, "created", map[string]any{"ruleVersion": rule.RuleVersion}, meta.CorrelationID, meta.CausationID); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Entry{OrganizationID: rule.OrganizationID, ActorType: meta.Actor.Type, ActorID: meta.Actor.ID, Action: "commission_rule.created", ResourceType: "commission_rule", ResourceID: rule.ID, NewValues: rule.Snapshot(), RequestID: meta.RequestID, CorrelationID: meta.CorrelationID})
	})
	return rule, err
}

func (s *Service) SetWalletStatus(ctx context.Context, meta Meta, organizationID, walletID, status string, expectedVersion int64, restrictions []string) (*models.Wallet, error) {
	if err := meta.validate(); err != nil {
		return nil, err
	}
	permission := financeauth.PermWalletRestrict
	if status == models.StatusSuspended || status == models.StatusClosed {
		permission = financeauth.PermWalletSuspend
	}
	if err := s.require(ctx, meta, permission); err != nil {
		return nil, err
	}
	wallet, err := s.accounts.GetWalletByID(ctx, nil, organizationID, walletID)
	if err != nil {
		return nil, err
	}
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.accounts.SetWalletStatus(ctx, tx, walletID, expectedVersion, status, restrictions); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, organizationID, "WALLET", walletID, wallet.Status, status, meta.Actor.Type, meta.Actor.ID, "status changed", nil, meta.CorrelationID, meta.CausationID); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Entry{OrganizationID: organizationID, ActorType: meta.Actor.Type, ActorID: meta.Actor.ID, Action: "wallet.status_changed", ResourceType: "wallet", ResourceID: walletID, PreviousValues: map[string]any{"status": wallet.Status}, NewValues: map[string]any{"status": status, "restrictions": restrictions}, RequestID: meta.RequestID, CorrelationID: meta.CorrelationID})
	})
	if err != nil {
		return nil, err
	}
	return s.accounts.GetWalletByID(ctx, nil, organizationID, walletID)
}

func (s *Service) ReplayOperation(ctx context.Context, meta Meta, operation, id string) error {
	if err := s.require(ctx, meta, financeauth.PermReconciliationRun); err != nil {
		return err
	}
	switch strings.ToUpper(operation) {
	case "SETTLEMENT":
		return s.operations.ReplaySettlement(ctx, id)
	case "REFUND":
		return s.operations.ReplayRefund(ctx, id)
	case "REVERSAL":
		return s.reversals.Replay(ctx, nil, id)
	default:
		return ErrInvalid
	}
}

type SubscribeInput struct {
	Meta                     Meta
	OrganizationID, PlanCode string
	DueDays                  int
}

func (s *Service) CreateSubscriptionInvoice(ctx context.Context, input SubscribeInput) (*models.Subscription, *models.Invoice, error) {
	if err := input.Meta.validate(); err != nil {
		return nil, nil, err
	}
	if err := s.require(ctx, input.Meta, financeauth.PermSubscriptionManage); err != nil {
		return nil, nil, err
	}
	fp := fingerprint(input.OrganizationID, input.PlanCode, input.DueDays)
	if existing, err := s.billing.FindSubscriptionByIdempotencyKey(ctx, nil, input.OrganizationID, input.Meta.IdempotencyKey); err == nil {
		if existing.RequestFingerprint != fp {
			return nil, nil, ErrIdempotencyReuse
		}
		invoice, loadErr := s.billing.GetInvoice(ctx, nil, input.OrganizationID, existing.LatestInvoiceID)
		return existing, invoice, loadErr
	} else if !errors.Is(err, repositories.ErrNotFound) {
		return nil, nil, err
	}
	plan, err := s.billing.GetPlanByCode(ctx, nil, input.PlanCode)
	if err != nil {
		return nil, nil, err
	}
	now := s.now()
	if !plan.Billable(now) {
		return nil, nil, ErrPricingMissing
	}
	if input.DueDays <= 0 {
		input.DueDays = 7
	}
	sub := &models.Subscription{OrganizationID: input.OrganizationID, PlanID: plan.ID, Currency: plan.Currency, Status: models.SubscriptionPending, IdempotencyKey: input.Meta.IdempotencyKey, RequestFingerprint: fp}
	invoice := &models.Invoice{OrganizationID: input.OrganizationID, Currency: plan.Currency, SubtotalMinor: *plan.PriceMinor, TotalMinor: *plan.PriceMinor, Status: models.InvoiceOpen, DueAt: now.AddDate(0, 0, input.DueDays), IdempotencyKey: input.Meta.IdempotencyKey + ":invoice", RequestFingerprint: fp, CorrelationID: input.Meta.CorrelationID}
	var resultInvoice *models.Invoice
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.billing.CreateSubscription(ctx, tx, sub); err != nil {
			return err
		}
		invoice.SubscriptionID = sub.ID
		invoice.Number, err = s.billing.NextInvoiceNumber(ctx, tx, "INV-"+now.Format("200601"))
		if err != nil {
			return err
		}
		line := models.InvoiceLine{Description: plan.Name, Quantity: 1, UnitAmountMinor: *plan.PriceMinor, AmountMinor: *plan.PriceMinor, Currency: plan.Currency}
		if err := s.billing.CreateInvoice(ctx, tx, invoice, []models.InvoiceLine{line}); err != nil {
			return err
		}
		if err := s.billing.TransitionSubscription(ctx, tx, sub.ID, sub.Version, models.SubscriptionPending, nil, nil, nil, invoice.ID); err != nil {
			return err
		}
		sub.Version++
		sub.LatestInvoiceID = invoice.ID
		resultInvoice = invoice
		if err := repositories.AppendHistory(ctx, tx, input.OrganizationID, "SUBSCRIPTION", sub.ID, "", models.SubscriptionPending, input.Meta.Actor.Type, input.Meta.Actor.ID, "created", map[string]any{"planId": plan.ID, "invoiceId": invoice.ID}, input.Meta.CorrelationID, input.Meta.CausationID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: input.OrganizationID, ActorType: input.Meta.Actor.Type, ActorID: input.Meta.Actor.ID, Action: "subscription.invoice_created", ResourceType: "invoice", ResourceID: invoice.ID, NewValues: map[string]any{"totalMinor": invoice.TotalMinor, "currency": invoice.Currency}, RequestID: input.Meta.RequestID, CorrelationID: input.Meta.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventInvoiceCreated, "INVOICE", invoice.ID, invoice.Version, input.OrganizationID, input.Meta.CorrelationID, input.Meta.CausationID, map[string]any{"invoiceId": invoice.ID, "subscriptionId": sub.ID, "organizationId": input.OrganizationID, "amountMinor": invoice.TotalMinor, "currency": invoice.Currency, "status": invoice.Status, "dueAt": invoice.DueAt})
	})
	return sub, resultInvoice, err
}

type AllocationInput struct {
	Meta                                                                                                   Meta
	OrganizationID, SourceService, JobReference, ServiceType, ProviderReference, ProviderService, Currency string
	GrossMinor                                                                                             int64
}

type InvoicePaymentInput struct {
	Meta              Meta
	OrganizationID    string
	InvoiceID         string
	AmountMinor       int64
	Currency          string
	Method            string
	ExternalReference string
}

// ConfirmInvoicePayment records a payment already confirmed by Wallet's
// authoritative provider integration. A Fleet caller cannot hold the required
// permission and therefore cannot mark its own invoice paid.
func (s *Service) ConfirmInvoicePayment(ctx context.Context, input InvoicePaymentInput) (*models.Payment, error) {
	if err := input.Meta.validate(); err != nil {
		return nil, err
	}
	if err := s.require(ctx, input.Meta, financeauth.PermPaymentConfirm); err != nil {
		return nil, err
	}
	if input.AmountMinor <= 0 {
		return nil, ErrInvalid
	}
	fp := fingerprint(input.OrganizationID, input.InvoiceID, input.AmountMinor, input.Currency, input.Method, input.ExternalReference)
	if existing, err := s.billing.FindPaymentByIdempotencyKey(ctx, nil, input.OrganizationID, input.Meta.IdempotencyKey); err == nil {
		if existing.RequestFingerprint != fp {
			return nil, ErrIdempotencyReuse
		}
		return existing, nil
	} else if !errors.Is(err, repositories.ErrNotFound) {
		return nil, err
	}
	payment := &models.Payment{OrganizationID: input.OrganizationID, InvoiceID: input.InvoiceID, AmountMinor: input.AmountMinor, Currency: input.Currency, Method: input.Method, Status: models.PaymentSucceeded, ExternalReference: input.ExternalReference, IdempotencyKey: input.Meta.IdempotencyKey, RequestFingerprint: fp, CorrelationID: input.Meta.CorrelationID}
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		invoice, err := s.billing.LockInvoice(ctx, tx, input.InvoiceID)
		if err != nil {
			return err
		}
		if invoice.OrganizationID != input.OrganizationID {
			return financeauth.ErrForbidden
		}
		if invoice.Currency != input.Currency || input.AmountMinor > invoice.OutstandingMinor() {
			return ErrInvalid
		}
		clearing, err := s.ledger.EnsureAccount(ctx, tx, "", models.OwnerPlatform, "platform", models.AccountPlatformClearing, input.Currency, models.SideDebit)
		if err != nil {
			return err
		}
		revenue, err := s.ledger.EnsureAccount(ctx, tx, "", models.OwnerPlatform, "platform", models.AccountPlatformSubscription, input.Currency, models.SideCredit)
		if err != nil {
			return err
		}
		journal, err := s.ledger.Post(ctx, tx, repositories.Posting{OrganizationID: input.OrganizationID, JournalType: models.JournalInvoicePayment, Currency: input.Currency, Reference: input.ExternalReference, SourceService: "payment-provider", SourceReference: input.ExternalReference, IdempotencyKey: input.Meta.IdempotencyKey + ":journal", Fingerprint: fp, CorrelationID: input.Meta.CorrelationID, CausationID: input.Meta.CausationID, ActorType: input.Meta.Actor.Type, ActorID: input.Meta.Actor.ID, Lines: []repositories.PostingLine{{LedgerAccountID: clearing.ID, Side: models.SideDebit, AmountMinor: input.AmountMinor, Memo: "invoice payment"}, {LedgerAccountID: revenue.ID, Side: models.SideCredit, AmountMinor: input.AmountMinor, Memo: "subscription revenue"}}})
		if err != nil {
			return err
		}
		payment.JournalID = journal.ID
		if err := s.billing.CreatePayment(ctx, tx, payment); err != nil {
			return err
		}
		if err := s.billing.ApplyPaymentToInvoice(ctx, tx, invoice.ID, invoice.Version, input.AmountMinor); err != nil {
			return err
		}
		paid := input.AmountMinor >= invoice.OutstandingMinor()
		if paid && invoice.SubscriptionID != "" {
			sub, err := s.billing.GetSubscription(ctx, tx, input.OrganizationID, invoice.SubscriptionID)
			if err != nil {
				return err
			}
			plan, err := s.billing.GetPlan(ctx, tx, sub.PlanID)
			if err != nil {
				return err
			}
			start := s.now()
			end := plan.PeriodEnd(start)
			if err := s.billing.TransitionSubscription(ctx, tx, sub.ID, sub.Version, models.SubscriptionActive, &start, &end, nil, invoice.ID); err != nil {
				return err
			}
			if err := repositories.AppendHistory(ctx, tx, input.OrganizationID, "SUBSCRIPTION", sub.ID, sub.Status, models.SubscriptionActive, input.Meta.Actor.Type, input.Meta.Actor.ID, "invoice paid", map[string]any{"invoiceId": invoice.ID}, input.Meta.CorrelationID, input.Meta.CausationID); err != nil {
				return err
			}
		}
		if err := repositories.AppendHistory(ctx, tx, input.OrganizationID, "PAYMENT", payment.ID, "", payment.Status, input.Meta.Actor.Type, input.Meta.Actor.ID, "confirmed", map[string]any{"invoiceId": invoice.ID}, input.Meta.CorrelationID, input.Meta.CausationID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: input.OrganizationID, ActorType: input.Meta.Actor.Type, ActorID: input.Meta.Actor.ID, Action: "invoice.payment_confirmed", ResourceType: "payment", ResourceID: payment.ID, NewValues: map[string]any{"invoiceId": invoice.ID, "amountMinor": payment.AmountMinor, "currency": payment.Currency}, RequestID: input.Meta.RequestID, CorrelationID: input.Meta.CorrelationID}); err != nil {
			return err
		}
		event := EventInvoicePaid
		if !paid {
			event = EventInvoiceCreated
		}
		return s.events.Publish(ctx, tx, event, "INVOICE", invoice.ID, invoice.Version+1, input.OrganizationID, input.Meta.CorrelationID, input.Meta.CausationID, map[string]any{"invoiceId": invoice.ID, "paymentId": payment.ID, "amountPaidMinor": invoice.AmountPaidMinor + input.AmountMinor, "totalMinor": invoice.TotalMinor, "currency": invoice.Currency, "status": map[bool]string{true: models.InvoicePaid, false: models.InvoicePartiallyPaid}[paid]})
	})
	return payment, err
}

func (s *Service) PostEarningAllocation(ctx context.Context, input AllocationInput) (*models.EarningAllocation, error) {
	if err := input.Meta.validate(); err != nil {
		return nil, err
	}
	if input.GrossMinor <= 0 {
		return nil, ErrInvalid
	}
	fp := fingerprint(input.OrganizationID, input.SourceService, input.JobReference, input.ServiceType, input.ProviderReference, input.Currency, input.GrossMinor)
	if existing, err := s.marketplace.FindAllocationByIdempotencyKey(ctx, nil, input.OrganizationID, input.Meta.IdempotencyKey); err == nil {
		if existing.RequestFingerprint != fp {
			return nil, ErrIdempotencyReuse
		}
		return existing, nil
	} else if !errors.Is(err, repositories.ErrNotFound) {
		return nil, err
	}
	rule, err := s.marketplace.ResolveCommissionRule(ctx, nil, input.OrganizationID, input.ServiceType, input.Currency, s.now())
	if err != nil {
		return nil, err
	}
	split, err := rule.Apply(money.Money{AmountMinor: input.GrossMinor, Currency: input.Currency})
	if err != nil {
		return nil, err
	}
	var allocation *models.EarningAllocation
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		earning, _, err := s.accounts.EnsureEarningAccount(ctx, tx, input.ProviderService, input.ProviderReference, input.OrganizationID, input.Currency)
		if err != nil {
			return err
		}
		wallet, _, err := s.accounts.EnsureWallet(ctx, tx, input.OrganizationID, input.Currency)
		if err != nil {
			return err
		}
		receivable, err := s.ledger.EnsureAccount(ctx, tx, "", models.OwnerPlatform, "platform", models.AccountPlatformReceivable, input.Currency, models.SideDebit)
		if err != nil {
			return err
		}
		commission, err := s.ledger.EnsureAccount(ctx, tx, "", models.OwnerPlatform, "platform", models.AccountPlatformCommission, input.Currency, models.SideCredit)
		if err != nil {
			return err
		}
		fee, err := s.ledger.EnsureAccount(ctx, tx, "", models.OwnerPlatform, "platform", models.AccountPlatformFleetFee, input.Currency, models.SideCredit)
		if err != nil {
			return err
		}
		lines := []repositories.PostingLine{{LedgerAccountID: receivable.ID, Side: models.SideDebit, AmountMinor: input.GrossMinor, Memo: "job gross"}}
		for _, line := range []repositories.PostingLine{{LedgerAccountID: commission.ID, Side: models.SideCredit, AmountMinor: split.Commission.AmountMinor, Memo: "commission"}, {LedgerAccountID: fee.ID, Side: models.SideCredit, AmountMinor: split.FleetFee.AmountMinor, Memo: "fleet fee"}, {LedgerAccountID: earning.LedgerAccountID, Side: models.SideCredit, AmountMinor: split.ProviderEarning.AmountMinor, Memo: "provider earning"}, {LedgerAccountID: wallet.LedgerAccountID, Side: models.SideCredit, AmountMinor: split.OrganizationEarning.AmountMinor, Memo: "organization earning"}} {
			if line.AmountMinor > 0 {
				lines = append(lines, line)
			}
		}
		journal, err := s.ledger.Post(ctx, tx, repositories.Posting{OrganizationID: input.OrganizationID, JournalType: models.JournalEarningAllocation, Currency: input.Currency, Reference: input.JobReference, SourceService: input.SourceService, SourceReference: input.JobReference, IdempotencyKey: input.Meta.IdempotencyKey + ":journal", Fingerprint: fp, CorrelationID: input.Meta.CorrelationID, CausationID: input.Meta.CausationID, ActorType: input.Meta.Actor.Type, ActorID: input.Meta.Actor.ID, Lines: lines})
		if err != nil {
			return err
		}
		if split.ProviderEarning.AmountMinor > 0 {
			if err := s.accounts.MoveEarningBalances(ctx, tx, earning.ID, earning.Version, split.ProviderEarning.AmountMinor, 0, 0); err != nil {
				return err
			}
		}
		if split.OrganizationEarning.AmountMinor > 0 {
			if err := s.accounts.MoveWalletBalances(ctx, tx, wallet.ID, wallet.Version, split.OrganizationEarning.AmountMinor, 0, 0); err != nil {
				return err
			}
		}
		snapshot, _ := json.Marshal(rule.Snapshot())
		allocation = &models.EarningAllocation{OrganizationID: input.OrganizationID, SourceService: input.SourceService, JobReference: input.JobReference, Currency: input.Currency, RuleID: rule.ID, RuleVersion: rule.RuleVersion, RuleSnapshot: snapshot, GrossMinor: input.GrossMinor, CommissionMinor: split.Commission.AmountMinor, FleetFeeMinor: split.FleetFee.AmountMinor, ProviderEarningMinor: split.ProviderEarning.AmountMinor, OrganizationEarningMinor: split.OrganizationEarning.AmountMinor, ProviderReference: input.ProviderReference, EarningAccountID: earning.ID, WalletID: wallet.ID, JournalID: journal.ID, IdempotencyKey: input.Meta.IdempotencyKey, RequestFingerprint: fp, CorrelationID: input.Meta.CorrelationID, CausationID: input.Meta.CausationID}
		if err := s.marketplace.CreateAllocation(ctx, tx, allocation); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, input.OrganizationID, "ALLOCATION", allocation.ID, "", models.AllocationPosted, input.Meta.Actor.Type, input.Meta.Actor.ID, "job earning allocated", map[string]any{"journalId": journal.ID}, input.Meta.CorrelationID, input.Meta.CausationID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: input.OrganizationID, ActorType: input.Meta.Actor.Type, ActorID: input.Meta.Actor.ID, Action: "earning.posted", ResourceType: "allocation", ResourceID: allocation.ID, NewValues: map[string]any{"grossMinor": allocation.GrossMinor, "currency": allocation.Currency, "ruleVersion": allocation.RuleVersion}, RequestID: input.Meta.RequestID, CorrelationID: input.Meta.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventAllocationPosted, "ALLOCATION", allocation.ID, allocation.Version, input.OrganizationID, input.Meta.CorrelationID, input.Meta.CausationID, map[string]any{"allocationId": allocation.ID, "jobReference": input.JobReference, "earningAccountId": earning.ID, "walletId": wallet.ID, "grossMinor": allocation.GrossMinor, "commissionMinor": allocation.CommissionMinor, "fleetFeeMinor": allocation.FleetFeeMinor, "providerEarningMinor": allocation.ProviderEarningMinor, "organizationEarningMinor": allocation.OrganizationEarningMinor, "currency": allocation.Currency, "ruleVersion": allocation.RuleVersion})
	})
	return allocation, err
}

type HoldInput struct {
	Meta                                                                                                       Meta
	OrganizationID, SubjectType, SubjectID, ReasonCode, Reason, AuthorityService, AuthorityReference, Currency string
	AmountMinor, ExpectedVersion                                                                               int64
	ReferenceVerified                                                                                          bool
	ExpiresAt                                                                                                  *time.Time
}

func (s *Service) CreateHold(ctx context.Context, input HoldInput) (*models.Hold, error) {
	if err := input.Meta.validate(); err != nil {
		return nil, err
	}
	if err := s.require(ctx, input.Meta, financeauth.PermHoldCreate); err != nil {
		return nil, err
	}
	if input.AmountMinor <= 0 {
		return nil, ErrInvalid
	}
	if !input.ReferenceVerified && input.ExpiresAt == nil {
		return nil, fmt.Errorf("%w: unverified references require expiry", ErrInvalid)
	}
	fp := fingerprint(input.OrganizationID, input.SubjectType, input.SubjectID, input.AmountMinor, input.Currency, input.ReasonCode, input.AuthorityReference)
	if existing, err := s.holds.FindByIdempotencyKey(ctx, nil, input.OrganizationID, input.Meta.IdempotencyKey); err == nil {
		if existing.RequestFingerprint != fp {
			return nil, ErrIdempotencyReuse
		}
		return existing, nil
	} else if !errors.Is(err, repositories.ErrNotFound) {
		return nil, err
	}
	var hold *models.Hold
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		var availableID, heldID string
		var version int64
		switch input.SubjectType {
		case models.HoldSubjectWallet:
			wallet, err := s.accounts.LockWallet(ctx, tx, input.SubjectID)
			if err != nil {
				return err
			}
			if wallet.OrganizationID != input.OrganizationID || wallet.Currency != input.Currency {
				return financeauth.ErrForbidden
			}
			if !wallet.Spendable() || wallet.AvailableMinor < input.AmountMinor {
				return ErrInsufficientFunds
			}
			if input.ExpectedVersion != wallet.Version {
				return repositories.ErrVersionMismatch
			}
			availableID = wallet.LedgerAccountID
			version = wallet.Version
			held, err := s.ledger.EnsureAccount(ctx, tx, input.OrganizationID, models.OwnerOrganizationWallet, wallet.ID, models.AccountWalletHeld, input.Currency, models.SideCredit)
			if err != nil {
				return err
			}
			heldID = held.ID
		case models.HoldSubjectEarningAccount:
			account, err := s.accounts.LockEarningAccount(ctx, tx, input.SubjectID)
			if err != nil {
				return err
			}
			if account.OrganizationID != input.OrganizationID || account.Currency != input.Currency {
				return financeauth.ErrForbidden
			}
			if !account.Payable() || account.PayableMinor < input.AmountMinor {
				return ErrInsufficientFunds
			}
			if input.ExpectedVersion != account.Version {
				return repositories.ErrVersionMismatch
			}
			availableID = account.LedgerAccountID
			version = account.Version
			held, err := s.ledger.EnsureAccount(ctx, tx, input.OrganizationID, models.OwnerProviderEarning, account.ID, models.AccountEarningHeld, input.Currency, models.SideCredit)
			if err != nil {
				return err
			}
			heldID = held.ID
		default:
			return ErrInvalid
		}
		journal, err := s.ledger.Post(ctx, tx, repositories.Posting{OrganizationID: input.OrganizationID, JournalType: models.JournalHoldPlaced, Currency: input.Currency, Reference: input.AuthorityReference, SourceService: input.AuthorityService, SourceReference: input.AuthorityReference, IdempotencyKey: input.Meta.IdempotencyKey + ":journal", Fingerprint: fp, CorrelationID: input.Meta.CorrelationID, CausationID: input.Meta.CausationID, ActorType: input.Meta.Actor.Type, ActorID: input.Meta.Actor.ID, Lines: []repositories.PostingLine{{LedgerAccountID: availableID, Side: models.SideDebit, AmountMinor: input.AmountMinor, Memo: "hold placed"}, {LedgerAccountID: heldID, Side: models.SideCredit, AmountMinor: input.AmountMinor, Memo: "hold placed"}}})
		if err != nil {
			return err
		}
		if input.SubjectType == models.HoldSubjectWallet {
			if err := s.accounts.MoveWalletBalances(ctx, tx, input.SubjectID, version, -input.AmountMinor, input.AmountMinor, 0); err != nil {
				return err
			}
		} else {
			if err := s.accounts.MoveEarningBalances(ctx, tx, input.SubjectID, version, -input.AmountMinor, input.AmountMinor, 0); err != nil {
				return err
			}
		}
		hold = &models.Hold{OrganizationID: input.OrganizationID, SubjectType: input.SubjectType, ReasonCode: input.ReasonCode, Reason: input.Reason, AmountMinor: input.AmountMinor, Currency: input.Currency, AuthorityService: input.AuthorityService, AuthoritativeReference: input.AuthorityReference, ReferenceVerified: input.ReferenceVerified, State: models.HoldActive, ExpiresAt: input.ExpiresAt, JournalID: journal.ID, IdempotencyKey: input.Meta.IdempotencyKey, RequestFingerprint: fp, CorrelationID: input.Meta.CorrelationID, CausationID: input.Meta.CausationID}
		if input.SubjectType == models.HoldSubjectWallet {
			hold.WalletID = input.SubjectID
		} else {
			hold.EarningAccountID = input.SubjectID
		}
		if err := s.holds.Create(ctx, tx, hold); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, input.OrganizationID, "HOLD", hold.ID, "", hold.State, input.Meta.Actor.Type, input.Meta.Actor.ID, input.ReasonCode, nil, input.Meta.CorrelationID, input.Meta.CausationID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: input.OrganizationID, ActorType: input.Meta.Actor.Type, ActorID: input.Meta.Actor.ID, Action: "hold.created", ResourceType: "hold", ResourceID: hold.ID, NewValues: map[string]any{"amountMinor": input.AmountMinor, "currency": input.Currency, "state": hold.State}, RequestID: input.Meta.RequestID, CorrelationID: input.Meta.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventHoldCreated, "HOLD", hold.ID, hold.Version, input.OrganizationID, input.Meta.CorrelationID, input.Meta.CausationID, map[string]any{"holdId": hold.ID, "subjectType": hold.SubjectType, "subjectId": input.SubjectID, "amountMinor": hold.AmountMinor, "currency": hold.Currency, "state": hold.State, "expiresAt": hold.ExpiresAt})
	})
	return hold, err
}

func (s *Service) ReleaseHold(ctx context.Context, meta Meta, organizationID, id string, expectedVersion int64, targetState string) (*models.Hold, error) {
	if err := meta.validate(); err != nil {
		return nil, err
	}
	if err := s.require(ctx, meta, financeauth.PermHoldRelease); err != nil {
		return nil, err
	}
	if targetState != "" && targetState != models.HoldReleased && targetState != models.HoldExpired && targetState != models.HoldCancelled {
		return nil, ErrInvalid
	}
	if targetState == "" {
		targetState = models.HoldReleased
	}
	var result *models.Hold
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		hold, err := s.holds.Lock(ctx, tx, id)
		if err != nil {
			return err
		}
		if hold.OrganizationID != organizationID {
			return financeauth.ErrForbidden
		}
		if hold.Version != expectedVersion {
			return repositories.ErrVersionMismatch
		}
		if !hold.Holding() {
			result = hold
			return nil
		}
		original, err := s.ledger.GetJournal(ctx, tx, hold.JournalID)
		if err != nil {
			return err
		}
		reversal, err := s.ledger.Reverse(ctx, tx, original.ID, models.JournalHoldReleased, "hold released", meta.Actor.Type, meta.Actor.ID, meta.IdempotencyKey+":journal", meta.CorrelationID)
		if err != nil {
			return err
		}
		if hold.SubjectType == models.HoldSubjectWallet {
			wallet, err := s.accounts.LockWallet(ctx, tx, hold.WalletID)
			if err != nil {
				return err
			}
			if err := s.accounts.MoveWalletBalances(ctx, tx, wallet.ID, wallet.Version, hold.AmountMinor, -hold.AmountMinor, 0); err != nil {
				return err
			}
		} else {
			account, err := s.accounts.LockEarningAccount(ctx, tx, hold.EarningAccountID)
			if err != nil {
				return err
			}
			if err := s.accounts.MoveEarningBalances(ctx, tx, account.ID, account.Version, hold.AmountMinor, -hold.AmountMinor, 0); err != nil {
				return err
			}
		}
		if err := s.holds.Transition(ctx, tx, hold.ID, hold.Version, targetState, reversal.ID); err != nil {
			return err
		}
		hold.State = targetState
		hold.ReleaseJournalID = reversal.ID
		hold.Version++
		result = hold
		if err := repositories.AppendHistory(ctx, tx, organizationID, "HOLD", hold.ID, models.HoldActive, targetState, meta.Actor.Type, meta.Actor.ID, "released", nil, meta.CorrelationID, meta.CausationID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: organizationID, ActorType: meta.Actor.Type, ActorID: meta.Actor.ID, Action: "hold.released", ResourceType: "hold", ResourceID: hold.ID, PreviousValues: map[string]any{"state": models.HoldActive}, NewValues: map[string]any{"state": targetState}, RequestID: meta.RequestID, CorrelationID: meta.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventHoldReleased, "HOLD", hold.ID, hold.Version, organizationID, meta.CorrelationID, meta.CausationID, map[string]any{"holdId": hold.ID, "state": targetState, "amountMinor": hold.AmountMinor, "currency": hold.Currency})
	})
	return result, err
}

type RefundInput struct {
	Meta                                                  Meta
	OrganizationID, OriginalJournalID, ReasonCode, Reason string
	AmountMinor, ExpectedVersion                          int64
}

type SettlementInput struct {
	Meta                 Meta
	OrganizationID       string
	BeneficiaryType      string
	BeneficiaryReference string
	EarningAccountID     string
	WalletID             string
	Currency             string
	ExpectedVersion      int64
	Limit                int
}

func (s *Service) RequestSettlement(ctx context.Context, input SettlementInput) (*models.SettlementBatch, error) {
	if err := input.Meta.validate(); err != nil {
		return nil, err
	}
	if err := s.require(ctx, input.Meta, financeauth.PermSettlementInitiate); err != nil {
		return nil, err
	}
	if input.BeneficiaryType != models.BeneficiaryProvider && input.BeneficiaryType != models.BeneficiaryCompany {
		return nil, ErrInvalid
	}
	fp := fingerprint(input.OrganizationID, input.BeneficiaryType, input.BeneficiaryReference,
		input.EarningAccountID, input.WalletID, input.Currency, input.ExpectedVersion)
	if existing, err := s.operations.FindSettlementBatchByKey(ctx, nil, input.OrganizationID, input.Meta.IdempotencyKey); err == nil {
		if existing.RequestFingerprint != fp {
			return nil, ErrIdempotencyReuse
		}
		return existing, nil
	} else if !errors.Is(err, repositories.ErrNotFound) {
		return nil, err
	}
	var allocations []models.EarningAllocation
	if input.BeneficiaryType == models.BeneficiaryProvider {
		account, err := s.accounts.GetEarningAccountByID(ctx, nil, input.EarningAccountID)
		if err != nil {
			return nil, err
		}
		if account.OrganizationID != input.OrganizationID || account.Currency != input.Currency {
			return nil, financeauth.ErrForbidden
		}
		if account.Version != input.ExpectedVersion {
			return nil, repositories.ErrVersionMismatch
		}
		if !account.Payable() || account.PayoutDestinationRef == "" {
			return nil, fmt.Errorf("%w: earning account is not payable", ErrInvalid)
		}
		allocations, err = s.marketplace.SettleableAllocations(ctx, nil, account.ID, input.Currency, input.Limit)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("%w: company settlement requires allocated company earning items", ErrInvalid)
	}
	if len(allocations) == 0 {
		return nil, fmt.Errorf("%w: no settleable earnings", ErrInvalid)
	}
	batch := &models.SettlementBatch{OrganizationID: input.OrganizationID,
		BeneficiaryType: input.BeneficiaryType, BeneficiaryReference: input.BeneficiaryReference,
		EarningAccountID: input.EarningAccountID, WalletID: input.WalletID,
		Currency: input.Currency, Status: models.BatchPendingApproval,
		RequestedByType: input.Meta.Actor.Type, RequestedByID: input.Meta.Actor.ID,
		IdempotencyKey: input.Meta.IdempotencyKey, RequestFingerprint: fp,
		CorrelationID: input.Meta.CorrelationID}
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.operations.CreateSettlementBatch(ctx, tx, batch, allocations); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, input.OrganizationID, "SETTLEMENT_BATCH", batch.ID,
			"", batch.Status, input.Meta.Actor.Type, input.Meta.Actor.ID, "requested",
			map[string]any{"itemCount": batch.ItemCount}, input.Meta.CorrelationID, input.Meta.CausationID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: input.OrganizationID,
			ActorType: input.Meta.Actor.Type, ActorID: input.Meta.Actor.ID,
			Action: "settlement.requested", ResourceType: "settlement_batch", ResourceID: batch.ID,
			NewValues: map[string]any{"totalMinor": batch.TotalMinor, "currency": batch.Currency, "itemCount": batch.ItemCount},
			RequestID: input.Meta.RequestID, CorrelationID: input.Meta.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventSettlementChanged, "SETTLEMENT_BATCH", batch.ID,
			batch.Version, input.OrganizationID, input.Meta.CorrelationID, input.Meta.CausationID,
			map[string]any{"settlementBatchId": batch.ID, "status": batch.Status,
				"totalMinor": batch.TotalMinor, "currency": batch.Currency, "itemCount": batch.ItemCount})
	})
	return batch, err
}

func (s *Service) ApproveSettlement(ctx context.Context, meta Meta, organizationID, id string,
	expectedVersion int64) (*models.SettlementBatch, error) {
	if err := meta.validate(); err != nil {
		return nil, err
	}
	batch, err := s.operations.GetSettlementBatch(ctx, nil, organizationID, id)
	if err != nil {
		return nil, err
	}
	if batch.Version != expectedVersion {
		return nil, repositories.ErrVersionMismatch
	}
	if _, err := s.authorizer.RequireApproval(ctx, meta.Actor, financeauth.PermSettlementApprove,
		financeauth.OperationSettlementApproval, batch.RequesterIdentity()); err != nil {
		return nil, err
	}
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.operations.ApproveSettlementBatch(ctx, tx, id, expectedVersion, meta.Actor.Type, meta.Actor.ID); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, organizationID, "SETTLEMENT_BATCH", id,
			batch.Status, models.BatchApproved, meta.Actor.Type, meta.Actor.ID, "approved", nil,
			meta.CorrelationID, meta.CausationID); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Entry{OrganizationID: organizationID,
			ActorType: meta.Actor.Type, ActorID: meta.Actor.ID, Action: "settlement.approved",
			ResourceType: "settlement_batch", ResourceID: id,
			PreviousValues: map[string]any{"status": batch.Status}, NewValues: map[string]any{"status": models.BatchApproved},
			RequestID: meta.RequestID, CorrelationID: meta.CorrelationID})
	})
	if err != nil {
		return nil, err
	}
	return s.operations.GetSettlementBatch(ctx, nil, organizationID, id)
}

func (s *Service) RequestRefund(ctx context.Context, input RefundInput) (*models.Refund, error) {
	if err := input.Meta.validate(); err != nil {
		return nil, err
	}
	if err := s.require(ctx, input.Meta, financeauth.PermRefundRequest); err != nil {
		return nil, err
	}
	if input.AmountMinor <= 0 {
		return nil, ErrInvalid
	}
	fp := fingerprint(input.OrganizationID, input.OriginalJournalID, input.AmountMinor, input.ReasonCode)
	if existing, err := s.operations.FindRefundByKey(ctx, nil, input.OrganizationID, input.Meta.IdempotencyKey); err == nil {
		if existing.RequestFingerprint != fp {
			return nil, ErrIdempotencyReuse
		}
		return existing, nil
	} else if !errors.Is(err, repositories.ErrNotFound) {
		return nil, err
	}
	original, err := s.ledger.GetJournal(ctx, nil, input.OriginalJournalID)
	if err != nil {
		return nil, err
	}
	if original.OrganizationID != input.OrganizationID || original.Currency == "" {
		return nil, financeauth.ErrForbidden
	}
	remaining, err := s.ledger.RefundableMinor(ctx, nil, original.ID)
	if err != nil {
		return nil, err
	}
	if input.AmountMinor > remaining {
		return nil, fmt.Errorf("%w: refund exceeds refundable amount", ErrInvalid)
	}
	refund := &models.Refund{OrganizationID: input.OrganizationID, OriginalJournalID: original.ID, OriginalReference: original.Reference, AmountMinor: input.AmountMinor, Currency: original.Currency, ReasonCode: input.ReasonCode, Reason: input.Reason, Status: models.RefundRequested, RequestedByType: input.Meta.Actor.Type, RequestedByID: input.Meta.Actor.ID, IdempotencyKey: input.Meta.IdempotencyKey, RequestFingerprint: fp, CorrelationID: input.Meta.CorrelationID}
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.operations.CreateRefund(ctx, tx, refund); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, input.OrganizationID, "REFUND", refund.ID, "", refund.Status, input.Meta.Actor.Type, input.Meta.Actor.ID, input.ReasonCode, nil, input.Meta.CorrelationID, input.Meta.CausationID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: input.OrganizationID, ActorType: input.Meta.Actor.Type, ActorID: input.Meta.Actor.ID, Action: "refund.requested", ResourceType: "refund", ResourceID: refund.ID, NewValues: map[string]any{"amountMinor": refund.AmountMinor, "currency": refund.Currency}, RequestID: input.Meta.RequestID, CorrelationID: input.Meta.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventRefundChanged, "REFUND", refund.ID, refund.Version, input.OrganizationID, input.Meta.CorrelationID, input.Meta.CausationID, map[string]any{"refundId": refund.ID, "status": refund.Status, "amountMinor": refund.AmountMinor, "currency": refund.Currency, "originalJournalId": refund.OriginalJournalID})
	})
	return refund, err
}

func (s *Service) ApproveRefund(ctx context.Context, meta Meta, organizationID, id string, expectedVersion int64) (*models.Refund, error) {
	if err := meta.validate(); err != nil {
		return nil, err
	}
	refund, err := s.operations.GetRefund(ctx, nil, organizationID, id)
	if err != nil {
		return nil, err
	}
	if refund.Version != expectedVersion {
		return nil, repositories.ErrVersionMismatch
	}
	if _, err := s.authorizer.RequireApproval(ctx, meta.Actor, financeauth.PermRefundApprove, financeauth.OperationRefundApproval, refund.RequesterIdentity()); err != nil {
		return nil, err
	}
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.operations.ApproveRefund(ctx, tx, id, expectedVersion, meta.Actor.Type, meta.Actor.ID); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, organizationID, "REFUND", id, refund.Status, models.RefundApproved, meta.Actor.Type, meta.Actor.ID, "approved", nil, meta.CorrelationID, meta.CausationID); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Entry{OrganizationID: organizationID, ActorType: meta.Actor.Type, ActorID: meta.Actor.ID, Action: "refund.approved", ResourceType: "refund", ResourceID: id, PreviousValues: map[string]any{"status": refund.Status}, NewValues: map[string]any{"status": models.RefundApproved}, RequestID: meta.RequestID, CorrelationID: meta.CorrelationID})
	})
	if err != nil {
		return nil, err
	}
	return s.operations.GetRefund(ctx, nil, organizationID, id)
}

func (s *Service) Reconcile(ctx context.Context, meta Meta, scope string) (*models.ReconciliationRun, error) {
	if err := s.require(ctx, meta, financeauth.PermReconciliationRun); err != nil {
		return nil, err
	}
	run := &models.ReconciliationRun{Scope: scope}
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error { return s.operations.CreateReconciliationRun(ctx, tx, run) })
	if err != nil {
		return nil, err
	}
	checked, findings := 0, 0
	unbalanced, err := s.ledger.UnbalancedJournals(ctx, nil, 200)
	if err != nil {
		return nil, err
	}
	for _, id := range unbalanced {
		finding := &models.ReconciliationFinding{RunID: run.ID, EntityType: "JOURNAL", EntityID: id, FindingCode: models.FindingUnbalancedJournal}
		if err := s.operations.AddFinding(ctx, nil, finding); err != nil {
			return nil, err
		}
		findings++
	}
	checked += len(unbalanced)
	drifted, err := s.ledger.DriftedBalances(ctx, nil, 200)
	if err != nil {
		return nil, err
	}
	for _, balance := range drifted {
		detail, _ := json.Marshal(map[string]any{"currency": balance.Currency, "projectedDebitMinor": balance.DebitMinor, "projectedCreditMinor": balance.CreditMinor})
		finding := &models.ReconciliationFinding{RunID: run.ID, EntityType: "LEDGER_ACCOUNT", EntityID: balance.LedgerAccountID, FindingCode: models.FindingBalanceDrift, Details: detail}
		if err := s.operations.AddFinding(ctx, nil, finding); err != nil {
			return nil, err
		}
		findings++
	}
	checked += len(drifted)
	// Reversals whose outcome stayed ambiguous, and reversals that claim
	// completion without a compensating journal, are both reported rather than
	// repaired: an automatic "fix" to a financial correction is how a single
	// error becomes two.
	stuck, err := s.reversals.StuckReversals(ctx, nil, s.now().Add(-15*time.Minute), 200)
	if err != nil {
		return nil, err
	}
	for _, reversal := range stuck {
		detail, _ := json.Marshal(map[string]any{"status": reversal.Status,
			"attempts": reversal.Attempts, "safeErrorCode": reversal.SafeErrorCode})
		finding := &models.ReconciliationFinding{RunID: run.ID,
			OrganizationID: reversal.OrganizationID, EntityType: "REVERSAL",
			EntityID: reversal.ID, FindingCode: models.FindingStuckReversal, Details: detail}
		if err := s.operations.AddFinding(ctx, nil, finding); err != nil {
			return nil, err
		}
		findings++
	}
	checked += len(stuck)
	unposted, err := s.reversals.CompletedWithoutJournal(ctx, nil, 200)
	if err != nil {
		return nil, err
	}
	for _, id := range unposted {
		finding := &models.ReconciliationFinding{RunID: run.ID, EntityType: "REVERSAL",
			EntityID: id, FindingCode: models.FindingReversalNoJournal}
		if err := s.operations.AddFinding(ctx, nil, finding); err != nil {
			return nil, err
		}
		findings++
	}
	checked += len(unposted)
	if err := s.operations.CompleteReconciliationRun(ctx, nil, run.ID, checked, findings, ""); err != nil {
		return nil, err
	}
	run.Status = "COMPLETED"
	run.CheckedCount = checked
	run.FindingsCount = findings
	now := s.now()
	run.CompletedAt = &now
	return run, nil
}

func (s *Service) GenerateStatement(ctx context.Context, meta Meta, organizationID, ownerType, ownerRef, currency, statementType string, from, to time.Time) (*models.Statement, error) {
	if err := s.require(ctx, meta, financeauth.PermStatementExport); err != nil {
		return nil, err
	}
	if !from.Before(to) {
		return nil, ErrInvalid
	}
	lines, err := s.ledger.StatementLines(ctx, nil, ownerType, ownerRef, currency, from, to)
	if err != nil {
		return nil, err
	}
	statement := &models.Statement{OrganizationID: organizationID, StatementType: statementType, Currency: currency, PeriodStart: from, PeriodEnd: to, LineCount: len(lines), GeneratedByType: meta.Actor.Type, GeneratedByID: meta.Actor.ID, Lines: lines}
	h := sha256.New()
	for _, line := range lines {
		_, _ = fmt.Fprintf(h, "%s|%s|%d|%s|%s\n", line.JournalID, line.Side, line.AmountMinor, line.Currency, line.PostedAt.UTC().Format(time.RFC3339Nano))
		if line.Side == models.SideDebit {
			statement.TotalDebitMinor += line.AmountMinor
		} else {
			statement.TotalCreditMinor += line.AmountMinor
		}
	}
	statement.ContentHash = hex.EncodeToString(h.Sum(nil))
	if err := s.operations.CreateStatement(ctx, nil, statement); err != nil {
		return nil, err
	}
	return statement, nil
}

func (s *Service) Accounts() *repositories.AccountRepository        { return s.accounts }
func (s *Service) Billing() *repositories.BillingRepository         { return s.billing }
func (s *Service) Marketplace() *repositories.MarketplaceRepository { return s.marketplace }
func (s *Service) Holds() *repositories.HoldRepository              { return s.holds }
func (s *Service) Operations() *repositories.OperationsRepository   { return s.operations }
func (s *Service) Reversals() *repositories.ReversalRepository      { return s.reversals }
func (s *Service) JobPayments() *repositories.JobPaymentRepository  { return s.jobPayments }
func (s *Service) Ledger() *repositories.LedgerRepository           { return s.ledger }
func (s *Service) LedgerDB() database.Querier                       { return s.pool }
