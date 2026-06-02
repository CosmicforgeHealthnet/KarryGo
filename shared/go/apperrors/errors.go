package apperrors

import (
	"errors"
	"net/http"
)

type Code string

const (
	CodeBadRequest       Code = "bad_request"
	CodeValidationFailed Code = "validation_failed"
	CodeUnauthorized     Code = "unauthorized"
	CodeForbidden        Code = "forbidden"
	CodeNotFound         Code = "not_found"
	CodeConflict         Code = "conflict"
	CodeRateLimited      Code = "rate_limited"
	CodeUnavailable      Code = "service_unavailable"
	CodeInternal         Code = "internal_error"
)

type FieldViolation struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type Error struct {
	Code    Code                   `json:"code"`
	Message string                 `json:"message"`
	Status  int                    `json:"-"`
	Fields  []FieldViolation       `json:"fields,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
	Cause   error                  `json:"-"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}

	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Cause
}

func New(status int, code Code, message string, cause error) *Error {
	if status == 0 {
		status = http.StatusInternalServerError
	}

	if code == "" {
		code = CodeInternal
	}

	if message == "" {
		message = "Something went wrong. Please try again."
	}

	return &Error{
		Code:    code,
		Message: message,
		Status:  status,
		Cause:   cause,
	}
}

func BadRequest(message string, cause error) *Error {
	return New(http.StatusBadRequest, CodeBadRequest, message, cause)
}

func Validation(message string, fields []FieldViolation) *Error {
	err := New(http.StatusUnprocessableEntity, CodeValidationFailed, message, nil)
	err.Fields = fields
	return err
}

func Unauthorized(message string, cause error) *Error {
	return New(http.StatusUnauthorized, CodeUnauthorized, message, cause)
}

func Forbidden(message string, cause error) *Error {
	return New(http.StatusForbidden, CodeForbidden, message, cause)
}

func NotFound(message string, cause error) *Error {
	return New(http.StatusNotFound, CodeNotFound, message, cause)
}

func Conflict(message string, cause error) *Error {
	return New(http.StatusConflict, CodeConflict, message, cause)
}

func RateLimited(message string, cause error) *Error {
	return New(http.StatusTooManyRequests, CodeRateLimited, message, cause)
}

func Unavailable(message string, cause error) *Error {
	return New(http.StatusServiceUnavailable, CodeUnavailable, message, cause)
}

func Internal(message string, cause error) *Error {
	return New(http.StatusInternalServerError, CodeInternal, message, cause)
}

func From(err error) *Error {
	if err == nil {
		return nil
	}

	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr
	}

	return Internal("Something went wrong. Please try again.", err)
}
