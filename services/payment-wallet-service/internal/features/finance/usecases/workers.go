package usecases

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/repositories"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/audit"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/financeauth"
)

const (
	OutcomeSucceeded = "SUCCEEDED"
	OutcomeUnknown   = "UNKNOWN"
	OutcomeRetryable = "RETRYABLE"
	OutcomePermanent = "PERMANENT"
)

// ExternalOutcome is the classified result of a payout/refund request or read-back.
type ExternalOutcome struct {
	Kind              string
	ExternalReference string
	SafeErrorCode     string
}

// MoneyGateway is the authoritative external execution/read-back boundary.
// A timeout must return OutcomeUnknown; workers will read back rather than issue
// a duplicate operation.
type MoneyGateway interface {
	ExecuteSettlement(context.Context, models.SettlementBatch, models.SettlementItem) (ExternalOutcome, error)
	ReadSettlement(context.Context, models.SettlementItem) (ExternalOutcome, error)
	ExecuteRefund(context.Context, models.Refund) (ExternalOutcome, error)
	ReadRefund(context.Context, models.Refund) (ExternalOutcome, error)
	// ExecuteReversal performs whatever external movement a reversal requires
	// before the compensating journal may be posted. A reversal that is purely
	// internal returns OutcomeSucceeded immediately; an ambiguous provider
	// result must return OutcomeUnknown so ReadReversal resolves it.
	ExecuteReversal(context.Context, models.Reversal) (ExternalOutcome, error)
	ReadReversal(context.Context, models.Reversal) (ExternalOutcome, error)
	Configured() bool
}

type WorkerConfig struct {
	Owner       string
	BatchSize   int
	Lease       time.Duration
	Interval    time.Duration
	MaxAttempts int
	BaseBackoff time.Duration
}

func (c WorkerConfig) normalized() WorkerConfig {
	if c.Owner == "" {
		c.Owner = "payment-wallet-service"
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 25
	}
	if c.Lease <= 0 {
		c.Lease = 30 * time.Second
	}
	if c.Interval <= 0 {
		c.Interval = 5 * time.Second
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 8
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = 2 * time.Second
	}
	return c
}

