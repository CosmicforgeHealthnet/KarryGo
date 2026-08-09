// Package redact is the single, centralized sensitive-key detector every
// logging and audit-summary path in this service must go through.
//
// Before this package existed, internal/observability/logging.go and
// internal/platform/audit.go each kept their own hand-maintained key list —
// observability's used an exact (non-substring) map lookup, so a caller
// logging a field named "access_token" or "apiKey" was never redacted even
// though "token"/"api_key" both being sensitive was the evident intent. One
// list, matched consistently, closes that drift permanently rather than
// requiring the next new sensitive-field name to be added correctly in two
// places at once.
package redact

import "strings"

// SensitiveKeySubstrings are matched case-insensitively as substrings of a
// field name (so "access_token", "refreshToken" and "Authorization-Header"
// all match), per Section 28's requirement to support common naming
// variants rather than exact key names only.
var SensitiveKeySubstrings = []string{
	"password", "passcode", "token", "authorization", "cookie", "secret",
	"api_key", "apikey", "otp", "mfa", "nin", "bvn", "bank",
	"account_number", "card", "cvv", "signed_url", "signedurl",
	"passport", "ssn", "document",
}

// IsSensitiveKey reports whether key should never appear unredacted in a log
// line, audit summary or error message.
func IsSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, needle := range SensitiveKeySubstrings {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}
