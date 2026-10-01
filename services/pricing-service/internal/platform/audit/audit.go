// Package audit records who changed pricing configuration, why, and what it
// looked like before and after.
//
// The audit trail is deliberately separate from the outbox event stream. Events
// tell other services what happened; the audit trail is the evidence for a
// financial reviewer, and it answers a different question: which human asked for
// this, which service carried it, what did they say their reason was, and who
// approved it.
//
// Every entry is written in the SAME transaction as the change it describes, so
// a change can never exist without its audit row. The table is append-only,
// enforced by a database trigger rather than convention.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"cosmicforge/logistics/services/pricing-service/internal/database"
)

// Entry is one recorded change.
type Entry struct {
	// ActorCaller is the verified signing service, e.g. admin-backoffice-service.
	ActorCaller string
	// PerformedBy is the human the caller named. Both are stored: the service
	// is proven by signature, the person is asserted by the service.
	PerformedBy string
	// Action is a stable verb such as POLICY_VERSION_APPROVED.
	Action string
	// Subject identifies what changed.
	SubjectType string
	SubjectID   string
	// Reason is mandatory. A price change without a stated reason is not
	// reviewable later.
	Reason string
	// Before and After are optional snapshots or references.
	Before any
	After  any
	// RequestID ties the entry to the HTTP request in the logs.
	RequestID string
}

// ErrIncomplete is returned when an entry is missing evidence that a reviewer
// would need.
var ErrIncomplete = errors.New("audit: entry is incomplete")

// Recorder writes audit entries.
type Recorder interface {
	Record(ctx context.Context, q database.Querier, entry Entry) error
}

// PostgresRecorder writes to pricing_audit_log.
type PostgresRecorder struct{}

// NewPostgresRecorder returns the production recorder.
func NewPostgresRecorder() PostgresRecorder { return PostgresRecorder{} }

// Record appends entry using q, which must be the transaction performing the
// change.
func (PostgresRecorder) Record(ctx context.Context, q database.Querier, entry Entry) error {
	if err := entry.validate(); err != nil {
		return err
	}

	before, err := marshalOptional(entry.Before)
	if err != nil {
		return fmt.Errorf("audit: before: %w", err)
	}
	after, err := marshalOptional(entry.After)
	if err != nil {
		return fmt.Errorf("audit: after: %w", err)
	}

	_, err = q.Exec(ctx, `
		INSERT INTO pricing_audit_log
			(actor_caller, performed_by, action, subject_type, subject_id, reason,
			 before_snapshot, after_snapshot, request_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		entry.ActorCaller, entry.PerformedBy, entry.Action, entry.SubjectType,
		entry.SubjectID, entry.Reason, before, after, entry.RequestID)
	if err != nil {
		return fmt.Errorf("audit: record %s: %w", entry.Action, err)
	}
	return nil
}

func (e Entry) validate() error {
	var missing []string
	if strings.TrimSpace(e.ActorCaller) == "" {
		missing = append(missing, "actor_caller")
	}
	if strings.TrimSpace(e.PerformedBy) == "" {
		missing = append(missing, "performed_by")
	}
	if strings.TrimSpace(e.Action) == "" {
		missing = append(missing, "action")
	}
	if strings.TrimSpace(e.SubjectType) == "" {
		missing = append(missing, "subject_type")
	}
	if strings.TrimSpace(e.SubjectID) == "" {
		missing = append(missing, "subject_id")
	}
	if strings.TrimSpace(e.Reason) == "" {
		missing = append(missing, "reason")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: missing %s", ErrIncomplete, strings.Join(missing, ", "))
	}
	return nil
}

func marshalOptional(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return json.Marshal(value)
}
