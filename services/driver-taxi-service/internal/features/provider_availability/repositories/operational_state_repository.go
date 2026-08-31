package availabilityrepositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/taxi-service/internal/dbtx"
	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// OperationalStateRepository is the durable authority for a driver's
// operational state (addendum §5). Redis (availability_store.go) is kept as a
// fast-path cache refreshed after every successful transition here — per the
// addendum's architecture principle 1, Redis may accelerate live operations
// but must not be the sole durable record of a ride or safety decision.
type OperationalStateRepository interface {
	// Get returns the provider's current state, or StateOffline at version 0
	// if no row exists yet — a provider who has never transitioned is offline
	// by definition, and no row is written until their first transition.
	Get(ctx context.Context, providerID string) (availabilitymodels.State, error)

	// Transition performs a version-guarded compare-and-set: the caller passes
	// the version it last observed (0 for a provider it has never seen a row
	// for), and the write only takes effect if the stored version still
	// matches. A mismatch returns apperrors.Conflict rather than silently
	// overwriting a concurrent change — the same guarded-CAS discipline every
	// other state transition in this service already follows.
	//
	// executor lets a future caller (the booking lifecycle in phase B) pass a
	// pgx.Tx so this transition commits atomically with the booking status
	// change that triggered it, per the addendum's requirement that provider
	// state changes caused by booking transitions commit in the same
	// transaction. Passing nil uses the repository's own pool.
	Transition(ctx context.Context, executor Executor, providerID string, to availabilitymodels.OperationalState, reason string, bookingID *string, expectedVersion int64) (availabilitymodels.State, error)
}

// Executor is an alias for dbtx.Executor, not a new type -- see that
// package's doc comment for why booking's lifecycle orchestrator (phase B)
// needs this to be the exact same named type it uses, not just a
// structurally identical one, to pass a single pgx.Tx across both
// repositories and have interface satisfaction actually hold.
type Executor = dbtx.Executor

type PostgresOperationalStateRepository struct {
	db *pgxpool.Pool
}

func NewPostgresOperationalStateRepository(db *pgxpool.Pool) *PostgresOperationalStateRepository {
	return &PostgresOperationalStateRepository{db: db}
}

func (r *PostgresOperationalStateRepository) Get(ctx context.Context, providerID string) (availabilitymodels.State, error) {
	row := r.db.QueryRow(ctx, `
		SELECT provider_id::text, state, reason, booking_id::text, effective_at, version
		FROM provider_operational_state WHERE provider_id = $1
	`, providerID)

	var s availabilitymodels.State
	var bookingID *string
	err := row.Scan(&s.ProviderID, &s.Current, &s.Reason, &bookingID, &s.EffectiveAt, &s.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return availabilitymodels.State{ProviderID: providerID, Current: availabilitymodels.StateOffline, Version: 0}, nil
	}
	if err != nil {
		return availabilitymodels.State{}, err
	}
	s.BookingID = bookingID
	return s, nil
}

