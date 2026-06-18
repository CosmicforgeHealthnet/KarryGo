package messagemodels

import (
	"time"
)

const (
	StatusQueued       = "queued"
	StatusProcessing   = "processing"
	StatusSent         = "sent"
	StatusSkipped      = "skipped"
	StatusSuppressed   = "suppressed"
	StatusRetrying     = "retrying"
	StatusFailed       = "failed"
	StatusDeadLettered = "dead_lettered"
)

type Message struct {
	ID             string
	IdempotencyKey string
	SourceService  string
	EventType      string
	RecipientType  string
	RecipientID    string
	RecipientEmail *string
	RecipientPhone *string
	Channels       []string
	TemplateKey    *string
	Locale         string
	Title          string
	Body           string
	Data           map[string]interface{}
	TemplateData   map[string]interface{}
	Priority       string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Delivery struct {
	ID                string
	MessageID         string
	Channel           string
	Status            string
	Attempts          int
	Provider          *string
	ProviderMessageID *string
	LastError         *string
	NextAttemptAt     *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Template struct {
	Key             string
	Locale          string
	Title           string
	Body            string
	DefaultChannels []string
	Active          bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Device struct {
	ID            string
	RecipientType string
	RecipientID   string
	Token         string
	Platform      string
	App           string
	Active        bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Preference struct {
	RecipientType string
	RecipientID   string
	Channel       string
	Enabled       bool
}
