package jobs

import (
	"errors"
	"time"
)

// ErrorClass standardizes why an attempt failed, so retry behavior is a
// function of the class rather than ad hoc per-handler judgment calls.
type ErrorClass string

const (
	ErrorClassTransient              ErrorClass = "TRANSIENT"
	ErrorClassPermanent              ErrorClass = "PERMANENT"
	ErrorClassConflict               ErrorClass = "CONFLICT"
	ErrorClassDependencyUnavailable  ErrorClass = "DEPENDENCY_UNAVAILABLE"
	ErrorClassRateLimited            ErrorClass = "RATE_LIMITED"
	ErrorClassAuthentication         ErrorClass = "AUTHENTICATION"
	ErrorClassAuthorization          ErrorClass = "AUTHORIZATION"
	ErrorClassValidation             ErrorClass = "VALIDATION"
	ErrorClassUnknownExternalOutcome ErrorClass = "UNKNOWN_EXTERNAL_OUTCOME"
	ErrorClassLeaseLost              ErrorClass = "LEASE_LOST"
	ErrorClassCancelled              ErrorClass = "CANCELLED"
)

// Retryable reports whether a handler failure of this class should be
// retried automatically.
//
// Validation and permanent failures never retry on their own — the input or
// the code is wrong, and repeating the exact same call will fail the exact
// same way. Everything else may legitimately succeed on a later attempt.
func (c ErrorClass) Retryable() bool {
	switch c {
	case ErrorClassValidation, ErrorClassPermanent, ErrorClassAuthorization:
		return false
	default:
		return true
	}
}

// RequiresReadback reports whether this class means the external side
// effect's outcome is not actually known, and must be confirmed by an
// authoritative read-back before any further state-changing write is
// attempted — retrying blind risks a duplicate effect, and giving up blind
// risks silently losing one that already happened.
func (c ErrorClass) RequiresReadback() bool {
	return c == ErrorClassUnknownExternalOutcome
}

// ClassifiedError pairs a Go error with its Section 27 error class and a
// safe, credential-free message a handler wants recorded on the attempt.
// Handlers that want their failure classified return one of these instead of
// a bare error; a bare error defaults to ErrorClassTransient (retry), which
// is the safe default for an error nobody classified.
type ClassifiedError struct {
	Class       ErrorClass
	Code        string
	SafeMessage string
	// RetryAfter overrides exponential backoff for an upstream instruction
	// such as HTTP 429 Retry-After. Queue.Fail still bounds this value before
	// persisting it, so an external party cannot pause a job indefinitely.
	RetryAfter time.Duration
	Err        error
}

func (c *ClassifiedError) Error() string {
	if c.Err != nil {
		return c.Err.Error()
	}
	return c.SafeMessage
}

func (c *ClassifiedError) Unwrap() error { return c.Err }

// Classify extracts the error class, code and safe message from a handler
// error. A plain, unclassified error is treated as TRANSIENT (retry) with no
// code — never as PERMANENT, so a handler that has not been updated to
// classify its errors keeps the pre-Section-27 retry behavior rather than
// silently losing retries it used to get.
func Classify(err error) (ErrorClass, string, string) {
	if err == nil {
		return "", "", ""
	}
	var classified *ClassifiedError
	if ce, ok := err.(*ClassifiedError); ok {
		classified = ce
	}
	if classified != nil {
		class := classified.Class
		if class == "" {
			class = ErrorClassTransient
		}
		message := classified.SafeMessage
		if message == "" {
			message = redact(err)
		}
		return class, classified.Code, message
	}
	return ErrorClassTransient, "", redact(err)
}

// RetryDelay returns a bounded upstream retry delay when one was supplied.
func RetryDelay(err error) time.Duration {
	var classified *ClassifiedError
	if !errors.As(err, &classified) || classified.RetryAfter <= 0 {
		return 0
	}
	if classified.RetryAfter > 24*time.Hour {
		return 24 * time.Hour
	}
	return classified.RetryAfter
}
