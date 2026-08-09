package observability

import (
	"encoding/json"
	"log"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/redact"
)

// Log writes one structured JSON log line. Values for sensitive keys
// (internal/platform/redact.IsSensitiveKey) are replaced with "[REDACTED]".
// Use low-cardinality, non-sensitive fields only.
func Log(service string, environment string, level string, message string, fields map[string]any) {
	entry := map[string]any{
		"timestamp":   time.Now().UTC().Format(time.RFC3339Nano),
		"level":       level,
		"service":     service,
		"environment": environment,
		"message":     message,
	}
	for k, v := range fields {
		if redact.IsSensitiveKey(k) {
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
