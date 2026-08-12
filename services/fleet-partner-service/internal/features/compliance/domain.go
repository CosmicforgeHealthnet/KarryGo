package compliance

import (
	"encoding/json"
	"time"
)

const (
	StateUnknown       = "UNKNOWN"
	StatePending       = "PENDING"
	StateCompliant     = "COMPLIANT"
	StateDueSoon       = "DUE_SOON"
	StateExpired       = "EXPIRED"
	StateNonCompliant  = "NON_COMPLIANT"
	StateRestricted    = "RESTRICTED"
	StateNotApplicable = "NOT_APPLICABLE"
)

type Flags struct {
	Compliance, Restrictions, Workers, KillSwitch bool
}

type Requirement struct {
	ID, OrganizationID, Code, Name, SubjectType, ServiceType, AssetType     string
	JurisdictionReference, LocationReference, RequirementType               string
	RequiredDocumentType, RequiredQualificationID, RequiredTrainingCourseID string
	ValidityRules                                                           json.RawMessage
	Severity                                                                string
	BlocksOperation                                                         bool
	EffectiveFrom                                                           time.Time
	EffectiveUntil                                                          *time.Time
	Status, CreatedBy                                                       string
	Version                                                                 int64
	CreatedAt, UpdatedAt                                                    time.Time
}

type Status struct {
	ID, OrganizationID, SubjectType, SubjectReference, RequirementID, State string
	DueAt, ExpiresAt                                                        *time.Time
	SourceType, SourceReference                                             string
	SourceVersion                                                           int64
	EvaluatedAt                                                             time.Time
	ReasonCodes                                                             json.RawMessage
	RestrictionReference                                                    string
	Version                                                                 int64
	CreatedAt, UpdatedAt                                                    time.Time
}

type Restriction struct {
	ID, OrganizationID, SubjectType, SubjectReference, ReasonCode string
	SourceType, SourceReference, Severity, State                  string
	RequiredAuthorities                                           json.RawMessage
	EffectiveAt                                                   time.Time
	ClearedAt                                                     *time.Time
	Version                                                       int64
	CreatedBy, ClearedBy, ClearanceReason                         string
	CreatedAt, UpdatedAt                                          time.Time
}

type EvaluateInput struct {
	SubjectType, SubjectReference, RequirementID string
	SourceType, SourceReference                  string
	SourceVersion                                int64
}

type RestrictInput struct {
	SubjectType, SubjectReference, ReasonCode, SourceType, SourceReference, Severity string
	ExpectedVersion                                                                  int64
}

type Eligibility struct {
	Eligible   bool     `json:"eligible"`
	HardBlocks []string `json:"hardBlocks"`
	Warnings   []string `json:"warnings"`
}

func (r Requirement) ActiveAt(now time.Time) bool {
	return r.Status == "ACTIVE" && !now.Before(r.EffectiveFrom) && (r.EffectiveUntil == nil || now.Before(*r.EffectiveUntil))
}

func RequiredAuthorities(subject string) []string {
	switch subject {
	case "PROVIDER", "ASSET", "ORGANIZATION":
		return []string{"DISPATCH", "HAULING"}
	default:
		return nil
	}
}
