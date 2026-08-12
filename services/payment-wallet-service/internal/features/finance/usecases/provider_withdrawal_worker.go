package usecases

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/repositories"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/audit"
)

// ProviderPayoutGateway is the external payout boundary for provider
// withdrawals.
//
// It is a separate interface from MoneyGateway rather than more methods on it,
// so an existing gateway implementation keeps compiling and a deployment that
// has not yet wired provider payouts fails closed — the worker refuses to run
// — instead of appearing to run against a gateway that cannot pay anyone.
type ProviderPayoutGateway interface {
	ExecuteProviderWithdrawal(context.Context, models.ProviderWithdrawal) (ExternalOutcome, error)
	ReadProviderWithdrawal(context.Context, models.ProviderWithdrawal) (ExternalOutcome, error)
	Configured() bool
}

// RunProviderWithdrawalWorker executes approved withdrawals until ctx ends.
func (s *Service) RunProviderWithdrawalWorker(ctx context.Context, gateway ProviderPayoutGateway,
	config WorkerConfig) error {
	if gateway == nil || !gateway.Configured() {
		return ErrWithdrawalGatewayUnavailable
	}
	config = config.normalized()
	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()
	for {
		if err := s.runProviderWithdrawalPass(ctx, gateway, config); err != nil && ctx.Err() == nil {
			// Row-level state is durable; a pass-level error must not stop
			// recovery for every other withdrawal.
			_ = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// RunProviderWithdrawalPass executes one bounded pass, for tests and for
// deterministic operator recovery.
func (s *Service) RunProviderWithdrawalPass(ctx context.Context, gateway ProviderPayoutGateway,
	config WorkerConfig) error {
	if gateway == nil || !gateway.Configured() {
		return ErrWithdrawalGatewayUnavailable
	}
	return s.runProviderWithdrawalPass(ctx, gateway, config.normalized())
}

func (s *Service) runProviderWithdrawalPass(ctx context.Context, gateway ProviderPayoutGateway,
	config WorkerConfig) error {
	_, _ = s.providerOps.RecoverWithdrawalLeases(ctx)
	claimed, err := s.providerOps.ClaimWithdrawals(ctx, config.Owner, config.Lease, config.BatchSize)
	if err != nil {
		return err
	}
	for i := range claimed {
		if err := s.processProviderWithdrawal(ctx, gateway, config, claimed[i]); err != nil {
			return err
		}
	}
	return nil
}

// processProviderWithdrawal runs one attempt.
//
// A withdrawal that already carries an external reference, or that comes back
// from a recovered lease, is READ BACK rather than executed again. Re-issuing
// a payout because the first answer was lost is how a provider gets paid
// twice, and the provider is not the party who would notice.
func (s *Service) processProviderWithdrawal(ctx context.Context, gateway ProviderPayoutGateway,
	config WorkerConfig, withdrawal models.ProviderWithdrawal) error {
	var outcome ExternalOutcome
	var err error
	if withdrawal.ExternalReference != "" || withdrawal.SafeErrorCode == "lease_expired" ||
		withdrawal.Attempts > 0 {
		outcome, err = gateway.ReadProviderWithdrawal(ctx, withdrawal)
	} else {
		outcome, err = gateway.ExecuteProviderWithdrawal(ctx, withdrawal)
	}
	if err != nil && outcome.Kind == "" {
		outcome = ExternalOutcome{Kind: OutcomeUnknown, SafeErrorCode: "gateway_unavailable"}
	}
	next := time.Now().UTC().Add(backoff(config.BaseBackoff, withdrawal.Attempts))

	switch outcome.Kind {
	case OutcomeSucceeded:
		return s.completeProviderWithdrawal(ctx, config, withdrawal, outcome, next)
	case OutcomePermanent:
		return s.failProviderWithdrawal(ctx, config, withdrawal, outcome, next,
			models.WithdrawalFailedPermanent, true)
	default:
		// UNKNOWN and RETRYABLE both stay retryable: the next pass reads back.
		// Reserved funds stay reserved, because an unknown payout may well have
		// happened and releasing them would allow the same money to leave twice.
		return s.failProviderWithdrawal(ctx, config, withdrawal, outcome, next,
			models.WithdrawalFailedRetryable, false)
	}
}

func (s *Service) completeProviderWithdrawal(ctx context.Context, config WorkerConfig,
	withdrawal models.ProviderWithdrawal, outcome ExternalOutcome, next time.Time) error {
	return database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		account, err := s.accounts.LockEarningAccount(ctx, tx, withdrawal.EarningAccountID)
		if err != nil {
			return err
		}
		clearing, err := s.ledger.EnsureAccount(ctx, tx, "", models.OwnerPlatform, "platform",
			models.AccountPlatformClearing, withdrawal.Currency, models.SideCredit)
		if err != nil {
			return err
		}
		journal, err := s.ledger.Post(ctx, tx, repositories.Posting{
			OrganizationID: withdrawal.OrganizationID,
			JournalType:    models.JournalProviderWithdrawalPayout,
			Currency:       withdrawal.Currency, Reference: outcome.ExternalReference,
			SourceService: "payment-wallet-service", SourceReference: withdrawal.ID,
			IdempotencyKey: withdrawal.IdempotencyKey + ":payout",
			Fingerprint:    withdrawal.RequestFingerprint,
			ActorType:      "SYSTEM", ActorID: config.Owner,
			Lines: []repositories.PostingLine{
				{LedgerAccountID: account.LedgerAccountID, Side: models.SideDebit,
					AmountMinor: withdrawal.AmountMinor, Memo: "provider withdrawal payout"},
				{LedgerAccountID: clearing.ID, Side: models.SideCredit,
					AmountMinor: withdrawal.AmountMinor, Memo: "payout clearing"}},
		})
		if err != nil {
			return err
		}
		// Held becomes settled: the reservation made at request time is what is
		// now leaving, so no second deduction from the payable balance happens.
		if err := s.accounts.MoveEarningBalances(ctx, tx, account.ID, account.Version,
			0, -withdrawal.AmountMinor, withdrawal.AmountMinor); err != nil {
			return err
		}
		if err := s.providerOps.SetWithdrawalOutcome(ctx, tx, withdrawal.ID, config.Owner,
			models.WithdrawalCompleted, outcome.ExternalReference, journal.ID, "", next,
			config.MaxAttempts); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, withdrawal.OrganizationID,
			"PROVIDER_WITHDRAWAL", withdrawal.ID, withdrawal.Status, models.WithdrawalCompleted,
			"SYSTEM", config.Owner, "payout confirmed", map[string]any{"journalId": journal.ID},
			withdrawal.CorrelationID, withdrawal.CausationID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: withdrawal.OrganizationID,
			ActorType: "SYSTEM", ActorID: config.Owner, Action: "provider_withdrawal.completed",
			ResourceType: "provider_withdrawal", ResourceID: withdrawal.ID,
			PreviousValues: map[string]any{"status": withdrawal.Status},
			NewValues: map[string]any{"status": models.WithdrawalCompleted,
				"amountMinor": withdrawal.AmountMinor, "currency": withdrawal.Currency},
			CorrelationID: withdrawal.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventWithdrawalCompleted, "PROVIDER_WITHDRAWAL",
			withdrawal.ID, withdrawal.Version+1, withdrawal.OrganizationID,
			withdrawal.CorrelationID, withdrawal.CausationID,
			map[string]any{"withdrawalId": withdrawal.ID,
				"providerSubWalletId": withdrawal.EarningAccountID,
				"amountMinor":         withdrawal.AmountMinor, "currency": withdrawal.Currency,
				"status": models.WithdrawalCompleted})
	})
}

