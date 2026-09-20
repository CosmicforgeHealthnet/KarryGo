package policyingest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Tables names the two tables one service uses.
//
// The store is parameterized rather than duplicated because the SQL is where
// the safety properties actually live -- the ON CONFLICT that makes claiming
// atomic, the transaction that keeps snapshot and inbox in step, the guard
// that keeps versions monotonic. Two hand-maintained copies of that would be
// two chances to weaken one of them, and this service already has a documented
// history of column lists drifting apart when copied.
//
// Each service still OWNS its schema: it creates its own tables in its own
// migration and passes their names here. Nothing in this package reaches
// across a service boundary.
type Tables struct {
	// Publications is the inbox table (e.g. hauling_policy_publications).
	Publications string
	// Snapshots is the immutable snapshot table (e.g. hauling_policy_snapshots).
	Snapshots string
}

// Valid reports whether both names look like plain identifiers.
//
// They are interpolated into SQL because a table name cannot be a bind
// parameter. That makes validating them a security control, not a nicety:
// these values come from a service's own wiring rather than from a request,
// but a constructor that accepts arbitrary text is one refactor away from
// accepting one that does.
func (t Tables) Valid() bool {
	return validIdentifier(t.Publications) && validIdentifier(t.Snapshots)
}

func validIdentifier(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || len(trimmed) > 63 {
		return false
	}
	for i, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z', r == '_':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// ErrInvalidTables is returned when a caller supplies unusable table names.
var ErrInvalidTables = errors.New("policyingest: table names are invalid")

// PostgresStore implements Store against one service's own schema.
type PostgresStore struct {
	db     *pgxpool.Pool
	tables Tables
}

// NewPostgresStore builds the store.
func NewPostgresStore(db *pgxpool.Pool, tables Tables) (*PostgresStore, error) {
	if db == nil {
		return nil, errors.New("policyingest: database pool is required")
	}
	if !tables.Valid() {
		return nil, fmt.Errorf("%w: %+v", ErrInvalidTables, tables)
	}
	return &PostgresStore{db: db, tables: tables}, nil
}

// ClaimEvent inserts the inbox row, returning false when the event_id is
// already present.
//
// ON CONFLICT DO NOTHING plus a RETURNING is what makes this atomic under
// concurrent delivery: exactly one of two simultaneous inserts gets a row
// back. A SELECT-then-INSERT would let both callers see "not present" and
// both proceed to apply.
func (s *PostgresStore) ClaimEvent(ctx context.Context, record EventRecord) (bool, error) {
	statement := `
INSERT INTO ` + s.tables.Publications + `
	(event_id, policy_kind, policy_version, source_service, source_version, schema_version,
	 checksum, document_ref, state, correlation_id, occurred_at, effective_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'PENDING_READBACK',$9,$10,$11)
ON CONFLICT (event_id) DO NOTHING
RETURNING event_id`

	// The row starts PENDING_READBACK and is moved to its real state by the
	// same call that decides it. Starting in a state that means "not finished"
	// is what makes a crash between claim and apply recoverable: the reconcile
	// worker finds it rather than it being silently lost.
	var claimed string
	err := s.db.QueryRow(ctx, statement,
		record.EventID, record.PolicyKind, record.PolicyVersion, record.SourceService,
		record.SourceVersion, record.SchemaVersion, record.Checksum, record.DocumentRef,
		record.CorrelationID, record.OccurredAt.UTC(), record.EffectiveAt.UTC(),
	).Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// A freshly claimed publication has no next attempt scheduled yet; the
	// caller either completes it now or parks it with a real schedule.
	_, err = s.db.Exec(ctx,
		`UPDATE `+s.tables.Publications+` SET next_attempt_at = NULL WHERE event_id = $1`,
		record.EventID)
	return true, err
}

// AppliedSourceVersion returns the newest applied source version for a kind.
func (s *PostgresStore) AppliedSourceVersion(ctx context.Context, policyKind string) (int64, bool, error) {
	var version int64
	err := s.db.QueryRow(ctx,
		`SELECT source_version FROM `+s.tables.Publications+`
		  WHERE policy_kind = $1 AND state = 'APPLIED'
		  ORDER BY source_version DESC LIMIT 1`, policyKind).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return version, true, nil
}

// ApplySnapshot writes the snapshot and marks the event applied atomically.
//
// If these could land separately the pair would disagree in one of two ways,
// both bad: an applied snapshot with an unmarked event is re-applied on the
// next delivery, and a marked event with no snapshot is skipped forever.
func (s *PostgresStore) ApplySnapshot(ctx context.Context, snapshot Snapshot) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// DO NOTHING rather than DO UPDATE: a snapshot is immutable, and the
	// table's own trigger would refuse an overwrite anyway. Re-publishing the
	// same (kind, version) is normal and must be a no-op, not an error.
	if _, err = tx.Exec(ctx, `
INSERT INTO `+s.tables.Snapshots+`
	(policy_kind, policy_version, payload, checksum, effective_at, applied_at, source_version, event_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (policy_kind, policy_version) DO NOTHING`,
		snapshot.PolicyKind, snapshot.PolicyVersion, []byte(snapshot.Document),
		snapshot.Checksum, snapshot.EffectiveAt.UTC(), snapshot.AppliedAt.UTC(),
		snapshot.SourceVersion, snapshot.EventID); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
UPDATE `+s.tables.Publications+`
   SET state='APPLIED', reason_code=$2, applied_at=$3, next_attempt_at=NULL, last_error=''
 WHERE event_id=$1`, snapshot.EventID, ReasonApplied, snapshot.AppliedAt.UTC()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MarkTerminal records a decided no-op or rejection.
func (s *PostgresStore) MarkTerminal(ctx context.Context, eventID string, outcome Outcome, reasonCode string) error {
	state := string(outcome)
	switch outcome {
	case OutcomeStale, OutcomeRejected, OutcomeDuplicate:
	default:
		// Only decided outcomes belong here. APPLIED goes through
		// ApplySnapshot so it is always paired with a snapshot, and
		// RECONCILIATION_REQUIRED is not terminal at all.
		return fmt.Errorf("policyingest: %s is not a terminal inbox state", outcome)
	}
	_, err := s.db.Exec(ctx, `
UPDATE `+s.tables.Publications+`
   SET state=$2, reason_code=$3, next_attempt_at=NULL
 WHERE event_id=$1`, eventID, state, reasonCode)
	return err
}

// MarkPendingReadBack parks a publication whose read-back outcome is unknown.
func (s *PostgresStore) MarkPendingReadBack(ctx context.Context, eventID, reasonCode string, nextAttemptAt time.Time) error {
	_, err := s.db.Exec(ctx, `
UPDATE `+s.tables.Publications+`
   SET state='PENDING_READBACK', reason_code=$2, last_error=$2,
       attempts=attempts+1, next_attempt_at=$3
 WHERE event_id=$1`, eventID, reasonCode, nextAttemptAt.UTC())
	return err
}

// PendingReadBacks returns publications whose retry is due.
//
// The NULL check matters: a row claimed a moment ago has no schedule yet and
// is still being handled by the request that claimed it. Picking it up here
// would have the worker race the ingress on the same publication.
func (s *PostgresStore) PendingReadBacks(ctx context.Context, now time.Time, limit int) ([]EventRecord, error) {
	rows, err := s.db.Query(ctx, `
SELECT event_id, policy_kind, policy_version, source_service, source_version,
       schema_version, checksum, document_ref, correlation_id, occurred_at,
       effective_at, attempts
  FROM `+s.tables.Publications+`
 WHERE state='PENDING_READBACK' AND next_attempt_at IS NOT NULL AND next_attempt_at <= $1
 ORDER BY next_attempt_at
 LIMIT $2`, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EventRecord
	for rows.Next() {
		var record EventRecord
		if err := rows.Scan(&record.EventID, &record.PolicyKind, &record.PolicyVersion,
			&record.SourceService, &record.SourceVersion, &record.SchemaVersion,
			&record.Checksum, &record.DocumentRef, &record.CorrelationID,
			&record.OccurredAt, &record.EffectiveAt, &record.Attempts); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

// PendingCount reports how many publications are awaiting reconciliation, for
// the operational metric. A number that only grows is the signal that this
// deployment has stopped being able to reach the control plane.
func (s *PostgresStore) PendingCount(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRow(ctx,
		`SELECT count(*) FROM `+s.tables.Publications+` WHERE state='PENDING_READBACK'`).Scan(&count)
	return count, err
}
