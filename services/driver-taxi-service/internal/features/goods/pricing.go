package goods

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PricingPolicy is an ops-owned GOODS_DELIVERY namespace. It deliberately
// does not call passenger fare estimation or inherit passenger rate tables.
type PricingPolicy struct {
	BaseMinor         int64  `json:"base_minor"`
	PerKilometreMinor int64  `json:"per_kilometre_minor"`
	PerMinuteMinor    int64  `json:"per_minute_minor"`
	HandlingMinor     int64  `json:"handling_minor"`
	MinimumMinor      int64  `json:"minimum_minor"`
	Currency          string `json:"currency"`
	Version           string `json:"version"`
}

func ParsePricingPolicy(raw string) (PricingPolicy, error) {
	var policy PricingPolicy
	if raw == "" {
		return policy, fmt.Errorf("TAXI_GOODS_PRICING is required when Taxi Goods is enabled")
	}
	if err := json.Unmarshal([]byte(raw), &policy); err != nil {
		return policy, fmt.Errorf("parse TAXI_GOODS_PRICING: %w", err)
	}
	policy.Currency = strings.ToUpper(strings.TrimSpace(policy.Currency))
	policy.Version = strings.TrimSpace(policy.Version)
	if policy.BaseMinor < 0 || policy.PerKilometreMinor < 0 || policy.PerMinuteMinor < 0 ||
		policy.HandlingMinor < 0 || policy.MinimumMinor < 0 || len(policy.Currency) != 3 || policy.Version == "" {
		return policy, fmt.Errorf("TAXI_GOODS_PRICING contains invalid values")
	}
	return policy, nil
}

func (p PricingPolicy) Price(distanceKM float64, minutes int, handlingCount int) (int64, int64) {
	distance := int64(distanceKM*float64(p.PerKilometreMinor) + 0.5)
	total := p.BaseMinor + distance + int64(minutes)*p.PerMinuteMinor + int64(handlingCount)*p.HandlingMinor
	if total < p.MinimumMinor {
		total = p.MinimumMinor
	}
	return total, distance
}
