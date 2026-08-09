package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// WorkerConfig bounds one execution worker.
type WorkerConfig struct {
	Owner       string
	BatchSize   int
	Interval    time.Duration
	MaxAttempts int
}

func (c WorkerConfig) normalized() WorkerConfig {
	if c.Owner == "" {
		c.Owner = "fleet-companywallet"
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 25
	}
	if c.Interval <= 0 {
		c.Interval = 5 * time.Second
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 8
	}
	return c
}

// RunWorkers executes approved allocations and withdrawals until ctx ends.
//
// It returns on cancellation rather than on error: a pass-level failure must
// not stop recovery for every other row, and each row carries its own durable
// state so the next pass resumes exactly where this one stopped.
func (s *Service) RunWorkers(ctx context.Context, config WorkerConfig) error {
	config = config.normalized()
	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()
	for {
		if err := s.RunPass(ctx, config); err != nil && ctx.Err() == nil {
			s.count("fleet_partner_wallet_worker_pass_failures_total", nil)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// RunPass executes one bounded pass, for tests and operator recovery.
func (s *Service) RunPass(ctx context.Context, config WorkerConfig) error {
	config = config.normalized()
	if s.KillSwitchEngaged() || !s.walletConfigured() {
		return platformclients.ErrNotConfigured
	}
	if s.opts.Flags.AllocationEnabled {
		instructions, err := s.opts.Repository.ClaimAllocations(ctx, config.BatchSize)
		if err != nil {
			return err
		}
		for _, instruction := range instructions {
			if err := s.executeAllocation(ctx, config, instruction); err != nil {
				return err
			}
		}
	}
	if s.opts.Flags.WithdrawalEnabled {
		requests, err := s.opts.Repository.ClaimWithdrawals(ctx, config.BatchSize)
		if err != nil {
			return err
		}
		for _, request := range requests {
			if err := s.executeWithdrawal(ctx, config, request); err != nil {
				return err
			}
		}
	}
	return nil
}

// executeAllocation submits one instruction to Wallet, or reads it back.
//
// An instruction that is UNKNOWN or RECONCILING is read back rather than
// resubmitted. This is the whole point of the durable instruction: the answer
// to "did the payment happen?" is obtained by asking, never by trying again
// and hoping the second attempt is the only one.
func (s *Service) executeAllocation(ctx context.Context, config WorkerConfig,
	instruction models.AllocationInstruction) error {
	if instruction.Status == models.AllocationPendingApproval ||
		models.TerminalAllocationStatus(instruction.Status) {
		return nil
	}
	meta := platformclients.CallMeta{OrganizationID: instruction.OrganizationID,
		CorrelationID: instruction.CorrelationID, CausationID: instruction.CausationID,
		IdempotencyKey: instruction.IdempotencyKey}

	var result platformclients.WalletOperation
	var callErr error
	readBack := instruction.Status == models.AllocationUnknown ||
		instruction.Status == models.AllocationReconciling ||
		instruction.WalletOperationID != ""
	if readBack {
		result, callErr = s.opts.Wallet.ReadOperationByKey(ctx, meta, instruction.OrganizationID,
			"PROVIDER_ALLOCATION", instruction.IdempotencyKey)
	} else {
		payload, _ := json.Marshal(map[string]any{
			"earningAccountId":         instruction.ProviderSubWalletID,
			"organizationMembershipId": instruction.ProviderMembershipID,
			"providerId":               instruction.ProviderID,
			"allocationType":           instruction.AllocationType,
			"businessReferenceType":    instruction.BusinessReferenceType,
			"businessReferenceId":      instruction.BusinessReferenceID,
			"amountMinor":              instruction.AmountMinor,
			"currency":                 instruction.Currency,
			"reasonCode":               instruction.ReasonCode,
			"providerExplanation":      instruction.ProviderExplanation,
			"approvalReference":        instruction.ApprovalRequestID,
			"sourceService":            "fleet-partner-service",
			"sourceReference":          instruction.ID,
		})
		result, callErr = s.opts.Wallet.SubmitOperation(ctx, meta,
			platformclients.WalletOperationRequest{OperationType: "PROVIDER_ALLOCATION",
				OrganizationID: instruction.OrganizationID, SubjectID: instruction.ProviderSubWalletID,
				Payload: payload, PayloadVersion: 1, ExpectedVersion: instruction.ExpectedVersion,
				IdempotencyKey: instruction.IdempotencyKey})
	}

	status, code := classifyOutcome(callErr, models.AllocationCompleted,
		models.AllocationUnknown, models.AllocationFailedRetryable, models.AllocationFailedPermanent)
	if status == models.AllocationFailedRetryable && instruction.Version >= int64(config.MaxAttempts) {
		status = models.AllocationDeadLetter
	}

	t := tenant.Context{OrganizationID: instruction.OrganizationID, ActorType: tenant.ActorTypeSystem,
		ActorID: config.Owner, CorrelationID: instruction.CorrelationID}
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if writeErr := s.opts.Repository.SetAllocationStatus(ctx, tx, instruction.OrganizationID,
			instruction.ID, instruction.Version, status, result.ID, result.ID, code,
			config.Owner); writeErr != nil {
			return writeErr
		}
		if writeErr := s.opts.Repository.AppendAllocationHistory(ctx, tx,
			instruction.OrganizationID, instruction.ID, instruction.Status, status, code,
			"SYSTEM", config.Owner, instruction.CorrelationID); writeErr != nil {
			return writeErr
		}
		if writeErr := s.audit.Record(ctx, tx, audit.Entry{
			OrganizationID: instruction.OrganizationID, ActorType: "SYSTEM", ActorID: config.Owner,
			Action: "wallet.provider_allocation.status_changed", ResourceType: "provider_allocation",
			ResourceID:     instruction.ID,
			PreviousValues: map[string]any{"status": instruction.Status},
			NewValues:      map[string]any{"status": status, "safeErrorCode": code},
			CorrelationID:  instruction.CorrelationID}); writeErr != nil {
			return writeErr
		}
		event := "organization.provider_allocation.confirmed.v1"
		if status != models.AllocationCompleted {
			event = "organization.provider_allocation.failed.v1"
		}
		return s.publish(ctx, tx, event, "PROVIDER_ALLOCATION", instruction.ID,
			instruction.Version+1, t, map[string]any{"allocationId": instruction.ID,
				"membershipId": instruction.ProviderMembershipID, "status": status,
				"safeErrorCode": code, "walletTransactionReference": result.ID})
	})
	if err != nil {
		return err
	}
	s.count("fleet_partner_provider_allocation_outcomes_total",
		map[string]string{"status": status})
	if status == models.AllocationUnknown {
		s.count("fleet_partner_wallet_unknown_outcomes_total", map[string]string{"operation": "allocation"})
	}
	return nil
}

// executeWithdrawal submits one approved withdrawal to Wallet, or reads it back.
func (s *Service) executeWithdrawal(ctx context.Context, config WorkerConfig,
	request models.WithdrawalRequest) error {
	if request.Status == models.WithdrawalPendingApproval {
		return nil
	}
	meta := platformclients.CallMeta{OrganizationID: request.OrganizationID,
		CorrelationID: request.CorrelationID, CausationID: request.CausationID,
		IdempotencyKey: request.IdempotencyKey}

	var result platformclients.WalletOperation
	var callErr error
	readBack := request.Status == models.WithdrawalUnknown ||
		request.Status == models.WithdrawalReconciling || request.WalletOperationID != ""
	if readBack {
		result, callErr = s.opts.Wallet.ReadOperationByKey(ctx, meta, request.OrganizationID,
			"PROVIDER_WITHDRAWAL", request.IdempotencyKey)
	} else {
		payload, _ := json.Marshal(map[string]any{
			"earningAccountId":         request.ProviderSubWalletID,
			"organizationMembershipId": request.ProviderMembershipID,
			"providerId":               request.ProviderID,
			"amountMinor":              request.AmountMinor,
			"currency":                 request.Currency,
			"destinationReference":     request.DestinationReference,
			"policyMode":               request.PolicyMode,
			"requiresApproval":         false,
		})
		result, callErr = s.opts.Wallet.SubmitOperation(ctx, meta,
			platformclients.WalletOperationRequest{OperationType: "PROVIDER_WITHDRAWAL",
				OrganizationID: request.OrganizationID, SubjectID: request.ProviderSubWalletID,
				Payload: payload, PayloadVersion: 1, ExpectedVersion: request.ExpectedVersion,
				IdempotencyKey: request.IdempotencyKey})
	}

	status, code := classifyOutcome(callErr, models.WithdrawalCompleted,
		models.WithdrawalUnknown, models.WithdrawalFailedRetryable, models.WithdrawalFailedPermanent)
	if status == models.WithdrawalFailedRetryable && request.Version >= int64(config.MaxAttempts) {
		status = models.WithdrawalDeadLetter
	}

	t := tenant.Context{OrganizationID: request.OrganizationID, ActorType: tenant.ActorTypeSystem,
		ActorID: config.Owner, CorrelationID: request.CorrelationID}
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if writeErr := s.opts.Repository.SetWithdrawalStatus(ctx, tx, request.OrganizationID,
			request.ID, request.Version, status, result.ID, result.ID, code); writeErr != nil {
			return writeErr
		}
		if writeErr := s.opts.Repository.AppendWithdrawalHistory(ctx, tx, request.OrganizationID,
			request.ID, request.Status, status, code, "SYSTEM", config.Owner,
			request.CorrelationID); writeErr != nil {
			return writeErr
		}
		if writeErr := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: request.OrganizationID,
			ActorType: "SYSTEM", ActorID: config.Owner,
			Action:       "wallet.provider_withdrawal.status_changed",
			ResourceType: "provider_withdrawal", ResourceID: request.ID,
			PreviousValues: map[string]any{"status": request.Status},
			NewValues:      map[string]any{"status": status, "safeErrorCode": code},
			CorrelationID:  request.CorrelationID}); writeErr != nil {
			return writeErr
		}
		return s.publish(ctx, tx, "organization.provider_withdrawal.status_changed.v1",
			"PROVIDER_WITHDRAWAL", request.ID, request.Version+1, t,
			map[string]any{"withdrawalId": request.ID,
				"membershipId": request.ProviderMembershipID, "status": status,
				"safeErrorCode": code})
	})
	if err != nil {
		return err
	}
	s.count("fleet_partner_provider_withdrawal_outcomes_total", map[string]string{"status": status})
	if status == models.WithdrawalUnknown {
		s.count("fleet_partner_wallet_unknown_outcomes_total", map[string]string{"operation": "withdrawal"})
	}
	return nil
}

