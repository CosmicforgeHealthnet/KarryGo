package delivery

import "net/http"

// FieldError is a single field-level validation failure.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// APIError is a Partner API error carrying an HTTP status, a stable code, a
// human message, and optional field errors. Service methods return *APIError so
// the HTTP layer can translate them into the standard Partner API envelope.
type APIError struct {
	Status  int
	Code    string
	Message string
	Fields  []FieldError
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

func newValidationError(fields []FieldError) *APIError {
	return &APIError{
		Status:  http.StatusBadRequest,
		Code:    "validation_error",
		Message: "Request validation failed.",
		Fields:  fields,
	}
}

func errIdempotencyConflict() *APIError {
	return &APIError{
		Status:  http.StatusConflict,
		Code:    "idempotency_conflict",
		Message: "This idempotency key was already used with a different request.",
		Fields:  []FieldError{},
	}
}

func errBookingNotFound() *APIError {
	return &APIError{
		Status:  http.StatusNotFound,
		Code:    "booking_not_found",
		Message: "No delivery was found for this account.",
		Fields:  []FieldError{},
	}
}

func errCannotCancel() *APIError {
	return &APIError{
		Status:  http.StatusConflict,
		Code:    "cannot_cancel",
		Message: "This delivery can no longer be cancelled.",
		Fields:  []FieldError{},
	}
}

func errPaymentFailed() *APIError {
	return &APIError{
		Status:  http.StatusPaymentRequired,
		Code:    "payment_failed",
		Message: "Payment could not be completed for this delivery.",
		Fields:  []FieldError{},
	}
}

func errDispatchUnavailable() *APIError {
	return &APIError{
		Status:  http.StatusServiceUnavailable,
		Code:    "dispatch_unavailable",
		Message: "The dispatch service is temporarily unavailable. Please retry.",
		Fields:  []FieldError{},
	}
}

// ErrInternal is the generic fallback error used by the HTTP layer when a
// non-APIError is returned.
func ErrInternal() *APIError {
	return &APIError{
		Status:  http.StatusInternalServerError,
		Code:    "internal_error",
		Message: "Something went wrong. Please try again.",
		Fields:  []FieldError{},
	}
}

// --- exported constructors (shared by the webhooks feature, GW-G/GW-I) -------
// These let other partner sub-features return the same *APIError type so every
// Partner API error is rendered through one consistent envelope.

// NewAPIError builds an arbitrary Partner API error.
func NewAPIError(status int, code, message string, fields []FieldError) *APIError {
	if fields == nil {
		fields = []FieldError{}
	}
	return &APIError{Status: status, Code: code, Message: message, Fields: fields}
}

// ValidationError builds a 400 validation_error with field details.
func ValidationError(fields []FieldError) *APIError { return newValidationError(fields) }

// WebhookNotFound builds a 404 webhook_not_found error.
func WebhookNotFound() *APIError {
	return &APIError{
		Status:  http.StatusNotFound,
		Code:    "webhook_not_found",
		Message: "No webhook was found for this account.",
		Fields:  []FieldError{},
	}
}

// WebhookDeliveryFailed builds a 502 webhook_delivery_failed error.
func WebhookDeliveryFailed(message string) *APIError {
	if message == "" {
		message = "The webhook endpoint did not accept the delivery."
	}
	return &APIError{
		Status:  http.StatusBadGateway,
		Code:    "webhook_delivery_failed",
		Message: message,
		Fields:  []FieldError{},
	}
}
