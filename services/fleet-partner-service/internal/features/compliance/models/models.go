// Package models holds the compliance domain: requirements, evaluation
// states, restrictions, checks and the pure rules over them.
package models

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ErrNotFound is returned by the repository when a scoped row is absent.
var ErrNotFound = errors.New("compliance: not found")

// Job types run by the Section 19 workers.
const (
	JobComplianceSweep      = "section19.compliance_sweep"
	JobRestrictionReconcile = "section19.restriction_reconcile"
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

// NormalizeRequirement canonicalizes a requirement before it is stored.
func NormalizeRequirement(v *Requirement) {
	v.Code = strings.ToUpper(strings.TrimSpace(v.Code))
	v.SubjectType = strings.ToUpper(strings.TrimSpace(v.SubjectType))
	v.RequirementType = strings.ToUpper(strings.TrimSpace(v.RequirementType))
	v.Severity = strings.ToUpper(strings.TrimSpace(v.Severity))
	if v.Status == "" {
		v.Status = "DRAFT"
	}
	if v.EffectiveFrom.IsZero() {
		v.EffectiveFrom = time.Now().UTC()
	}
	if len(v.ValidityRules) == 0 {
		v.ValidityRules = json.RawMessage(`{}`)
	}
}

// ValidSubject reports whether v is a supported compliance subject type.
func ValidSubject(v string) bool {
	switch v {
	case "ORGANIZATION", "PROVIDER", "ASSET", "ORDER", "SHIPMENT", "CARGO", "WAREHOUSE", "LOCATION":
		return true
	}
	return false
}

// Check is one immutable compliance check run (spec 49.1 compliance_checks):
// every active requirement that applies to a subject, evaluated together.
// The strictest result governs (spec 49.2).
type Check struct {
	ID                     string        `json:"id"`
	OrganizationID         string        `json:"organizationId"`
	SubjectType            string        `json:"subjectType"`
	SubjectReference       string        `json:"subjectReference"`
	SourceType             string        `json:"sourceType"`
	SourceReference        string        `json:"sourceReference"`
	SourceVersion          int64         `json:"sourceVersion"`
	OverallState           string        `json:"overallState"`
	GoverningRequirementID string        `json:"governingRequirementId"`
	RequirementCount       int           `json:"requirementCount"`
	Results                []CheckResult `json:"results"`
	RestrictionReferences  []string      `json:"restrictionReferences"`
	RequestedBy            string        `json:"requestedBy"`
	CorrelationID          string        `json:"correlationId"`
	CreatedAt              time.Time     `json:"createdAt"`
}

// CheckResult links one requirement to the evaluation the check produced.
type CheckResult struct {
	RequirementID        string `json:"requirementId"`
	RequirementCode      string `json:"requirementCode"`
	Severity             string `json:"severity"`
	BlocksOperation      bool   `json:"blocksOperation"`
	ComplianceStatusID   string `json:"complianceStatusId"`
	State                string `json:"state"`
	RestrictionReference string `json:"restrictionReference,omitempty"`
}

// CheckInput requests a compliance check of one subject.
type CheckInput struct {
	SubjectType      string `json:"subjectType"`
	SubjectReference string `json:"subjectReference"`
	SourceType       string `json:"sourceType"`
	SourceReference  string `json:"sourceReference"`
	SourceVersion    int64  `json:"sourceVersion"`
}

// StateRank orders evaluation states from least to most restrictive, so the
// strictest applicable requirement governs the overall result.
var StateRank = map[string]int{
	StateNotApplicable: 0, StateCompliant: 1, StateDueSoon: 2, StatePending: 3,
	StateUnknown: 4, StateExpired: 5, StateNonCompliant: 6, StateRestricted: 7,
}

// Propagation is the confirmation state of one restriction at one execution
// authority (spec 19.1: confirmed everywhere by reconciliation).
type Propagation struct {
	Authority        string     `json:"authority"`
	Action           string     `json:"action"`
	Status           string     `json:"status"`
	Attempts         int        `json:"attempts"`
	LastErrorCode    string     `json:"lastErrorCode"`
	NextAttemptAt    time.Time  `json:"nextAttemptAt"`
	ConfirmedAt      *time.Time `json:"confirmedAt"`
	AuthorityVersion int64      `json:"authorityVersion"`
}

// RestrictionView is a restriction with its propagation state.
type RestrictionView struct {
	Restriction
	Propagation     []Propagation `json:"propagation"`
	FullyPropagated bool          `json:"fullyPropagated"`
}

// RestrictionOperation is one pending propagation of a restriction to one
// execution authority.
type RestrictionOperation struct {
	ID, Authority, Action, IdempotencyKey, Status string
	ExpectedVersion                               int64
	Attempts                                      int
}
