package financeops

// FLEET-13.11: the delivery half of managed settlement.
//
// managed_settlement.go defines the instruction on the wire; this file sends
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

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// Delivery dispositions. Each one tells the caller exactly what it may do
// next: a caller that has to guess between "retry" and "read back" is a
// caller that eventually pays twice.
const (
	// ManagedSettlementPosted: Wallet confirmed the journal for this
	// instruction. Nothing further may be sent.
	ManagedSettlementPosted = "POSTED"
	// ManagedSettlementRetry: a transient failure. Resending the same
	// instruction is safe because Wallet keys its posting on the instruction.
	ManagedSettlementRetry = "RETRY"
	// ManagedSettlementOutcomeUnknown: the outcome could not be confirmed.
	// The next attempt reads back before it sends anything.
	ManagedSettlementOutcomeUnknown = "OUTCOME_UNKNOWN"
	// ManagedSettlementRefused: Wallet (or Fleet's own validation) refused
	// the instruction as stated. Resending it unchanged cannot help.
	ManagedSettlementRefused = "REFUSED"
	// ManagedSettlementUnavailable: posting is switched off, killed, or no
	// Wallet adapter is configured. Nothing was sent and nothing was recorded.
	ManagedSettlementUnavailable = "UNAVAILABLE"
)

// Sentinels wrapped by every non-posted result, so a caller cannot read a
// failed delivery as a success by ignoring the disposition.
var (
	ErrManagedSettlementUnavailable    = errors.New("financeops: managed settlement posting is unavailable")
	ErrManagedSettlementRetry          = errors.New("financeops: managed settlement delivery failed transiently")
	ErrManagedSettlementOutcomeUnknown = errors.New("financeops: managed settlement outcome is unknown")
	ErrManagedSettlementRefused        = errors.New("financeops: managed settlement was refused")
)

// ManagedSettlementResult is what one delivery attempt established.
type ManagedSettlementResult struct {
	OperationID      string
	Disposition      string
	Reference        string
	ConfirmedVersion int64
	ErrorCode        string
}

// Confirmed reports whether the money authority has posted the instruction.
func (r ManagedSettlementResult) Confirmed() bool { return r.Disposition == ManagedSettlementPosted }

// SubmitManagedSettlement delivers one instruction to the money authority.
//
// It never posts when posting is disabled, killed or unconfigured, and it never
// sends a second request for an instruction whose posting operation already
// confirmed. An operation left OUTCOME_UNKNOWN by an earlier attempt is read
// back first and only resent when Wallet has no record of it; resending is
// then safe because Wallet keys the journal on the instruction identity and
// refuses the same identity with different figures.
func (s *Service) SubmitManagedSettlement(
	ctx context.Context, t tenant.Context, in ManagedSettlementSubmission,
) (ManagedSettlementResult, error) {
	if err := in.Validate(); err != nil {
		return ManagedSettlementResult{Disposition: ManagedSettlementRefused, ErrorCode: "invalid_instruction"},
			fmt.Errorf("%w: %v", ErrManagedSettlementRefused, err)
	}
	if t.OrganizationID != "" && t.OrganizationID != in.OrganizationID {
		return ManagedSettlementResult{Disposition: ManagedSettlementRefused, ErrorCode: "tenant_mismatch"},
			fmt.Errorf("%w: settlement instruction belongs to another organization", ErrManagedSettlementRefused)
	}
	t.OrganizationID = in.OrganizationID
	if s.o.Flags.KillSwitch || !s.o.Flags.Posting || s.o.Wallet == nil || !s.o.Wallet.Configured(OperationManagedWorkSettlement) {
		return ManagedSettlementResult{Disposition: ManagedSettlementUnavailable, ErrorCode: "posting_unavailable"},
			ErrManagedSettlementUnavailable
	}
	body, err := in.payload()
	if err != nil {
		return ManagedSettlementResult{Disposition: ManagedSettlementRefused, ErrorCode: "invalid_instruction"},
			fmt.Errorf("%w: %v", ErrManagedSettlementRefused, err)
	}
	request := PostingRequest{
		OrganizationID: in.OrganizationID,
		RecordType:     "MANAGED_WORK_SETTLEMENT_INSTRUCTION",
		RecordID:       in.InstructionID,
		OperationType:  OperationManagedWorkSettlement,
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
			return ManagedSettlementResult{Disposition: ManagedSettlementRefused, ErrorCode: "idempotency_conflict"},
				fmt.Errorf("%w: %v", ErrManagedSettlementRefused, err)
		}
		return ManagedSettlementResult{Disposition: ManagedSettlementRetry, ErrorCode: "posting_record_failed"},
			fmt.Errorf("%w: %v", ErrManagedSettlementRetry, err)
	}
	state, err := s.postingState(ctx, in.OrganizationID, operationID)
	if err != nil {
		return ManagedSettlementResult{OperationID: operationID, Disposition: ManagedSettlementRetry, ErrorCode: "posting_read_failed"},
			fmt.Errorf("%w: %v", ErrManagedSettlementRetry, err)
	}
	switch state.Status {
	case "CONFIRMED":
		// Already posted under this identity: no call is made.
		return managedConfirmed(operationID, PostingOutcome{Reference: state.Reference, SourceVersion: state.SourceVersion}, in)
	case "FAILED", "DEAD_LETTERED", "REVERSED":
		code := state.LastErrorCode
		if code == "" {
			code = "posting_" + strings.ToLower(state.Status)
		}
		return ManagedSettlementResult{OperationID: operationID, Disposition: ManagedSettlementRefused, ErrorCode: code},
			ErrManagedSettlementRefused
	case "OUTCOME_UNKNOWN":
		out, confirmed, notFound, readErr := s.readBackPosting(ctx, t, operationID, request)
		if confirmed {
			return managedConfirmed(operationID, out, in)
		}
		if !notFound {
			return ManagedSettlementResult{OperationID: operationID, Disposition: ManagedSettlementOutcomeUnknown, ErrorCode: "read_back_pending"},
				fmt.Errorf("%w: %v", ErrManagedSettlementOutcomeUnknown, readErr)
		}
		// Wallet holds no posting under this key, so the earlier request
		// never reached it. Sending now is the first send, not a second one.
	}

	out, err := s.executePosting(ctx, t, operationID, request)
	if err == nil {
		return managedConfirmed(operationID, out, in)
	}
	var clientErr *platformclients.Error
	if errors.As(err, &clientErr) {
		switch clientErr.Outcome {
		case platformclients.OutcomeValidation, platformclients.OutcomeNotFound, platformclients.OutcomeConflict:
			code := "wallet_refused_" + string(clientErr.Outcome)
			if markErr := s.markPostingFailed(ctx, in.OrganizationID, operationID, code); markErr != nil {
				return ManagedSettlementResult{OperationID: operationID, Disposition: ManagedSettlementRetry, ErrorCode: "posting_record_failed"},
					fmt.Errorf("%w: %v", ErrManagedSettlementRetry, markErr)
			}
			return ManagedSettlementResult{OperationID: operationID, Disposition: ManagedSettlementRefused, ErrorCode: code},
				fmt.Errorf("%w: %v", ErrManagedSettlementRefused, err)
		}
	}
	after, stateErr := s.postingState(ctx, in.OrganizationID, operationID)
	if stateErr == nil && after.Status == "OUTCOME_UNKNOWN" {
		return ManagedSettlementResult{OperationID: operationID, Disposition: ManagedSettlementOutcomeUnknown, ErrorCode: after.LastErrorCode},
			fmt.Errorf("%w: %v", ErrManagedSettlementOutcomeUnknown, err)
	}
	return ManagedSettlementResult{OperationID: operationID, Disposition: ManagedSettlementRetry, ErrorCode: "wallet_unavailable"},
		fmt.Errorf("%w: %v", ErrManagedSettlementRetry, err)
}

