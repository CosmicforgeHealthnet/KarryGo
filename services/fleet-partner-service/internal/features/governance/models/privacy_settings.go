package models

// Privacy settings (spec 59.10) are a view over the organization's existing,
// enforced privacy controls, not a second policy store. Today the only
// organization-configurable control with real behaviour is category-based
// retention for the classes a retention executor actually deletes or
// anonymizes; legal holds always override it. Classes that hold financial,
// payroll, safety, compliance or audit records are reported read-only:
// spec 56.1 forbids ordinary endpoints from shortening how long they are
// kept.

// ConfigurableRetentionClasses are the retention classes an organization may
// tune through privacy settings: exactly the classes that have a retention
// executor (internal/platform/retentionexec). A test pins the two lists
// together so a class cannot be offered here without enforcement behind it.
var ConfigurableRetentionClasses = []string{"TEMPORARY", "OPERATIONAL", "SUPPORT", "COMPANY_CONFIDENTIAL"}

// ProtectedRetentionClasses are reported for transparency but can never be
// changed through privacy settings.
var ProtectedRetentionClasses = []string{"FINANCIAL", "PAYROLL", "SAFETY_EVIDENCE", "INCIDENT_EVIDENCE", "COMPLIANCE", "AUDIT"}

// RetentionSetting is one class as privacy settings report it.
type RetentionSetting struct {
	RetentionDays       int  `json:"retentionDays"`
	PlatformMinimumDays int  `json:"platformMinimumDays"`
	Configurable        bool `json:"configurable"`
	// ExecutionAvailable reports whether this deployment can enforce the
	// class right now (its executor is registered); it never gates a write.
	ExecutionAvailable bool `json:"executionAvailable"`
}

// RetentionValue is one class's stored policy as the retention store returns
// it: the configured (or platform-minimum) days and its row version.
type RetentionValue struct {
	RetentionDays       int
	PlatformMinimumDays int
	Version             int64
}

// PrivacySettings is the organization's privacy-settings resource.
type PrivacySettings struct {
	OrganizationID string `json:"organizationId"`
	// Version is the If-Match token: the sum of the row versions of every
	// configurable class. Every write to any of them increments it, so it
	// never repeats an earlier value.
	Version            int64                       `json:"version"`
	Retention          map[string]RetentionSetting `json:"retention"`
	ProtectedRetention map[string]RetentionSetting `json:"protectedRetention"`
	// LegalHoldsOverrideRetention is always true: an active legal hold
	// blocks deletion regardless of any retention setting (spec 56.1).
	LegalHoldsOverrideRetention bool `json:"legalHoldsOverrideRetention"`
}

// RetentionPatch is one class in a privacy-settings merge patch.
type RetentionPatch struct {
	RetentionDays *int `json:"retentionDays"`
}

// PrivacySettingsPatch is the JSON Merge Patch body of PATCH
// /privacy-settings. Only retention is writable.
type PrivacySettingsPatch struct {
	Retention       map[string]RetentionPatch `json:"retention"`
	ExpectedVersion int64                     `json:"-"`
}