// RunFinancialWorkers runs settlement/refund execution and read-back until ctx ends.
func (s *Service) RunFinancialWorkers(ctx context.Context, gateway MoneyGateway, config WorkerConfig) error {
	if gateway == nil || !gateway.Configured() {
		return errors.New("finance: money gateway is not configured")
	}
	config = config.normalized()
	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()
	for {
		if err := s.runFinancialPass(ctx, gateway, config); err != nil && ctx.Err() == nil {
			// The next interval retries. Individual rows retain durable state; a
			// process-level error must not terminate all financial recovery.
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Service) runFinancialPass(ctx context.Context, gateway MoneyGateway, config WorkerConfig) error {
	_, _ = s.operations.RecoverExpiredLeases(ctx)
	items, err := s.operations.ClaimSettlementItems(ctx, config.Owner, config.Lease, config.BatchSize)
	if err != nil {
		return err
	}
	for i := range items {
		if err := s.processSettlement(ctx, gateway, config, items[i]); err != nil {
			return err
		}
	}
	refunds, err := s.operations.ClaimRefunds(ctx, config.Owner, config.Lease, config.BatchSize)
	if err != nil {
		return err
	}
	for i := range refunds {
		if err := s.processRefund(ctx, gateway, config, refunds[i]); err != nil {
			return err
		}
	}
	// Reversals share the same lease/retry/read-back machinery rather than a
	// second execution framework.
	_, _ = s.reversals.RecoverExpiredLeases(ctx)
	reversals, err := s.reversals.Claim(ctx, config.Owner, config.Lease, config.BatchSize)
	if err != nil {
		return err
	}
	for i := range reversals {
		if err := s.processReversal(ctx, gateway, config, reversals[i]); err != nil {
			return err
		}
	}
	return nil
}

// RunFinancialPass executes one bounded pass for deterministic operator recovery.
func (s *Service) RunFinancialPass(ctx context.Context, gateway MoneyGateway, config WorkerConfig) error {
	if gateway == nil || !gateway.Configured() {
		return errors.New("finance: money gateway is not configured")
	}
	return s.runFinancialPass(ctx, gateway, config.normalized())
}

func (s *Service) processSettlement(ctx context.Context, gateway MoneyGateway,
	config WorkerConfig, item models.SettlementItem) error {
	batch, err := s.operations.GetSettlementBatch(ctx, nil, item.OrganizationID, item.BatchID)
	if err != nil {
		return err
	}
	var outcome ExternalOutcome
	if item.NeedsReadBack() || item.ExternalReference != "" {
		outcome, err = gateway.ReadSettlement(ctx, item)
	} else {
		outcome, err = gateway.ExecuteSettlement(ctx, *batch, item)
	}
	if err != nil && outcome.Kind == "" {
		outcome = ExternalOutcome{Kind: OutcomeUnknown, SafeErrorCode: "gateway_unavailable"}
	}
	next := time.Now().UTC().Add(backoff(config.BaseBackoff, item.Attempts))
	switch outcome.Kind {
	case OutcomeSucceeded:
		err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
			account, loadErr := s.accounts.LockEarningAccount(ctx, tx, batch.EarningAccountID)
			if loadErr != nil {
				return loadErr
			}
			if account.PayableMinor < item.AmountMinor {
				return ErrInsufficientFunds
			}
			clearing, loadErr := s.ledger.EnsureAccount(ctx, tx, "", models.OwnerPlatform,
				"platform", models.AccountPlatformClearing, item.Currency, models.SideCredit)
			if loadErr != nil {
				return loadErr
			}
			journal, postErr := s.ledger.Post(ctx, tx, repositories.Posting{
				OrganizationID: item.OrganizationID, JournalType: models.JournalSettlementPayout,
				Currency: item.Currency, Reference: outcome.ExternalReference,
				SourceService: "payment-wallet-service", SourceReference: item.ID,
				IdempotencyKey: item.IdempotencyKey + ":journal", Fingerprint: fingerprint(item.ID, item.AmountMinor, item.Currency),
				ActorType: "SYSTEM", ActorID: config.Owner,
				Lines: []repositories.PostingLine{{LedgerAccountID: account.LedgerAccountID, Side: models.SideDebit, AmountMinor: item.AmountMinor, Memo: "settlement payout"},
					{LedgerAccountID: clearing.ID, Side: models.SideCredit, AmountMinor: item.AmountMinor, Memo: "settlement clearing"}},
			})
			if postErr != nil {
				return postErr
			}
			if moveErr := s.accounts.MoveEarningBalances(ctx, tx, account.ID, account.Version, -item.AmountMinor, 0, item.AmountMinor); moveErr != nil {
				return moveErr
			}
			if historyErr := repositories.AppendHistory(ctx, tx, item.OrganizationID, "SETTLEMENT_ITEM", item.ID,
				item.Status, models.SettlementSucceeded, "SYSTEM", config.Owner, "payout confirmed", nil, "", item.BatchID); historyErr != nil {
				return historyErr
			}
			if outcomeErr := s.operations.SetSettlementOutcome(ctx, tx, item.ID, config.Owner, models.SettlementSucceeded, outcome.ExternalReference, journal.ID, next, "", config.MaxAttempts); outcomeErr != nil {
				return outcomeErr
			}
			if refreshErr := s.operations.RefreshSettlementBatch(ctx, tx, item.BatchID); refreshErr != nil {
				return refreshErr
			}
			if auditErr := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: item.OrganizationID, ActorType: "SYSTEM", ActorID: config.Owner, Action: "settlement.succeeded", ResourceType: "settlement_item", ResourceID: item.ID, PreviousValues: map[string]any{"status": item.Status}, NewValues: map[string]any{"status": models.SettlementSucceeded, "journalId": journal.ID}, CorrelationID: item.BatchID}); auditErr != nil {
				return auditErr
			}
			return s.events.Publish(ctx, tx, EventSettlementChanged, "SETTLEMENT_ITEM", item.ID, item.Version+1, item.OrganizationID, item.BatchID, item.BatchID, map[string]any{"settlementBatchId": item.BatchID, "settlementItemId": item.ID, "status": models.SettlementSucceeded, "amountMinor": item.AmountMinor, "currency": item.Currency, "journalId": journal.ID})
		})
		if err != nil {
			outcome.Kind = OutcomeRetryable
			outcome.SafeErrorCode = "posting_failed"
		}
		if outcome.Kind == OutcomeSucceeded {
			return nil
		}
		fallthrough
	case OutcomeRetryable:
		return s.recordSettlementOutcome(ctx, config, item, models.SettlementRetryableFailure, outcome, next)
	case OutcomePermanent:
		return s.recordSettlementOutcome(ctx, config, item, models.SettlementPermanentFailure, outcome, next)
	default:
		return s.recordSettlementOutcome(ctx, config, item, models.SettlementUnknown, outcome, next)
	}
}

