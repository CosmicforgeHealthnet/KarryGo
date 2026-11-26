// Package models holds the Section 28/56 governance domain: the
// organization's security-event read model and privacy requests.
package models

import "encoding/json"

// Flags gate the governance capabilities; all default off.
type Flags struct{ PrivacyRequests, SecurityEvents, KillSwitch bool }

// PrivacyRequestInput submits a privacy request.
type PrivacyRequestInput struct {
	RequestType      string `json:"requestType"`
	SubjectType      string `json:"subjectType"`
	SubjectReference string `json:"subjectReference"`
	Reason           string `json:"reason"`
}

// PrivacyDecisionInput approves, rejects or completes a request.
type PrivacyDecisionInput struct {
	Decision         string          `json:"decision"`
	Reason           string          `json:"reason"`
	CompletionReport json.RawMessage `json:"completionReport"`
	ExpectedVersion  int64           `json:"expectedVersion"`
}

// PrivacyState is a locked request.
type PrivacyState struct {
	RequestType, SubjectType, SubjectReference, Status, RequestedBy string
	Version                                                         int64
}

var (
	PrivacyRequestTypes = map[string]bool{"EXPORT": true, "ERASURE": true, "RECTIFICATION": true, "RESTRICTION": true}
	PrivacySubjectTypes = map[string]bool{"PROVIDER": true, "CUSTOMER": true, "ORGANIZATION_USER": true}
)

// HoldScopeFor maps a privacy subject to the legal-hold scope that covers it.
func HoldScopeFor(subjectType string) string {
	switch subjectType {
	case "PROVIDER":
		return "PROVIDER"
	case "CUSTOMER":
		return "CUSTOMER"
	}
	return ""
}
