package usecases

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/repositories"
	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Requesting actor types for a withdrawal.
const (
	RequesterProvider = "PROVIDER"
	RequesterStaff    = "ORGANIZATION_USER"
)

// WithdrawalInput is a request to move value out of a provider sub-wallet.
//
// MembershipID identifies whose sub-wallet. For a provider-authenticated
// request the caller must have resolved the membership from the provider's own
// session; a membership taken from request input would let anyone name someone
// else's account.
type WithdrawalInput struct {
	Tenant               tenant.Context
	BranchID             string
	MembershipID         string
	RequestedByActorType string
	AmountMinor          int64
	Currency             string
	DestinationReference string
	IdempotencyKey       string
	ExpectedVersion      int64
}

// RequestWithdrawal validates policy, gates and durably records a withdrawal.
func (s *Service) RequestWithdrawal(ctx context.Context, in WithdrawalInput) (models.WithdrawalRequest, error) {
	var zero models.WithdrawalRequest
	if err := s.writesEnabled(); err != nil {
		return zero, err
	}
	if !s.opts.Flags.WithdrawalEnabled {
		return zero, notFound("Provider sub-wallet")
	}
	if err := requireText(in.IdempotencyKey, "Idempotency-Key"); err != nil {
		return zero, err
	}
	if err := requireText(in.DestinationReference, "destinationBankAccountReference"); err != nil {
		return zero, err
	}
	if in.AmountMinor <= 0 {
		return zero, apperrors.BadRequest("amountMinor must be greater than zero.", nil)
	}
	currency, err := validCurrency(in.Currency)
	if err != nil {
		return zero, err
	}
	requesterType := upper(in.RequestedByActorType)
	if requesterType != RequesterProvider && requesterType != RequesterStaff {
		requesterType = RequesterStaff
	}
	if !in.Tenant.BranchPermitted(in.BranchID) {
		return zero, notFound("Branch")
	}

	policy, err := s.opts.Repository.GetPolicy(ctx, nil, in.Tenant.OrganizationID)
	if err != nil {
		return zero, err
	}
	// Organization policy narrows what may happen. It never widens it: the
	// Wallet-side compliance, verification and account-status rules are applied
	// independently and are not consulted here at all.
	if upper(policy.PolicyMode) == models.PolicyDisabled {
		return zero, errorcodes.Conflict(errorcodes.ProviderWithdrawalDisabled,
			"Withdrawals are disabled for this organization.")
	}
	if requesterType == RequesterProvider && !policy.AllowsProviderRequest() {
		return zero, errorcodes.Conflict(errorcodes.ProviderWithdrawalDisabled,
			"Providers may not open withdrawals under the current policy.")
	}
	if policy.MinimumAmountMinor > 0 && in.AmountMinor < policy.MinimumAmountMinor {
		return zero, errorcodes.UnprocessableEntity(errorcodes.ProviderWithdrawalDisabled,
			"The requested amount is below the organization minimum.")
	}

	if err := s.requireEligible(ctx, in.Tenant, in.BranchID,
		eligibilitymodels.OperationFinancialRelease); err != nil {
		return zero, err
	}
	if _, err := s.requireOperationalWallet(ctx, in.Tenant); err != nil {
		return zero, err
	}
	subWallet, err := s.requireOperationalSubWallet(ctx, in.Tenant, in.MembershipID)
	if err != nil {
		return zero, err
	}
	if subWallet.Currency != "" && subWallet.Currency != currency {
		return zero, errorcodes.Conflict(errorcodes.WalletOperationConflict,
			"The withdrawal currency does not match the provider sub-wallet currency.")
	}
	if in.ExpectedVersion != 0 && in.ExpectedVersion != subWallet.SourceVersion {
		return zero, errorcodes.Conflict(errorcodes.WalletOperationConflict,
			"The provider sub-wallet changed since it was read.")
	}

	status := models.WithdrawalApproved
	if policy.RequiresApproval(in.AmountMinor) {
		status = models.WithdrawalPendingApproval
	}

	request := models.WithdrawalRequest{
		OrganizationID: in.Tenant.OrganizationID, BranchID: in.BranchID,
		ProviderMembershipID: in.MembershipID, ProviderID: subWallet.ProviderID,
		ProviderSubWalletID: subWallet.WalletAccountID, AmountMinor: in.AmountMinor,
		Currency: currency, DestinationReference: in.DestinationReference,
		PolicyMode: upper(policy.PolicyMode), RequestedByActorType: requesterType,
		RequestedBy: in.Tenant.ActorID, IdempotencyKey: in.IdempotencyKey,
		RequestFingerprint: fingerprint(in.Tenant.OrganizationID, in.MembershipID,
			strconv.FormatInt(in.AmountMinor, 10), currency, in.DestinationReference),
		ExpectedVersion: subWallet.SourceVersion, Status: status,
		FreshnessState: models.FreshnessUnknown, CorrelationID: in.Tenant.CorrelationID,
	}

	created := false
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		var writeErr error
		created, writeErr = s.opts.Repository.CreateWithdrawal(ctx, tx, &request)
		if writeErr != nil {
			return writeErr
		}
		if !created {
			return nil
		}
		if writeErr = s.opts.Repository.AppendWithdrawalHistory(ctx, tx, in.Tenant.OrganizationID,
			request.ID, "", status, "requested", string(in.Tenant.ActorType), in.Tenant.ActorID,
			in.Tenant.CorrelationID); writeErr != nil {
			return writeErr
		}
		if writeErr = s.audit.Record(ctx, tx, audit.Entry{OrganizationID: in.Tenant.OrganizationID,
			BranchID: in.BranchID, ActorType: string(in.Tenant.ActorType), ActorID: in.Tenant.ActorID,
			Action: "wallet.provider_withdrawal.requested", ResourceType: "provider_withdrawal",
			ResourceID: request.ID,
			NewValues: map[string]any{"amountMinor": in.AmountMinor, "currency": currency,
				"policyMode": request.PolicyMode, "status": status},
			RequestID: in.Tenant.RequestID, CorrelationID: in.Tenant.CorrelationID}); writeErr != nil {
			return writeErr
		}
		// The destination reference is not published. It is opaque, but an
		// opaque handle to someone's bank account is still a handle.
		return s.publish(ctx, tx, "organization.provider_withdrawal.requested.v1",
			"PROVIDER_WITHDRAWAL", request.ID, request.Version, in.Tenant,
			map[string]any{"withdrawalId": request.ID, "membershipId": in.MembershipID,
				"amountMinor": in.AmountMinor, "currency": currency, "status": status})
	})
	if err != nil {
		return zero, translateWithdrawalError(err)
	}
	if created {
		s.count("fleet_partner_provider_withdrawals_total", map[string]string{"status": status})
	}

	if created && status == models.WithdrawalPendingApproval && s.opts.Approvals != nil {
		approvalID, approvalErr := s.opts.Approvals.Request(ctx, ApprovalRequestInput{
			OrganizationID:     in.Tenant.OrganizationID,
			OrganizationUserID: in.Tenant.OrganizationUserID, BranchID: in.BranchID,
			ResourceType: "provider_withdrawal", ResourceID: request.ID,
			Action:                 "wallet.provider_withdrawals.approve",
			OperationType:          eligibilitymodels.OperationFinancialRelease,
			Payload:                withdrawalApprovalPayload(request),
			ExpectedSubjectVersion: request.Version,
			IdempotencyKey:         "withdrawal-approval:" + request.ID,
			CorrelationID:          in.Tenant.CorrelationID})
		if approvalErr == nil && approvalID != "" {
			request.ApprovalRequestID = approvalID
		}
	}
	return request, nil
}

