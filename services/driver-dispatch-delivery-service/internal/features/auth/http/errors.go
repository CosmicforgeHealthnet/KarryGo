package authhttp

import (
	"net/http"

	"karrygo/shared/go/apperrors"
)

const (
	CodeValidationFailed   apperrors.Code = "validation_failed"
	CodeUnauthorized       apperrors.Code = "unauthorized"
	CodeForbidden          apperrors.Code = "forbidden"
	CodeRateLimited        apperrors.Code = "rate_limited"
	CodeServiceUnavailable apperrors.Code = "service_unavailable"
	CodeInternalError      apperrors.Code = "internal_error"
	CodeOTPExpired         apperrors.Code = "otp_expired"
	CodeOTPInvalid         apperrors.Code = "otp_invalid"
	CodeSessionInvalid     apperrors.Code = "session_invalid"
	CodeSessionExpired     apperrors.Code = "session_expired"
	CodeTokenInvalid       apperrors.Code = "token_invalid"
	CodeTokenExpired       apperrors.Code = "token_expired"
)

func ServiceUnavailable(message string, cause error) *apperrors.Error {
	return apperrors.New(http.StatusServiceUnavailable, CodeServiceUnavailable, message, cause)
}

func TokenInvalid(message string) *apperrors.Error {
	return apperrors.New(http.StatusUnauthorized, CodeTokenInvalid, message, nil)
}

func TokenExpired(message string) *apperrors.Error {
	return apperrors.New(http.StatusUnauthorized, CodeTokenExpired, message, nil)
}
