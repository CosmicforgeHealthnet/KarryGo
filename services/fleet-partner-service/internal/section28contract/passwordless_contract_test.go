package section28contract

import (
	"strings"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/riskregistry"
)

// FLEET-8.3 was superseded by the approved authentication v2 decision
// (services/fleet-partner-service/docs/passwordless-authentication.md):
// Fleet signs people in with an email OTP only, with no password and no SMS.
// This contract pins that decision so a password login can never be added
// silently: no live route and no registry entry may offer one, and the
// passwordless sign-in routes that replace it must stay mounted.
func TestFleet8_3_PasswordLoginSupersededByPasswordlessOTP(t *testing.T) {
	var live []liveRoute
	live = append(live, enumerateManifestBackedRoutes(t)...)
	live = append(live, manifestlessRoutes()...)
	live = append(live, staticPlatformRoutes()...)

	forbidden := func(path string) bool {
		p := strings.ToLower(path)
		return strings.HasSuffix(p, "/auth/login") || strings.Contains(p, "password")
	}
	for _, r := range live {
		if forbidden(r.path) {
			t.Errorf("live route offers password login: %s %s", r.method, r.path)
		}
	}
	for _, e := range riskregistry.All() {
		if forbidden(e.Path) {
			t.Errorf("registry classifies a password route: %s %s", e.Method, e.Path)
		}
	}
	for _, want := range []string{
		"POST /fleet-partner/v1/auth/sign-in/start",
		"POST /fleet-partner/v1/auth/sign-up/start",
		"POST /fleet-partner/v1/auth/verify",
	} {
		parts := strings.SplitN(want, " ", 2)
		if _, ok := riskregistry.Lookup(parts[0], parts[1]); !ok {
			t.Errorf("passwordless replacement route missing: %s", want)
		}
	}
}
