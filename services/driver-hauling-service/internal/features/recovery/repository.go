package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound means no such case or booking.
	ErrNotFound = errors.New("recovery: not found")
	// ErrConflict means a guarded write lost: a stale version, a terminal
	// state, or a transition that would move the case backwards.
	ErrConflict = errors.New("recovery: conflict")
)

// Repository is the Postgres store.
type Repository struct {
	db *pgxpool.Pool
	q  querier
}

// querier is satisfied by both the pool and a caller-owned transaction.
type querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// NewRepository builds the store.
func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db, q: db} }

// WithTx returns a view of the store whose every read and guarded write runs
// inside the caller's transaction (administrative replacement uses it so the
// case advance, booking rebind and owner decision commit together).
func (r *Repository) WithTx(tx pgx.Tx) *Repository { return &Repository{db: r.db, q: tx} }

// DB exposes the pool so the service can run a replacement and its custody
// transfer in ONE transaction. Without that, there is a window in which the
// ledger says the new provider holds the cargo and the booking says the old
// one does -- and a crash inside that window leaves it there permanently.
func (r *Repository) DB() *pgxpool.Pool { return r.db }

const caseColumns = `id::text, booking_id::text, mission_ref, trigger_code, state,
	COALESCE(original_provider_id::text,''), COALESCE(original_truck_id::text,''),
	COALESCE(replacement_provider_id::text,''), COALESCE(replacement_truck_id::text,''),
	custody_transfer_required, custody_transfer_ref, evidence_ref,
	reason_code, operation_ref, correlation_id,
	detected_at, started_at, resolved_at, version`

func scanCase(row pgx.Row) (Case, error) {
	var recoveryCase Case
	var state string
	err := row.Scan(&recoveryCase.ID, &recoveryCase.BookingID, &recoveryCase.MissionRef,
		&recoveryCase.TriggerCode, &state,
		&recoveryCase.OriginalProviderID, &recoveryCase.OriginalTruckID,
		&recoveryCase.ReplacementProviderID, &recoveryCase.ReplacementTruckID,
		&recoveryCase.CustodyTransferRequired, &recoveryCase.CustodyTransferRef,
		&recoveryCase.EvidenceRef, &recoveryCase.ReasonCode, &recoveryCase.OperationRef,
		&recoveryCase.CorrelationID, &recoveryCase.DetectedAt, &recoveryCase.StartedAt,
		&recoveryCase.ResolvedAt, &recoveryCase.Version)
	recoveryCase.State = State(state)
	return recoveryCase, err
}

// BookingSnapshot is the slice of the booking recovery needs.
type BookingSnapshot struct {
	ID                 string
	Status             string
	ProviderID         string
	TruckID            string
	MissionRef         string
	CargoWeightKg      int
	PreferredTruckType string
	IsFragile          bool
	PickupLat          float64
	PickupLng          float64
	// CustodyTaken reports whether the provider is already holding the cargo.
	// It is derived from the lifecycle status rather than stored, because the
	// status IS the authority on where the goods are.
	CustodyTaken bool
}

// LoadBooking reads the booking under recovery.
func (r *Repository) LoadBooking(ctx context.Context, bookingID string) (BookingSnapshot, error) {
	var snapshot BookingSnapshot
	// The commerce mission is joined rather than stored on the booking: a
	// direct booking has none, and Hauling links the two through
	// commerce_missions.booking_id. An empty mission_ref therefore means
	// "direct booking", which is exactly what the transfer path needs to know
	// -- custody manifests only exist for commerce work.
	err := r.q.QueryRow(ctx, `
SELECT b.id::text, b.status, COALESCE(b.provider_id::text,''), COALESCE(b.truck_id::text,''),
       COALESCE(m.mission_ref,''), b.cargo_weight_kg, b.preferred_truck_type,
       COALESCE(b.is_fragile,false), b.pickup_lat, b.pickup_lng
  FROM haulage_bookings b
  LEFT JOIN commerce_missions m ON m.booking_id = b.id
 WHERE b.id = $1`, bookingID).Scan(
		&snapshot.ID, &snapshot.Status, &snapshot.ProviderID, &snapshot.TruckID,
		&snapshot.MissionRef, &snapshot.CargoWeightKg, &snapshot.PreferredTruckType,
		&snapshot.IsFragile, &snapshot.PickupLat, &snapshot.PickupLng)
	if errors.Is(err, pgx.ErrNoRows) {
		return BookingSnapshot{}, ErrNotFound
	}
	if err != nil {
		return BookingSnapshot{}, err
	}
	snapshot.CustodyTaken = custodyTaken(snapshot.Status)
	return snapshot, nil
}

