// Package engagement implements organization communications, support projections,
// internal quality, SLA snapshots and escalations. Delivery and platform support
// lifecycles remain owned by their authoritative services.
package engagement

import (
	"context"
	"encoding/json"
	"time"
)

type Flags struct {
	Communications, NotificationRules, Support, Quality, SLA, Escalations bool
	Workers, KillSwitch                                                   bool
}
type ParticipantInput struct {
	ParticipantType      string `json:"participantType"`
	ParticipantReference string `json:"participantReference"`
	AccessLevel          string `json:"accessLevel"`
}
type ThreadInput struct {
	BranchID         string             `json:"branchId"`
	SubjectType      string             `json:"subjectType"`
	SubjectReference string             `json:"subjectReference"`
	Title            string             `json:"title"`
	Visibility       string             `json:"visibility"`
	OwnerUserID      string             `json:"ownerUserId"`
	Status           string             `json:"status"`
	Participants     []ParticipantInput `json:"participants"`
	ExpectedVersion  int64              `json:"expectedVersion"`
}
type MessageInput struct {
	Channel          string    `json:"channel"`
	Text             string    `json:"text"`
	ReplyToMessageID string    `json:"replyToMessageId"`
	MediaFileIDs     []string  `json:"mediaFileIds"`
	SourceTimestamp  time.Time `json:"sourceTimestamp"`
}
type NotificationRuleInput struct {
	EventType         string          `json:"eventType"`
	RecipientScope    string          `json:"recipientScope"`
	Timezone          string          `json:"timezone"`
	SeverityThreshold string          `json:"severityThreshold"`
	Channels          json.RawMessage `json:"channels"`
	QuietHours        json.RawMessage `json:"quietHours"`
	EscalationPolicy  json.RawMessage `json:"escalationPolicy"`
	Enabled           *bool           `json:"enabled"`
	ExpectedVersion   int64           `json:"expectedVersion"`
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
type DeliveryOutcome struct {
	Reference, Status string
	SourceVersion     int64
}

type SupportPort interface {
	Create(context.Context, string, string, SupportCreateInput) (SupportProjection, error)
	Read(context.Context, string, string) (SupportProjection, error)
	Configured() bool
}
type NotificationPort interface {
	Submit(context.Context, string, string, json.RawMessage) (DeliveryOutcome, error)
	Read(context.Context, string, string) (DeliveryOutcome, error)
	Configured() bool
}
