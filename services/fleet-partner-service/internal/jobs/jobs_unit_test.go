package jobs

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestValidateTenantMatch(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"organizationId": "org_1"})
	if err := ValidateTenant(Job{OrganizationID: "org_1", Payload: payload}); err != nil {
		t.Fatalf("matching tenant rejected: %v", err)
	}
}

func TestRetryDelayHonorsAndBoundsUpstreamRetryAfter(t *testing.T) {
	if got := RetryDelay(errors.New("plain")); got != 0 {
		t.Fatalf("plain error retry delay = %s, want zero", got)
	}
	if got := RetryDelay(&ClassifiedError{Class: ErrorClassRateLimited, RetryAfter: 45 * time.Second}); got != 45*time.Second {
		t.Fatalf("retry delay = %s, want 45s", got)
	}
	if got := RetryDelay(&ClassifiedError{Class: ErrorClassRateLimited, RetryAfter: 72 * time.Hour}); got != 24*time.Hour {
		t.Fatalf("unbounded retry delay = %s, want 24h", got)
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
