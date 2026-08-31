package logging

import (
	"encoding/json"
	"log"
	"strings"
	"time"
)

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
