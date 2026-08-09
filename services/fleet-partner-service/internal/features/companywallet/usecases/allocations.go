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
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// AllocationInput is a company request to move value to a provider sub-wallet.
type AllocationInput struct {
	Tenant                tenant.Context
	BranchID              string
	MembershipID          string
	AllocationType        string
	AmountMinor           int64
	Currency              string
	BusinessReferenceType string
	BusinessReferenceID   string
	ReasonCode            string
	ProviderExplanation   string
	PrivateNote           string
	LegalBasisReference   string
	IdempotencyKey        string
	ExpectedVersion       int64
	// RequireRecipientConfirmation tightens the completed-work check for
	// per-job allocations where the organization's own policy demands it.
	RequireRecipientConfirmation bool
}

// RequestAllocation validates, gates and durably records a company allocation.
//
// Nothing is submitted to Wallet inside this call. The instruction is written
// with an idempotency key and a fingerprint, and the execution worker submits
// it; that separation is what makes a lost HTTP response harmless, because the
// retry finds the same instruction rather than creating a second payment.
func (s *Service) RequestAllocation(ctx context.Context, in AllocationInput) (models.AllocationInstruction, error) {
	var zero models.AllocationInstruction
	if err := s.writesEnabled(); err != nil {
		return zero, err
	}
	if !s.opts.Flags.AllocationEnabled {
		return zero, notFound("Provider sub-wallet")
	}
	allocationType := upper(in.AllocationType)
	referenceType := upper(in.BusinessReferenceType)
	if !models.ValidAllocationType(allocationType) {
		return zero, apperrors.BadRequest("allocationType is not a supported allocation type.", nil)
	}
	if !models.ValidBusinessReferenceType(referenceType) {
		// Section 15 reference names land here. They are not "not yet
		// supported" to the caller — they are simply not valid, because
		// accepting one would mean moving money against work that does not
		// exist in this system yet.
		return zero, apperrors.BadRequest("businessReferenceType is not a supported reference type.", nil)
	}
	if err := requireText(in.BusinessReferenceID, "businessReferenceId"); err != nil {
		return zero, err
	}
	if err := requireText(in.ReasonCode, "reasonCode"); err != nil {
		return zero, err
	}
	if err := requireText(in.IdempotencyKey, "Idempotency-Key"); err != nil {
		return zero, err
	}
	// The replay lookup comes before every business check. A retry of a request
	// that already succeeded must return the same instruction, not be rejected
	// by the very duplicate rule its own first attempt created.
	if existing, findErr := s.opts.Repository.FindAllocationByKey(ctx, nil,
		in.Tenant.OrganizationID, in.IdempotencyKey); findErr == nil {
		if existing.RequestFingerprint != fingerprint(in.Tenant.OrganizationID, in.MembershipID,
			allocationType, referenceType, in.BusinessReferenceID,
			strconv.FormatInt(in.AmountMinor, 10), upper(in.Currency), in.ReasonCode) {
			return zero, errorcodes.Conflict(errorcodes.WalletOperationConflict,
				"This idempotency key was already used with different values.")
		}
		return existing, nil
	} else if !errors.Is(findErr, repositories.ErrNotFound) {
		return zero, findErr
	}
	if in.AmountMinor <= 0 {
		return zero, apperrors.BadRequest("amountMinor must be greater than zero.", nil)
	}
	currency, err := validCurrency(in.Currency)
	if err != nil {
		return zero, err
	}
	if !in.Tenant.BranchPermitted(in.BranchID) {
		return zero, notFound("Branch")
	}

	// A deduction takes money from someone who is not in the room. It carries
	// a mandatory legal or policy basis and a provider-visible explanation, and
	// it is refused without them regardless of who is asking.
	if models.DebitsProvider(allocationType) {
		if err := requireText(in.ProviderExplanation, "providerVisibleExplanation"); err != nil {
			return zero, errorcodes.BadRequest(errorcodes.ProviderAllocationNotAllowed,
				"A deduction requires a provider visible explanation.")
		}
		if err := requireText(in.LegalBasisReference, "legalBasisReference"); err != nil {
			return zero, errorcodes.BadRequest(errorcodes.ProviderAllocationNotAllowed,
				"A deduction requires a legal, policy or contract basis reference.")
		}
	}

	if err := s.requireEligible(ctx, in.Tenant, in.BranchID,
		eligibilityOperation(allocationType)); err != nil {
		return zero, err
	}

	walletLink, err := s.requireOperationalWallet(ctx, in.Tenant)
	if err != nil {
		return zero, err
	}
	subWallet, err := s.requireOperationalSubWallet(ctx, in.Tenant, in.MembershipID)
	if err != nil {
		return zero, err
	}
	if subWallet.Currency != "" && subWallet.Currency != currency {
		return zero, errorcodes.Conflict(errorcodes.ProviderAllocationNotAllowed,
			"The allocation currency does not match the provider sub-wallet currency.")
	}
	if walletLink.Currency != "" && walletLink.Currency != currency {
		return zero, errorcodes.Conflict(errorcodes.ProviderAllocationNotAllowed,
			"The allocation currency does not match the organization wallet currency.")
	}
	if in.ExpectedVersion != 0 && in.ExpectedVersion != subWallet.SourceVersion {
		return zero, errorcodes.Conflict(errorcodes.WalletOperationConflict,
			"The provider sub-wallet changed since it was read.")
	}

	// Work that failed, was cancelled or never completed cannot fund a per-job
	// allocation. The completion fact belongs to Dispatch; Fleet asks rather
	// than inferring it from its own projection.
	if models.RequiresCompletedWork(allocationType, referenceType) {
		if s.opts.Work == nil {
			return zero, errorcodes.Conflict(errorcodes.ProviderAllocationNotAllowed,
				"Job completion cannot be confirmed right now.")
		}
		if err := s.opts.Work.RequireCompletedDelivery(ctx, in.Tenant.OrganizationID,
			in.BusinessReferenceID, in.RequireRecipientConfirmation); err != nil {
			return zero, errorcodes.Conflict(errorcodes.ProviderAllocationNotAllowed,
				"The referenced job is not confirmed complete.")
		}
	}

	duplicate, err := s.opts.Repository.CountLiveAllocationForPurpose(ctx, nil,
		in.Tenant.OrganizationID, in.MembershipID, referenceType, in.BusinessReferenceID,
		allocationType)
	if err != nil {
		return zero, err
	}
	if duplicate {
		return zero, errorcodes.Conflict(errorcodes.ProviderAllocationDuplicate,
			"An allocation already exists for this business reference.")
	}

	policy, err := s.opts.Repository.GetPolicy(ctx, nil, in.Tenant.OrganizationID)
	if err != nil {
		return zero, err
	}
	needsApproval := policy.RequiresAllocationApproval(in.AmountMinor, allocationType)

	instruction := models.AllocationInstruction{
		OrganizationID: in.Tenant.OrganizationID, BranchID: in.BranchID,
		ProviderMembershipID: in.MembershipID, ProviderID: subWallet.ProviderID,
		ProviderSubWalletID:  subWallet.WalletAccountID,
		OrganizationWalletID: walletLink.WalletAccountID, AllocationType: allocationType,
		AmountMinor: in.AmountMinor, Currency: currency,
		BusinessReferenceType: referenceType, BusinessReferenceID: in.BusinessReferenceID,
		ReasonCode: in.ReasonCode, ProviderExplanation: in.ProviderExplanation,
		PrivateNote: in.PrivateNote, LegalBasisReference: in.LegalBasisReference,
		RequestedBy: in.Tenant.ActorID, IdempotencyKey: in.IdempotencyKey,
		RequestFingerprint: fingerprint(in.Tenant.OrganizationID, in.MembershipID, allocationType,
			referenceType, in.BusinessReferenceID, strconv.FormatInt(in.AmountMinor, 10),
			currency, in.ReasonCode),
		ExpectedVersion: subWallet.SourceVersion, Status: models.AllocationApproved,
		FreshnessState: models.FreshnessUnknown, CorrelationID: in.Tenant.CorrelationID,
		CreatedBy: in.Tenant.ActorID,
	}
	if needsApproval {
		instruction.Status = models.AllocationPendingApproval
	}

	created := false
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		var writeErr error
		created, writeErr = s.opts.Repository.CreateAllocation(ctx, tx, &instruction)
		if writeErr != nil {
			return writeErr
		}
		if !created {
			return nil
		}
		if writeErr = s.opts.Repository.AppendAllocationHistory(ctx, tx,
			in.Tenant.OrganizationID, instruction.ID, "", instruction.Status, in.ReasonCode,
			string(in.Tenant.ActorType), in.Tenant.ActorID, in.Tenant.CorrelationID); writeErr != nil {
			return writeErr
		}
		if writeErr = s.audit.Record(ctx, tx, audit.Entry{
			OrganizationID: in.Tenant.OrganizationID, BranchID: in.BranchID,
			ActorType: string(in.Tenant.ActorType), ActorID: in.Tenant.ActorID,
			Action: "wallet.provider_allocation.requested", ResourceType: "provider_allocation",
			ResourceID: instruction.ID,
			NewValues: map[string]any{"allocationType": allocationType,
				"amountMinor": in.AmountMinor, "currency": currency,
				"reasonCode": in.ReasonCode, "status": instruction.Status},
			Reason: in.ReasonCode, RequestID: in.Tenant.RequestID,
			CorrelationID: in.Tenant.CorrelationID}); writeErr != nil {
			return writeErr
		}
		return s.publish(ctx, tx, "organization.provider_allocation.requested.v1",
			"PROVIDER_ALLOCATION", instruction.ID, instruction.Version, in.Tenant,
			map[string]any{"allocationId": instruction.ID, "membershipId": in.MembershipID,
				"allocationType": allocationType, "amountMinor": in.AmountMinor,
				"currency": currency, "businessReferenceType": referenceType,
				"status": instruction.Status})
	})
	if err != nil {
		return zero, translateAllocationError(err)
	}
	if created {
		s.count("fleet_partner_provider_allocations_total",
			map[string]string{"type": allocationType, "status": instruction.Status})
	}

	// The approval request is raised outside the instruction transaction on
	// purpose: an approval-framework failure must not lose the instruction that
	// has already been recorded, and the instruction is inert until approved.
	if created && needsApproval && s.opts.Approvals != nil {
		approvalID, approvalErr := s.opts.Approvals.Request(ctx, ApprovalRequestInput{
			OrganizationID: in.Tenant.OrganizationID, OrganizationUserID: in.Tenant.OrganizationUserID,
			BranchID: in.BranchID, ResourceType: "provider_allocation", ResourceID: instruction.ID,
			Action:        "wallet.provider_allocations.approve",
			OperationType: eligibilityOperation(allocationType),
			Payload:       approvalPayload(instruction), ExpectedSubjectVersion: instruction.Version,
			IdempotencyKey: "allocation-approval:" + instruction.ID,
			CorrelationID:  in.Tenant.CorrelationID})
		if approvalErr == nil && approvalID != "" {
			instruction.ApprovalRequestID = approvalID
		}
	}
	return instruction, nil
}

