package providercompat

// Section 14 VIS-017 / VIS-018: keeping concealed financial fields out of
// notification payloads and logs.
//
// Both surfaces share one weakness: they take free-form maps. A notification's
// data bag and a structured log's fields are convenient precisely because
// anything can be put in them, which means a field the provider API takes care
// to withhold can reach a provider's phone, or an operator's log search, by a
// route nobody classified.
//
// The registry that decides what a provider may see is reused here rather than
// duplicated, so a field classified once is protected everywhere.

import (
	"sort"
	"strings"
)

// RedactedPlaceholder is what a concealed value is replaced with.
//
// A marker rather than deletion: an operator reading a log needs to know a
// field was present and withheld, not be left wondering whether the producer
// forgot to set it.
const RedactedPlaceholder = "[redacted]"

// ConcealedFinancialFields returns the keys in a payload that a provider must
// never see.
//
// Nested maps are walked, because a data bag whose top level is clean and whose
// nested "settlement" object carries the organization's share is exactly the
// shape that passes a shallow check.
func ConcealedFinancialFields(payload map[string]any) []string {
	var found []string
	collectConcealedFields("", payload, &found)
	sort.Strings(found)
	return found
}

func collectConcealedFields(prefix string, payload map[string]any, found *[]string) {
	for key, value := range payload {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if class, classified := ClassifyFinancialField(key); classified && class.AlwaysConcealed() {
			*found = append(*found, path)
			continue
		}
		switch nested := value.(type) {
		case map[string]any:
			collectConcealedFields(path, nested, found)
		case []any:
			for _, element := range nested {
				if elementMap, isMap := element.(map[string]any); isMap {
					collectConcealedFields(path, elementMap, found)
				}
			}
		}
	}
}

// RedactConcealedFinancialFields returns a copy of a payload with every
// concealed financial field replaced.
//
// A copy, so a caller that logs a redacted view and then acts on the original
// is not silently working with blanked-out figures.
func RedactConcealedFinancialFields(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	redacted := make(map[string]any, len(payload))
	for key, value := range payload {
		if class, classified := ClassifyFinancialField(key); classified && class.AlwaysConcealed() {
			redacted[key] = RedactedPlaceholder
			continue
		}
		switch nested := value.(type) {
		case map[string]any:
			redacted[key] = RedactConcealedFinancialFields(nested)
		case []any:
			elements := make([]any, len(nested))
			for index, element := range nested {
				if elementMap, isMap := element.(map[string]any); isMap {
					elements[index] = RedactConcealedFinancialFields(elementMap)
					continue
				}
				elements[index] = element
			}
			redacted[key] = elements
		default:
			redacted[key] = value
		}
	}
	return redacted
}

// SecretFieldKeys are the credential-bearing names that must never be logged,
// whatever else a log line carries.
//
// Separate from the financial registry because the harm is different: a leaked
// organization share is a privacy failure, a leaked signing secret is an
// authentication failure.
var secretFieldKeys = map[string]bool{
	"authorization": true, "authorisation": true,
	"xservicesignature": true, "servicesignature": true, "signature": true,
	"xservicesecret": true, "servicesecret": true, "secret": true,
	"token": true, "accesstoken": true, "refreshtoken": true, "bearer": true,
	"password": true, "pin": true, "pinhash": true, "otp": true, "otpcode": true,
	"apikey": true, "privatekey": true, "hmac": true, "hmackey": true,
	"cookie": true, "setcookie": true,
}

// IsSecretField reports whether a field name carries a credential.
func IsSecretField(name string) bool {
	return secretFieldKeys[normalizeFieldName(name)]
}

// RedactLogFields prepares a map for structured logging.
//
// It removes credentials and conceals organization and platform economics,
// while leaving everything an operator actually needs -- ids, statuses, the
// provider's own figures. A redactor that blanked the whole payload would be
// obeyed by nobody, and a log nobody keeps is not a protection.
func RedactLogFields(fields map[string]any) map[string]any {
	if fields == nil {
		return nil
	}
	redacted := make(map[string]any, len(fields))
	for key, value := range fields {
		if IsSecretField(key) {
			redacted[key] = RedactedPlaceholder
			continue
		}
		if class, classified := ClassifyFinancialField(key); classified && class.AlwaysConcealed() {
			redacted[key] = RedactedPlaceholder
			continue
		}
		switch nested := value.(type) {
		case map[string]any:
			redacted[key] = RedactLogFields(nested)
		default:
			redacted[key] = value
		}
	}
	return redacted
}

// normalizeFieldName matches the registry's own normalisation so a field is
// recognised however it is spelled.
func normalizeFieldName(name string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(strings.TrimSpace(name)) {
		if character == '_' || character == '-' || character == ' ' || character == '.' {
			continue
		}
		builder.WriteRune(character)
	}
	return builder.String()
}
