package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The movement emitter.
//
// An execution authority publishes movement events by RECORDING what it
// currently believes, not by hooking every transition. Emit is given the
// authority's own current view of a movement; it compares that view with the
// last one published, and when they differ it assigns the next source_version
// and queues an envelope in the same transaction. Nothing here can invent a
// movement the authority does not hold, and a crash between the state change
// and the publish loses nothing, because the next sweep sees the same
// difference again.

// EmissionResult reports what one Emit call decided.
type EmissionResult struct {
	Emitted int
	Skipped int
}

// Emitter writes movement emissions and outbox rows for one service.
type Emitter struct {
	Pool          *pgxpool.Pool
	SourceService string
	// EntityType names the authority's own aggregate, for the envelope.
	EntityType string
	Now        func() time.Time
}

func (e *Emitter) now() time.Time {
	if e == nil || e.Now == nil {
		return time.Now().UTC()
	}
	return e.Now().UTC()
}

// fingerprint is the comparable content of a movement, excluding anything
// that changes on every read.
func fingerprint(summary MovementSummary) string {
	raw, err := json.Marshal(summary)
	if err != nil {
		return "unfingerprintable"
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// EventNameFor maps an authority and state to the canonical event name.
func EventNameFor(authority, state string) string {
	prefix := map[string]string{
		MovementAuthorityTaxi:     "taxi.booking",
		MovementAuthorityDispatch: "dispatch.delivery",
		MovementAuthorityHauling:  "hauling.haulage",
	}[authority]
	if prefix == "" {
		return ""
	}
	suffix := map[string]string{
		MovementStateRequested:  "created",
		MovementStateSearching:  "created",
		MovementStateAssigned:   "assigned",
		MovementStateArrived:    "provider_arrived",
		MovementStateInProgress: "started",
		MovementStateDelivered:  "delivered",
		MovementStateCompleted:  "completed",
		MovementStateCancelled:  "cancelled",
		MovementStateUnmatched:  "unmatched",
		MovementStateFailed:     "failed",
	}[state]
	if suffix == "" {
		return ""
	}
	return prefix + "." + suffix
}

var ErrEmitterNotConfigured = errors.New("movement emitter is not configured")

// Emit publishes every summary whose content differs from the last published
// view of the same movement. A summary that has not changed is skipped, which
// is what makes repeated sweeps and replays safe.
func (e *Emitter) Emit(ctx context.Context, summaries []MovementSummary) (EmissionResult, error) {
	if e == nil || e.Pool == nil {
		return EmissionResult{}, ErrEmitterNotConfigured
	}
	var result EmissionResult
	for _, summary := range summaries {
		emitted, err := e.emitOne(ctx, summary)
		if err != nil {
			return result, err
		}
		if emitted {
			result.Emitted++
			continue
		}
		result.Skipped++
	}
	return result, nil
}

func (e *Emitter) emitOne(ctx context.Context, summary MovementSummary) (bool, error) {
	if summary.CustomerID == "" || summary.SourceRef == "" || summary.Authority == "" || summary.State == "" {
		return false, fmt.Errorf("%w: incomplete movement summary", ErrInvalidEnvelope)
	}
	eventName := EventNameFor(summary.Authority, summary.State)
	if eventName == "" {
		return false, fmt.Errorf("%w: no event name for %s/%s", ErrInvalidEnvelope, summary.Authority, summary.State)
	}
	current := fingerprint(summary)
	now := e.now()

	tx, err := e.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var storedFingerprint string
	var version int64
	err = tx.QueryRow(ctx, `SELECT fingerprint, source_version FROM customer_movement_emissions
		WHERE authority=$1 AND source_ref=$2 FOR UPDATE`, summary.Authority, summary.SourceRef).Scan(&storedFingerprint, &version)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		version = 0
	case err != nil:
		return false, err
	case storedFingerprint == current:
		return false, tx.Commit(ctx)
	}
	version++

	if _, err = tx.Exec(ctx, `INSERT INTO customer_movement_emissions(authority,source_ref,state,fingerprint,source_version,emitted_at)
		VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT (authority,source_ref) DO UPDATE SET state=EXCLUDED.state, fingerprint=EXCLUDED.fingerprint,
			source_version=EXCLUDED.source_version, emitted_at=EXCLUDED.emitted_at`,
		summary.Authority, summary.SourceRef, summary.State, current, version, now); err != nil {
		return false, err
	}

	envelope, err := NewMovementEnvelope(uuid.NewString(), eventName, e.SourceService, e.EntityType, version, now, summary.SourceRef, "", summary)
	if err != nil {
		return false, err
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO customer_movement_outbox(event_id,authority,source_ref,source_version,payload,created_at,next_attempt_at)
		VALUES($1,$2,$3,$4,$5,$6,$6)`, envelope.EventID, summary.Authority, summary.SourceRef, version, payload, now); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
