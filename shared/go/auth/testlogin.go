package auth

import (
	"fmt"
	"strings"
)

// TestLoginPrefix is the only number block test sign-in accepts. +234 800
// numbers are toll-free lines, never personal mobiles, so a fixed code can
// never open a real person's account even if a number is mistyped here.
const TestLoginPrefix = "+23480000"

// TestLogin lets a short list of reserved test phone numbers sign in with one
// fixed code, so testers can use a staging environment without an SMS
// provider. Every other number still gets a random code. It is meant for
// staging only and refuses to start in production.
type TestLogin struct {
	code    string
	numbers map[string]bool
}

// ParseTestLogin builds the switch from a comma-separated list of numbers and
// a code. Empty numbers mean the switch is off (nil, no error). normalize
// turns each number into the form the service stores (e.g. +234...).
func ParseTestLogin(appEnv, numbersCSV, code string, normalize func(string) (string, error)) (*TestLogin, error) {
	numbersCSV = strings.TrimSpace(numbersCSV)
	if numbersCSV == "" {
		return nil, nil
	}
	if strings.EqualFold(strings.TrimSpace(appEnv), "production") {
		return nil, fmt.Errorf("test sign-in numbers must not be set in production")
	}
	code = strings.TrimSpace(code)
	if len(code) != DefaultOTPLength || strings.Trim(code, "0123456789") != "" {
		return nil, fmt.Errorf("test sign-in code must be exactly %d digits", DefaultOTPLength)
	}
	t := &TestLogin{code: code, numbers: map[string]bool{}}
	for _, raw := range strings.Split(numbersCSV, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		number, err := normalize(raw)
		if err != nil {
			return nil, fmt.Errorf("test sign-in number %q: %w", raw, err)
		}
		if !strings.HasPrefix(number, TestLoginPrefix) {
			return nil, fmt.Errorf("test sign-in number %s is outside the reserved %s block", number, TestLoginPrefix)
		}
		t.numbers[number] = true
	}
	return t, nil
}

// CodeFor returns the fixed code when (identifierType, value) is a listed
// test phone number. Safe to call on a nil TestLogin.
func (t *TestLogin) CodeFor(identifierType, value string) (string, bool) {
	if t == nil || identifierType != "phone" || !t.numbers[value] {
		return "", false
	}
	return t.code, true
}

// Count is how many test numbers are listed (for a startup log line).
func (t *TestLogin) Count() int {
	if t == nil {
		return 0
	}
	return len(t.numbers)
}
