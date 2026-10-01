// Package models holds the rollout and market-freeze domain.
//
// Together these are the switches that decide whether this service is merely
// calculating or actually deciding what customers pay.
package models

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Modes. A market moves LEGACY -> SHADOW -> CENTRAL, and can go back instantly.
const (
	ModeLegacy  = "LEGACY"
	ModeShadow  = "SHADOW"
	ModeCentral = "CENTRAL"
)

// Capabilities are switched separately, so quoting centrally does not oblige the
// platform to reconcile centrally on the same day.
const (
	CapabilityQuote     = "QUOTE"
	CapabilityBind      = "BIND"
	CapabilityReconcile = "RECONCILE"
)

// Freeze actions.
const (
	ActionFreeze   = "FREEZE"
	ActionUnfreeze = "UNFREEZE"
)

// ErrNotCentral is returned when a bindable quote is requested for a market that
// has not been switched on here.
//
// This is the pricing half of the two-key switch: a consuming service also has
// its own flag, and BOTH must agree before real prices come from this service. A
// single mistaken deployment setting therefore cannot make pricing authoritative.
var ErrNotCentral = errors.New("rollouts: this market is not switched to central pricing")

// Rollout is one switch event.
type Rollout struct {
	ID                string
	Scope             string
	Capability        string
	MarketCode        string
	Mode              string
	PerformedBy       string
	Reason            string
	ApprovalRequestID string
	CreatedAt         time.Time
}

// Freeze is one freeze or unfreeze event.
type Freeze struct {
	ID                string
	MarketCode        string
	Action            string
	PerformedBy       string
	Reason            string
	ApprovalRequestID string
	CreatedAt         time.Time
}

// FreezeState is the current freeze position of a market.
type FreezeState struct {
	Frozen bool
	// Since is when the freeze began. Resolution uses it as the effective as-of
	// time, so a frozen market keeps pricing with the policy set that was live
	// when it was frozen rather than picking up later activations.
	Since  time.Time
	Reason string
}

// ValidMode reports whether mode is known.
func ValidMode(mode string) bool {
	switch mode {
	case ModeLegacy, ModeShadow, ModeCentral:
		return true
	}
	return false
}

// ValidCapability reports whether capability is known.
func ValidCapability(capability string) bool {
	switch capability {
	case CapabilityQuote, CapabilityBind, CapabilityReconcile:
		return true
	}
	return false
}

// ValidateRollout checks a switch change is complete.
func ValidateRollout(scope, capability, marketCode, mode, performedBy, reason string) error {
	var problems []string
	if strings.TrimSpace(scope) == "" {
		problems = append(problems, "scope is required")
	}
	if !ValidCapability(capability) {
		problems = append(problems, fmt.Sprintf("capability %q is unknown", capability))
	}
	if strings.TrimSpace(marketCode) == "" {
		problems = append(problems, "market_code is required")
	}
	if !ValidMode(mode) {
		problems = append(problems, fmt.Sprintf("mode %q is unknown", mode))
	}
	if strings.TrimSpace(performedBy) == "" {
		problems = append(problems, "performed_by is required")
	}
	if strings.TrimSpace(reason) == "" {
		problems = append(problems, "a reason is required")
	}
	if len(problems) > 0 {
		return fmt.Errorf("rollouts: %s", strings.Join(problems, "; "))
	}
	return nil
}
