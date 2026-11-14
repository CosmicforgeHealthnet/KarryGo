package models

import "strings"

// Provider performance snapshots (spec 17 and 46.2, FLEET-17.10).
//
// Formula v2 counts BOTH company assignment channels -- marketplace
// assignments (fleet_partner_provider_assignments with their immutable
// provider responses) and private assignments -- and keeps the categories the
// spec requires apart: company assignment quality, provider response, safety,
// service quality and productivity. Decline, expiry and cancellation are
// distinct outcomes and are never folded into each other.
//
// A snapshot is a set of explainable counts. It never ranks providers, never
// triggers a suspension, penalty or deduction, and a safety-related decline is
// counted only so it can be reviewed in context -- never as misconduct. Every
// assignment that contributed is listed in metric_inputs, so each number can
// be traced back to its records. No fare, price, margin or other money field
// is read or stored.
//
// Snapshots are append-only and keyed by (membership, period, formula
// version): v1 snapshots generated before v2 stay readable and unchanged.

// PerformanceFormulaVersion is the current formula.
const PerformanceFormulaVersion = "v2"

// MaxExplainedAssignments bounds the per-assignment explanation list.
const MaxExplainedAssignments = 500

// Assignment outcomes as counted by the formula.
const (
	OutcomeCompleted               = "COMPLETED"
	OutcomeInProgress              = "IN_PROGRESS"
	OutcomePendingResponse         = "PENDING_RESPONSE"
	OutcomeDeclined                = "DECLINED"
	OutcomeExpired                 = "EXPIRED"
	OutcomeCancelledBeforeResponse = "CANCELLED_BEFORE_RESPONSE"
	OutcomeCancelledAfterAccept    = "CANCELLED_AFTER_ACCEPTANCE"
	OutcomeOther                   = "OTHER"
)

// ExplainedAssignment is one assignment that contributed to a snapshot.
type ExplainedAssignment struct {
	AssignmentID    string `json:"assignmentId"`
	Origin          string `json:"origin"`
	SourceStatus    string `json:"sourceStatus"`
	Outcome         string `json:"outcome"`
	DeclineCategory string `json:"declineCategory,omitempty"`
	SafetyRelated   bool   `json:"safetyRelated,omitempty"`
}

// ClassifyAssignment maps an assignment record to one counted outcome.
func ClassifyAssignment(status string, accepted, declined, expired bool) string {
	switch {
	case status == "COMPLETED":
		return OutcomeCompleted
	case declined || status == "DECLINED":
		return OutcomeDeclined
	case status == "CANCELLED" && accepted:
		return OutcomeCancelledAfterAccept
	case status == "CANCELLED":
		return OutcomeCancelledBeforeResponse
	case expired || status == "EXPIRED":
		return OutcomeExpired
	case status == "CREATED" || status == "PENDING_PROVIDER_RESPONSE":
		return OutcomePendingResponse
	case accepted || status == "ACCEPTED" || status == "READY_FOR_EXECUTION" || status == "IN_PROGRESS" ||
		status == "EXECUTION_START_PENDING" || status == "RECONCILIATION_REQUIRED":
		return OutcomeInProgress
	default:
		return OutcomeOther
	}
}

// PerformanceMetrics is the v2 calculated_metrics document.
type PerformanceMetrics struct {
	CompanyAssignmentQuality struct {
		AssignmentsOffered               int `json:"assignmentsOffered"`
		MarketplaceAssignmentsOffered    int `json:"marketplaceAssignmentsOffered"`
		PrivateAssignmentsOffered        int `json:"privateAssignmentsOffered"`
		CancelledByCompanyBeforeResponse int `json:"cancelledByCompanyBeforeResponse"`
	} `json:"companyAssignmentQuality"`
	ProviderResponse struct {
		Accepted                  int `json:"accepted"`
		Declined                  int `json:"declined"`
		Expired                   int `json:"expired"`
		PendingResponse           int `json:"pendingResponse"`
		AcceptanceRateBasisPoints int `json:"acceptanceRateBasisPoints"`
	} `json:"providerResponse"`
	DeclineReasons map[string]int `json:"declineReasons"`
	Safety         struct {
		SafetyRelatedDeclines int `json:"safetyRelatedDeclines"`
	} `json:"safety"`
	ServiceQuality struct {
		CompletedAfterAcceptance int `json:"completedAfterAcceptance"`
		CancelledAfterAcceptance int `json:"cancelledAfterAcceptance"`
	} `json:"serviceQuality"`
	Productivity struct {
		JobsCompleted  int `json:"jobsCompleted"`
		JobsInProgress int `json:"jobsInProgress"`
	} `json:"productivity"`
	ByOrigin map[string]map[string]int `json:"byOrigin"`
}