func (s *Service) recordSettlementOutcome(ctx context.Context, config WorkerConfig, item models.SettlementItem, status string, outcome ExternalOutcome, next time.Time) error {
	return database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.operations.SetSettlementOutcome(ctx, tx, item.ID, config.Owner, status, outcome.ExternalReference, "", next, outcome.SafeErrorCode, config.MaxAttempts); err != nil {
			return err
		}
		if err := s.operations.RefreshSettlementBatch(ctx, tx, item.BatchID); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, item.OrganizationID, "SETTLEMENT_ITEM", item.ID, item.Status, status, "SYSTEM", config.Owner, outcome.SafeErrorCode, nil, item.BatchID, item.BatchID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: item.OrganizationID, ActorType: "SYSTEM", ActorID: config.Owner, Action: "settlement.status_changed", ResourceType: "settlement_item", ResourceID: item.ID, PreviousValues: map[string]any{"status": item.Status}, NewValues: map[string]any{"status": status, "safeErrorCode": outcome.SafeErrorCode}, CorrelationID: item.BatchID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventSettlementChanged, "SETTLEMENT_ITEM", item.ID, item.Version+1, item.OrganizationID, item.BatchID, item.BatchID, map[string]any{"settlementBatchId": item.BatchID, "settlementItemId": item.ID, "status": status, "safeErrorCode": outcome.SafeErrorCode})
	})
}

func (s *Service) processRefund(ctx context.Context, gateway MoneyGateway,
	config WorkerConfig, refund models.Refund) error {
	var outcome ExternalOutcome
	var err error
	if refund.NeedsReadBack() || refund.ExternalReference != "" {
		outcome, err = gateway.ReadRefund(ctx, refund)
	} else {
		outcome, err = gateway.ExecuteRefund(ctx, refund)
	}
	if err != nil && outcome.Kind == "" {
		outcome = ExternalOutcome{Kind: OutcomeUnknown, SafeErrorCode: "gateway_unavailable"}
	}
	next := time.Now().UTC().Add(backoff(config.BaseBackoff, refund.Attempts))
	switch outcome.Kind {
	case OutcomeSucceeded:
		err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
			original, loadErr := s.ledger.GetJournal(ctx, tx, refund.OriginalJournalID)
			if loadErr != nil {
				return loadErr
			}
			if original.Currency != refund.Currency {
				return fmt.Errorf("finance: refund currency mismatch")
			}
			refundAccount, loadErr := s.ledger.EnsureAccount(ctx, tx, refund.OrganizationID,
				models.OwnerPlatform, refund.OrganizationID, models.AccountPlatformRefund, refund.Currency, models.SideDebit)
			if loadErr != nil {
				return loadErr
			}
			receivable, loadErr := s.ledger.EnsureAccount(ctx, tx, "", models.OwnerPlatform,
				"platform", models.AccountPlatformReceivable, refund.Currency, models.SideDebit)
			if loadErr != nil {
				return loadErr
			}
			journal, postErr := s.ledger.Post(ctx, tx, repositories.Posting{
				OrganizationID: refund.OrganizationID, JournalType: models.JournalRefund,
				Currency: refund.Currency, Reference: outcome.ExternalReference,
				SourceService: "payment-wallet-service", SourceReference: refund.ID,
				IdempotencyKey: refund.IdempotencyKey + ":journal", Fingerprint: refund.RequestFingerprint,
				CausationID: refund.OriginalJournalID, ActorType: "SYSTEM", ActorID: config.Owner,
				Lines: []repositories.PostingLine{{LedgerAccountID: refundAccount.ID, Side: models.SideDebit, AmountMinor: refund.AmountMinor, Memo: "refund"},
					{LedgerAccountID: receivable.ID, Side: models.SideCredit, AmountMinor: refund.AmountMinor, Memo: "refund paid"}},
			})
			if postErr != nil {
				return postErr
			}
			if historyErr := repositories.AppendHistory(ctx, tx, refund.OrganizationID, "REFUND", refund.ID,
				refund.Status, models.RefundCompleted, "SYSTEM", config.Owner, "refund confirmed", nil, refund.CorrelationID, refund.OriginalJournalID); historyErr != nil {
				return historyErr
			}
			if outcomeErr := s.operations.SetRefundOutcome(ctx, tx, refund.ID, config.Owner, models.RefundCompleted, outcome.ExternalReference, journal.ID, next, "", config.MaxAttempts); outcomeErr != nil {
				return outcomeErr
			}
			if auditErr := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: refund.OrganizationID, ActorType: "SYSTEM", ActorID: config.Owner, Action: "refund.completed", ResourceType: "refund", ResourceID: refund.ID, PreviousValues: map[string]any{"status": refund.Status}, NewValues: map[string]any{"status": models.RefundCompleted, "journalId": journal.ID}, CorrelationID: refund.CorrelationID}); auditErr != nil {
				return auditErr
			}
			return s.events.Publish(ctx, tx, EventRefundChanged, "REFUND", refund.ID, refund.Version+1, refund.OrganizationID, refund.CorrelationID, refund.OriginalJournalID, map[string]any{"refundId": refund.ID, "status": models.RefundCompleted, "amountMinor": refund.AmountMinor, "currency": refund.Currency, "journalId": journal.ID})
		})
		if err != nil {
			outcome.Kind = OutcomeRetryable
			outcome.SafeErrorCode = "posting_failed"
		}
		if outcome.Kind == OutcomeSucceeded {
			return nil
		}
		fallthrough
	case OutcomeRetryable:
		return s.recordRefundOutcome(ctx, config, refund, models.RefundFailed, outcome, next, config.MaxAttempts)
	case OutcomePermanent:
		return s.recordRefundOutcome(ctx, config, refund, models.RefundFailed, outcome, next, 1)
	default:
		return s.recordRefundOutcome(ctx, config, refund, models.RefundUnknown, outcome, next, config.MaxAttempts)
	}
}

