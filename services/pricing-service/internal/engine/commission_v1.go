package engine

import (
	"fmt"

	"cosmicforge/logistics/services/pricing-service/internal/platform/money"
)

// What commission is charged on.
const (
	CommissionOnCustomerPayable = "CUSTOMER_PAYABLE"
	CommissionOnGross           = "GROSS"
)

// CommissionV1 is schema version 1 of the platform's commercial share.
//
// Commission lives in versioned policy rather than in the wallet's environment
// because it must be able to differ by service and channel, and because a change
// to KarryGo's take is exactly the sort of change that needs an approver's name
// against it.
//
// ByScopeChannel keys are "SCOPE/CHANNEL", e.g. "DISPATCH/COMMERCE". That entry
// matters for V1 price-neutrality: commerce deliveries currently pay the rider
// the whole delivery fee, so their commission is 0 basis points. A single flat
// rate would quietly cut rider pay on every vendor order.
type CommissionV1 struct {
	SchemaVersion  int                `json:"schema_version"`
	DefaultBPS     int64              `json:"default_bps"`
	ByScope        map[string]int64   `json:"by_scope,omitempty"`
	ByScopeChannel map[string]int64   `json:"by_scope_channel,omitempty"`
	AppliedTo      string             `json:"applied_to"`
	Rounding       money.RoundingMode `json:"rounding"`
}

// Kind returns the policy kind this body belongs to.
func (CommissionV1) Kind() string { return KindCommission }

// Validate reports every problem with the body at once.
func (c CommissionV1) Validate() error {
	var problems []string
	if c.SchemaVersion != 1 {
		problems = append(problems, "schema_version must be 1")
	}
	check := func(label string, bps int64) {
		if bps < 0 || bps > 10_000 {
			problems = append(problems, fmt.Sprintf("%s must be between 0 and 10000 basis points", label))
		}
	}
	check("default_bps", c.DefaultBPS)
	for scope, bps := range c.ByScope {
		if !Scope(scope).Valid() {
			problems = append(problems, fmt.Sprintf("by_scope has unknown scope %q", scope))
		}
		check("by_scope["+scope+"]", bps)
	}
	for key, bps := range c.ByScopeChannel {
		scope, channel, ok := splitScopeChannel(key)
		if !ok || !scope.Valid() || !channel.Valid() {
			problems = append(problems, fmt.Sprintf("by_scope_channel key %q must be SCOPE/CHANNEL with known values", key))
		}
		check("by_scope_channel["+key+"]", bps)
	}
	switch c.AppliedTo {
	case CommissionOnCustomerPayable, CommissionOnGross:
	default:
		problems = append(problems, fmt.Sprintf("applied_to %q is unknown", c.AppliedTo))
	}
	if !c.Rounding.Valid() {
		problems = append(problems, fmt.Sprintf("rounding %q is unknown", c.Rounding))
	}
	if len(problems) > 0 {
		return fmt.Errorf("commission v1: %s", join(problems))
	}
	return nil
}

// BPSFor returns the basis points for a scope and channel, most specific first.
func (c CommissionV1) BPSFor(scope Scope, channel Channel) int64 {
	if bps, ok := c.ByScopeChannel[string(scope)+"/"+string(channel)]; ok {
		return bps
	}
	if bps, ok := c.ByScope[string(scope)]; ok {
		return bps
	}
	return c.DefaultBPS
}

// Split divides gross between the provider and the platform.
//
// The provider receives what is left after the platform fee, so the two always
// add back to gross exactly and no kobo can go missing in a rounding step.
func (c CommissionV1) Split(gross int64, scope Scope, channel Channel) (Allocation, int64, error) {
	bps := c.BPSFor(scope, channel)
	fee, err := money.BasisPoints(gross, bps, c.Rounding)
	if err != nil {
		return Allocation{}, 0, err
	}
	if fee > gross {
		return Allocation{}, 0, fmt.Errorf("commission v1: fee %d exceeds gross %d", fee, gross)
	}
	return Allocation{ProviderKobo: gross - fee, PlatformFeeKobo: fee}, bps, nil
}

func splitScopeChannel(key string) (Scope, Channel, bool) {
	for i := 0; i < len(key); i++ {
		if key[i] == '/' {
			return Scope(key[:i]), Channel(key[i+1:]), true
		}
	}
	return "", "", false
}
