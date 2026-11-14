package models

import (
	"errors"
	"fmt"
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

// ManagedConfirmed turns a confirmed posting into a result, checking that
// Wallet confirmed the same instruction version Fleet sent. A disagreement is
// not something to retry away: it is surfaced as an unknown outcome so the
// instruction lands in reconciliation for a person to look at.
func ManagedConfirmed(operationID string, out PostingOutcome, in ManagedSettlementSubmission) (ManagedSettlementResult, error) {
	if out.SourceVersion != 0 && out.SourceVersion != in.InstructionVersion {
		return ManagedSettlementResult{OperationID: operationID, Disposition: ManagedSettlementOutcomeUnknown,
				Reference: out.Reference, ConfirmedVersion: out.SourceVersion, ErrorCode: "wallet_version_mismatch"},
			fmt.Errorf("%w: wallet confirmed instruction version %d, sent %d",
				ErrManagedSettlementOutcomeUnknown, out.SourceVersion, in.InstructionVersion)
	}
	return ManagedSettlementResult{OperationID: operationID, Disposition: ManagedSettlementPosted,
		Reference: out.Reference, ConfirmedVersion: in.InstructionVersion}, nil
}

// PostingOperationState is the stored state of one posting operation.
type PostingOperationState struct {
	Status, Reference, LastErrorCode string
	SourceVersion                    int64
}
