package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestAuthoritativeCaptureMetadataMigration(t *testing.T) {
	raw, err := os.ReadFile("000003_authoritative_capture_metadata.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, field := range []string{"captured_at", "source_timestamp", "capture_method", "device_reference", "actor_reference", "original_mime_type", "original_size_bytes", "integrity_hash"} {
		if !strings.Contains(text, field) {
			t.Errorf("missing %s", field)
		}
	}
}
