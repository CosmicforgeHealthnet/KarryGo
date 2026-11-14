package models

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
