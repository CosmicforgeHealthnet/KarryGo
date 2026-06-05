package authmodels

import (
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"
)

func ParseRefreshTokenSessionID(refreshToken string) (string, error) {
	if refreshToken == "" {
		return "", apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "refresh_token", Message: "Refresh token is required."},
		})
	}

	parts := strings.SplitN(refreshToken, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", apperrors.Unauthorized("Your session has expired. Please sign in again.", nil)
	}

	return parts[0], nil
}
