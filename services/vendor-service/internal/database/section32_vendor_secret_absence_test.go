package database

import (
	"os"
	"strings"
	"testing"
)

func TestSection32VendorProjectionCannotStoreHandoverSecrets(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/000113_section32_handover_projection.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	finalRaw, err := os.ReadFile("../../migrations/000115_section32_vendor_owned_handover_projection.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.ToLower(string(raw) + string(finalRaw))
	for _, forbidden := range []string{"otp_hash", "qr_hash", "qr_token", "credential", "attempt_count", "resend_count", "handover_secret"} {
		if strings.Contains(schema, forbidden) {
			t.Fatalf("Vendor Section 32 projection contains forbidden authority field %q", forbidden)
		}
	}
	for _, required := range []string{"handover_ref", "source_version", "authority_mission_ref"} {
		if !strings.Contains(schema, required) {
			t.Fatalf("Vendor projection missing opaque field %q", required)
		}
	}
}