// managedConfirmed turns a confirmed posting into a result, checking that
// Wallet confirmed the same instruction version Fleet sent. A disagreement is
// not something to retry away: it is surfaced as an unknown outcome so the
// instruction lands in reconciliation for a person to look at.
func managedConfirmed(operationID string, out PostingOutcome, in ManagedSettlementSubmission) (ManagedSettlementResult, error) {
	if out.SourceVersion != 0 && out.SourceVersion != in.InstructionVersion {
		return ManagedSettlementResult{OperationID: operationID, Disposition: ManagedSettlementOutcomeUnknown,
				Reference: out.Reference, ConfirmedVersion: out.SourceVersion, ErrorCode: "wallet_version_mismatch"},
			fmt.Errorf("%w: wallet confirmed instruction version %d, sent %d",
				ErrManagedSettlementOutcomeUnknown, out.SourceVersion, in.InstructionVersion)
	}
	return ManagedSettlementResult{OperationID: operationID, Disposition: ManagedSettlementPosted,
		Reference: out.Reference, ConfirmedVersion: in.InstructionVersion}, nil
}

type postingOperationState struct {
	Status, Reference, LastErrorCode string
	SourceVersion                    int64
}

func (s *Service) postingState(ctx context.Context, organizationID, operationID string) (postingOperationState, error) {
	var st postingOperationState
	err := s.o.Pool.QueryRow(ctx, `SELECT status,external_operation_reference,last_error_code,source_version FROM fleet_partner_financial_posting_operations WHERE organization_id=$1 AND id=$2`,
		organizationID, operationID).Scan(&st.Status, &st.Reference, &st.LastErrorCode, &st.SourceVersion)
	return st, err
}

// readBackPosting asks Wallet what it holds under the operation's key before
// anything is resent. It returns (outcome, confirmed, notFound, error).
func (s *Service) readBackPosting(ctx context.Context, t tenant.Context, operationID string, req PostingRequest) (PostingOutcome, bool, bool, error) {
	start := s.o.Now().UTC()
	out, err := s.o.Wallet.ReadByKey(ctx, operationMeta(t, req.IdempotencyKey), req)
	if err == nil && postingConfirmed(out.Status) {
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
	_, err := s.o.Pool.Exec(ctx, `UPDATE fleet_partner_financial_posting_operations SET status='FAILED',last_error_code=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status<>'CONFIRMED'`,
		organizationID, operationID, code)
	return err
}