func (s *Service) recordRefundOutcome(ctx context.Context, config WorkerConfig, refund models.Refund, status string, outcome ExternalOutcome, next time.Time, maxAttempts int) error {
	return database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.operations.SetRefundOutcome(ctx, tx, refund.ID, config.Owner, status, outcome.ExternalReference, "", next, outcome.SafeErrorCode, maxAttempts); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, refund.OrganizationID, "REFUND", refund.ID, refund.Status, status, "SYSTEM", config.Owner, outcome.SafeErrorCode, nil, refund.CorrelationID, refund.OriginalJournalID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: refund.OrganizationID, ActorType: "SYSTEM", ActorID: config.Owner, Action: "refund.status_changed", ResourceType: "refund", ResourceID: refund.ID, PreviousValues: map[string]any{"status": refund.Status}, NewValues: map[string]any{"status": status, "safeErrorCode": outcome.SafeErrorCode}, CorrelationID: refund.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventRefundChanged, "REFUND", refund.ID, refund.Version+1, refund.OrganizationID, refund.CorrelationID, refund.OriginalJournalID, map[string]any{"refundId": refund.ID, "status": status, "safeErrorCode": outcome.SafeErrorCode})
	})
}

func backoff(base time.Duration, attempts int) time.Duration {
	if attempts < 0 {
		attempts = 0
	}
	if attempts > 8 {
		attempts = 8
	}
	value := base * time.Duration(1<<attempts)
	if value > 5*time.Minute {
		return 5 * time.Minute
	}
	return value
}