// approvalPayload is the exact controlled action an approver signs off.
//
// It excludes the private note deliberately: an approver decides on the money
// movement and its stated reason, and internal commentary is not part of what
// is being approved, so it cannot change the fingerprint either.
func approvalPayload(instruction models.AllocationInstruction) map[string]any {
	return map[string]any{
		"membershipId":          instruction.ProviderMembershipID,
		"allocationType":        instruction.AllocationType,
		"amountMinor":           instruction.AmountMinor,
		"currency":              instruction.Currency,
		"businessReferenceType": instruction.BusinessReferenceType,
		"businessReferenceId":   instruction.BusinessReferenceID,
		"reasonCode":            instruction.ReasonCode,
	}
}

func translateAllocationError(err error) error {
	switch {
	case errors.Is(err, repositories.ErrFingerprint):
		return errorcodes.Conflict(errorcodes.WalletOperationConflict,
			"This idempotency key was already used with different values.")
	case errors.Is(err, repositories.ErrDuplicate):
		return errorcodes.Conflict(errorcodes.ProviderAllocationDuplicate,
			"An allocation already exists for this business reference.")
	default:
		return err
	}
}

// DecideAllocation approves or rejects a pending allocation.
//
// Approving executes nothing: it moves the instruction into the worker's queue.
// The worker submits under the instruction's own idempotency key, so approving
// twice cannot produce two payments even if both decisions somehow landed.
func (s *Service) DecideAllocation(ctx context.Context, t tenant.Context, id string,
	expectedVersion int64, approve bool, reasonCode string) (models.AllocationInstruction, error) {
	var result models.AllocationInstruction
	if err := s.writesEnabled(); err != nil {
		return result, err
	}
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		instruction, readErr := s.opts.Repository.LockAllocation(ctx, tx, t.OrganizationID, id)
		if readErr != nil {
			if errors.Is(readErr, repositories.ErrNotFound) {
				return notFound("Allocation")
			}
			return readErr
		}
		if instruction.Status != models.AllocationPendingApproval {
			return errorcodes.Conflict(errorcodes.WalletOperationConflict,
				"This allocation is no longer awaiting a decision.")
		}
		if expectedVersion != 0 && expectedVersion != instruction.Version {
			return errorcodes.Conflict(errorcodes.WalletOperationConflict,
				"The allocation changed since it was read.")
		}
		// The approved payload must be exactly the one being executed. The
		// approval framework compares fingerprints; a payload that drifted
		// since approval is refused rather than executed on the old decision.
		if approve && instruction.ApprovalRequestID != "" && s.opts.Approvals != nil {
			if approvalErr := s.opts.Approvals.RequireApproved(ctx, t.OrganizationID,
				instruction.ApprovalRequestID, approvalPayload(instruction)); approvalErr != nil {
				return approvalErr
			}
		}
		// Separation of duties: the person who asked for the money to move is
		// not the person who may authorize it.
		if approve && instruction.RequestedBy == t.ActorID {
			return errorcodes.Forbidden(errorcodes.ApprovalSelfDecisionForbidden,
				"You cannot approve an allocation you requested.")
		}
		next := models.AllocationApproved
		if !approve {
			next = models.AllocationRejected
		}
		if writeErr := s.opts.Repository.SetAllocationStatus(ctx, tx, t.OrganizationID, id,
			instruction.Version, next, "", "", "", t.ActorID); writeErr != nil {
			return writeErr
		}
		if writeErr := s.opts.Repository.AppendAllocationHistory(ctx, tx, t.OrganizationID, id,
			instruction.Status, next, reasonCode, string(t.ActorType), t.ActorID,
			t.CorrelationID); writeErr != nil {
			return writeErr
		}
		if writeErr := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: t.OrganizationID,
			ActorType: string(t.ActorType), ActorID: t.ActorID,
			Action: "wallet.provider_allocation.decided", ResourceType: "provider_allocation",
			ResourceID: id, PreviousValues: map[string]any{"status": instruction.Status},
			NewValues: map[string]any{"status": next, "reasonCode": reasonCode},
			Reason:    reasonCode, RequestID: t.RequestID, CorrelationID: t.CorrelationID}); writeErr != nil {
			return writeErr
		}
		instruction.Status = next
		instruction.Version++
		result = instruction
		return s.publish(ctx, tx, "organization.provider_allocation.decided.v1",
			"PROVIDER_ALLOCATION", id, instruction.Version, t,
			map[string]any{"allocationId": id, "status": next, "reasonCode": reasonCode})
	})
	if errors.Is(err, repositories.ErrVersionConflict) {
		return result, errorcodes.Conflict(errorcodes.WalletOperationConflict,
			"The allocation changed since it was read.")
	}
	return result, err
}