// NewPerformanceMetrics returns an empty v2 metrics document.
func NewPerformanceMetrics() *PerformanceMetrics {
	m := &PerformanceMetrics{
		DeclineReasons: map[string]int{"safety": 0, "operational": 0, "availability": 0, "personal": 0, "technical": 0, "other": 0},
		ByOrigin:       map[string]map[string]int{"MARKETPLACE": {}, "PRIVATE": {}},
	}
	return m
}

// Count adds one classified assignment.
func (m *PerformanceMetrics) Count(a ExplainedAssignment) {
	m.CompanyAssignmentQuality.AssignmentsOffered++
	if a.Origin == "MARKETPLACE" {
		m.CompanyAssignmentQuality.MarketplaceAssignmentsOffered++
	} else {
		m.CompanyAssignmentQuality.PrivateAssignmentsOffered++
	}
	m.ByOrigin[a.Origin][a.Outcome]++
	switch a.Outcome {
	case OutcomeCompleted:
		m.ProviderResponse.Accepted++
		m.ServiceQuality.CompletedAfterAcceptance++
		m.Productivity.JobsCompleted++
	case OutcomeInProgress:
		m.ProviderResponse.Accepted++
		m.Productivity.JobsInProgress++
	case OutcomeCancelledAfterAccept:
		m.ProviderResponse.Accepted++
		m.ServiceQuality.CancelledAfterAcceptance++
	case OutcomeDeclined:
		m.ProviderResponse.Declined++
		m.DeclineReasons[a.DeclineCategory]++
		if a.SafetyRelated {
			m.Safety.SafetyRelatedDeclines++
		}
	case OutcomeExpired:
		m.ProviderResponse.Expired++
	case OutcomePendingResponse:
		m.ProviderResponse.PendingResponse++
	case OutcomeCancelledBeforeResponse:
		m.CompanyAssignmentQuality.CancelledByCompanyBeforeResponse++
	}
}

// Finish derives the rates once every assignment has been counted.
func (m *PerformanceMetrics) Finish() {
	answered := m.ProviderResponse.Accepted + m.ProviderResponse.Declined + m.ProviderResponse.Expired
	if answered > 0 {
		m.ProviderResponse.AcceptanceRateBasisPoints = m.ProviderResponse.Accepted * 10000 / answered
	}
}

// PerformanceExplanation documents formula v2 on every snapshot.
var PerformanceExplanation = map[string]any{
	"formula": "v2: every marketplace and private company assignment created in [periodStart, periodEnd) for the " +
		"membership is classified into exactly one outcome and counted. Accepted = completed + in progress + " +
		"cancelled after acceptance. acceptanceRateBasisPoints = accepted * 10000 / (accepted + declined + expired).",
	"categories": map[string]string{
		"companyAssignmentQuality": "What the company offered, and offers the company itself withdrew before the provider answered.",
		"providerResponse":         "How the provider answered: accepted, declined and expired are counted separately.",
		"safety":                   "Declines flagged safety-related. Counted for contextual review only.",
		"serviceQuality":           "Accepted work that completed versus accepted work that was later cancelled.",
		"productivity":             "Completed and in-progress company work.",
	},
	"declinesExpiriesAndCancellationsAreDistinct": true,
	"ranking":                     false,
	"disciplinaryAction":          false,
	"automaticPunishment":         false,
	"safetyDeclinesAreMisconduct": false,
	"moneyFieldsRead":             false,
}

// DeclineCategory maps a free-text decline reason to its reporting category.
func DeclineCategory(reason string) string {
	r := strings.ToUpper(reason)
	switch {
	case strings.Contains(r, "UNSAFE") || strings.Contains(r, "SAFETY") || strings.Contains(r, "DEFECT"):
		return "Safety"
	case strings.Contains(r, "AVAILABLE") || strings.Contains(r, "BUSY"):
		return "Availability"
	case strings.Contains(r, "TECH"):
		return "Technical"
	case strings.Contains(r, "PERSONAL"):
		return "Personal"
	case strings.Contains(r, "OPERATION"):
		return "Operational"
	default:
		return "Other"
	}
}

// AssignmentRecord is one marketplace or private company assignment as the
// performance formula reads it.
type AssignmentRecord struct {
	Origin, AssignmentID, SourceStatus string
	Accepted, Declined, Expired        bool
	Reason                             string
	SafetyRelated                      bool
	Version                            int64
}
