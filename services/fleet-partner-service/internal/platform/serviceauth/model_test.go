package serviceauth

import (
	"testing"
	"time"
)

func TestCredential_IsActive(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name string
		cred Credential
		want bool
	}{
		{"active, no expiry", Credential{ActiveFrom: past}, true},
		{"not yet active", Credential{ActiveFrom: future}, false},
		{"expired", Credential{ActiveFrom: past, ExpiresAt: &past}, false},
		{"expires exactly now (inclusive boundary)", Credential{ActiveFrom: past, ExpiresAt: &now}, false},
		{"active, expires in the future", Credential{ActiveFrom: past, ExpiresAt: &future}, true},
		{"revoked, otherwise active", Credential{ActiveFrom: past, RevokedAt: &past}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cred.IsActive(now); got != tc.want {
				t.Errorf("IsActive() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCredential_AllowsMethod(t *testing.T) {
	cred := Credential{AllowedMethods: []string{"GET", "POST"}}
	if !cred.AllowsMethod("get") {
		t.Error("expected case-insensitive method match")
	}
	if cred.AllowsMethod("DELETE") {
		t.Error("expected DELETE to be refused")
	}

	wildcard := Credential{AllowedMethods: []string{Wildcard}}
	if !wildcard.AllowsMethod("DELETE") {
		t.Error("expected wildcard to allow any method")
	}
}

func TestCredential_AllowsRoute_NeverSubstringMatches(t *testing.T) {
	cred := Credential{AllowedRoutePatterns: []string{"/internal/fleet-partner/v1/documents/media-events"}}
	if !cred.AllowsRoute("/internal/fleet-partner/v1/documents/media-events") {
		t.Error("expected exact route match to pass")
	}
	// A substring-matching implementation would wrongly allow either of
	// these; an exact-membership implementation correctly refuses both.
	if cred.AllowsRoute("/internal/fleet-partner/v1/documents/media-events/extra") {
		t.Error("must not match a route that is only a PREFIX of an allowed pattern")
	}
	if cred.AllowsRoute("/internal/fleet-partner/v1/documents") {
		t.Error("must not match a route that an allowed pattern is only a SUFFIX of")
	}
	if cred.AllowsRoute("/internal/fleet-partner/v1/documents/media-events-2") {
		t.Error("must not match a route sharing only a substring with an allowed pattern")
	}
}

func TestCredential_AllowsEventType(t *testing.T) {
	scoped := Credential{AllowedEventTypes: []string{"trip.completed"}}
	if !scoped.AllowsEventType("trip.completed") {
		t.Error("expected declared event type to pass")
	}
	if scoped.AllowsEventType("trip.cancelled") {
		t.Error("expected undeclared event type to be refused")
	}
	if !scoped.AllowsEventType("") {
		t.Error("expected empty event type (non-event-carrying route) to always pass")
	}

	wildcard := Credential{AllowedEventTypes: []string{Wildcard}}
	if !wildcard.AllowsEventType("anything.at.all") {
		t.Error("expected wildcard to allow any event type")
	}
}
