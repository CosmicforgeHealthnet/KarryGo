// Package retentionpolicy models organization-configurable retention
// duration per retention class, bounded by a platform legal/operational
// minimum that an organization can extend but never shorten.
//
// This is a distinct concern from internal/features/retention (Section 27's
// operational worker, scoped only to fleet_partner_jobs and
// fleet_partner_scheduled_task_runs, which already disclaims doing anything
// for business-domain tables). This package is the Section 28/29/30 policy
// model: it stores and validates the org's requested duration per class and
// exposes EffectiveRetentionDays as a query any future domain worker can
// call before deleting or anonymizing a record. It does not itself delete or
// anonymize business-domain data — wiring an actual deletion/anonymization
// worker into every one of the fourteen record types below is future work,
// same disclosed-scope posture internal/features/retention's own package doc
// already takes for the tables it doesn't touch.
package models

// Retention classes, matching Section 28/30's list exactly.
const (
	ClassTemporary           = "TEMPORARY"
	ClassOperational         = "OPERATIONAL"
	ClassCustomerRecord      = "CUSTOMER_RECORD"
	ClassProviderRecord      = "PROVIDER_RECORD"
	ClassTripExecution       = "TRIP_EXECUTION"
	ClassSafetyEvidence      = "SAFETY_EVIDENCE"
	ClassIncidentEvidence    = "INCIDENT_EVIDENCE"
	ClassCompliance          = "COMPLIANCE"
	ClassFinancial           = "FINANCIAL"
	ClassPayroll             = "PAYROLL"
	ClassSupport             = "SUPPORT"
	ClassAudit               = "AUDIT"
	ClassLegalHold           = "LEGAL_HOLD"
	ClassCompanyConfidential = "COMPANY_CONFIDENTIAL"
)

// PlatformMinimumDays is the legal/operational floor per class: an
// organization's configured retention_days may never be set below this.
// These are illustrative platform defaults, not a substitute for a
// jurisdiction-specific legal review — see docs/section28-security-privacy-audit.md.
// LEGAL_HOLD has no independent minimum of its own: a record's legal-hold
// state is governed entirely by internal/features/legalhold (an ACTIVE hold
// blocks deletion regardless of what this table says), so this class exists
// in the enum for documentation completeness rather than as something an
// organization tunes a day count for.
var PlatformMinimumDays = map[string]int{
	ClassTemporary:           1,
	ClassOperational:         90,
	ClassCustomerRecord:      365,
	ClassProviderRecord:      365,
	ClassTripExecution:       365,
	ClassSafetyEvidence:      1825,
	ClassIncidentEvidence:    1825,
	ClassCompliance:          2555,
	ClassFinancial:           2555,
	ClassPayroll:             2555,
	ClassSupport:             730,
	ClassAudit:               1825,
	ClassLegalHold:           0,
	ClassCompanyConfidential: 1825,
}

// ValidClass reports whether class is a recognized retention class.
func ValidClass(class string) bool {
	_, ok := PlatformMinimumDays[class]
	return ok
}

// Policy is one organization's configured retention duration for one class.
type Policy struct {
	OrganizationID string
	RetentionClass string
	RetentionDays  int
	Version        int64
}

// EffectiveDays returns the greater of the organization's configured value
// and the platform minimum -- an organization-configured value can never
// make retention shorter than the platform floor, even if the stored row
// somehow has a lower value than validation would allow today (a floor
// raised in a later release, for example).
func (p Policy) EffectiveDays() int {
	minimum := PlatformMinimumDays[p.RetentionClass]
	if p.RetentionDays > minimum {
		return p.RetentionDays
	}
	return minimum
}
