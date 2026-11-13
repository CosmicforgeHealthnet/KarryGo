package orgprovider

import "testing"

func TestStateMachine(t *testing.T) {
	cases := []struct {
		from, to string
		any, org bool
	}{
		{StateRequested, StateActive, true, false},
		{StateRequested, StateRejected, true, false},
		{StateRequested, StateRevoked, true, true},
		{StateActive, StateSuspended, true, true},
		{StateActive, StateRevoked, true, true},
		{StateSuspended, StateActive, true, true},
		{StateActive, StateRequested, false, false},
		{StateRevoked, StateActive, false, false},
		{StateRejected, StateActive, false, false},
		{"requested", "active", true, false},
	}
	for _, c := range cases {
		if got := CanTransition(c.from, c.to); got != c.any {
			t.Fatalf("CanTransition(%s,%s)=%v want %v", c.from, c.to, got, c.any)
		}
		if got := OrganizationMayTransition(c.from, c.to); got != c.org {
			t.Fatalf("OrganizationMayTransition(%s,%s)=%v want %v", c.from, c.to, got, c.org)
		}
	}
}

func TestProviderDecisionAndLive(t *testing.T) {
	if ProviderDecision(true) != StateActive || ProviderDecision(false) != StateRejected {
		t.Fatal("provider decision mapping changed")
	}
	for _, s := range []string{StateRequested, StateActive, StateSuspended} {
		if !Live(s) {
			t.Fatalf("%s should be live", s)
		}
	}
	for _, s := range []string{StateRevoked, StateRejected, ""} {
		if Live(s) {
			t.Fatalf("%q should not be live", s)
		}
	}
}

func TestHelpers(t *testing.T) {
	if NormalizeEmail("  Ada@Example.COM ") != "ada@example.com" {
		t.Fatal("email not normalized")
	}
	first, last := SplitLegalName("  Ada  Lovelace Byron ")
	if first != "Ada" || last != "Lovelace Byron" {
		t.Fatalf("split = %q %q", first, last)
	}
	if f, l := SplitLegalName(""); f != "" || l != "" {
		t.Fatal("empty name must split to empty parts")
	}
}
