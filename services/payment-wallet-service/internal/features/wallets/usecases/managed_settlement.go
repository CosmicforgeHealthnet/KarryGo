package walletusecases

// Section 12: the Wallet side of Fleet-managed work settlement.
//
// One rule shapes every function here: Wallet posts what Fleet decided, and
// verifies it against the money it actually holds. It never resolves a
// compensation plan, never applies a commission rate, and never decides that a
// provider "should" get something. If Fleet's instruction does not reconcile
// with the held payment, the settlement is refused -- not adjusted to fit.

import (
	"context"
	"errors"
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
)

// FleetServiceCaller is the only service identity permitted to instruct a
// managed settlement.
//
// Compensation authority is Fleet's alone. An execution service holding a
// valid signature for its own contracts must not be able to reach in here and
// pay its own provider.
const FleetServiceCaller = "fleet-partner-service"

// ManagedSettlementEvents are the durable announcements this path makes.
const (
	EventManagedSettlementPosted   = "wallet.managed_settlement.posted.v1"
	EventManagedSettlementFailed   = "wallet.managed_settlement.failed.v1"
	EventManagedSettlementReversed = "wallet.managed_settlement.reversed.v1"
)

// PostManagedSettlement posts one Fleet settlement instruction.
//
// Retrying the same instruction returns the original posting rather than
// creating a second one, which is what makes a lost response safe: a timeout
// is never evidence that nothing happened.
func (s *WalletService) PostManagedSettlement(
	ctx context.Context, caller string, instruction walletmodels.ManagedSettlementInstruction,
) (walletmodels.ManagedSettlement, error) {
	if strings.TrimSpace(caller) != FleetServiceCaller {
		return walletmodels.ManagedSettlement{}, apperrors.Forbidden(
			"Only the fleet partner service may instruct a managed settlement.", nil)
	}
	instruction = instruction.Normalized()
	if err := instruction.Validate(); err != nil {
		return walletmodels.ManagedSettlement{}, apperrors.Validation(
			"Check the settlement instruction.", []apperrors.FieldViolation{
				{Field: "instruction", Message: err.Error()},
			})
	}

	settlement, err := s.repository.PostManagedSettlement(ctx, instruction)
	if err != nil {
		if errors.Is(err, walletrepositories.ErrManagedSettlementConflict) {
			return walletmodels.ManagedSettlement{}, apperrors.Conflict(
				"This settlement identity was already used with different amounts.", nil)
		}
		return walletmodels.ManagedSettlement{}, err
	}
	return settlement, nil
}

// GetManagedSettlement reads one settlement back by instruction identity
// (WAL-016).
func (s *WalletService) GetManagedSettlement(
	ctx context.Context, caller, instructionID string, version int64,
) (walletmodels.ManagedSettlement, error) {
	if err := requireFinanceReader(caller); err != nil {
		return walletmodels.ManagedSettlement{}, err
	}
	if strings.TrimSpace(instructionID) == "" || version <= 0 {
		return walletmodels.ManagedSettlement{}, apperrors.BadRequest(
			"Instruction id and a positive version are required.", nil)
	}
	return s.repository.GetManagedSettlementByInstruction(ctx, strings.TrimSpace(instructionID), version)
}

// GetManagedSettlementByKey reads one settlement back by idempotency key.
func (s *WalletService) GetManagedSettlementByKey(
	ctx context.Context, caller, key string,
) (walletmodels.ManagedSettlement, error) {
	if err := requireFinanceReader(caller); err != nil {
		return walletmodels.ManagedSettlement{}, err
	}
	if strings.TrimSpace(key) == "" {
		return walletmodels.ManagedSettlement{}, apperrors.BadRequest("Idempotency key is required.", nil)
	}
	return s.repository.GetManagedSettlementByIdempotencyKey(ctx, strings.TrimSpace(key))
}

// ListManagedSettlementsForJob answers "did my money actually move?" for one
// execution job.
//
// This is the read an execution service makes when its own completion call
// timed out. Fleet may also use it to reconcile an instruction it believes is
// still pending.
func (s *WalletService) ListManagedSettlementsForJob(
	ctx context.Context, caller, sourceService, sourceJobID string,
) ([]walletmodels.ManagedSettlement, error) {
	if err := requireFinanceReader(caller); err != nil {
		return nil, err
	}
	sourceService = strings.TrimSpace(sourceService)
	sourceJobID = strings.TrimSpace(sourceJobID)
	if sourceService == "" || sourceJobID == "" {
		return nil, apperrors.BadRequest("Source service and job id are required.", nil)
	}
	return s.repository.ListManagedSettlementsForSource(ctx, sourceService, sourceJobID)
}

// MarkJobManaged records that a job settles through Fleet, so the legacy
// direct-payout path refuses it (WAL-010).
//
// The execution service calls this when it assigns an organization-managed
// executor -- the moment managed-ness becomes known, and before any completion
// can race the legacy path.
func (s *WalletService) MarkJobManaged(ctx context.Context, caller, sourceService, sourceJobID string) error {
	if err := requireManagedMarker(caller); err != nil {
		return err
	}
	sourceService = strings.TrimSpace(sourceService)
	sourceJobID = strings.TrimSpace(sourceJobID)
	if sourceService == "" || sourceJobID == "" {
		return apperrors.BadRequest("Source service and job id are required.", nil)
	}
	return s.repository.MarkPaymentIntentManaged(ctx, sourceService, sourceJobID)
}

// requireFinanceReader allows Fleet and the three execution services to read a
// settlement back.
//
// An execution service legitimately needs to know whether its own completed
// job settled; what it must never be able to do is instruct one.
func requireFinanceReader(caller string) error {
	switch strings.TrimSpace(caller) {
	case FleetServiceCaller,
		"driver-taxi-service", "driver-dispatch-delivery-service", "driver-hauling-service":
		return nil
	default:
		return apperrors.Forbidden("Caller may not read managed settlements.", nil)
	}
}

// requireManagedMarker allows Fleet and the execution services to mark a job
// managed. Marking only ever closes the legacy path; it cannot move money.
func requireManagedMarker(caller string) error { return requireFinanceReader(caller) }

// ReverseManagedSettlement reverses a posted managed settlement (WAL-018).
//
// Reversal authority stays with Fleet for the same reason posting does: the
// entitlement being unwound is Fleet's to decide. The ledger keeps both
// journals -- the original and the compensating one -- so the history explains
// itself.
func (s *WalletService) ReverseManagedSettlement(
	ctx context.Context, caller, instructionID string, version int64, reasonCode string,
) (walletmodels.ManagedSettlement, error) {
	if strings.TrimSpace(caller) != FleetServiceCaller {
		return walletmodels.ManagedSettlement{}, apperrors.Forbidden(
			"Only the fleet partner service may reverse a managed settlement.", nil)
	}
	if strings.TrimSpace(instructionID) == "" || version <= 0 {
		return walletmodels.ManagedSettlement{}, apperrors.BadRequest(
			"Instruction id and a positive version are required.", nil)
	}
	reasonCode = strings.TrimSpace(reasonCode)
	if reasonCode == "" {
		// An unexplained reversal is indistinguishable from a mistake when it
		// is read back a quarter later.
		return walletmodels.ManagedSettlement{}, apperrors.Validation(
			"A reversal reason is required.", []apperrors.FieldViolation{
				{Field: "reason_code", Message: "Reason code is required."},
			})
	}
	return s.repository.ReverseManagedSettlement(ctx, strings.TrimSpace(instructionID), version, reasonCode)
}
