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

func TestResolution(t *testing.T) {
	a := &Match{ProviderID: "a", PlatformStatus: AccountActive, ManagedByOrgID: "org_1", SourceVersion: 3}
	b := &Match{ProviderID: "b"}
	if got := Resolution("TAXI", a, b); got.Outcome != ResolveConflict || got.ProviderID != "" || got.ServiceCategory != "TAXI" {
		t.Fatalf("split identity: %+v", got)
	}
	if got := Resolution("TAXI", nil, nil); got.Outcome != ResolveNotFound {
		t.Fatalf("no match: %+v", got)
	}
	if got := Resolution("TAXI", nil, a); got.Outcome != ResolveMatched || got.ProviderID != "a" || got.ManagedByOrgID != "org_1" || got.SourceVersion != 3 {
		t.Fatalf("phone-only match: %+v", got)
	}
	if got := Resolution("TAXI", a, &Match{ProviderID: "a"}); got.Outcome != ResolveMatched || got.PlatformStatus != AccountActive {
		t.Fatalf("both contacts, same provider: %+v", got)
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
