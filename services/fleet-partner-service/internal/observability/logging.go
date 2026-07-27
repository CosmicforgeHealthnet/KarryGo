package observability

import (
	"encoding/json"
	"log"
	"strings"
	"time"
)

// Sensitive field names that must never be logged; Log drops them defensively.
var redactedFields = map[string]bool{
	"authorization": true,
	"cookie":        true,
	"secret":        true,
	"token":         true,
	"password":      true,
	"refresh_token": true,
}

// Log writes one structured JSON log line. Values for sensitive keys are
// replaced with "[REDACTED]". Use low-cardinality, non-sensitive fields only.
func Log(service string, environment string, level string, message string, fields map[string]any) {
	entry := map[string]any{
		"timestamp":   time.Now().UTC().Format(time.RFC3339Nano),
		"level":       level,
		"service":     service,
		"environment": environment,
		"message":     message,
	}
	for k, v := range fields {
		if redactedFields[strings.ToLower(k)] {
			entry[k] = "[REDACTED]"
			continue
		}
		entry[k] = v
	}
	line, err := json.Marshal(entry)
	if err != nil {
		log.Printf(`{"level":"error","message":"log marshal failed"}`)
		return
	}
	log.Print(string(line))
}
