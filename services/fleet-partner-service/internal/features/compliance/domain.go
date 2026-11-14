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
	ID                       string          `json:"id"`
	OrganizationID           string          `json:"organizationId"`
	Code                     string          `json:"code"`
	Name                     string          `json:"name"`
	SubjectType              string          `json:"subjectType"`
	ServiceType              string          `json:"serviceType"`
	AssetType                string          `json:"assetType"`
	JurisdictionReference    string          `json:"jurisdictionReference"`
	LocationReference        string          `json:"locationReference"`
	RequirementType          string          `json:"requirementType"`
	RequiredDocumentType     string          `json:"requiredDocumentType"`
	RequiredQualificationID  string          `json:"requiredQualificationId"`
	RequiredTrainingCourseID string          `json:"requiredTrainingCourseId"`
	ValidityRules            json.RawMessage `json:"validityRules"`
	Severity                 string          `json:"severity"`
	BlocksOperation          bool            `json:"blocksOperation"`
	EffectiveFrom            time.Time       `json:"effectiveFrom"`
	EffectiveUntil           *time.Time      `json:"effectiveUntil"`
	Status                   string          `json:"status"`
	CreatedBy                string          `json:"createdBy"`
	Version                  int64           `json:"version"`
	CreatedAt                time.Time       `json:"createdAt"`
	UpdatedAt                time.Time       `json:"updatedAt"`
}

type Status struct {
	ID                   string          `json:"id"`
	OrganizationID       string          `json:"organizationId"`
	SubjectType          string          `json:"subjectType"`
	SubjectReference     string          `json:"subjectReference"`
	RequirementID        string          `json:"requirementId"`
	State                string          `json:"state"`
	DueAt                *time.Time      `json:"dueAt"`
	ExpiresAt            *time.Time      `json:"expiresAt"`
	SourceType           string          `json:"sourceType"`
	SourceReference      string          `json:"sourceReference"`
	SourceVersion        int64           `json:"sourceVersion"`
	EvaluatedAt          time.Time       `json:"evaluatedAt"`
	ReasonCodes          json.RawMessage `json:"reasonCodes"`
	RestrictionReference string          `json:"restrictionReference"`
	Version              int64           `json:"version"`
	CreatedAt            time.Time       `json:"createdAt"`
	UpdatedAt            time.Time       `json:"updatedAt"`
}

type Restriction struct {
	ID                  string          `json:"id"`
	OrganizationID      string          `json:"organizationId"`
	SubjectType         string          `json:"subjectType"`
	SubjectReference    string          `json:"subjectReference"`
	ReasonCode          string          `json:"reasonCode"`
	SourceType          string          `json:"sourceType"`
	SourceReference     string          `json:"sourceReference"`
	Severity            string          `json:"severity"`
	State               string          `json:"state"`
	RequiredAuthorities json.RawMessage `json:"requiredAuthorities"`
	EffectiveAt         time.Time       `json:"effectiveAt"`
	ClearedAt           *time.Time      `json:"clearedAt"`
	Version             int64           `json:"version"`
	CreatedBy           string          `json:"createdBy"`
	ClearedBy           string          `json:"clearedBy"`
	ClearanceReason     string          `json:"clearanceReason"`
	CreatedAt           time.Time       `json:"createdAt"`
	UpdatedAt           time.Time       `json:"updatedAt"`
}

type EvaluateInput struct {
	SubjectType      string `json:"subjectType"`
	SubjectReference string `json:"subjectReference"`
	RequirementID    string `json:"requirementId"`
	SourceType       string `json:"sourceType"`
	SourceReference  string `json:"sourceReference"`
	SourceVersion    int64  `json:"sourceVersion"`
}

type RestrictInput struct {
	SubjectType      string `json:"subjectType"`
	SubjectReference string `json:"subjectReference"`
	ReasonCode       string `json:"reasonCode"`
	SourceType       string `json:"sourceType"`
	SourceReference  string `json:"sourceReference"`
	Severity         string `json:"severity"`
	ExpectedVersion  int64  `json:"expectedVersion"`
}

type Eligibility struct {
	Eligible   bool     `json:"eligible"`
	HardBlocks []string `json:"hardBlocks"`
	Warnings   []string `json:"warnings"`
}

func (r Requirement) ActiveAt(now time.Time) bool {
	return r.Status == "ACTIVE" && !now.Before(r.EffectiveFrom) && (r.EffectiveUntil == nil || now.Before(*r.EffectiveUntil))
}

// AuthorityTaxi names driver-taxi-service as a restriction propagation
// target. It is added per deployment (Service.requiredAuthorities), not here,
// because Taxi is optional where Dispatch and Hauling are not.
const AuthorityTaxi = "TAXI"

func RequiredAuthorities(subject string) []string {
	switch subject {
	case "PROVIDER", "ASSET", "ORGANIZATION":
		return []string{"DISPATCH", "HAULING"}
	default:
		return nil
	}
}