// custodyTaken reports whether the cargo is already in the provider's truck.
//
// Everything from picked_up onwards means the goods have physically changed
// hands. A replacement from that point is a custody TRANSFER, never a field
// rewrite: repointing provider_id would leave the ledger asserting one person
// holds goods that are sitting in someone else's truck.
func custodyTaken(status string) bool {
	switch status {
	case "picked_up", "en_route_delivery", "delivered":
		return true
	default:
		return false
	}
}

// Open inserts a case, or returns the existing one for the same operation_ref.
//
// Idempotency is by operation_ref and the open-case uniqueness is by booking:
// the first makes a retried command safe, the second makes two DIFFERENT
// reports of the same failure safe. Both are enforced in the schema, so
// neither depends on a caller remembering to check.
func (r *Repository) Open(ctx context.Context, command StartCommand, booking BookingSnapshot, now time.Time) (Case, bool, error) {
	audit, _ := json.Marshal([]map[string]any{{
		"at": now.UTC(), "state": string(StateDetected),
		"actor": command.Actor, "trigger": command.TriggerCode,
	}})

	row := r.q.QueryRow(ctx, `
INSERT INTO hauling_recovery_cases
	(booking_id, mission_ref, trigger_code, state, original_provider_id, original_truck_id,
	 custody_transfer_required, operation_ref, correlation_id, detected_at, audit_json)
VALUES ($1,$2,$3,'DETECTED',NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,$6,$7,$8,$9,$10)
ON CONFLICT (operation_ref) DO NOTHING
RETURNING `+caseColumns,
		booking.ID, booking.MissionRef, command.TriggerCode,
		booking.ProviderID, booking.TruckID, booking.CustodyTaken,
		command.OperationRef, command.CorrelationID, now.UTC(), audit)

	opened, err := scanCase(row)
	if err == nil {
		// Point the booking at its open case in the same breath, so the
		// lifecycle can refuse transitions without scanning the case table.
		if _, err = r.q.Exec(ctx,
			`UPDATE haulage_bookings SET active_recovery_id=$2, recovery_count=recovery_count+1 WHERE id=$1`,
			booking.ID, opened.ID); err != nil {
			return Case{}, false, err
		}
		return opened, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Case{}, false, err
	}

	// The operation_ref already exists: this is a retry. Return the original.
	existing, err := r.ByOperationRef(ctx, command.OperationRef)
	return existing, false, err
}

