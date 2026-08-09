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
type ParticipantInput struct{ ParticipantType, ParticipantReference, AccessLevel string }
type ThreadInput struct {
	BranchID, SubjectType, SubjectReference, Title, Visibility, OwnerUserID, Status string
	Participants                                                                    []ParticipantInput
	ExpectedVersion                                                                 int64
}
type MessageInput struct {
	Channel, Text, ReplyToMessageID string
	MediaFileIDs                    []string
	SourceTimestamp                 time.Time
}
type NotificationRuleInput struct {
	EventType, RecipientScope, Timezone, SeverityThreshold string
	Channels, QuietHours, EscalationPolicy                 json.RawMessage
	Enabled                                                *bool
	ExpectedVersion                                        int64
}
type SupportCreateInput struct {
	CaseType, Priority, Subject, Description            string
	CustomerReference, ProviderReference, TripReference string
	OrderReference, AssignmentReference                 string
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
	BranchID, Category, Severity, SourceType, SourceReference string
	CustomerReference, ProviderReference, OwnerUserID         string
	Status, RootCause, CorrectiveActionSummary                string
	ExpectedVersion                                           int64
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
