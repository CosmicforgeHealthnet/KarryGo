// Package models holds the Section 18 inspection domain: checklist templates,
// inspections and the pure rule that turns checklist answers into an overall
// result.
package models

import (
	"encoding/json"
	"errors"
	"time"
)

type Template struct {
	ID               string          `json:"id"`
	OrganizationID   string          `json:"organizationId"`
	Name             string          `json:"name"`
	Status           string          `json:"status"`
	AssetTypes       []string        `json:"assetTypes"`
	ServiceTypes     []string        `json:"serviceTypes"`
	ChecklistItems   json.RawMessage `json:"checklistItems"`
	CriticalityRules json.RawMessage `json:"criticalityRules"`
	Version          int64           `json:"version"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}
type Inspection struct {
	ID                    string          `json:"id"`
	OrganizationID        string          `json:"organizationId"`
	AssetID               string          `json:"assetId"`
	TemplateID            string          `json:"templateId"`
	InspectorMembershipID string          `json:"inspectorMembershipId"`
	AssignmentID          string          `json:"assignmentId"`
	TripReference         string          `json:"tripReference"`
	Status                string          `json:"status"`
	OverallResult         string          `json:"overallResult"`
	DefectSummary         json.RawMessage `json:"defectSummary"`
	EvidenceMediaIDs      json.RawMessage `json:"evidenceMediaIds"`
	StartedAt             *time.Time      `json:"startedAt"`
	CompletedAt           *time.Time      `json:"completedAt"`
	Version               int64           `json:"version"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             time.Time       `json:"updatedAt"`
}
type Answer struct {
	ItemCode         string   `json:"itemCode"`
	Passed           bool     `json:"passed"`
	Advisory         bool     `json:"advisory"`
	EvidenceMediaIDs []string `json:"evidenceMediaIds"`
}

// Page selects one keyset page of inspections.
type Page struct {
	Status  string
	Limit   int
	After   *time.Time
	AfterID string
}

type CompleteInspectionInput struct {
	ExpectedVersion int64           `json:"expectedVersion"`
	Answers         []Answer        `json:"answers"`
	DefectSummary   json.RawMessage `json:"defectSummary"`
}

// ChecklistItem is one item of a template checklist.
type ChecklistItem struct {
	Code                                 string `json:"code"`
	Required, EvidenceRequired, Critical bool
}

// Overall results of a completed inspection.
const (
	ResultPass             = "PASS"
	ResultPassWithAdvisory = "PASS_WITH_ADVISORY"
	ResultFail             = "FAIL"
	ResultCriticalFail     = "CRITICAL_FAIL"
)

var (
	// ErrMandatoryAnswerMissing: a required checklist item has no answer.
	ErrMandatoryAnswerMissing = errors.New("every mandatory inspection item requires an answer")
	// ErrEvidenceMissing: an item that requires evidence was answered without.
	ErrEvidenceMissing = errors.New("required inspection evidence is missing")
)

// Evaluate checks the answers against the checklist in item order and
// returns the overall result. Validation stops at the first failing item, so
// a missing answer on an earlier item is reported before missing evidence on
// a later one. checkEvidence is called for each answered item's evidence and
// may stop the evaluation with its own error.
func Evaluate(items []ChecklistItem, answers []Answer, checkEvidence func(Answer) error) (string, error) {
	byCode := map[string]Answer{}
	for _, a := range answers {
		byCode[a.ItemCode] = a
	}
	result := ResultPass
	for _, it := range items {
		a, ok := byCode[it.Code]
		if it.Required && !ok {
			return "", ErrMandatoryAnswerMissing
		}
		if !ok {
			continue
		}
		if it.EvidenceRequired && len(a.EvidenceMediaIDs) == 0 {
			return "", ErrEvidenceMissing
		}
		if checkEvidence != nil {
			if e := checkEvidence(a); e != nil {
				return "", e
			}
		}
		if !a.Passed {
			if it.Critical {
				result = ResultCriticalFail
			} else if result != ResultCriticalFail {
				result = ResultFail
			}
		} else if a.Advisory && result == ResultPass {
			result = ResultPassWithAdvisory
		}
	}
	return result, nil
}

// ItemFor returns the checklist item an answer refers to (the last match), or
// a zero item.
func ItemFor(items []ChecklistItem, code string) ChecklistItem {
	it := ChecklistItem{}
	for _, x := range items {
		if x.Code == code {
			it = x
		}
	}
	return it
}

// SafetyDecline is a provider decline of company work for an unsafe vehicle,
// read from the durable Section 15 assignment projections.
type SafetyDecline struct {
	OrganizationID, AssignmentID, AssetReference, MembershipID, ReasonCode, Note string
	// Source is PRIVATE_ASSIGNMENT or MARKETPLACE_ASSIGNMENT (spec 11.13,
	// 18.11): an unsafe-vehicle decline starts the same safety workflow
	// whichever kind of company work it was declined from.
	Source string
}
