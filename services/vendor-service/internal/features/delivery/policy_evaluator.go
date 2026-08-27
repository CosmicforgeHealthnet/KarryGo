package delivery

// Deterministic delivery-policy evaluation. This is the single decision path
// shared by vendor-owned and HYBRID planning; it never prices platform
// logistics and never writes a Dispatch/Hauling record.

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidPolicy = errors.New("delivery: invalid policy configuration")
	ErrNoVendorFee   = errors.New("delivery: no deterministic vendor fee rule")
)

const (
	ReasonModePlatform       = "PLATFORM_MODE"
	ReasonVendorUnavailable  = "VENDOR_DELIVERY_UNAVAILABLE"
	ReasonNoRider            = "NO_RIDER"
	ReasonOutsideHours       = "OUTSIDE_OPERATING_HOURS"
	ReasonCapacityExceeded   = "CAPACITY_EXCEEDED"
	ReasonVehicleUnavailable = "VEHICLE_UNAVAILABLE"
	ReasonSLARisk            = "SLA_RISK"
	ReasonNoFeeRule          = "VENDOR_FEE_RULE_UNAVAILABLE"
	ReasonCustodyTransferred = "CUSTODY_TRANSFERRED"
)

// ExecutionOwner is the selected fulfilment owner, independent of the
// configured product mode.
type ExecutionOwner string

const (
	OwnerVendor   ExecutionOwner = "VENDOR"
	OwnerPlatform ExecutionOwner = "PLATFORM"
	OwnerNone     ExecutionOwner = "NONE"
)

// PriceGuarantee is the customer-facing contribution frozen at quote/order
// acceptance. Actual logistics cost and later funding attribution are separate.
type PriceGuarantee struct {
	Currency         string `json:"currency"`
	CustomerFeeMinor int64  `json:"customer_fee_minor"`
	Source           string `json:"source"`
	Preserved        bool   `json:"preserved"`
}

// EvaluationRequest contains server-resolved evidence. ZoneID and distance
// must come from the location authority; customer-submitted kilometres are not
// accepted as authoritative input.
type EvaluationRequest struct {
	Policy Policy

	OrderSubtotalMinor          int64
	Currency                    string
	DistanceMeters              int64
	DestinationZoneID           string
	Now                         time.Time
	RequiredVehicleClass        string
	PackageCount                int64
	WeightGrams                 int64
	VolumeMl                    int64
	VendorRiderAvailability     RiderAvailability
	VendorEstimateMinutes       int
	PromisedMaxMinutes          int
	PlatformEligible            bool
	CustomerDeliveryFeeMinor    int64
	CustomerDeliveryFeeProvided bool
	BeforeCustody               bool
}

// Evaluation is an auditable answer. For a platform decision, FeeMinor is
// always zero: platform quote authority remains responsible for platform fees.
type Evaluation struct {
	Eligible              bool                   `json:"eligible"`
	Owner                 ExecutionOwner         `json:"owner"`
	Mode                  DeliveryMode           `json:"delivery_mode"`
	FallbackTrigger       OverflowTrigger        `json:"fallback_trigger,omitempty"`
	ReasonCode            string                 `json:"reason_code"`
	DecisionEvidence      string                 `json:"decision_evidence"`
	PolicyVersion         int64                  `json:"policy_version"`
	Currency              string                 `json:"currency,omitempty"`
	FeeMinor              int64                  `json:"vendor_fee_minor"`
	PriceGuarantee        PriceGuarantee         `json:"price_guarantee"`
	DistanceMeters        int64                  `json:"distance_meters"`
	PolicySnapshot        json.RawMessage        `json:"policy_snapshot,omitempty"`
	HybridFallbackFunding FallbackFundingSponsor `json:"hybrid_fallback_funding_sponsor"`
	HybridVendorShareBPS  int                    `json:"hybrid_vendor_share_bps"`
}

func normalizeMode(mode DeliveryMode) DeliveryMode {
	if mode == "" {
		return ModeVendor // legacy Section 17C policies were vendor-owned.
	}
	return mode
}

func normalizeRider(status RiderAvailability) RiderAvailability {
	if status == "" {
		return RiderUnknown
	}
	return status
}

func normalizeFallbackFunding(sponsor FallbackFundingSponsor, vendorShareBPS int) (FallbackFundingSponsor, int) {
	if sponsor == "" {
		return FallbackFundingVendor, 10000
	}
	return sponsor, vendorShareBPS
}

