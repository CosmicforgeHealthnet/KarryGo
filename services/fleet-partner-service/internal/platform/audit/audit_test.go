package audit

import (
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

func TestRedactMasksSensitiveKeys(t *testing.T) {
	redacted := Redact(map[string]any{
		"legal_name":          "Acme Logistics",
		"bank_account_number": "0123456789",
		"api_token":           "secret-value",
		"document_number":     "A1234567",
	})
	if redacted["legal_name"] != "Acme Logistics" {
		t.Fatalf("non-sensitive value altered: %v", redacted["legal_name"])
	}
	for _, key := range []string{"bank_account_number", "api_token", "document_number"} {
		if redacted[key] != "[REDACTED]" {
			t.Fatalf("%s not redacted: %v", key, redacted[key])
		}
	}
}

func TestEntryFromTenant(t *testing.T) {
	entry := EntryFromTenant(tenant.Context{
		OrganizationID: "org_1",
		ActorType:      tenant.ActorTypeOrganizationUser,
		ActorID:        "usr_1",
		RequestID:      "req_1",
		CorrelationID:  "cor_1",
	}, "organization.suspend", "organization", "org_1")

	if entry.OrganizationID != "org_1" || entry.ActorID != "usr_1" ||
		entry.RequestID != "req_1" || entry.CorrelationID != "cor_1" {
		t.Fatalf("entry = %+v", entry)
	}
}
