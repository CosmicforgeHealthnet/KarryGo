package phonenumber

import (
	"regexp"
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"
)

var nonDigitPhoneChars = regexp.MustCompile(`[^\d+]`)

func NormalizeNigerianPhoneNumber(raw string) (string, error) {
	phone := strings.TrimSpace(raw)
	phone = nonDigitPhoneChars.ReplaceAllString(phone, "")

	if strings.HasPrefix(phone, "+234") && len(phone) == 14 {
		return phone, nil
	}
	if strings.HasPrefix(phone, "234") && len(phone) == 13 {
		return "+" + phone, nil
	}
	if strings.HasPrefix(phone, "0") && len(phone) == 11 {
		return "+234" + phone[1:], nil
	}

	return "", apperrors.Validation("Check your details.", []apperrors.FieldViolation{
		{Field: "phone", Message: "Enter a valid Nigerian phone number."},
	})
}
