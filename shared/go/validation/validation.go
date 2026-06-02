package validation

import "karrygo/shared/go/apperrors"

type FieldError = apperrors.FieldViolation

func Required(field string, label string) FieldError {
	if label == "" {
		label = field
	}

	return FieldError{
		Field:   field,
		Message: label + " is required.",
	}
}