// RunMaintenanceWorkers processes invoice/subscription transitions, expiring
// holds, reconciliation and nonce retention under a recoverable database lease.
func (s *Service) RunMaintenanceWorkers(ctx context.Context, config WorkerConfig) error {
	config = config.normalized()
	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()
	for {
		_ = s.runMaintenancePass(ctx, config)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Service) runMaintenancePass(ctx context.Context, config WorkerConfig) error {
	acquired, err := repositories.AcquireWorkerLease(ctx, s.pool, "finance-maintenance", config.Owner, config.Lease)
	if err != nil || !acquired {
		return err
	}
	defer func() {
		_ = repositories.ReleaseWorkerLease(context.Background(), s.pool, "finance-maintenance", config.Owner)
	}()
	now := time.Now().UTC()
	holds, err := s.holds.ExpiredHolds(ctx, nil, now, config.BatchSize)
	if err != nil {
		return err
	}
	for _, hold := range holds {
		meta := Meta{Actor: financeauth.Actor{Type: financeauth.ActorService, ID: "payment-wallet-service", OrganizationID: hold.OrganizationID}, IdempotencyKey: "expire:" + hold.ID + fmt.Sprintf(":%d", hold.Version), CausationID: hold.ID}
		if _, releaseErr := s.ReleaseHold(ctx, meta, hold.OrganizationID, hold.ID, hold.Version, models.HoldExpired); releaseErr != nil && !errors.Is(releaseErr, repositories.ErrVersionMismatch) {
			return releaseErr
		}
	}
	due, err := s.billing.DueSubscriptions(ctx, nil, now, config.BatchSize)
	if err != nil {
		return err
	}
	for i := range due {
		if err := s.processDueSubscription(ctx, due[i], now); err != nil && !errors.Is(err, repositories.ErrVersionMismatch) {
			return err
		}
	}
	overdue, err := s.billing.OverdueInvoices(ctx, nil, now, config.BatchSize)
	if err != nil {
		return err
	}
	for i := range overdue {
		invoice := overdue[i]
		err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
			if err := s.billing.SetInvoiceStatus(ctx, tx, invoice.ID, invoice.Version, models.InvoiceExpired); err != nil {
				return err
			}
			return repositories.AppendHistory(ctx, tx, invoice.OrganizationID, "INVOICE", invoice.ID, invoice.Status, models.InvoiceExpired, "SYSTEM", config.Owner, "due date elapsed", nil, "", "")
		})
		if err != nil && !errors.Is(err, repositories.ErrVersionMismatch) {
			return err
		}
	}
	_, _ = repositories.DeleteExpiredServiceNonces(ctx, s.pool, 1000)
	_, err = s.Reconcile(ctx, Meta{Actor: financeauth.Actor{Type: financeauth.ActorService, ID: "payment-wallet-service"}, IdempotencyKey: "maintenance-reconcile"}, "MAINTENANCE")
	return err
}

func (s *Service) processDueSubscription(ctx context.Context, sub models.Subscription, now time.Time) error {
	return database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		plan, err := s.billing.GetPlan(ctx, tx, sub.PlanID)
		if err != nil {
			return err
		}
		if !plan.Billable(now) {
			return s.billing.TransitionSubscription(ctx, tx, sub.ID, sub.Version, models.SubscriptionSuspended, nil, nil, nil, "")
		}
		number, err := s.billing.NextInvoiceNumber(ctx, tx, "INV-"+now.Format("200601"))
		if err != nil {
			return err
		}
		periodEnd := plan.PeriodEnd(now)
		invoice := &models.Invoice{OrganizationID: sub.OrganizationID, SubscriptionID: sub.ID, Number: number, Currency: plan.Currency, SubtotalMinor: *plan.PriceMinor, TotalMinor: *plan.PriceMinor, Status: models.InvoiceOpen, PeriodStart: &now, PeriodEnd: &periodEnd, DueAt: now.AddDate(0, 0, max(1, plan.GraceDays)), IdempotencyKey: "renewal:" + sub.ID + ":" + now.Format("20060102"), RequestFingerprint: fingerprint(sub.ID, sub.Version, plan.ID, plan.Version), CorrelationID: "billing-worker"}
		line := models.InvoiceLine{Description: plan.Name + " renewal", Quantity: 1, UnitAmountMinor: *plan.PriceMinor, AmountMinor: *plan.PriceMinor, Currency: plan.Currency}
		if err := s.billing.CreateInvoice(ctx, tx, invoice, []models.InvoiceLine{line}); err != nil {
			if errors.Is(err, repositories.ErrConflict) {
				return nil
			}
			return err
		}
		status := models.SubscriptionPastDue
		var grace *time.Time
		if plan.GraceDays > 0 {
			status = models.SubscriptionGracePeriod
			value := now.AddDate(0, 0, plan.GraceDays)
			grace = &value
		}
		if err := s.billing.TransitionSubscription(ctx, tx, sub.ID, sub.Version, status, nil, nil, grace, invoice.ID); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, sub.OrganizationID, "SUBSCRIPTION", sub.ID, sub.Status, status, "SYSTEM", "payment-wallet-service", "renewal due", map[string]any{"invoiceId": invoice.ID}, "billing-worker", sub.ID); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventInvoiceCreated, "INVOICE", invoice.ID, invoice.Version, sub.OrganizationID, "billing-worker", sub.ID, map[string]any{"invoiceId": invoice.ID, "subscriptionId": sub.ID, "amountMinor": invoice.TotalMinor, "currency": invoice.Currency, "status": invoice.Status, "dueAt": invoice.DueAt})
	})
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
