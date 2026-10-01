package logistics

// Normalized logistics error categories (VND-021).
//
// Three logistics services, three client packages and three sets of sentinel
// errors reach Vendor's fulfilment code. Commerce does not need to know which
// one it is looking at; it needs to know one thing per failure: may this be
// retried, is it a refusal the customer must be told about, or does it mean
// Vendor has lost track of an execution that may already exist?
//
// A category answers exactly that. The originating error is kept for logs and
// never rendered to a customer, so an internal message or stack cannot leak
// through a commerce surface.

import (
	"errors"
	"strings"

	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/logisticscapability"
)

// FailureCategory is Vendor's safe classification of a logistics failure.
type FailureCategory string

const (
	// FailureNone means no failure.
	FailureNone FailureCategory = ""

	// FailureNotServiceable means the authority will not serve this request:
	// out of area, outside a serving window, or otherwise refused. It is an
	// answer, not a fault, and retrying it unchanged will not help.
	FailureNotServiceable FailureCategory = "NOT_SERVICEABLE"

	// FailureCapabilityMismatch means the request exceeds what the authority
	// can physically execute — too many pickups, too heavy, a handover the
	// authority cannot protect.
	FailureCapabilityMismatch FailureCategory = "CAPABILITY_MISMATCH"

	// FailureProviderUnavailable means the authority is willing but has no
	// provider. Retrying later is reasonable.
	FailureProviderUnavailable FailureCategory = "PROVIDER_UNAVAILABLE"

	// FailureAssignmentTimeout means matching ran out of time without a
	// provider accepting.
	FailureAssignmentTimeout FailureCategory = "ASSIGNMENT_TIMEOUT"

	// FailureExecutionConflict means the authority already holds a different
	// execution under this identity. It is never grounds for retrying with a
	// new identity: that is how a second vehicle gets dispatched.
	FailureExecutionConflict FailureCategory = "EXECUTION_CREATION_CONFLICT"

	// FailureCredential means a pickup or delivery credential was not
	// verified. A protected handover stays protected; it does not degrade
	// into an unverified completion.
	FailureCredential FailureCategory = "CREDENTIAL_FAILURE"

	// FailureDependencyUnavailable means the integration could not be reached
	// or is switched off. The mission's real state is unchanged by it.
	FailureDependencyUnavailable FailureCategory = "DEPENDENCY_UNAVAILABLE"

	// FailureReconciliationRequired means the outcome is genuinely unknown:
	// the request may have been executed. Vendor must resolve it against the
	// authority rather than assume either way.
	FailureReconciliationRequired FailureCategory = "RECONCILIATION_REQUIRED"
)

// Retryable reports whether re-sending the same request, under the same
// idempotency identity, is a reasonable next step.
func (c FailureCategory) Retryable() bool {
	switch c {
	case FailureDependencyUnavailable, FailureProviderUnavailable, FailureAssignmentTimeout:
		return true
	default:
		return false
	}
}

// NeedsReconciliation reports whether Vendor must ask the authority what
// actually happened before deciding anything else.
func (c FailureCategory) NeedsReconciliation() bool {
	return c == FailureReconciliationRequired || c == FailureExecutionConflict
}

// Message is the customer-safe sentence for a category.
//
// It is deliberately the only text that leaves this package: the underlying
// error's own message may name an internal host, a table or a stack, and none
// of that belongs on a commerce response.
func (c FailureCategory) Message() string {
	switch c {
	case FailureNotServiceable:
		return "This delivery cannot be served by the selected service."
	case FailureCapabilityMismatch:
		return "This delivery is outside what the selected service can carry."
	case FailureProviderUnavailable:
		return "No provider is available for this delivery right now."
	case FailureAssignmentTimeout:
		return "No provider accepted this delivery in time."
	case FailureExecutionConflict:
		return "This delivery is already being arranged."
	case FailureCredential:
		return "Handover verification did not succeed."
	case FailureDependencyUnavailable:
		return "The delivery service is temporarily unavailable."
	case FailureReconciliationRequired:
		return "This delivery is being confirmed with the delivery service."
	default:
		return ""
	}
}

// Failure is a classified logistics failure.
type Failure struct {
	Category  FailureCategory
	Authority Authority

	// ReasonCode is the authority's own refusal code when it supplied one.
	// It is a code, never prose, and it is safe to surface.
	ReasonCode string

	// cause is kept for structured logging and is never rendered.
	cause error
}

