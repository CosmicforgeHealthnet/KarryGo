package fleetpartner

// CMP-001 / CMP-019: turning the compensation resolver on.
//
// A boolean would let one deployment step take the service from "computes
// nothing" to "decides what every corporate provider is paid". There is no way
// to check the figures first, and no way back that does not involve money
// already having moved on figures nobody verified.
//
// So there is a middle state. In shadow the resolver runs and records exactly
// what it would pay, and nothing downstream acts on it: no settlement
// instruction, no journal, no payslip. The figures can be compared against what
// the organization expects for as long as it takes to believe them.

import (
	"fmt"
	"strings"
)

// Compensation resolver modes.
const (
	// ResolverOff computes nothing. Completed work is still recorded, because
	// ingestion is what makes the later stages possible at all.
	ResolverOff = "OFF"

	// ResolverShadow computes and records resolutions that nothing acts on.
	// The point is to be able to read the figures before any money depends on
	// them.
	ResolverShadow = "SHADOW"

	// ResolverOn computes resolutions that the settlement stage may act on --
	// if its own flag is also enabled. Two gates, because "resolve" and "pay"
	// are different decisions and an organization may reasonably want the
	// first without the second.
	ResolverOn = "ON"
)

// ValidResolverMode reports whether a mode is one this service implements.
func ValidResolverMode(mode string) bool {
	switch strings.ToUpper(strings.TrimSpace(mode)) {
	case ResolverOff, ResolverShadow, ResolverOn:
		return true
	default:
		return false
	}
}

// NormalizeResolverMode resolves configuration into a mode.
//
// An unset mode is OFF, and an unrecognised one is refused rather than
// defaulted: a typo that silently became ON would start paying people.
func NormalizeResolverMode(mode string, legacyEnabled bool) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(mode))
	if normalized == "" {
		// The boolean this replaced still works, so an existing deployment
		// keeps behaving exactly as it did until it opts into the new setting.
		if legacyEnabled {
			return ResolverOn, nil
		}
		return ResolverOff, nil
	}
	if !ValidResolverMode(normalized) {
		return "", fmt.Errorf("%w: compensation resolver mode %q is not supported", ErrValidation, mode)
	}
	return normalized, nil
}

// ResolvesCompensation reports whether the resolver runs at all.
func ResolvesCompensation(mode string) bool {
	switch strings.ToUpper(strings.TrimSpace(mode)) {
	case ResolverShadow, ResolverOn:
		return true
	default:
		return false
	}
}

// ActsOnResolution reports whether anything downstream may use the figures.
//
// False in shadow, which is the whole reason shadow exists: the resolution is
// recorded and inert.
func ActsOnResolution(mode string) bool {
	return strings.ToUpper(strings.TrimSpace(mode)) == ResolverOn
}

// ResolverStage describes what a mode does, for an operator deciding whether
// to advance it.
func ResolverStage(mode string) string {
	switch strings.ToUpper(strings.TrimSpace(mode)) {
	case ResolverOff:
		return "completed work is recorded; no compensation is computed"
	case ResolverShadow:
		return "compensation is computed and recorded; nothing acts on it"
	case ResolverOn:
		return "compensation is computed and the settlement stage may act on it"
	default:
		return "unknown"
	}
}
