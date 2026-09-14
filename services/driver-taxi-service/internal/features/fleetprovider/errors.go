package fleetprovider

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/providercompat"
)

// FleetError is a classified failure of a Fleet call.
//
// The classification is the whole point. Before this type every Fleet failure
// collapsed into one "temporarily unavailable" 503, which made a suspended
// membership indistinguishable from a Fleet outage -- and made it impossible
// to honour Section 8's rule that a dependency failure is retryable while
// every authoritative rejection is final.
type FleetError struct {
	// Kind is the canonical Sections 8/9 classification.
	Kind providercompat.FailureKind
	// Status is the HTTP status Fleet actually returned, when there was one.
	Status int
	// Code is Fleet's structured error code, when it supplied one.
	Code string
	// Message is a provider-safe message. Fleet's own message text is never
	// forwarded verbatim: it may quote organization-internal detail.
	Message string
	Err     error
}

func (e *FleetError) Error() string {
	parts := []string{string(e.Kind)}
	if e.Code != "" {
		parts = append(parts, "code="+e.Code)
	}
	out := strings.Join(parts, " ")
	if e.Err != nil {
		return out + ": " + e.Err.Error()
	}
	return out
}

func (e *FleetError) Unwrap() error { return e.Err }

// Retryable reports whether the caller should retry rather than reject.
func (e *FleetError) Retryable() bool { return e != nil && e.Kind.Retryable() }

// ErrFleetUnavailable is retained for callers that only need to know Fleet
// could not be reached. New code should classify with FleetError instead.
var ErrFleetUnavailable = errors.New("fleet provider work is unavailable")

// dependencyError builds a retryable dependency failure.
func dependencyError(err error) *FleetError {
	return &FleetError{
		Kind: providercompat.FailureDependency, Err: err,
		Message: "Fleet is temporarily unavailable. Try again.",
	}
}

// fleetErrorEnvelope is Fleet's structured error body.
type fleetErrorEnvelope struct {
	Error struct {
		Code      string `json:"code"`
		Retryable bool   `json:"retryable"`
	} `json:"error"`
}

// classifyResponse turns a non-2xx Fleet response into a typed failure.
//
// The mapping deliberately refuses to guess: anything that is not clearly an
// authoritative rejection is treated as a dependency failure, because
// downgrading an unknown outcome into a rejection would deny a provider work
// they are actually entitled to.
func classifyResponse(status int, body []byte) *FleetError {
	var envelope fleetErrorEnvelope
	_ = json.Unmarshal(body, &envelope)
	code := strings.TrimSpace(envelope.Error.Code)

	if envelope.Error.Retryable {
		return &FleetError{Kind: providercompat.FailureDependency, Status: status, Code: code,
			Message: "Fleet is temporarily unavailable. Try again."}
	}

	switch {
	case status == http.StatusNotFound:
		return &FleetError{Kind: providercompat.FailureProviderMismatch, Status: status, Code: code,
			Message: "This corporate work is no longer available to you."}
	case status == http.StatusForbidden:
		return &FleetError{Kind: providercompat.FailureProviderMismatch, Status: status, Code: code,
			Message: "You are not authorized for this corporate work."}
	case status == http.StatusUnauthorized:
		// Our own service credentials are wrong. From the provider's side this
		// is an outage, not their fault, and it is not retryable by them.
		return &FleetError{Kind: providercompat.FailureDependency, Status: status, Code: code,
			Message: "Fleet is temporarily unavailable. Try again."}
	case status == http.StatusConflict:
		return &FleetError{Kind: providercompat.FailureOperatingMode, Status: status, Code: code,
			Message: "This corporate work was already resolved."}
	case status == http.StatusUnprocessableEntity || status == http.StatusBadRequest:
		return &FleetError{Kind: providercompat.FailureServiceIneligible, Status: status, Code: code,
			Message: "This corporate work cannot be actioned."}
	case status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		return &FleetError{Kind: providercompat.FailureDependency, Status: status, Code: code,
			Message: "Fleet is temporarily unavailable. Try again."}
	default:
		return &FleetError{Kind: providercompat.FailureDependency, Status: status, Code: code,
			Message: "Fleet is temporarily unavailable. Try again."}
	}
}

// AsAPIError maps any Fleet or managed-authorization failure onto the
// provider-facing API error.
//
// A dependency failure becomes a retryable 503; every authoritative rejection
// becomes a definite 4xx. Nothing here can produce a success, and nothing here
// can downgrade a managed operation into independent behaviour.
func AsAPIError(err error) error {
	if err == nil {
		return nil
	}
	var fleetErr *FleetError
	if errors.As(err, &fleetErr) {
		return apiErrorFor(fleetErr.Kind, fleetErr.Message, err)
	}
	var ctxErr *providercompat.ContextError
	if errors.As(err, &ctxErr) {
		return apiErrorFor(ctxErr.Kind, messageForKind(ctxErr.Kind), err)
	}
	if errors.Is(err, ErrFleetUnavailable) {
		return apperrors.Unavailable("Fleet provider work is temporarily unavailable.", err)
	}
	// Unclassified: treat as a dependency failure so an unknown cause can
	// never be mistaken for an authoritative rejection.
	return apperrors.Unavailable("Fleet provider work is temporarily unavailable.", err)
}

func apiErrorFor(kind providercompat.FailureKind, message string, err error) error {
	if strings.TrimSpace(message) == "" {
		message = messageForKind(kind)
	}
	switch kind {
	case providercompat.FailureDependency:
		return apperrors.Unavailable(message, err)
	case providercompat.FailureNotManaged, providercompat.FailureMembershipInvalid,
		providercompat.FailureServiceIneligible, providercompat.FailureProviderMismatch,
		providercompat.FailureOrganizationMismatch, providercompat.FailureOperatingMode:
		return apperrors.Forbidden(message, err)
	default:
		return apperrors.Unavailable(message, err)
	}
}

func messageForKind(kind providercompat.FailureKind) string {
	switch kind {
	case providercompat.FailureDependency:
		return "Fleet is temporarily unavailable. Try again."
	case providercompat.FailureNotManaged:
		return "This action is only available to organization-managed drivers."
	case providercompat.FailureMembershipInvalid:
		return "Your organization membership is not active."
	case providercompat.FailureServiceIneligible:
		return "Your organization membership does not cover taxi work."
	case providercompat.FailureProviderMismatch:
		return "You are not authorized for this corporate work."
	case providercompat.FailureOrganizationMismatch:
		return "This work belongs to a different organization."
	case providercompat.FailureOperatingMode:
		return "Your organization's operating mode does not permit this action."
	default:
		return "Fleet provider work is temporarily unavailable."
	}
}