// GetAllocation reads one instruction inside the caller's scope.
func (s *Service) GetAllocation(ctx context.Context, t tenant.Context,
	id string) (models.AllocationInstruction, error) {
	instruction, err := s.opts.Repository.GetAllocation(ctx, nil, t.OrganizationID, id,
		t.PermittedBranchIDs)
	if errors.Is(err, repositories.ErrNotFound) {
		return instruction, notFound("Allocation")
	}
	return instruction, err
}

// ListAllocations returns a tenant-scoped page.
func (s *Service) ListAllocations(ctx context.Context, t tenant.Context,
	filter repositories.AllocationFilter) ([]models.AllocationInstruction, error) {
	return s.opts.Repository.ListAllocations(ctx, t.OrganizationID, t.PermittedBranchIDs, filter)
}

// ReplayAllocation returns a dead-lettered instruction to the execution queue.
func (s *Service) ReplayAllocation(ctx context.Context, t tenant.Context, id string) error {
	if err := s.writesEnabled(); err != nil {
		return err
	}
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Repository.ReplayAllocation(ctx, tx, t.OrganizationID, id); err != nil {
			if errors.Is(err, repositories.ErrNotFound) {
				return notFound("Allocation")
			}
			return err
		}
		return s.audit.Record(ctx, tx, audit.Entry{OrganizationID: t.OrganizationID,
			ActorType: string(t.ActorType), ActorID: t.ActorID,
			Action: "wallet.provider_allocation.replayed", ResourceType: "provider_allocation",
			ResourceID: id, RequestID: t.RequestID, CorrelationID: t.CorrelationID})
	})
}
