package fleetpartner

import "testing"

// CMP-019. The existing boolean keeps deciding until a deployment opts into the
// new setting, so enabling the mode is a deliberate act rather than a side
// effect of upgrading.
func TestAnUnsetModeFallsBackToTheExistingBoolean(t *testing.T) {
	off, err := NormalizeResolverMode("", false)
	if err != nil || off != ResolverOff {
		t.Fatalf("unset with the boolean off = %q (%v), want OFF", off, err)
	}
	on, err := NormalizeResolverMode("", true)
	if err != nil || on != ResolverOn {
		t.Fatalf("unset with the boolean on = %q (%v), want ON", on, err)
	}
}

// A typo that silently became ON would start paying people.
func TestAnUnrecognisedModeIsRefused(t *testing.T) {
	for _, mode := range []string{"ENABLED", "TRUE", "shadow-mode", "on "} {
		if _, err := NormalizeResolverMode(mode, false); err == nil && mode != "on " {
			t.Fatalf("mode %q was accepted", mode)
		}
	}
	// Whitespace and case are normalised, because those are not typos.
	normalized, err := NormalizeResolverMode("  shadow ", false)
	if err != nil || normalized != ResolverShadow {
		t.Fatalf("  shadow  normalised to %q (%v)", normalized, err)
	}
}

// The three stages, and what each one is allowed to do.
func TestEachModeDoesExactlyWhatItSays(t *testing.T) {
	for _, stage := range []struct {
		mode     string
		resolves bool
		acts     bool
	}{
		{ResolverOff, false, false},
		{ResolverShadow, true, false},
		{ResolverOn, true, true},
	} {
		if ResolvesCompensation(stage.mode) != stage.resolves {
			t.Errorf("%s resolves = %t, want %t", stage.mode, ResolvesCompensation(stage.mode), stage.resolves)
		}
		if ActsOnResolution(stage.mode) != stage.acts {
			t.Errorf("%s acts = %t, want %t", stage.mode, ActsOnResolution(stage.mode), stage.acts)
		}
		if ResolverStage(stage.mode) == "unknown" {
			t.Errorf("%s has no operator description", stage.mode)
		}
	}
}

// Shadow's entire purpose: the figures exist and nothing acts on them. If
// shadow could act, there would be no safe way to check the resolver before
// money depended on it.
func TestShadowComputesButNothingActsOnIt(t *testing.T) {
	if !ResolvesCompensation(ResolverShadow) {
		t.Fatal("shadow does not compute, so there is nothing to check")
	}
	if ActsOnResolution(ResolverShadow) {
		t.Fatal("shadow acts on its own figures, which defeats the point of a shadow stage")
	}
}

// Rollback: a service configured back to OFF or SHADOW stops immediately.
func TestRollingBackStopsTheResolverImmediately(t *testing.T) {
	service := &Service{flags: Flags{ResolverMode: ResolverOn}}
	if service.ResolverMode() != ResolverOn {
		t.Fatalf("mode = %q, want ON", service.ResolverMode())
	}
	if !ActsOnResolution(service.ResolverMode()) {
		t.Fatal("ON does not act")
	}

	service.flags.ResolverMode = ResolverShadow
	if ActsOnResolution(service.ResolverMode()) {
		t.Fatal("rolling back to shadow still acts on resolutions")
	}
	if !ResolvesCompensation(service.ResolverMode()) {
		t.Fatal("rolling back to shadow stopped recording figures, which is the thing worth keeping")
	}

	service.flags.ResolverMode = ResolverOff
	if ResolvesCompensation(service.ResolverMode()) || ActsOnResolution(service.ResolverMode()) {
		t.Fatal("rolling back to OFF still resolves")
	}
}

// A misconfigured mode must not fall through to paying anybody. For a stage
// that decides money, the safe direction is to do nothing.
func TestAMisconfiguredModeDoesNothingRatherThanGuessing(t *testing.T) {
	service := &Service{flags: Flags{ResolverMode: "MAYBE"}}
	if service.ResolverMode() != ResolverOff {
		t.Fatalf("a misconfigured mode resolved to %q, want OFF", service.ResolverMode())
	}
	if ResolvesCompensation(service.ResolverMode()) {
		t.Fatal("a misconfigured mode resolved compensation")
	}
}

// The legacy boolean alone still means ON, so nothing changes for a deployment
// that has not adopted the mode.
func TestTheLegacyBooleanStillTurnsTheResolverOn(t *testing.T) {
	service := &Service{flags: Flags{CompensationResolver: true}}
	if service.ResolverMode() != ResolverOn {
		t.Fatalf("legacy boolean gave mode %q, want ON", service.ResolverMode())
	}
	// And the explicit mode wins over it, so a rollback to shadow works even
	// on a deployment that still sets the boolean.
	service.flags.ResolverMode = ResolverShadow
	if ActsOnResolution(service.ResolverMode()) {
		t.Fatal("the legacy boolean overrode an explicit rollback to shadow")
	}
}