// withdrawalApprovalPayload is the exact controlled action an approver signs.
//
// The destination reference IS part of it: approving a payout to one account
// must not authorize the same amount to a different one.
func withdrawalApprovalPayload(request models.WithdrawalRequest) map[string]any {
	return map[string]any{
		"membershipId":         request.ProviderMembershipID,
		"amountMinor":          request.AmountMinor,
		"currency":             request.Currency,
		"destinationReference": request.DestinationReference,
	}
}

func translateWithdrawalError(err error) error {
	switch {
	case errors.Is(err, repositories.ErrFingerprint):
		return errorcodes.Conflict(errorcodes.WalletOperationConflict,
			"This idempotency key was already used with different values.")
	case errors.Is(err, repositories.ErrDuplicate):
		return errorcodes.Conflict(errorcodes.ProviderWithdrawalAlreadyResolved,
			"An open withdrawal already exists for this provider.")
	default:
		return err
	}
}

// DecideWithdrawal approves or rejects a pending withdrawal.
func (s *Service) DecideWithdrawal(ctx context.Context, t tenant.Context, membershipID, id string,
	expectedVersion int64, approve bool, reasonCode string) (models.WithdrawalRequest, error) {
	var result models.WithdrawalRequest
	if err := s.writesEnabled(); err != nil {
		return result, err
	}
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		request, readErr := s.opts.Repository.LockWithdrawal(ctx, tx, t.OrganizationID, id)
		if readErr != nil {
			if errors.Is(readErr, repositories.ErrNotFound) {
				return errorcodes.NotFound(errorcodes.ProviderWithdrawalNotFound,
					"The withdrawal was not found.")
			}
			return readErr
		}
		if membershipID != "" && request.ProviderMembershipID != membershipID {
			return errorcodes.NotFound(errorcodes.ProviderWithdrawalNotFound,
				"The withdrawal was not found.")
		}
		if !models.DecidableWithdrawal(request.Status) {
			return errorcodes.Conflict(errorcodes.ProviderWithdrawalAlreadyResolved,
				"This withdrawal has already been resolved.")
		}
		if expectedVersion != 0 && expectedVersion != request.Version {
			return errorcodes.Conflict(errorcodes.WalletOperationConflict,
				"The withdrawal changed since it was read.")
		}
		if approve && request.ApprovalRequestID != "" && s.opts.Approvals != nil {
			if approvalErr := s.opts.Approvals.RequireApproved(ctx, t.OrganizationID,
				request.ApprovalRequestID, withdrawalApprovalPayload(request)); approvalErr != nil {
				return approvalErr
			}
		}
		// A provider cannot approve their own withdrawal, and neither can the
		// staff member who opened it on their behalf.
		if approve && request.RequestedBy == t.ActorID {
			return errorcodes.Forbidden(errorcodes.ApprovalSelfDecisionForbidden,
				"You cannot approve a withdrawal you requested.")
		}
		next := models.WithdrawalApproved
		event := "organization.provider_withdrawal.approved.v1"
		if !approve {
			next = models.WithdrawalRejected
			event = "organization.provider_withdrawal.rejected.v1"
		}
		if writeErr := s.opts.Repository.DecideWithdrawal(ctx, tx, t.OrganizationID, id,
			request.Version, next, t.ActorID, reasonCode, request.ApprovalRequestID); writeErr != nil {
			return writeErr
		}
		if writeErr := s.opts.Repository.AppendWithdrawalHistory(ctx, tx, t.OrganizationID, id,
			request.Status, next, reasonCode, string(t.ActorType), t.ActorID,
			t.CorrelationID); writeErr != nil {
			return writeErr
		}
		if writeErr := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: t.OrganizationID,
			ActorType: string(t.ActorType), ActorID: t.ActorID,
			Action: "wallet.provider_withdrawal.decided", ResourceType: "provider_withdrawal",
			ResourceID: id, PreviousValues: map[string]any{"status": request.Status},
			NewValues: map[string]any{"status": next, "reasonCode": reasonCode},
			Reason:    reasonCode, RequestID: t.RequestID, CorrelationID: t.CorrelationID}); writeErr != nil {
			return writeErr
		}
		request.Status = next
		request.Version++
		result = request
		return s.publish(ctx, tx, event, "PROVIDER_WITHDRAWAL", id, request.Version, t,
			map[string]any{"withdrawalId": id, "membershipId": request.ProviderMembershipID,
				"status": next, "reasonCode": reasonCode})
	})
	if errors.Is(err, repositories.ErrVersionConflict) {
		return result, errorcodes.Conflict(errorcodes.ProviderWithdrawalAlreadyResolved,
			"This withdrawal has already been resolved.")
	}
	return result, err
}

