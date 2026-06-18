package authmodels

import (
	"regexp"
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/phonenumber"
)

const (
	IdentifierTypePhone = "phone"
	IdentifierTypeEmail = "email"
)

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type AuthIdentifier struct {
	Type  string
	Value string
}

func NormalizeAuthIdentifier(phone string, email string) (AuthIdentifier, error) {
	phone = strings.TrimSpace(phone)
	email = strings.TrimSpace(email)

	if phone != "" && email != "" {
		return AuthIdentifier{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "identifier", Message: "Enter either a phone number or email address."},
		})
	}
	if phone == "" && email == "" {
		return AuthIdentifier{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "identifier", Message: "Phone number or email address is required."},
		})
	}

	if phone != "" {
		normalized, err := phonenumber.NormalizeNigerianPhoneNumber(phone)
		if err != nil {
			return AuthIdentifier{}, err
		}
		return AuthIdentifier{Type: IdentifierTypePhone, Value: normalized}, nil
	}

	normalized := strings.ToLower(email)
	if !emailPattern.MatchString(normalized) {
		return AuthIdentifier{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "email", Message: "Enter a valid email address."},
		})
	}

	return AuthIdentifier{Type: IdentifierTypeEmail, Value: normalized}, nil
}

func (i AuthIdentifier) Key() string {
	return i.Type + ":" + i.Value
}
