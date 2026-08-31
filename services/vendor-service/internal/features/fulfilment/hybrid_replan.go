package fulfilment

import (
	"errors"
	"strings"
)

// CustodyState is operational evidence, not a commercial order status.
type CustodyState string

const (
	CustodyNotTransferred CustodyState = "NOT_TRANSFERRED"
	CustodyVendor         CustodyState = "IN_VENDOR_CUSTODY"
	CustodyPlatform       CustodyState = "IN_PLATFORM_CUSTODY"
	CustodyReturnRequired CustodyState = "RETURN_REQUIRED"
	CustodyReturned       CustodyState = "RETURNED"
)

var (
	ErrCustodyTransferRequired = errors.New("delivery fallback requires authoritative custody transfer or return")
	ErrFallbackTriggerInvalid  = errors.New("delivery fallback trigger is not permitted")
	ErrFallbackAlreadyOwned    = errors.New("delivery fallback already has an authoritative owner")
)

// FallbackInput carries an already-authoritative platform quote. Fulfilment
// replans the existing mission; it never invents a platform fee or provider.
type FallbackInput struct {
	Plan            Plan
	PlatformMission Mission
	Trigger         string
	IdempotencyKey  string
	Custody         CustodyState
}

func validFallbackTrigger(trigger string) bool {
	switch strings.TrimSpace(trigger) {
	case "OWN_RIDER_FIRST", "DISTANCE_OVERFLOW", "TIME_OVERFLOW", "CAPACITY_OVERFLOW", "VEHICLE_OVERFLOW", "SLA_OVERFLOW":
		return true
	default:
		return false
	}
}

// ReplanBeforeCustody switches the existing fulfilment mission to an
// authoritative platform quote while preserving mission/group identities and
// the customer price guarantee. Repeating the same idempotency key is a no-op.
func ReplanBeforeCustody(input FallbackInput) (Plan, error) {
	if !validFallbackTrigger(input.Trigger) || strings.TrimSpace(input.IdempotencyKey) == "" {
		return Plan{}, ErrFallbackTriggerInvalid
	}
	if input.Custody != "" && input.Custody != CustodyNotTransferred {
		return Plan{}, ErrCustodyTransferRequired
	}
	if input.Plan.Status != StatusPlanned || len(input.Plan.Missions) == 0 {
		return Plan{}, ErrFallbackAlreadyOwned
	}
	if input.PlatformMission.ID == "" || input.PlatformMission.Authority != AuthorityPlatform || input.PlatformMission.Quote.QuoteID == "" {
		return Plan{}, ErrDependencyMissing
	}

	result := input.Plan
	changed := false
	for i := range result.Missions {
		current := &result.Missions[i]
		if current.Authority != AuthorityVendorOwn {
			if current.FallbackTrigger == input.Trigger && current.DeliveryMode == "HYBRID" {
				continue // idempotent read-back of the already-authoritative replan
			}
			continue
		}
		replacement := input.PlatformMission
		replacement.ID = current.ID // preserve the physical/commercial mission identity
		replacement.GroupIDs = append([]string(nil), current.GroupIDs...)
		replacement.CustomerPriceGuaranteeMinor = current.CustomerPriceGuaranteeMinor
		replacement.DeliveryMode = "HYBRID"
		replacement.FallbackTrigger = input.Trigger
		replacement.CustodyState = string(CustodyNotTransferred)
		current = &result.Missions[i]
		*current = replacement
		changed = true
	}
	if !changed {
		return input.Plan, nil
	}
	for i := range result.Groups {
		if result.Groups[i].Authority == AuthorityVendorOwn {
			result.Groups[i].Authority = AuthorityPlatform
			result.Groups[i].DeliveryMode = "HYBRID"
			result.Groups[i].FallbackTrigger = input.Trigger
		}
	}
	result.PlanRevision++
	result.Version++
	return result, nil
}

// PostCustodyFallback deliberately refuses a silent owner switch. The owning
// logistics service must provide transfer/return evidence before a takeover.
func PostCustodyFallback(custody CustodyState) error {
	if custody == CustodyNotTransferred || custody == "" {
		return nil
	}
	return ErrCustodyTransferRequired
}
