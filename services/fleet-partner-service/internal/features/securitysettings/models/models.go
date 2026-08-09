// Package models defines per-organization security policy settings.
//
// Settings are a single versioned row per organization. Every write is
// validated against platform minimums before being persisted — an
// organization may only ever tighten (never loosen) the platform floor, per
// Section 28's "organization settings cannot weaken platform minimums" rule.
package models

// Platform minimums. An organization's settings can only be at least this
// strict; a write attempting to go below any of these is rejected before it
// ever reaches SQL.
const (
	MinStepUpFreshnessSeconds  = 300  // 5 minutes: matches this service's own step-up grant TTL floor.
	MaxStepUpFreshnessSeconds  = 3600 // 1 hour: an organization cannot make step-up proofs valid indefinitely.
	MinRetentionPreferenceDays = 0    // 0 means "defer entirely to the platform/record-type minimum".
)

// Session revocation policy values.
const (
	SessionRevocationImmediate = "IMMEDIATE"
	SessionRevocationStandard  = "STANDARD"
)

// Allowed-provider-access policy values.
const (
	ProviderAccessActiveMembershipOnly = "ACTIVE_MEMBERSHIP_ONLY"
	ProviderAccessAssignedOnly         = "ASSIGNED_ONLY"
)

// Audit IP/device capture policy values.
const (
	AuditCaptureFull = "FULL"
	AuditCaptureNone = "NONE"
)

var validSessionRevocationPolicies = map[string]bool{
	SessionRevocationImmediate: true, SessionRevocationStandard: true,
}
var validProviderAccessPolicies = map[string]bool{
	ProviderAccessActiveMembershipOnly: true, ProviderAccessAssignedOnly: true,
}
var validAuditCapturePolicies = map[string]bool{
	AuditCaptureFull: true, AuditCaptureNone: true,
}

// ValidSessionRevocationPolicy reports whether value is a recognized policy.
func ValidSessionRevocationPolicy(value string) bool { return validSessionRevocationPolicies[value] }

// ValidProviderAccessPolicy reports whether value is a recognized policy.
func ValidProviderAccessPolicy(value string) bool { return validProviderAccessPolicies[value] }

// ValidAuditCapturePolicy reports whether value is a recognized policy.
func ValidAuditCapturePolicy(value string) bool { return validAuditCapturePolicies[value] }

// Settings is one organization's security policy.
type Settings struct {
	OrganizationID                   string
	MFARequiredForSensitiveActions   bool
	StepUpFreshnessSeconds           int
	SessionRevocationPolicy          string
	AllowedProviderAccessPolicy      string
	SensitiveExportThresholdKobo     int64
	RestrictedDocumentStepUpRequired bool
	AuditCapturePolicy               string
	RetentionPreferenceDays          int
	Version                          int64
}

// Defaults is what a caller reads before an organization has ever written
// its own settings: platform minimums, not organization-specific choices.
// Version 0 signals "no row exists yet" to the create/update path.
func Defaults(organizationID string) Settings {
	return Settings{
		OrganizationID:                   organizationID,
		MFARequiredForSensitiveActions:   true,
		StepUpFreshnessSeconds:           MinStepUpFreshnessSeconds,
		SessionRevocationPolicy:          SessionRevocationImmediate,
		AllowedProviderAccessPolicy:      ProviderAccessActiveMembershipOnly,
		SensitiveExportThresholdKobo:     0,
		RestrictedDocumentStepUpRequired: true,
		AuditCapturePolicy:               AuditCaptureFull,
		RetentionPreferenceDays:          MinRetentionPreferenceDays,
		Version:                          0,
	}
}

// ValidationError names the offending field for a 422 response.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string { return e.Field + ": " + e.Message }

// Validate enforces platform minimums/maximums and enum membership. It
// mutates nothing; the caller applies validated values explicitly.
func (s Settings) Validate() []ValidationError {
	var errs []ValidationError
	if s.StepUpFreshnessSeconds < MinStepUpFreshnessSeconds {
		errs = append(errs, ValidationError{"stepUpFreshnessSeconds", "must be at least the platform minimum (300 seconds)"})
	}
	if s.StepUpFreshnessSeconds > MaxStepUpFreshnessSeconds {
		errs = append(errs, ValidationError{"stepUpFreshnessSeconds", "must not exceed the platform maximum (3600 seconds)"})
	}
	if !ValidSessionRevocationPolicy(s.SessionRevocationPolicy) {
		errs = append(errs, ValidationError{"sessionRevocationPolicy", "must be IMMEDIATE or STANDARD"})
	}
	if !ValidProviderAccessPolicy(s.AllowedProviderAccessPolicy) {
		errs = append(errs, ValidationError{"allowedProviderAccessPolicy", "must be ACTIVE_MEMBERSHIP_ONLY or ASSIGNED_ONLY"})
	}
	if !ValidAuditCapturePolicy(s.AuditCapturePolicy) {
		errs = append(errs, ValidationError{"auditCapturePolicy", "must be FULL or NONE"})
	}
	if s.SensitiveExportThresholdKobo < 0 {
		errs = append(errs, ValidationError{"sensitiveExportThresholdKobo", "must not be negative"})
	}
	if s.RetentionPreferenceDays < MinRetentionPreferenceDays {
		errs = append(errs, ValidationError{"retentionPreferenceDays", "must not be negative"})
	}
	return errs
}
