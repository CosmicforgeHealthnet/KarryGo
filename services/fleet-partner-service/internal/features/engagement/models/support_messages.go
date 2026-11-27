package models

import "time"

// Support-case message statuses in Fleet's submission ledger.
const (
	SupportMessageSubmitting     = "SUBMITTING"
	SupportMessageOutcomeUnknown = "OUTCOME_UNKNOWN"
	SupportMessageConfirmed      = "CONFIRMED"
	SupportMessageRejected       = "REJECTED"
)

// Support-case message visibility (support-dispute-service enforces it):
// SHARED is visible to the organization and platform support; INTERNAL is
// for platform support only and never shown on complainant routes.
const (
	SupportMessageShared   = "SHARED"
	SupportMessageInternal = "INTERNAL"
)

// SupportMessageInput is POST /support-cases/{id}/messages.
type SupportMessageInput struct {
	ClientMessageID string   `json:"clientMessageId"`
	Text            string   `json:"text"`
	FileIDs         []string `json:"fileIds"`
	Visibility      string   `json:"visibility"`
}

// SupportMessageRequest is the message as submitted to support-dispute-service.
type SupportMessageRequest struct {
	OrganizationID, SupportCaseID, ClientMessageID, SenderReference, Text, Visibility string
	FileIDs                                                                            []string
}

// SupportMessage is the authoritative message as support-dispute-service
// returns it.
type SupportMessage struct {
	ID, ClientMessageID, Text, Visibility string
	FileIDs                               []string
	CreatedAt                             time.Time
}

// SupportMessageLedger is Fleet's submission record for one message.
type SupportMessageLedger struct {
	ID               string     `json:"id"`
	OrganizationID   string     `json:"organizationId"`
	ProjectionID     string     `json:"supportCaseProjectionId"`
	SupportCaseID    string     `json:"supportCaseId"`
	ClientMessageID  string     `json:"clientMessageId"`
	RequestHash      string     `json:"-"`
	Visibility       string     `json:"visibility"`
	FileIDs          []string   `json:"fileIds"`
	Status           string     `json:"status"`
	SupportMessageID string     `json:"supportMessageId,omitempty"`
	Attempts         int        `json:"attempts"`
	LastErrorCode    string     `json:"lastErrorCode,omitempty"`
	CreatedBy        string     `json:"createdBy"`
	Version          int64      `json:"version"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	ConfirmedAt      *time.Time `json:"confirmedAt,omitempty"`
}