// failProviderWithdrawal records a failed attempt. releaseFunds is true only
// for a permanent failure, where the payout provably did not and will not
// happen; an ambiguous outcome keeps the reservation.
func (s *Service) failProviderWithdrawal(ctx context.Context, config WorkerConfig,
	withdrawal models.ProviderWithdrawal, outcome ExternalOutcome, next time.Time,
	status string, releaseFunds bool) error {
	return database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if releaseFunds {
			account, err := s.accounts.LockEarningAccount(ctx, tx, withdrawal.EarningAccountID)
			if err != nil {
				return err
			}
			if err := s.accounts.MoveEarningBalances(ctx, tx, account.ID, account.Version,
				withdrawal.AmountMinor, -withdrawal.AmountMinor, 0); err != nil {
				return err
			}
		}
		if err := s.providerOps.SetWithdrawalOutcome(ctx, tx, withdrawal.ID, config.Owner,
			status, outcome.ExternalReference, "", outcome.SafeErrorCode, next,
			config.MaxAttempts); err != nil {
			return err
		}
		if err := repositories.AppendHistory(ctx, tx, withdrawal.OrganizationID,
			"PROVIDER_WITHDRAWAL", withdrawal.ID, withdrawal.Status, status, "SYSTEM",
			config.Owner, outcome.SafeErrorCode, nil, withdrawal.CorrelationID,
			withdrawal.CausationID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: withdrawal.OrganizationID,
			ActorType: "SYSTEM", ActorID: config.Owner, Action: "provider_withdrawal.attempt_failed",
			ResourceType: "provider_withdrawal", ResourceID: withdrawal.ID,
			PreviousValues: map[string]any{"status": withdrawal.Status},
			NewValues:      map[string]any{"status": status, "safeErrorCode": outcome.SafeErrorCode},
			CorrelationID:  withdrawal.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, EventWithdrawalFailed, "PROVIDER_WITHDRAWAL",
			withdrawal.ID, withdrawal.Version+1, withdrawal.OrganizationID,
			withdrawal.CorrelationID, withdrawal.CausationID,
			map[string]any{"withdrawalId": withdrawal.ID,
				"providerSubWalletId": withdrawal.EarningAccountID,
				"status":              status, "safeErrorCode": outcome.SafeErrorCode})
	})
}
