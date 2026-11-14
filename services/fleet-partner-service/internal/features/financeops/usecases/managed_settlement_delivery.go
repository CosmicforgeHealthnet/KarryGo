package usecases

// FLEET-13.11: the delivery half of managed settlement.
//
// models/managed_settlement.go defines the instruction on the wire; this file sends
// it. It reuses the posting-operation machinery payroll already relies on
// (createPosting/executePosting): one durable operation per idempotency key,
// a recorded attempt per call, and an ambiguous outcome resolved by reading
// back rather than by assuming.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// SubmitManagedSettlement delivers one instruction to the money authority.
//
// It never posts when posting is disabled, killed or unconfigured, and it never
// sends a second request for an instruction whose posting operation already
// confirmed. An operation left OUTCOME_UNKNOWN by an earlier attempt is read
// back first and only resent when Wallet has no record of it; resending is
// then safe because Wallet keys the journal on the instruction identity and
// refuses the same identity with different figures.
func (s *Service) SubmitManagedSettlement(
	ctx context.Context, t tenant.Context, in models.ManagedSettlementSubmission,
) (models.ManagedSettlementResult, error) {
	if err := in.Validate(); err != nil {
		return models.ManagedSettlementResult{Disposition: models.ManagedSettlementRefused, ErrorCode: "invalid_instruction"},
			fmt.Errorf("%w: %v", models.ErrManagedSettlementRefused, err)
	}
	if t.OrganizationID != "" && t.OrganizationID != in.OrganizationID {
		return models.ManagedSettlementResult{Disposition: models.ManagedSettlementRefused, ErrorCode: "tenant_mismatch"},
			fmt.Errorf("%w: settlement instruction belongs to another organization", models.ErrManagedSettlementRefused)
	}
	t.OrganizationID = in.OrganizationID
	if s.o.Flags.KillSwitch || !s.o.Flags.Posting || s.o.Wallet == nil || !s.o.Wallet.Configured(models.OperationManagedWorkSettlement) {
		return models.ManagedSettlementResult{Disposition: models.ManagedSettlementUnavailable, ErrorCode: "posting_unavailable"},
			models.ErrManagedSettlementUnavailable
	}
	body, err := in.Payload()
	if err != nil {
		return models.ManagedSettlementResult{Disposition: models.ManagedSettlementRefused, ErrorCode: "invalid_instruction"},
			fmt.Errorf("%w: %v", models.ErrManagedSettlementRefused, err)
	}
	request := models.PostingRequest{
		OrganizationID: in.OrganizationID,
		RecordType:     "MANAGED_WORK_SETTLEMENT_INSTRUCTION",
		RecordID:       in.InstructionID,
		OperationType:  models.OperationManagedWorkSettlement,
		IdempotencyKey: in.IdempotencyKey,
		AmountMinor:    in.GrossBasisMinor,
		Currency:       in.Currency,
		Payload:        body,
	}

	operationID, _, err := s.createPosting(ctx, t, request)
	if err != nil {
		var appErr *apperrors.Error
		if errors.As(err, &appErr) && appErr.Status == http.StatusConflict {
			// The key was already used for different figures. That is a Fleet
			// defect, not a transient condition; resending cannot fix it.
			return models.ManagedSettlementResult{Disposition: models.ManagedSettlementRefused, ErrorCode: "idempotency_conflict"},
				fmt.Errorf("%w: %v", models.ErrManagedSettlementRefused, err)
		}
		return models.ManagedSettlementResult{Disposition: models.ManagedSettlementRetry, ErrorCode: "posting_record_failed"},
			fmt.Errorf("%w: %v", models.ErrManagedSettlementRetry, err)
	}
	state, err := s.postingState(ctx, in.OrganizationID, operationID)
	if err != nil {
		return models.ManagedSettlementResult{OperationID: operationID, Disposition: models.ManagedSettlementRetry, ErrorCode: "posting_read_failed"},
			fmt.Errorf("%w: %v", models.ErrManagedSettlementRetry, err)
	}
	switch state.Status {
	case "CONFIRMED":
		// Already posted under this identity: no call is made.
		return models.ManagedConfirmed(operationID, models.PostingOutcome{Reference: state.Reference, SourceVersion: state.SourceVersion}, in)
	case "FAILED", "DEAD_LETTERED", "REVERSED":
		code := state.LastErrorCode
		if code == "" {
			code = "posting_" + strings.ToLower(state.Status)
		}
		return models.ManagedSettlementResult{OperationID: operationID, Disposition: models.ManagedSettlementRefused, ErrorCode: code},
			models.ErrManagedSettlementRefused
	case "OUTCOME_UNKNOWN":
		out, confirmed, notFound, readErr := s.readBackPosting(ctx, t, operationID, request)
		if confirmed {
			return models.ManagedConfirmed(operationID, out, in)
		}
		if !notFound {
			return models.ManagedSettlementResult{OperationID: operationID, Disposition: models.ManagedSettlementOutcomeUnknown, ErrorCode: "read_back_pending"},
				fmt.Errorf("%w: %v", models.ErrManagedSettlementOutcomeUnknown, readErr)
		}
		// Wallet holds no posting under this key, so the earlier request
		// never reached it. Sending now is the first send, not a second one.
	}

	out, err := s.executePosting(ctx, t, operationID, request)
	if err == nil {
		return models.ManagedConfirmed(operationID, out, in)
	}
	var clientErr *platformclients.Error
	if errors.As(err, &clientErr) {
		switch clientErr.Outcome {
		case platformclients.OutcomeValidation, platformclients.OutcomeNotFound, platformclients.OutcomeConflict:
			code := "wallet_refused_" + string(clientErr.Outcome)
			if markErr := s.markPostingFailed(ctx, in.OrganizationID, operationID, code); markErr != nil {
				return models.ManagedSettlementResult{OperationID: operationID, Disposition: models.ManagedSettlementRetry, ErrorCode: "posting_record_failed"},
					fmt.Errorf("%w: %v", models.ErrManagedSettlementRetry, markErr)
			}
			return models.ManagedSettlementResult{OperationID: operationID, Disposition: models.ManagedSettlementRefused, ErrorCode: code},
				fmt.Errorf("%w: %v", models.ErrManagedSettlementRefused, err)
		}
	}
	after, stateErr := s.postingState(ctx, in.OrganizationID, operationID)
	if stateErr == nil && after.Status == "OUTCOME_UNKNOWN" {
		return models.ManagedSettlementResult{OperationID: operationID, Disposition: models.ManagedSettlementOutcomeUnknown, ErrorCode: after.LastErrorCode},
			fmt.Errorf("%w: %v", models.ErrManagedSettlementOutcomeUnknown, err)
	}
	return models.ManagedSettlementResult{OperationID: operationID, Disposition: models.ManagedSettlementRetry, ErrorCode: "wallet_unavailable"},
		fmt.Errorf("%w: %v", models.ErrManagedSettlementRetry, err)
}

