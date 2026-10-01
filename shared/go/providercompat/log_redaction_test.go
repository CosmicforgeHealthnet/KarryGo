package providercompat

import (
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/logging"
)

// An organization's margin must not reach a log line. It is not
// secret-sounding and it is a plain integer, so neither of the logger's own
// rules catches it; only the financial registry knows what it is.
func TestOrganizationAndPlatformEconomicsAreConcealedInLogs(t *testing.T) {
	fields := logging.SafeFields(map[string]any{
		"organizationShareMinor": 45000,
		"organization_gross":     120000,
		"platformShareMinor":     5000,
		"platform_margin_minor":  5000,
	})
	for name, value := range fields {
		if value != "[REDACTED]" {
			t.Fatalf("%s reached the log as %v", name, value)
		}
	}
}

// The redaction must stay narrow. A log that conceals the identifiers and
// outcomes an operator needs is a log nobody keeps, and the corporate
// economics would then leak through whatever people use instead.
func TestOperationalFieldsAndProviderOwnFiguresStillLog(t *testing.T) {
	fields := logging.SafeFields(map[string]any{
		"service":            "driver-hauling-service",
		"organizationId":     "org_123",
		"membershipId":       "mem_456",
		"outcome":            "delivered",
		"attempt":            3,
		"providerShareMinor": 38000,
		"ownEarningMinor":    38000,
	})
	for _, name := range []string{"service", "organizationId", "membershipId", "outcome", "attempt"} {
		if fields[name] == "[REDACTED]" {
			t.Fatalf("%s was concealed; an operator cannot trace anything without it", name)
		}
	}
	// The provider's own share is theirs to see. Concealing it here would be
	// a different bug from leaking the organization's.
	if fields["providerShareMinor"] == "[REDACTED]" || fields["ownEarningMinor"] == "[REDACTED]" {
		t.Fatalf("the provider's own figures were concealed: %+v", fields)
	}
}

// Nesting is where a redactor usually fails: the top level is checked and the
// map inside it is not.
func TestConcealmentReachesNestedFields(t *testing.T) {
	fields := logging.SafeFields(map[string]any{
		"settlement": map[string]any{
			"providerShareMinor":     38000,
			"organizationShareMinor": 45000,
		},
	})
	nested, ok := fields["settlement"].(map[string]any)
	if !ok {
		t.Fatalf("the nested map was replaced wholesale: %T", fields["settlement"])
	}
	if nested["organizationShareMinor"] != "[REDACTED]" {
		t.Fatalf("a nested organization share reached the log: %v", nested["organizationShareMinor"])
	}
	if nested["providerShareMinor"] == "[REDACTED]" {
		t.Fatal("a nested provider share was concealed")
	}
}

// The registry's own normalisation must apply, so a field is caught however
// it is spelled.
func TestSpellingDoesNotDefeatConcealment(t *testing.T) {
	for _, spelling := range []string{
		"organizationShareMinor", "organization_share_minor",
		"organization-share-minor", "ORGANIZATIONSHAREMINOR",
	} {
		fields := logging.SafeFields(map[string]any{spelling: 45000})
		if fields[spelling] != "[REDACTED]" {
			t.Fatalf("%q reached the log as %v", spelling, fields[spelling])
		}
	}
	if !strings.Contains("[REDACTED]", "REDACTED") {
		t.Fatal("placeholder changed")
	}
}