// Error implements error using the safe message only.
func (f Failure) Error() string {
	if f.ReasonCode != "" {
		return string(f.Category) + ": " + f.ReasonCode
	}
	return string(f.Category)
}

// Unwrap exposes the originating error to errors.Is/As for logging and tests,
// without putting its text on any response.
func (f Failure) Unwrap() error { return f.cause }

// Retryable reports whether the failure may be retried.
func (f Failure) Retryable() bool { return f.Category.Retryable() }

// Classify maps any logistics error onto a Vendor-safe category.
//
// Sentinels are matched first, because they are unambiguous. Only when no
// sentinel matches does it fall back to reading the message, and that fallback
// is conservative: anything it cannot place becomes RECONCILIATION_REQUIRED
// rather than a failure, since an unrecognised error after a create is exactly
// the case where an execution may already exist.
func Classify(authority Authority, err error) Failure {
	if err == nil {
		return Failure{Category: FailureNone, Authority: authority}
	}
	failure := Failure{Authority: authority, cause: err}

	var classified Failure
	if errors.As(err, &classified) {
		if classified.Authority == "" {
			classified.Authority = authority
		}
		return classified
	}

	switch {
	case errors.Is(err, commercemission.ErrNotConfigured),
		errors.Is(err, dispatchcommerce.ErrNotConfigured),
		errors.Is(err, logisticscapability.ErrNotConfigured),
		errors.Is(err, commercemission.ErrAuthorityUnavailable),
		errors.Is(err, dispatchcommerce.ErrAuthorityUnavailable),
		errors.Is(err, logisticscapability.ErrAuthorityUnavailable):
		failure.Category = FailureDependencyUnavailable

	case errors.Is(err, dispatchcommerce.ErrConflict),
		errors.Is(err, commercemission.ErrMissionMismatch):
		failure.Category = FailureExecutionConflict

	case errors.Is(err, commercemission.ErrRepriced):
		// An authority answering with a different fee is a contract conflict,
		// not a price negotiation: the customer's approved amount stands and
		// the mission is not created.
		failure.Category = FailureExecutionConflict

	case errors.Is(err, commercemission.ErrInvalidRequest),
		errors.Is(err, dispatchcommerce.ErrInvalidRequest),
		errors.Is(err, logisticscapability.ErrUnknownAuthority):
		failure.Category = FailureCapabilityMismatch

	default:
		failure.Category = categoryFromMessage(err)
	}
	return failure
}

// categoryFromMessage is the conservative fallback for errors that carry no
// sentinel. It reads only well-known refusal words; it never reproduces the
// message it read.
func categoryFromMessage(err error) FailureCategory {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "not serviceable"), strings.Contains(message, "out of range"),
		strings.Contains(message, "outside service area"), strings.Contains(message, "unserviceable"):
		return FailureNotServiceable
	case strings.Contains(message, "capacity"), strings.Contains(message, "capability"),
		strings.Contains(message, "too heavy"), strings.Contains(message, "exceeds"):
		return FailureCapabilityMismatch
	case strings.Contains(message, "no provider"), strings.Contains(message, "unmatched"),
		strings.Contains(message, "no rider"), strings.Contains(message, "no driver"):
		return FailureProviderUnavailable
	case strings.Contains(message, "timeout"), strings.Contains(message, "deadline exceeded"),
		strings.Contains(message, "timed out"):
		// A timeout on a create is an unknown outcome, not a clean failure.
		return FailureReconciliationRequired
	case strings.Contains(message, "credential"), strings.Contains(message, "verification failed"),
		strings.Contains(message, "otp"):
		return FailureCredential
	case strings.Contains(message, "conflict"), strings.Contains(message, "already exists"):
		return FailureExecutionConflict
	default:
		return FailureReconciliationRequired
	}
}

// NewFailure builds a classified failure directly, for the cases Vendor itself
// decides — a capability gate refusal, for instance, where there is no
// underlying transport error to classify.
func NewFailure(authority Authority, category FailureCategory, reasonCode string) Failure {
	return Failure{Category: category, Authority: authority, ReasonCode: strings.TrimSpace(reasonCode)}
}

// KnownFailureCategories lists every category, for contract tests.
func KnownFailureCategories() []FailureCategory {
	return []FailureCategory{
		FailureNotServiceable, FailureCapabilityMismatch, FailureProviderUnavailable,
		FailureAssignmentTimeout, FailureExecutionConflict, FailureCredential,
		FailureDependencyUnavailable, FailureReconciliationRequired,
	}
}
