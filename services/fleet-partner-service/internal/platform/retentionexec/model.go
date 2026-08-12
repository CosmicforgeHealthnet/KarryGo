// Package retentionexec implements Section 28's executable retention: real
// archive/anonymize execution behind internal/features/retentionpolicy's
// existing configuration (organization-configured days, bounded by a
// platform minimum -- unchanged, still the sole source of truth for HOW
// LONG a class is retained) and internal/features/legalhold's existing
// hold system (unchanged, still an absolute veto).
//
// Scoped to a small, explicit allowlist of ClassExecutor implementations --
// there is no generic "delete this table" capability anywhere in this
// package, deliberately: every table this schema owns that is immutable or
// evidentiary (fleet_partner_audit_records, fleet_partner_job_attempts,
// fleet_partner_scheduled_task_runs, and every financial, trip-execution,
// safety, incident and compliance table) has NO ClassExecutor and
// therefore can never be touched by this package no matter what a run
// requests -- Runner.RunOnce refuses to even start a run for a class with
// no registered executor (see runner.go). Adding a new executor is a
// reviewed code change, never a runtime configuration value.
package retentionexec

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
)

// Mode selects between reporting what retention WOULD do and actually
// doing it.
const (
	ModeDryRun  = "DRY_RUN"
	ModeExecute = "EXECUTE"
)

// Outcome is the immutable, per-record result of one retention decision.
const (
	OutcomeArchived                = "ARCHIVED"
	OutcomeAnonymized              = "ANONYMIZED"
	OutcomeSkippedMinimumRetention = "SKIPPED_MINIMUM_RETENTION"
	OutcomeSkippedLegalHold        = "SKIPPED_LEGAL_HOLD"
	OutcomeSkippedImmutable        = "SKIPPED_IMMUTABLE"
	OutcomeSkippedActiveReference  = "SKIPPED_ACTIVE_REFERENCE"
	OutcomeFailed                  = "FAILED"
)

// RunStatus is one run's own lifecycle state (distinct from any individual
// record's Outcome).
const (
	RunStatusPending   = "PENDING"
	RunStatusRunning   = "RUNNING"
	RunStatusCompleted = "COMPLETED"
	RunStatusFailed    = "FAILED"
)

// Candidate is one record a ClassExecutor found for consideration --
// nothing about a Candidate implies a decision has been made yet; every
// candidate still passes through the generic legal-hold and
// minimum-retention checks in runner.go before any executor method that
// could report ARCHIVED/ANONYMIZED is even called.
type Candidate struct {
	RecordType string
	RecordID   string
}

// ClassExecutor implements retention for exactly one retention class (see
// internal/features/retentionpolicy/models.go's Class* constants). A class
// with no registered ClassExecutor can never be executed -- see the
// package doc comment.
type ClassExecutor interface {
	// Class is the retentionpolicy.models class this executor implements.
	Class() string
	// FindCandidates returns up to limit records eligible for
	// CONSIDERATION (not yet a decision) for organizationID, in a stable
	// order, so repeated bounded batches make forward progress rather than
	// re-reading the same page forever. Must already exclude anything the
	// executor itself knows is not applicable (e.g. an organization that is
	// not CLOSED at all for a post-closure class) -- but must NOT itself
	// apply the legal-hold or minimum-retention checks; those are
	// cross-cutting and applied once, generically, by Runner.
	FindCandidates(ctx context.Context, q database.Querier, organizationID string, limit int) ([]Candidate, error)
	// ScopeForLegalHold maps a candidate to the (scopeType, scopeReference)
	// internal/features/legalhold's IsUnderHold expects.
	ScopeForLegalHold(candidate Candidate) (scopeType, scopeReference string)
	// EligibleSince returns the point in time FROM WHICH this candidate's
	// retention clock runs (e.g. the organization's closure completion
	// time) -- Runner compares time.Since(EligibleSince) against the
	// organization's effective retention days for this class.
	EligibleSince(ctx context.Context, q database.Querier, candidate Candidate) (time.Time, error)
	// Preview reports what WOULD happen (OutcomeArchived or
	// OutcomeAnonymized) without writing anything. q is read-only by
	// convention (a *pgxpool.Pool in every real call site, never a Tx) --
	// DRY_RUN mode's engine path never opens a transaction at all, so a
	// Preview implementation has no write-capable handle available even if
	// it tried.
	Preview(ctx context.Context, q database.Querier, candidate Candidate) (outcome, detail string, err error)
	// Execute performs the real action inside tx, called only for
	// EXECUTE-mode runs, only after every generic check already passed.
	Execute(ctx context.Context, tx pgx.Tx, candidate Candidate) (outcome, detail string, err error)
}

// LegalHoldChecker mirrors legalhold's usecases.Service.IsUnderHold as a
// narrow port -- this platform package does not import the legalhold
// feature's usecases package directly (composition-root wiring supplies
// the real adapter), matching this codebase's established port/adapter
// convention (internal/architecture_test.go).
type LegalHoldChecker interface {
	IsUnderHold(ctx context.Context, organizationID, scopeType, scopeReference string) (bool, error)
}

// EffectiveRetentionDaysReader resolves an organization's effective
// (organization-configured, platform-minimum-bounded) retention duration
// for a class -- mirrors retentionpolicy's Policy.EffectiveDays() without
// this package importing that feature's models package directly.
type EffectiveRetentionDaysReader interface {
	EffectiveRetentionDays(ctx context.Context, organizationID, class string) (int, error)
}

// Run is one initiated (organization, class, mode) retention pass.
type Run struct {
	ID                string
	OrganizationID    string
	RetentionClass    string
	Mode              string
	Status            string
	JobID             string
	InitiatedBy       string
	IdempotencyKey    string
	RecordsConsidered int
	RecordsArchived   int
	RecordsAnonymized int
	RecordsSkipped    int
	RecordsFailed     int
	ErrorMessage      string
	StartedAt         *time.Time
	CompletedAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// RunRecord is one immutable per-candidate outcome.
type RunRecord struct {
	ID             string
	RunID          string
	OrganizationID string
	RecordType     string
	RecordID       string
	Outcome        string
	Detail         string
	ProcessedAt    time.Time
}
