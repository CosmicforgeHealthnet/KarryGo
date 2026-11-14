// Package models holds the Section 23 communication domain: organization
// communication threads and messages, notification rules and the durable
// notification intents delivered through notification-service. Delivery
// itself stays owned by notification-service; Fleet records the intent and
// its delivery attempts.
package models

import (
	"encoding/json"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Flags gates the communication capabilities.
type Flags struct {
	Communications, NotificationRules bool
	Workers, KillSwitch               bool
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

// DeliveryOutcome is notification-service's answer for one intent.
type DeliveryOutcome struct {
	Reference, Status string
	SourceVersion     int64
}

// Recipient is one external thread participant a message is delivered to.
type Recipient struct{ Type, Reference string }

// IntentInput is one durable notification intent. When IgnoreDuplicate is set
// an intent with the same idempotency key is kept and no error is returned.
type IntentInput struct {
	OrganizationID     string
	SourceType         string
	SourceReference    string
	RecipientType      string
	RecipientReference string
	Channels           json.RawMessage
	TemplateCode       string
	SafePayload        json.RawMessage
	IdempotencyKey     string
	IgnoreDuplicate    bool
}

// Intent is the stored state of a notification intent the delivery worker
// submits.
type Intent struct {
	IdempotencyKey, Status, RecipientType, RecipientReference, TemplateCode string
	SafePayload, Channels                                                   []byte
	Attempts                                                                int
}

// Delivered reports whether an intent needs no further submission.
func (i Intent) Delivered() bool {
	return i.Status == "ACCEPTED" || i.Status == "DELIVERED" || i.Status == "NOT_APPLICABLE"
}

var validSubjects = map[string]bool{"CUSTOMER": true, "PROVIDER": true, "ORDER": true, "SHIPMENT": true, "ROUTE": true, "TRIP": true, "INCIDENT": true, "CLAIM": true, "INVOICE": true, "SUPPORT_CASE": true, "QUALITY_CASE": true, "OTHER": true}
var validVisibility = map[string]bool{"INTERNAL": true, "CUSTOMER_VISIBLE": true, "PROVIDER_VISIBLE": true, "RESTRICTED": true}
var validChannels = map[string]bool{"IN_APP": true, "EMAIL": true, "SMS": true, "PUSH": true, "WHATSAPP": true, "INTERNAL_NOTE": true}

// NormalizeThread upper-cases the enumerations and validates a new thread.
func NormalizeThread(in ThreadInput) (ThreadInput, error) {
	in.SubjectType = strings.ToUpper(in.SubjectType)
	in.Visibility = strings.ToUpper(in.Visibility)
	if !validSubjects[in.SubjectType] || !validVisibility[in.Visibility] || strings.TrimSpace(in.SubjectReference) == "" || strings.TrimSpace(in.Title) == "" {
		return in, apperrors.Validation("Communication thread subject, title and visibility are invalid.", nil)
	}
	return in, nil
}

// ThreadParticipants returns the creator plus the requested participants,
// normalized and de-duplicated, or a validation error.
func ThreadParticipants(actorID string, requested []ParticipantInput) ([]ParticipantInput, error) {
	participants := append([]ParticipantInput{{ParticipantType: "USER", ParticipantReference: actorID, AccessLevel: "READ_WRITE"}}, requested...)
	seen := map[string]bool{}
	out := make([]ParticipantInput, 0, len(participants))
	for _, p := range participants {
		p.ParticipantType = strings.ToUpper(p.ParticipantType)
		p.AccessLevel = strings.ToUpper(p.AccessLevel)
		if p.AccessLevel == "" {
			p.AccessLevel = "READ_WRITE"
		}
		key := p.ParticipantType + ":" + p.ParticipantReference
		if seen[key] {
			continue
		}
		seen[key] = true
		if p.ParticipantReference == "" || (p.ParticipantType != "USER" && p.ParticipantType != "CUSTOMER" && p.ParticipantType != "PROVIDER" && p.ParticipantType != "SERVICE") {
			return nil, apperrors.Validation("Thread participant is invalid.", nil)
		}
		out = append(out, p)
	}
	return out, nil
}

// NextThreadStatus validates an update of a thread from status to requested.
func NextThreadStatus(status, requested string) (string, error) {
	if status == "CLOSED" || status == "ARCHIVED" {
		return "", apperrors.Conflict("Closed communication threads are immutable.", nil)
	}
	next := strings.ToUpper(requested)
	if next == "" {
		next = status
	}
	if next != "OPEN" && next != "PENDING_RESPONSE" && next != "RESOLVED" {
		return "", apperrors.Validation("Thread status is invalid.", nil)
	}
	return next, nil
}

// ValidMessageChannel reports whether channel (upper-case) is supported.
func ValidMessageChannel(channel string) bool { return validChannels[channel] }

// ValidateQuietHours checks quiet hours use HH:MM.
func ValidateQuietHours(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var v struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return apperrors.Validation("Quiet hours must be valid JSON.", nil)
	}
	for _, x := range []string{v.Start, v.End} {
		if x == "" {
			continue
		}
		if _, err := time.Parse("15:04", x); err != nil {
			return apperrors.Validation("Quiet hours must use HH:MM.", nil)
		}
	}
	return nil
}

// RawOr returns v when it is valid JSON, otherwise fallback.
func RawOr(v json.RawMessage, fallback string) json.RawMessage {
	if len(v) == 0 || !json.Valid(v) {
		return json.RawMessage(fallback)
	}
	return v
}