// GetWithdrawal reads one request inside the caller's scope.
func (s *Service) GetWithdrawal(ctx context.Context, t tenant.Context,
	id string) (models.WithdrawalRequest, error) {
	request, err := s.opts.Repository.GetWithdrawal(ctx, nil, t.OrganizationID, id,
		t.PermittedBranchIDs)
	if errors.Is(err, repositories.ErrNotFound) {
		return request, errorcodes.NotFound(errorcodes.ProviderWithdrawalNotFound,
			"The withdrawal was not found.")
	}
	return request, err
}

// ListWithdrawals returns a tenant-scoped page.
func (s *Service) ListWithdrawals(ctx context.Context, t tenant.Context,
	filter repositories.WithdrawalFilter) ([]models.WithdrawalRequest, error) {
	return s.opts.Repository.ListWithdrawals(ctx, t.OrganizationID, t.PermittedBranchIDs, filter)
}

// ReplayWithdrawal returns a dead-lettered request to the execution queue.
func (s *Service) ReplayWithdrawal(ctx context.Context, t tenant.Context, id string) error {
	if err := s.writesEnabled(); err != nil {
		return err
	}
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Repository.ReplayWithdrawal(ctx, tx, t.OrganizationID, id); err != nil {
			if errors.Is(err, repositories.ErrNotFound) {
				return errorcodes.NotFound(errorcodes.ProviderWithdrawalNotFound,
					"The withdrawal was not found.")
			}
			return err
		}
		return s.audit.Record(ctx, tx, audit.Entry{OrganizationID: t.OrganizationID,
			ActorType: string(t.ActorType), ActorID: t.ActorID,
			Action: "wallet.provider_withdrawal.replayed", ResourceType: "provider_withdrawal",
			ResourceID: id, RequestID: t.RequestID, CorrelationID: t.CorrelationID})
	})
}
