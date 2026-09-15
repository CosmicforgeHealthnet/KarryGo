package logging

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"
)

// fieldRedactor is an extra rule a domain package contributes: given a field
// name, report whether its value must be concealed.
//
// The hook exists so this package keeps knowing nothing about any domain.
// The rule that matters in practice is corporate financial privacy: a field
// called organizationShareMinor is not secret-SOUNDING, so the word list
// below passes it, and it is a plain integer, so the type check passes it
// too — yet logging it leaks exactly what Section 14 spends a whole
// visibility policy concealing from providers. The package that owns that
// classification registers it; this one only asks.
type fieldRedactor func(name string) bool

var (
	redactorsMu sync.RWMutex
	redactors   []fieldRedactor
)

// RegisterFieldRedactor adds a concealment rule. Call it from an init in the
// package that owns the classification. It is additive and never removes a
// rule, so a caller cannot widen what gets logged.
func RegisterFieldRedactor(rule func(name string) bool) {
	if rule == nil {
		return
	}
	redactorsMu.Lock()
	defer redactorsMu.Unlock()
	redactors = append(redactors, rule)
}

// registeredRedaction reports whether any registered rule conceals the field.
func registeredRedaction(name string) bool {
	redactorsMu.RLock()
	defer redactorsMu.RUnlock()
	for _, rule := range redactors {
		if rule(name) {
			return true
		}
	}
	return false
}

// SafeFields extends the existing platform logger. It never serializes opaque
// structs, errors or Stringers, which may embed secret-bearing request bodies.
func SafeFields(fields map[string]any) map[string]any {
	out := make(map[string]any, len(fields))
	for key, value := range fields {
		k := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
		sensitive := false
		for _, word := range []string{"otp", "qr", "credential", "secret", "hash", "password", "passcode", "authorization", "token", "cookie", "apikey", "phone", "email", "address", "body", "payload", "header", "url", "bank", "card"} {
			if strings.Contains(k, word) {
				sensitive = true
				break
			}
		}
		if !sensitive {
			sensitive = registeredRedaction(key)
		}
		if sensitive {
			out[key] = "[REDACTED]"
			continue
		}
		switch v := value.(type) {
		case map[string]any:
			out[key] = SafeFields(v)
		case string, bool, int, int64, uint64, float64, nil:
			out[key] = v
		default:
			out[key] = "[REDACTED]"
		}
	}
	return out
}

func Structured(level, event string, fields map[string]any) {
	entry := SafeFields(fields)
	entry["level"], entry["event"], entry["timestamp"] = level, event, time.Now().UTC().Format(time.RFC3339Nano)
	line, err := json.Marshal(entry)
	if err != nil {
		log.Print(`{"level":"error","event":"log_encoding_failed"}`)
		return
	}
	log.Print(string(line))
}