// classifyOutcome maps a client error onto a durable status.
//
// A timeout is UNKNOWN, never a failure: the operation may well have happened,
// and treating "we did not hear back" as "it did not happen" is precisely how
// a retry becomes a second payment.
func classifyOutcome(err error, succeeded, unknown, retryable, permanent string) (string, string) {
	if err == nil {
		return succeeded, ""
	}
	var clientErr *platformclients.Error
	if errors.As(err, &clientErr) {
		switch clientErr.Outcome {
		case platformclients.OutcomeUnknown:
			return unknown, "wallet_outcome_unknown"
		case platformclients.OutcomeValidation, platformclients.OutcomeForbidden,
			platformclients.OutcomePermanent:
			return permanent, "wallet_rejected"
		case platformclients.OutcomeNotFound:
			// A read-back that finds nothing means the submission never landed.
			// It is retryable, not permanent: nothing was created to duplicate.
			return retryable, "wallet_operation_not_found"
		}
	}
	return retryable, "wallet_unavailable"
}

// RefreshProjections re-reads wallet and sub-wallet links that have gone stale.
//
// It only ever downgrades trust or restores it from an authoritative answer.
// It never invents a value, and a link it cannot confirm stays not-current so
// the gates keep refusing.
func (s *Service) RefreshProjections(ctx context.Context, config WorkerConfig, limit int) (int, error) {
	if !s.opts.Flags.ProjectionRefreshEnabled || s.KillSwitchEngaged() || !s.walletConfigured() {
		return 0, platformclients.ErrNotConfigured
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.opts.Pool.Query(ctx, `
		SELECT organization_id, COALESCE(wallet_account_id, ''), COALESCE(currency, '')
		FROM fleet_partner_organization_wallet_provisioning
		WHERE wallet_account_id IS NOT NULL
		  AND (freshness_state <> 'CURRENT' OR synced_at IS NULL OR synced_at < $1)
		ORDER BY synced_at NULLS FIRST LIMIT $2`, s.now().Add(-s.opts.StaleAfter), limit)
	if err != nil {
		return 0, err
	}
	type target struct{ organizationID, accountID, currency string }
	var targets []target
	for rows.Next() {
		var value target
		if scanErr := rows.Scan(&value.organizationID, &value.accountID, &value.currency); scanErr != nil {
			rows.Close()
			return 0, scanErr
		}
		targets = append(targets, value)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	refreshed := 0
	for _, value := range targets {
		if value.accountID == "" {
			continue
		}
		account, readErr := s.opts.Wallet.ReadAccount(ctx,
			platformclients.CallMeta{OrganizationID: value.organizationID}, value.accountID)
		if readErr != nil {
			s.count("fleet_partner_wallet_projection_staleness_total",
				map[string]string{"outcome": "unavailable"})
			continue
		}
		if writeErr := s.opts.Repository.UpsertOrganizationLink(ctx, nil, value.organizationID,
			account.AccountID, linkStatusForAccount(account.Status), value.currency,
			models.FreshnessCurrent, "", account.SourceVersion); writeErr != nil {
			continue
		}
		refreshed++
	}
	s.count("fleet_partner_wallet_projection_refresh_total",
		map[string]string{"count": strconv.Itoa(refreshed)})
	return refreshed, nil
}