func (p Policy) Validate() error {
	currency := strings.ToUpper(strings.TrimSpace(p.Currency))
	if len(currency) != 3 {
		return ErrInvalidPolicy
	}
	for _, r := range currency {
		if r < 'A' || r > 'Z' {
			return ErrInvalidPolicy
		}
	}
	mode := normalizeMode(p.Mode)
	if mode != ModePlatform && mode != ModeVendor && mode != ModeHybrid {
		return ErrInvalidPolicy
	}
	feeRule := p.FeeRule
	if feeRule == "" {
		feeRule = FeeDistanceTier
	}
	if feeRule != FeeFlat && feeRule != FeeZoneBased && feeRule != FeeDistanceTier {
		return ErrInvalidPolicy
	}
	if p.OperatingTimezone == "" {
		p.OperatingTimezone = "UTC"
	}
	if _, err := time.LoadLocation(p.OperatingTimezone); err != nil {
		return ErrInvalidPolicy
	}
	if p.BaseFeeMinor < 0 || p.PerKmFeeMinor < 0 || p.MinimumOrderMinor < 0 ||
		(p.FreeDeliveryThresholdMinor != nil && *p.FreeDeliveryThresholdMinor < 0) {
		return ErrInvalidPolicy
	}
	if p.FreeDeliveryThresholdMinor != nil && p.MinimumOrderMinor > 0 && *p.FreeDeliveryThresholdMinor < p.MinimumOrderMinor {
		return ErrInvalidPolicy
	}
	if p.MaxDeliveryRadiusMeters != nil && (*p.MaxDeliveryRadiusMeters < 1 || *p.MaxDeliveryRadiusMeters > 200000) {
		return ErrInvalidPolicy
	}
	if p.MaxPackageWeightGrams != nil && *p.MaxPackageWeightGrams < 1 {
		return ErrInvalidPolicy
	}
	if p.RiderAvailability != "" && p.RiderAvailability != RiderAvailable && p.RiderAvailability != RiderUnavailable && p.RiderAvailability != RiderUnknown {
		return ErrInvalidPolicy
	}
	for _, class := range p.SupportedVehicleClasses {
		class = strings.ToUpper(strings.TrimSpace(class))
		if class != "MOTORCYCLE" && class != "VAN" && class != "TRUCK" {
			return ErrInvalidPolicy
		}
	}
	for _, trigger := range p.FallbackTriggers {
		switch trigger {
		case TriggerOwnRiderFirst, TriggerDistance, TriggerTime, TriggerCapacity, TriggerVehicle, TriggerSLA:
		default:
			return ErrInvalidPolicy
		}
	}
	sponsor, vendorShare := normalizeFallbackFunding(p.HybridFallbackFunding, p.HybridVendorShareBPS)
	switch sponsor {
	case FallbackFundingVendor:
		if (vendorShare != 0 && vendorShare != 10000) || strings.TrimSpace(p.HybridPlatformApprovalRef) != "" {
			return ErrInvalidPolicy
		}
	case FallbackFundingPlatform:
		if vendorShare != 0 || strings.TrimSpace(p.HybridPlatformApprovalRef) == "" {
			return ErrInvalidPolicy
		}
	case FallbackFundingCombined:
		if vendorShare <= 0 || vendorShare >= 10000 || strings.TrimSpace(p.HybridPlatformApprovalRef) == "" {
			return ErrInvalidPolicy
		}
	default:
		return ErrInvalidPolicy
	}
	if p.MaxPackageCount != nil && *p.MaxPackageCount <= 0 {
		return ErrInvalidPolicy
	}
	if p.MaxVolumeMl != nil && *p.MaxVolumeMl <= 0 {
		return ErrInvalidPolicy
	}
	for _, z := range p.ServiceZones {
		if strings.TrimSpace(z.ID) == "" || z.MinDistanceMeters < 0 ||
			z.MaxDistanceMeters < 0 || (z.MaxDistanceMeters > 0 && z.MaxDistanceMeters <= z.MinDistanceMeters) || z.FeeMinor < 0 {
			return ErrInvalidPolicy
		}
	}
	for i, tier := range p.DistanceTiers {
		if tier.MinDistanceMeters < 0 || tier.FeeMinor < 0 ||
			(tier.MaxDistanceMeters != 0 && tier.MaxDistanceMeters <= tier.MinDistanceMeters) ||
			(tier.MaxDistanceMeters == 0 && i != len(p.DistanceTiers)-1) {
			return ErrInvalidPolicy
		}
		if i > 0 && p.DistanceTiers[i-1].MaxDistanceMeters != tier.MinDistanceMeters {
			return ErrInvalidPolicy
		}
	}
	for _, period := range p.OperatingPeriods {
		if period.Weekday < 1 || period.Weekday > 7 || period.StartMinute < 0 || period.StartMinute >= 1440 || period.EndMinute < 0 || period.EndMinute >= 1440 || period.StartMinute == period.EndMinute {
			return ErrInvalidPolicy
		}
	}
	return nil
}