func (s *Service) postingState(ctx context.Context, organizationID, operationID string) (models.PostingOperationState, error) {
	return s.o.Store.PostingState(ctx, organizationID, operationID)
}

// readBackPosting asks Wallet what it holds under the operation's key before
// anything is resent. It returns (outcome, confirmed, notFound, error).
func (s *Service) readBackPosting(ctx context.Context, t tenant.Context, operationID string, req models.PostingRequest) (models.PostingOutcome, bool, bool, error) {
	start := s.o.Now().UTC()
	out, err := s.o.Wallet.ReadByKey(ctx, operationMeta(t, req.IdempotencyKey), req)
	if err == nil && models.PostingConfirmed(out.Status) {
		if recErr := s.recordPostingAttempt(ctx, t, operationID, start, "READ_BACK_CONFIRMED", "CONFIRMED", out.Reference, out.SourceVersion, ""); recErr != nil {
			return out, false, false, recErr
		}
		return out, true, false, nil
	}
	var clientErr *platformclients.Error
	if err != nil && errors.As(err, &clientErr) && clientErr.Outcome == platformclients.OutcomeNotFound {
		if recErr := s.recordPostingAttempt(ctx, t, operationID, start, "READ_BACK_NOT_FOUND", "OUTCOME_UNKNOWN", "", 0, "read_back_not_found"); recErr != nil {
			return out, false, false, recErr
		}
		return out, false, true, nil
	}
	if err == nil {
		err = fmt.Errorf("authoritative posting status %q is not confirmed", out.Status)
	}
	if recErr := s.recordPostingAttempt(ctx, t, operationID, start, "READ_BACK_PENDING", "OUTCOME_UNKNOWN", out.Reference, out.SourceVersion, "read_back_pending"); recErr != nil {
		return out, false, false, recErr
	}
	return out, false, false, err
}

// markPostingFailed records that Wallet refused the operation as stated, so
// neither this delivery path nor the generic reconciliation pass resends it.
func (s *Service) markPostingFailed(ctx context.Context, organizationID, operationID, code string) error {
	return s.o.Store.MarkPostingFailed(ctx, organizationID, operationID, code)
}
