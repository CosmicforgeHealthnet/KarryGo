package ratelimit

import "time"

// Risk classes, matching Section 28's own taxonomy exactly.
const (
	RiskLow             = "LOW"
	RiskStandard        = "STANDARD"
	RiskSensitive       = "SENSITIVE"
	RiskHighRisk        = "HIGH_RISK"
	RiskInternalService = "INTERNAL_SERVICE"
)

// DefaultRiskRules are the platform default burst/sustained bound per risk
// class, keyed independently per IP/user/organization/service-caller
// dimension by the caller of Allow (see internal/middleware.RateLimit) —
// this map only decides how many requests one dimension may make per
// window, not which dimensions get checked.
//
// These are illustrative platform defaults, not the product of a capacity
// or load-testing exercise; an operator running this in front of real
// traffic should tune them, same disclaimer
// docs/section28-security-privacy-audit.md gives the retention-class
// minimums.
var DefaultRiskRules = map[string]Rule{
	RiskLow:             {Limit: 300, Window: time.Minute},
	RiskStandard:        {Limit: 120, Window: time.Minute},
	RiskSensitive:       {Limit: 30, Window: time.Minute},
	RiskHighRisk:        {Limit: 10, Window: time.Minute},
	RiskInternalService: {Limit: 600, Window: time.Minute},
}

// ValidRiskClass reports whether class is one of the five recognized
// classes.
func ValidRiskClass(class string) bool {
	_, ok := DefaultRiskRules[class]
	return ok
}
