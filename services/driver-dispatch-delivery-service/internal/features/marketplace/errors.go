package marketplace

import (
	"net/http"

	"cosmicforge/logistics/shared/go/apperrors"
)

// errCoded builds a stable-coded API error.
//
// The code is part of the cross-service contract: fleet branches on it to tell
// a definite conflict apart from a retryable failure, so these strings must
// stay stable even if the human-readable message changes.
func errCoded(status int, code string, message string) error {
	return apperrors.New(status, apperrors.Code(code), message, nil)
}

func errRequestAlreadyClaimed() error {
	return errCoded(http.StatusConflict, "request_already_claimed",
		"Another organization has already claimed this request.")
}

func errIdempotencyConflict() error {
	return errCoded(http.StatusConflict, "idempotency_conflict",
		"This idempotency key was already used with a different request.")
}