// ByOperationRef reads a case by its idempotency key.
func (r *Repository) ByOperationRef(ctx context.Context, operationRef string) (Case, error) {
	found, err := scanCase(r.q.QueryRow(ctx,
		`SELECT `+caseColumns+` FROM hauling_recovery_cases WHERE operation_ref=$1`, operationRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return Case{}, ErrNotFound
	}
	return found, err
}

// OpenForBooking reads the booking's open case.
func (r *Repository) OpenForBooking(ctx context.Context, bookingID string) (Case, error) {
	found, err := scanCase(r.q.QueryRow(ctx,
		`SELECT `+caseColumns+` FROM hauling_recovery_cases
		  WHERE booking_id=$1 AND state NOT IN ('RESOLVED','FAILED')`, bookingID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Case{}, ErrNotFound
	}
	return found, err
}

// Advance moves a case forward under an optimistic-concurrency guard.
//
// The guard is what stops two racing operators each binding a different
// replacement to the same cargo. `version = $expected` in the WHERE clause
// means the loser updates zero rows and gets a conflict rather than a silent
// overwrite of the winner's replacement.
func (r *Repository) Advance(ctx context.Context, input AdvanceInput, now time.Time) (Case, error) {
	current, err := r.byID(ctx, input.CaseID)
	if err != nil {
		return Case{}, err
	}
	if input.ExpectedVersion != 0 && current.Version != input.ExpectedVersion {
		return Case{}, ErrConflict
	}
	if !current.State.CanAdvanceTo(input.State) {
		// Covers both a stale command trying to walk the case backwards and
		// any command at all against a terminal case.
		return Case{}, ErrConflict
	}

	audit, _ := json.Marshal(map[string]any{
		"at": now.UTC(), "state": string(input.State), "actor": input.Actor,
		"reason": input.ReasonCode, "replacement_provider": input.ReplacementProviderID,
	})

	var resolvedAt any
	if input.State.Terminal() {
		resolvedAt = now.UTC()
	}
	var startedAt any
	if input.State == StateReplacementSearch && current.StartedAt == nil {
		startedAt = now.UTC()
	}

	row := r.q.QueryRow(ctx, `
UPDATE hauling_recovery_cases SET
	state=$2,
	replacement_provider_id=CASE WHEN $3='' THEN replacement_provider_id ELSE $3::uuid END,
	replacement_truck_id  =CASE WHEN $4='' THEN replacement_truck_id   ELSE $4::uuid END,
	reason_code=COALESCE(NULLIF($5,''), reason_code),
	custody_transfer_ref=COALESCE(NULLIF($6,''), custody_transfer_ref),
	evidence_ref=COALESCE(NULLIF($7,''), evidence_ref),
	started_at=COALESCE(started_at, $8),
	resolved_at=$9,
	version=version+1,
	audit_json=audit_json || $10::jsonb
WHERE id=$1 AND version=$11 AND state NOT IN ('RESOLVED','FAILED')
RETURNING `+caseColumns,
		input.CaseID, string(input.State), input.ReplacementProviderID, input.ReplacementTruckID,
		input.ReasonCode, input.CustodyTransferRef, input.EvidenceRef,
		startedAt, resolvedAt, "["+string(audit)+"]", current.Version)

	advanced, err := scanCase(row)
	if errors.Is(err, pgx.ErrNoRows) {
		// Someone advanced the case between the read and the write.
		return Case{}, ErrConflict
	}
	if err != nil {
		return Case{}, err
	}
	if advanced.State.Terminal() {
		if _, err = r.q.Exec(ctx,
			`UPDATE haulage_bookings SET active_recovery_id=NULL WHERE id=$1 AND active_recovery_id=$2`,
			advanced.BookingID, advanced.ID); err != nil {
			return Case{}, err
		}
	}
	return advanced, nil
}

// AdvanceInput is one state transition.
type AdvanceInput struct {
	CaseID                string
	State                 State
	ReplacementProviderID string
	ReplacementTruckID    string
	ReasonCode            string
	CustodyTransferRef    string
	EvidenceRef           string
	ExpectedVersion       int64
	Actor                 string
}

func (r *Repository) byID(ctx context.Context, caseID string) (Case, error) {
	found, err := scanCase(r.q.QueryRow(ctx,
		`SELECT `+caseColumns+` FROM hauling_recovery_cases WHERE id=$1`, caseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Case{}, ErrNotFound
	}
	return found, err
}

// RebindProviderTx repoints the booking at the replacement inside a
// caller-owned transaction.
//
// The booking id, its payment reference, its cargo requirement and its
// handover binding are all untouched: only who is driving changes. That is the
// whole point of rebinding rather than re-creating -- a new booking would
// strand the customer's held funds against work nobody is doing.
//
// The WHERE clause asserts the ORIGINAL provider is still the assigned one.
// If the original provider completed, cancelled or was already replaced
// between the search and this write, zero rows update and the caller gets a
// conflict instead of overwriting whatever happened.
func (r *Repository) RebindProviderTx(
	ctx context.Context, tx pgx.Tx, bookingID, originalProviderID, replacementProviderID, replacementTruckID string,
) error {
	tag, err := tx.Exec(ctx, `
UPDATE haulage_bookings
   SET provider_id=$3::uuid, truck_id=NULLIF($4,'')::uuid, updated_at=now()
 WHERE id=$1
   AND provider_id=$2::uuid
   AND status NOT IN ('completed','cancelled','unmatched')`,
		bookingID, originalProviderID, replacementProviderID, replacementTruckID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// ListOpen returns open cases, for the operator surface and the stall sweeper.
func (r *Repository) ListOpen(ctx context.Context, limit int) ([]Case, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.q.Query(ctx,
		`SELECT `+caseColumns+` FROM hauling_recovery_cases
		  WHERE state NOT IN ('RESOLVED','FAILED') ORDER BY detected_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Case
	for rows.Next() {
		found, err := scanCase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, found)
	}
	return out, rows.Err()
}

// CaseForReplacement returns the newest recovery case that made providerID
// the booking's replacement, or "" when there is none. The settlement guard
// uses it to justify moving a held payment (Section 17).
func (r *Repository) CaseForReplacement(ctx context.Context, bookingID, providerID string) (string, error) {
	var id string
	err := r.q.QueryRow(ctx, `
		SELECT id::text FROM hauling_recovery_cases
		 WHERE booking_id::text = $1 AND replacement_provider_id::text = $2
		 ORDER BY detected_at DESC LIMIT 1`, bookingID, providerID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}
