// Package audit is vendor-service's audit-trail recorder (who did what, when,
// to which aggregate, with what before/after state). Section 5 (Staff IAM)
// is the first feature to use it for real, via PostgresRecorder
// (postgres_recorder.go) persisting to vendor_audit_log
// (migrations/000004_vendor_auth_iam.up.sql). Modeled on
// fleet-partner-service's mutation+audit-in-one-transaction convention (see
// docs/architecture/09_SECURITY_AND_AUTHORITY_MODEL.md).
//
// Never given raw OTP, access tokens, refresh tokens, or step-up secrets —
// Section 5's own rule 34 forbids it, and no code path in
// internal/features/auth ever constructs an Entry containing one.
package audit

import "context"

// Recorder writes one audit entry.
type Recorder interface {
	Record(ctx context.Context, entry Entry) error
}

// Entry is one audit-trail row.
type Entry struct {
	ActorType     string // e.g. "vendor_staff", "system"
	ActorID       string
	Action        string
	AggregateType string
	AggregateID   string
	VendorID      string
	BranchID      string
	Before        any
	After         any
	Reason        string
}

// NoopRecorder discards entries. Used by features/tests that don't yet need
// real persistence.
type NoopRecorder struct{}

func (NoopRecorder) Record(context.Context, Entry) error { return nil }