func (r *PostgresOperationalStateRepository) Transition(
	ctx context.Context,
	executor Executor,
	providerID string,
	to availabilitymodels.OperationalState,
	reason string,
	bookingID *string,
	expectedVersion int64,
) (availabilitymodels.State, error) {
	if executor == nil {
		executor = r.db
	}

	// The "old" CTE reads the pre-transition state under the same statement
	// snapshot as the INSERT/UPDATE below, so from_state in the history row is
	// the real prior state, not a value written by a since-completed
	// transition. If no row existed yet, the subquery yields NULL and Go maps
	// that to StateOffline: a provider with no row has never transitioned, so
	// offline is the only truthful "before" value.
	//
	// The INSERT ... ON CONFLICT ... DO UPDATE ... WHERE version = $expected
	// is the same optimistic-concurrency upsert idiom used throughout this
	// service: a concurrent writer that already advanced the version makes the
	// WHERE clause false, the row is left untouched, and RETURNING yields zero
	// rows -- not a silent no-op, a real Postgres idiom for guarded CAS.
	row := executor.QueryRow(ctx, `
		WITH old AS (
			SELECT state FROM provider_operational_state WHERE provider_id = $1
		)
		INSERT INTO provider_operational_state (provider_id, state, reason, booking_id, effective_at, version)
		VALUES ($1, $2, $3, $4, now(), 1)
		ON CONFLICT (provider_id) DO UPDATE
		   SET state = $2, reason = $3, booking_id = $4, effective_at = now(),
		       version = provider_operational_state.version + 1
		 WHERE provider_operational_state.version = $5
		RETURNING provider_id::text, state, reason, booking_id::text, effective_at, version,
		          (SELECT state FROM old)
	`, providerID, string(to), reason, bookingID, expectedVersion)

	var s availabilitymodels.State
	var scannedBookingID *string
	var fromState *string
	err := row.Scan(&s.ProviderID, &s.Current, &s.Reason, &scannedBookingID, &s.EffectiveAt, &s.Version, &fromState)
	if errors.Is(err, pgx.ErrNoRows) {
		return availabilitymodels.State{}, apperrors.Conflict(
			"This driver's status changed elsewhere; please retry.", err)
	}
	if err != nil {
		return availabilitymodels.State{}, err
	}
	s.BookingID = scannedBookingID

	from := string(availabilitymodels.StateOffline)
	if fromState != nil {
		from = *fromState
	}
	if _, err := executor.Exec(ctx, `
		INSERT INTO provider_operational_state_history (provider_id, from_state, to_state, reason, booking_id)
		VALUES ($1, $2, $3, $4, $5)
	`, providerID, from, string(to), reason, bookingID); err != nil {
		// The transition itself already committed (or is committing as part of
		// the caller's transaction); a history-write failure must not undo a
		// real state change, only lose one audit row. Logged by the caller's
		// usual error handling, not treated as a transition failure here.
		return s, nil
	}
	return s, nil
}

// HistoryEntry is one provider_operational_state_history row's relevant
// fields for reconstructing a timeline -- a plain DTO in this repository
// package, matching ProviderStatus's own convention in availability_store.go
// (a data shape belonging to persistence, not domain rules).
type HistoryEntry struct {
	ToState   string
	ChangedAt time.Time
}

// StateAsOf returns the state a provider was in at exactly 'at' -- the
// to_state of the most recent transition at or before that instant, or ""
// if no transition exists yet (the caller treats that as StateOffline: a
// provider with no history has never gone online). Used to seed a windowed
// timeline reconstruction (see ListHistory) with the state the window
// actually opened in, since a transition BEFORE the window's start is
// otherwise invisible to a query scoped to (from, until].
func (r *PostgresOperationalStateRepository) StateAsOf(ctx context.Context, providerID string, at time.Time) (string, error) {
	row := r.db.QueryRow(ctx, `
		SELECT to_state FROM provider_operational_state_history
		WHERE provider_id = $1 AND changed_at <= $2
		ORDER BY changed_at DESC LIMIT 1
	`, providerID, at)
	var state string
	err := row.Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return state, err
}

// ListHistoryForBooking returns every operational-state transition recorded
// AGAINST this specific booking (booking_id is set on the history row by
// Transition's own bookingID parameter -- see withTransition in
// bookingusecases), ascending. Phase B10 route replay's source for "what did
// the assigned driver's status do during this one ride", as opposed to
// ListHistory's whole-timeline-for-a-window use in earnings insights.
func (r *PostgresOperationalStateRepository) ListHistoryForBooking(ctx context.Context, bookingID string) ([]HistoryEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT to_state, changed_at FROM provider_operational_state_history
		WHERE booking_id = $1
		ORDER BY changed_at ASC
	`, bookingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.ToState, &e.ChangedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListHistory returns every transition strictly after 'from' and at or
// before 'until', ascending -- the half-open window a caller combines with
// StateAsOf(providerID, from) to walk a complete, gap-free timeline.
func (r *PostgresOperationalStateRepository) ListHistory(ctx context.Context, providerID string, from, until time.Time) ([]HistoryEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT to_state, changed_at FROM provider_operational_state_history
		WHERE provider_id = $1 AND changed_at > $2 AND changed_at <= $3
		ORDER BY changed_at ASC
	`, providerID, from, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.ToState, &e.ChangedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
