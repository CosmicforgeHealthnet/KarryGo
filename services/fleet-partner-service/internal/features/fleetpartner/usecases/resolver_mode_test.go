package usecases

import "cosmicforge/logistics/services/fleet-partner-service/internal/features/fleetpartner/models"
import "testing"

// Rollback: a service configured back to OFF or SHADOW stops immediately.
func TestRollingBackStopsTheResolverImmediately(t *testing.T) {
	service := &Service{flags: Flags{ResolverMode: models.ResolverOn}}
	if service.ResolverMode() != models.ResolverOn {
		t.Fatalf("mode = %q, want ON", service.ResolverMode())
	}
	if !models.ActsOnResolution(service.ResolverMode()) {
		t.Fatal("ON does not act")
	}

	service.flags.ResolverMode = models.ResolverShadow
	if models.ActsOnResolution(service.ResolverMode()) {
		t.Fatal("rolling back to shadow still acts on resolutions")
	}
	if !models.ResolvesCompensation(service.ResolverMode()) {
		t.Fatal("rolling back to shadow stopped recording figures, which is the thing worth keeping")
	}

	service.flags.ResolverMode = models.ResolverOff
	if models.ResolvesCompensation(service.ResolverMode()) || models.ActsOnResolution(service.ResolverMode()) {
		t.Fatal("rolling back to OFF still resolves")
	}
}

// A misconfigured mode must not fall through to paying anybody. For a stage
// that decides money, the safe direction is to do nothing.
func TestAMisconfiguredModeDoesNothingRatherThanGuessing(t *testing.T) {
	service := &Service{flags: Flags{ResolverMode: "MAYBE"}}
	if service.ResolverMode() != models.ResolverOff {
		t.Fatalf("a misconfigured mode resolved to %q, want OFF", service.ResolverMode())
	}
	if models.ResolvesCompensation(service.ResolverMode()) {
		t.Fatal("a misconfigured mode resolved compensation")
	}
}

// The legacy boolean alone still means ON, so nothing changes for a deployment
// that has not adopted the mode.
func TestTheLegacyBooleanStillTurnsTheResolverOn(t *testing.T) {
	service := &Service{flags: Flags{CompensationResolver: true}}
	if service.ResolverMode() != models.ResolverOn {
		t.Fatalf("legacy boolean gave mode %q, want ON", service.ResolverMode())
	}
	// And the explicit mode wins over it, so a rollback to shadow works even
	// on a deployment that still sets the boolean.
	service.flags.ResolverMode = models.ResolverShadow
	if models.ActsOnResolution(service.ResolverMode()) {
		t.Fatal("the legacy boolean overrode an explicit rollback to shadow")
	}
}
