package authusecases

import (
	"net/http"

	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	CodeOTPExpired     apperrors.Code = "otp_expired"
	CodeOTPInvalid     apperrors.Code = "otp_invalid"
	CodeSessionInvalid apperrors.Code = "session_invalid"
	CodeSessionExpired apperrors.Code = "session_expired"
	CodeTokenInvalid   apperrors.Code = "token_invalid"
	CodeTokenExpired   apperrors.Code = "token_expired"
)

func otpExpired(message string) *apperrors.Error {
	return apperrors.New(http.StatusUnauthorized, CodeOTPExpired, message, nil)
}

func otpInvalid(message string) *apperrors.Error {
	return apperrors.New(http.StatusUnauthorized, CodeOTPInvalid, message, nil)
}

func otpLocked(message string) *apperrors.Error {
	return apperrors.RateLimited(message, nil)
}

func sessionInvalid(message string) *apperrors.Error {
	return apperrors.New(http.StatusUnauthorized, CodeSessionInvalid, message, nil)
}

func sessionExpired(message string) *apperrors.Error {
	return apperrors.New(http.StatusUnauthorized, CodeSessionExpired, message, nil)
}

func tokenInvalid(message string) *apperrors.Error {
	return apperrors.New(http.StatusUnauthorized, CodeTokenInvalid, message, nil)
}

func tokenExpired(message string) *apperrors.Error {
	return apperrors.New(http.StatusUnauthorized, CodeTokenExpired, message, nil)
}

func refreshSessionUnauthorized() *apperrors.Error {
	return apperrors.Unauthorized("Invalid or expired session.", nil)
}

func logoutSessionUnauthorized() *apperrors.Error {
	return apperrors.Unauthorized("Access token is invalid.", nil)
}
