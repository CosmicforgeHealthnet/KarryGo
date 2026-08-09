package models

import "testing"

func TestValidScopeTypes_DoesNotIncludeDocument(t *testing.T) {
	if ValidScopeTypes["DOCUMENT"] {
		t.Fatal("DOCUMENT must not be a valid generic legal-hold scope type: document legal hold already has its own media-propagated path")
	}
}

func TestValidScopeTypes_IncludesSpecScopes(t *testing.T) {
	for _, scope := range []string{
		ScopeOrganization, ScopeCustomer, ScopeProvider, ScopeTrip, ScopeAssignment,
		ScopeIncident, ScopeSupportCase, ScopeInvoice, ScopeExport, ScopeAuditRange,
	} {
		if !ValidScopeTypes[scope] {
			t.Errorf("expected %q to be a valid scope type", scope)
		}
	}
}

func TestNormalizeScopeType(t *testing.T) {
	if got := NormalizeScopeType("  provider  "); got != "PROVIDER" {
		t.Fatalf("NormalizeScopeType lowercase+space = %q, want PROVIDER", got)
	}
}

func TestHold_Active(t *testing.T) {
	if !(Hold{Status: StatusActive}).Active() {
		t.Fatal("ACTIVE hold should report Active() == true")
	}
	if (Hold{Status: StatusReleased}).Active() {
		t.Fatal("RELEASED hold should report Active() == false")
	}
}
