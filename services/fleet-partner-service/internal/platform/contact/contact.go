// Package contact normalizes and hashes the email/phone identity pair used by
// corporate passwordless authentication.
//
// Both an email address and a phone number are required to start any
// corporate authentication flow. Email is lowercased and trimmed for
// comparison; phone is normalized to E.164. Hashes are keyed so that a
// leaked table cannot be used to enumerate accounts by rainbow table.
//
// The shared repository helper `phonenumber.NormalizeNigerianPhoneNumber` is
// Nigeria-specific; corporate identities may be registered from any operating
// country, so general E.164 normalization lives here. It is intentionally not
// promoted to shared/go — no other service needs it yet.
package contact

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// Pair is a normalized email + phone identity pair.
type Pair struct {
	EmailNormalized string
	EmailDisplay    string
	PhoneE164       string
	PhoneDisplay    string
}

var (
	emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$`)
	digitsOnly   = regexp.MustCompile(`[^0-9]`)
)

// NormalizeEmail trims, lowercases and validates an email address.
func NormalizeEmail(raw string) (normalized string, display string, err error) {
	display = strings.TrimSpace(raw)
	if display == "" {
		return "", "", fmt.Errorf("contact: email is required")
	}
	if len(display) > 254 {
		return "", "", fmt.Errorf("contact: email is too long")
	}
	normalized = strings.ToLower(display)
	if !emailPattern.MatchString(normalized) {
		return "", "", fmt.Errorf("contact: email is not a valid address")
	}
	return normalized, display, nil
}

// NormalizePhone converts a phone number to E.164.
//
// Accepted inputs: "+<country><subscriber>", or a national number with a
// leading 0 combined with defaultCountryCode (for example "0801..." + "234").
// The result is always "+" followed by 8–15 digits.
func NormalizePhone(raw string, defaultCountryCode string) (e164 string, display string, err error) {
	display = strings.TrimSpace(raw)
	if display == "" {
		return "", "", fmt.Errorf("contact: phone number is required")
	}

	trimmed := strings.ReplaceAll(display, " ", "")
	trimmed = strings.ReplaceAll(trimmed, "-", "")
	trimmed = strings.ReplaceAll(trimmed, "(", "")
	trimmed = strings.ReplaceAll(trimmed, ")", "")

	var digits string
	switch {
	case strings.HasPrefix(trimmed, "+"):
		digits = digitsOnly.ReplaceAllString(trimmed[1:], "")
	case strings.HasPrefix(trimmed, "00"):
		digits = digitsOnly.ReplaceAllString(trimmed[2:], "")
	case strings.HasPrefix(trimmed, "0"):
		country := digitsOnly.ReplaceAllString(defaultCountryCode, "")
		if country == "" {
			return "", "", fmt.Errorf("contact: phone number must be in international format")
		}
		digits = country + digitsOnly.ReplaceAllString(trimmed[1:], "")
	default:
		digits = digitsOnly.ReplaceAllString(trimmed, "")
	}

	if len(digits) < 8 || len(digits) > 15 {
		return "", "", fmt.Errorf("contact: phone number is not a valid international number")
	}
	return "+" + digits, display, nil
}

// NormalizePair normalizes both halves of the identity pair.
func NormalizePair(email string, phone string, defaultCountryCode string) (Pair, error) {
	emailNormalized, emailDisplay, err := NormalizeEmail(email)
	if err != nil {
		return Pair{}, err
	}
	phoneE164, phoneDisplay, err := NormalizePhone(phone, defaultCountryCode)
	if err != nil {
		return Pair{}, err
	}
	return Pair{
		EmailNormalized: emailNormalized,
		EmailDisplay:    emailDisplay,
		PhoneE164:       phoneE164,
		PhoneDisplay:    phoneDisplay,
	}, nil
}

// Hash produces a keyed digest of a normalized contact value, used for
// rate-limit keys, security events and challenge lookups.
func Hash(secret []byte, value string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

// MaskEmail renders an email for safe display, e.g. "o***@example.com".
func MaskEmail(normalized string) string {
	at := strings.LastIndex(normalized, "@")
	if at <= 0 {
		return "***"
	}
	local, domain := normalized[:at], normalized[at+1:]
	if len(local) == 1 {
		return local + "***@" + domain
	}
	return local[:1] + strings.Repeat("*", 3) + "@" + domain
}