func fallbackAllowed(policy Policy, trigger OverflowTrigger) bool {
	if len(policy.FallbackTriggers) == 0 {
		return true // HYBRID's safe default is all bounded overflow causes.
	}
	for _, configured := range policy.FallbackTriggers {
		if configured == trigger {
			return true
		}
	}
	return false
}

func vendorHoursOpen(policy Policy, now time.Time) bool {
	if len(policy.OperatingPeriods) == 0 {
		return true
	}
	loc, err := time.LoadLocation(policy.OperatingTimezone)
	if err != nil {
		return false
	}
	local := now.In(loc)
	weekday := int(local.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	previousWeekday := weekday - 1
	if previousWeekday == 0 {
		previousWeekday = 7
	}
	minute := local.Hour()*60 + local.Minute()
	for _, period := range policy.OperatingPeriods {
		if period.StartMinute < period.EndMinute {
			if period.Weekday != weekday {
				continue
			}
			if minute >= period.StartMinute && minute < period.EndMinute {
				return true
			}
		} else {
			// The after-midnight half belongs to the previous local day.
			if period.Weekday == weekday && minute >= period.StartMinute {
				return true
			}
			if period.Weekday == previousWeekday && minute < period.EndMinute {
				return true
			}
		}
	}
	return false
}

func vendorZone(policy Policy, zoneID string, distance int64) (ServiceZone, bool) {
	var matches []ServiceZone
	for _, zone := range policy.ServiceZones {
		if !zone.Enabled || (zone.ID != "" && zoneID != "" && zone.ID != zoneID) {
			continue
		}
		if zone.ID != "" && zoneID == "" {
			continue
		}
		if zone.MinDistanceMeters > 0 && distance < zone.MinDistanceMeters {
			continue
		}
		if zone.MaxDistanceMeters > 0 && distance >= zone.MaxDistanceMeters {
			continue
		}
		matches = append(matches, zone)
	}
	if len(matches) == 0 {
		return ServiceZone{}, false
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Priority != matches[j].Priority {
			return matches[i].Priority > matches[j].Priority
		}
		return matches[i].ID < matches[j].ID
	})
	return matches[0], true
}

func vendorFee(policy Policy, distance int64, zoneID string) (int64, error) {
	feeRule := policy.FeeRule
	if feeRule == "" {
		feeRule = FeeDistanceTier
	}
	var fee int64
	switch feeRule {
	case FeeFlat:
		fee = policy.BaseFeeMinor
	case FeeZoneBased:
		zone, ok := vendorZone(policy, zoneID, distance)
		if !ok {
			return 0, ErrNoVendorFee
		}
		fee = zone.FeeMinor
	case FeeDistanceTier:
		if len(policy.DistanceTiers) == 0 {
			// Preserve the already accepted Section 17C base + per-km policy.
			km := distance / 1000
			if distance%1000 >= 500 {
				km++
			}
			if policy.PerKmFeeMinor > 0 && km > math.MaxInt64/policy.PerKmFeeMinor {
				return 0, ErrNoVendorFee
			}
			distanceFee := km * policy.PerKmFeeMinor
			if policy.BaseFeeMinor > math.MaxInt64-distanceFee {
				return 0, ErrNoVendorFee
			}
			fee = policy.BaseFeeMinor + distanceFee
		} else {
			for _, tier := range policy.DistanceTiers {
				if distance >= tier.MinDistanceMeters && (tier.MaxDistanceMeters == 0 || distance < tier.MaxDistanceMeters) {
					fee = tier.FeeMinor
					break
				}
			}
		}
	default:
		return 0, ErrNoVendorFee
	}
	if fee < 0 {
		return 0, ErrNoVendorFee
	}
	return fee, nil
}

