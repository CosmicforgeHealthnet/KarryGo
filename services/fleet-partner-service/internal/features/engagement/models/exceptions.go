package models

import "time"

// ExceptionInput raises or updates an operational exception (spec 53.2).
type ExceptionInput struct {
	BranchID        string   `json:"branchId"`
	ExceptionType   string   `json:"exceptionType"`
	Severity        string   `json:"severity"`
	SourceType      string   `json:"sourceType"`
	SourceReference string   `json:"sourceReference"`
	Summary         string   `json:"summary"`
	OwnerUserID     string   `json:"ownerUserId"`
	EvidenceFileIDs []string `json:"evidenceFileIds"`
	ExpectedVersion int64    `json:"expectedVersion"`
	RootCause       string   `json:"rootCause"`
	Resolution      string   `json:"resolution"`
	Note            string   `json:"note"`
}

// ExceptionState is a locked exception row.
type ExceptionState struct {
	Status, Severity, BranchID, OwnerUserID string
	EscalationLevel                         int
	Version                                 int64
	SLADueAt                                time.Time
}

// ExceptionTypes and ExceptionSeverities are the supported vocabularies.
var (
	ExceptionTypes      = map[string]bool{"DELAY": true, "DAMAGE": true, "LOSS": true, "CUSTODY": true, "TEMPERATURE": true, "PROVIDER_UNAVAILABLE": true, "VEHICLE": true, "CUSTOMER": true, "DOCUMENT": true, "SYSTEM": true, "OTHER": true}
	ExceptionSeverities = map[string]bool{"LOW": true, "MEDIUM": true, "HIGH": true, "CRITICAL": true}
)

// ExceptionSLA is the response deadline each severity carries.
func ExceptionSLA(severity string) time.Duration {
	switch severity {
	case "CRITICAL":
		return 15 * time.Minute
	case "HIGH":
		return time.Hour
	case "MEDIUM":
		return 4 * time.Hour
	}
	return 24 * time.Hour
}

// OverdueException is a live exception past its SLA.
type OverdueException struct {
	ID              string
	EscalationLevel int
}
