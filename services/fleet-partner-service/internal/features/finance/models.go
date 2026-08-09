package finance

import (
	"encoding/json"
	"time"
)

type Operation struct {
	ID, OrganizationID, BranchID                                string
	SubjectType, SubjectID, OperationType                       string
	Payload                                                     json.RawMessage
	PayloadVersion                                              int
	ExpectedVersion                                             int64
	IdempotencyKey, RequestFingerprint, WalletReference, Status string
	Attempts                                                    int
	NextAttemptAt                                               time.Time
	LeaseOwner                                                  string
	LeaseExpiresAt                                              *time.Time
	SafeErrorCode, CorrelationID, CausationID                   string
	RequestedByActorType, RequestedByActorID                    string
	Version                                                     int64
}
type Projection struct {
	ID, OrganizationID, ProjectionType, SubjectType, SubjectID, WalletReference string
	AmountMinor                                                                 *int64
	Currency, SafeStatus                                                        string
	SourceVersion                                                               int64
	FreshnessState, LastEventID                                                 string
	LastSyncedAt                                                                time.Time
	ReconciliationStatus                                                        string
	ReconciliationAttempts                                                      int
	ReconciliationNextAttemptAt                                                 time.Time
	ReconciliationLeaseOwner                                                    string
	ReconciliationLeaseExpiresAt                                                *time.Time
	ReconciliationSafeErrorCode                                                 string
	Version                                                                     int64
}

const (
	StatusPending     = "PENDING"
	StatusProcessing  = "PROCESSING"
	StatusUnknown     = "UNKNOWN"
	StatusReconciling = "RECONCILING"
	StatusSucceeded   = "SUCCEEDED"
	StatusRetryable   = "RETRYABLE_FAILURE"
	StatusPermanent   = "PERMANENT_FAILURE"
	StatusDeadLetter  = "DEAD_LETTER"
)
