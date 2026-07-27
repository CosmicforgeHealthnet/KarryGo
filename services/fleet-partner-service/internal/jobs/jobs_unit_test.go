package jobs

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateTenantMatch(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"organizationId": "org_1"})
	if err := ValidateTenant(Job{OrganizationID: "org_1", Payload: payload}); err != nil {
		t.Fatalf("matching tenant rejected: %v", err)
	}
}

func TestValidateTenantMismatchFailsSafely(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"organizationId": "org_attacker"})
	err := ValidateTenant(Job{OrganizationID: "org_1", Payload: payload})
	if !errors.Is(err, ErrTenantMismatch) {
		t.Fatalf("expected ErrTenantMismatch, got %v", err)
	}

	// Row scoped but payload missing the organization: also a mismatch.
	err = ValidateTenant(Job{OrganizationID: "org_1", Payload: []byte(`{}`)})
	if !errors.Is(err, ErrTenantMismatch) {
		t.Fatalf("expected ErrTenantMismatch for missing payload org, got %v", err)
	}
}

func TestValidateTenantPlatformScoped(t *testing.T) {
	if err := ValidateTenant(Job{Payload: []byte(`{}`)}); err != nil {
		t.Fatalf("platform-scoped job rejected: %v", err)
	}
}

func TestRedactBoundsErrorText(t *testing.T) {
	long := make([]byte, 1000)
	for i := range long {
		long[i] = 'x'
	}
	if got := redact(errors.New(string(long))); len(got) > 300 {
		t.Fatalf("redacted error too long: %d", len(got))
	}
	if redact(nil) != "" {
		t.Fatal("nil error should redact to empty")
	}
}
