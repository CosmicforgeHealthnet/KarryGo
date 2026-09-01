package securehandover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecureHandoverMigrationSecurityShape(t *testing.T) {
	path := filepath.Join("..", "..", "..", "migrations", "000047_secure_handover_chain_of_custody.up.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	required := []string{
		"create table dispatch_handover_challenges",
		"dispatch_handover_one_active_context",
		"dispatch_handover_one_active_stage_context",
		"create table dispatch_handover_attempts",
		"create table dispatch_evidence_capture_sessions",
		"create table dispatch_parcel_scan_events",
		"create table dispatch_evidence_refs",
		"dispatch_reject_chain_of_custody_mutation",
		"dispatch_trip_handover_identity_guard",
		"before update or delete on dispatch_evidence_refs",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Errorf("migration missing %q", fragment)
		}
	}
	for _, forbidden := range []string{"otp_plaintext", "raw_otp", "qr_plaintext", "raw_qr"} {
		if strings.Contains(sql, forbidden) {
			t.Errorf("migration contains forbidden credential field %q", forbidden)
		}
	}
}
