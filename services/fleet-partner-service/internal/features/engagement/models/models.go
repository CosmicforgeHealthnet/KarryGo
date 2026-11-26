// Package models holds the Section 23 engagement domain: support case
// projections, internal quality cases, SLA measurements and escalations.
// Platform support lifecycles stay owned by support-dispute-service.
// Communication threads, messages and notification rules live in
// features/communications.
package models

import (
	"encoding/json"
	"math/big"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Flags gates the engagement capabilities.
type Flags struct {
	Support, Quality, SLA, Escalations bool
	Exceptions                         bool
	Workers, KillSwitch                bool
}

type SupportCreateInput struct {
	CaseType            string `json:"caseType"`
	Priority            string `json:"priority"`
	Subject             string `json:"subject"`
	Description         string `json:"description"`
	CustomerReference   string `json:"customerReference"`
	ProviderReference   string `json:"providerReference"`
	TripReference       string `json:"tripReference"`
	OrderReference      string `json:"orderReference"`
	AssignmentReference string `json:"assignmentReference"`
}
type SupportProjection struct {
	CaseID, Status, Priority, OwnerReference string
	SourceVersion                            int64
}
type SupportEvent struct {
	EventID, OrganizationID, CaseID, EventType, Status, Priority, OwnerReference string
	SourceVersion                                                                int64
	SafePayload                                                                  json.RawMessage
}
type QualityInput struct {
	BranchID                string `json:"branchId"`
	Category                string `json:"category"`
	Severity                string `json:"severity"`
	SourceType              string `json:"sourceType"`
	SourceReference         string `json:"sourceReference"`
	CustomerReference       string `json:"customerReference"`
	ProviderReference       string `json:"providerReference"`
	OwnerUserID             string `json:"ownerUserId"`
	Status                  string `json:"status"`
	RootCause               string `json:"rootCause"`
	CorrectiveActionSummary string `json:"correctiveActionSummary"`
	ExpectedVersion         int64  `json:"expectedVersion"`
}
type MeasurementInput struct {
	OrganizationID, AgreementReference, ContractID, CustomerID, MetricCode, Unit string
	FormulaVersion                                                               int64
	PeriodStart, PeriodEnd                                                       time.Time
	TargetValue, MeasuredValue                                                   string
	Complete, HigherIsBetter                                                     bool
	SourceInputs, SourceVersions                                                 json.RawMessage
}
type EscalationInput struct {
	SourceType, SourceReference       string
	Level                             int
	RecipientType, RecipientReference string
	DueAt                             time.Time
}

// DueEscalation is a pending escalation whose due time has passed.
type DueEscalation struct {
	ID, RecipientType, RecipientReference string
	Version                               int64
}

// SLAObservation is the latest observation of one active service-level
// agreement, the input to an SLA measurement.
type SLAObservation struct {
	Agreement, Contract, Customer, Metric, Target, Unit, Measured, Outcome, SourceType, SourceReference string
	FormulaVersion, SourceVersion                                                                       int64
	Start, End                                                                                          time.Time
}

// Complete reports whether the observation carries a measured value that can
// be judged against its target.
func (o SLAObservation) Complete() bool {
	return o.Outcome != "UNKNOWN" && o.Outcome != "INSUFFICIENT_DATA" && o.Outcome != "NOT_EVALUATED" && o.Measured != ""
}

// SupportEventOutcome classifies a support event against the projection's
// current source version: DUPLICATE, GAP_DETECTED or APPLIED, plus the gap
// status the projection takes.
func SupportEventOutcome(current, incoming int64) (result, gap string) {
	result, gap = "APPLIED", "CURRENT"
	if incoming <= current {
		result = "DUPLICATE"
	} else if incoming > current+1 {
		result = "GAP_DETECTED"
		gap = "GAP_DETECTED"
	}
	return result, gap
}

// ValidQualitySeverity reports whether severity (upper-case) is supported.
func ValidQualitySeverity(severity string) bool {
	return severity == "LOW" || severity == "MEDIUM" || severity == "HIGH" || severity == "CRITICAL"
}

var qualityStatuses = map[string]bool{"OPEN": true, "UNDER_REVIEW": true, "ACTION_REQUIRED": true, "MONITORING": true, "RESOLVED": true, "CLOSED": true, "CANCELLED": true}

// NextQualityStatus validates a quality-case transition.
func NextQualityStatus(status, requested string, resolve bool) (string, error) {
	if status == "CLOSED" || status == "CANCELLED" {
		return "", apperrors.Conflict("Final quality cases are immutable.", nil)
	}
	next := strings.ToUpper(requested)
	if resolve {
		next = "RESOLVED"
	}
	if next == "" {
		next = status
	}
	if !qualityStatuses[next] {
		return "", apperrors.Validation("Quality case status is invalid.", nil)
	}
	return next, nil
}

// NextEscalationStatus validates an acknowledge or resolve command.
func NextEscalationStatus(status, command string) (string, error) {
	if command == "acknowledge" && (status == "PENDING" || status == "NOTIFIED") {
		return "ACKNOWLEDGED", nil
	}
	if command == "resolve" && (status == "NOTIFIED" || status == "ACKNOWLEDGED") {
		return "RESOLVED", nil
	}
	return "", apperrors.Conflict("Escalation command is invalid for its current state.", nil)
}

func parseRat(v string) (*big.Rat, error) {
	r := new(big.Rat)
	if _, ok := r.SetString(strings.TrimSpace(v)); !ok {
		return nil, apperrors.Validation("SLA values must be fixed decimal numbers.", nil)
	}
	return r, nil
}

// BreachStatus judges a measurement against its target with exact decimal
// arithmetic. An incomplete measurement is UNKNOWN and carries no measured
// value.
func BreachStatus(in MeasurementInput) (status string, measured any, err error) {
	target, err := parseRat(in.TargetValue)
	if err != nil {
		return "", nil, err
	}
	if !in.Complete {
		return "UNKNOWN", nil, nil
	}
	m, err := parseRat(in.MeasuredValue)
	if err != nil {
		return "", nil, err
	}
	cmp := m.Cmp(target)
	met := cmp <= 0
	if in.HigherIsBetter {
		met = cmp >= 0
	}
	if met {
		return "MET", in.MeasuredValue, nil
	}
	return "BREACHED", in.MeasuredValue, nil
}

// RawOr returns v when it is valid JSON, otherwise fallback.
func RawOr(v json.RawMessage, fallback string) json.RawMessage {
	if len(v) == 0 || !json.Valid(v) {
		return json.RawMessage(fallback)
	}
	return v
}