func vendorFailure(policy Policy, request EvaluationRequest) (string, OverflowTrigger) {
	if request.DistanceMeters < 0 {
		return ReasonInvalidCoordinate, TriggerDistance
	}
	if policy.MaxDeliveryRadiusMeters != nil && request.DistanceMeters > int64(*policy.MaxDeliveryRadiusMeters) {
		return ReasonOutsideArea, TriggerDistance
	}
	if policy.MinimumOrderMinor > request.OrderSubtotalMinor {
		return ReasonBelowMinimum, TriggerCapacity
	}
	if !vendorHoursOpen(policy, request.Now) {
		return ReasonOutsideHours, TriggerTime
	}
	if policy.MaxPackageCount != nil && request.PackageCount > *policy.MaxPackageCount {
		return ReasonCapacityExceeded, TriggerCapacity
	}
	if policy.MaxPackageWeightGrams != nil && request.WeightGrams > *policy.MaxPackageWeightGrams {
		return ReasonCapacityExceeded, TriggerCapacity
	}
	if policy.MaxVolumeMl != nil && request.VolumeMl > *policy.MaxVolumeMl {
		return ReasonCapacityExceeded, TriggerCapacity
	}
	if len(policy.SupportedVehicleClasses) > 0 {
		wanted := strings.ToUpper(strings.TrimSpace(request.RequiredVehicleClass))
		found := false
		for _, class := range policy.SupportedVehicleClasses {
			if strings.EqualFold(class, wanted) {
				found = true
				break
			}
		}
		if wanted == "" || !found {
			return ReasonVehicleUnavailable, TriggerVehicle
		}
	}
	if request.VendorEstimateMinutes > 0 && request.PromisedMaxMinutes > 0 && request.VendorEstimateMinutes > request.PromisedMaxMinutes {
		return ReasonSLARisk, TriggerSLA
	}
	if normalizeRider(request.VendorRiderAvailability) != RiderAvailable {
		return ReasonNoRider, TriggerOwnRiderFirst
	}
	if policy.FeeRule == FeeZoneBased {
		if _, ok := vendorZone(policy, request.DestinationZoneID, request.DistanceMeters); !ok {
			return ReasonOutsideArea, TriggerDistance
		}
	}
	return "", ""
}

// Evaluate resolves PLATFORM, VENDOR and HYBRID in one deterministic path.
func Evaluate(request EvaluationRequest) Evaluation {
	policy := request.Policy
	mode := normalizeMode(policy.Mode)
	sponsor, vendorShare := normalizeFallbackFunding(policy.HybridFallbackFunding, policy.HybridVendorShareBPS)
	result := Evaluation{Mode: mode, Owner: OwnerNone, PolicyVersion: policy.Version, DistanceMeters: request.DistanceMeters,
		HybridFallbackFunding: sponsor, HybridVendorShareBPS: vendorShare}
	result.PolicySnapshot, _ = json.Marshal(policy)
	result.Currency = strings.ToUpper(strings.TrimSpace(request.Currency))
	result.PriceGuarantee = PriceGuarantee{Currency: result.Currency, CustomerFeeMinor: request.CustomerDeliveryFeeMinor, Source: "CHECKOUT_QUOTE", Preserved: true}
	if mode == ModePlatform {
		result.Eligible = request.PlatformEligible
		result.Owner = OwnerPlatform
		result.ReasonCode = ReasonModePlatform
		result.DecisionEvidence = "platform mode bypasses vendor evaluation"
		return result
	}
	if err := policy.Validate(); err != nil {
		result.ReasonCode = ReasonVendorUnavailable
		result.DecisionEvidence = err.Error()
		return result
	}
	if request.Now.IsZero() {
		request.Now = time.Now().UTC()
	}
	if request.VendorRiderAvailability == "" && policy.RiderAvailability != "" {
		request.VendorRiderAvailability = policy.RiderAvailability
	}
	if failure, trigger := vendorFailure(policy, request); failure != "" {
		if mode != ModeHybrid || !request.PlatformEligible || !fallbackAllowed(policy, trigger) {
			result.ReasonCode = failure
			result.DecisionEvidence = "vendor mode is fail-closed"
			return result
		}
		result.Eligible = true
		result.Owner = OwnerPlatform
		result.FallbackTrigger = trigger
		result.ReasonCode = string(trigger)
		result.DecisionEvidence = failure
		return result
	}
	fee, err := vendorFee(policy, request.DistanceMeters, request.DestinationZoneID)
	if err != nil {
		if mode == ModeHybrid && request.PlatformEligible && fallbackAllowed(policy, TriggerDistance) {
			result.Eligible = true
			result.Owner = OwnerPlatform
			result.FallbackTrigger = TriggerDistance
			result.ReasonCode = string(TriggerDistance)
			result.DecisionEvidence = ReasonNoFeeRule
		}
		return result
	}
	result.Eligible = true
	result.Owner = OwnerVendor
	result.ReasonCode = ReasonServiceable
	result.DecisionEvidence = "vendor policy satisfied"
	result.FeeMinor = fee
	customerFee := fee
	if policy.FreeDeliveryThresholdMinor != nil && request.OrderSubtotalMinor >= *policy.FreeDeliveryThresholdMinor {
		customerFee = 0
	}
	if request.CustomerDeliveryFeeProvided {
		result.PriceGuarantee.CustomerFeeMinor = request.CustomerDeliveryFeeMinor
	} else {
		result.PriceGuarantee.CustomerFeeMinor = customerFee
		result.PriceGuarantee.Preserved = false
	}
	return result
}
