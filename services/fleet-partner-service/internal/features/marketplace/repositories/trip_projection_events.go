package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// TripEventOutcome classifies what one source event did to a projection.
type TripEventOutcome string

const (
	// TripEventCreated — the projection did not exist and was inserted.
	TripEventCreated TripEventOutcome = "CREATED"
	// TripEventApplied — the event was the next expected version.
	TripEventApplied TripEventOutcome = "APPLIED"
	// TripEventDuplicate — same version, same payload. Already applied.
	TripEventDuplicate TripEventOutcome = "DUPLICATE"
	// TripEventStale — an older version than we hold; ignored.
	TripEventStale TripEventOutcome = "STALE"
	// TripEventConflict — same version, DIFFERENT payload. The source has
	// contradicted itself; the projection is no longer trustworthy.
	TripEventConflict TripEventOutcome = "CONFLICT"
	// TripEventGap — newer than expected; versions are missing.
	TripEventGap TripEventOutcome = "GAP"
)

// ErrTripProjectionConflict reports two different payloads at one version.
var ErrTripProjectionConflict = errors.New("marketplace: trip projection source version conflict")

// TripSourceEvent is one canonical trip event ready to project.
//
// Only the fields fleet operations actually need are carried. There is
// deliberately no fare, currency, receiver or customer field: this struct is
// the boundary, and a field that does not exist here cannot leak downstream
// however carelessly a later handler is written.
type TripSourceEvent struct {
	OrganizationID           string
	SourceService            string
	PlatformTripID           string
	PlatformRequestID        string
	PlatformBookingID        string
	ExecutingProviderID      string
	OrganizationMembershipID string
	SettlementOrganizationID string
	AssignmentID             string
	ServiceType              string
	TripStatus               string
	SourceVersion            int64
	EventID                  string
	PayloadHash              string
	OccurredAt               time.Time
	CompletedAt              *time.Time
	CancelledAt              *time.Time
}

// ApplyTripSourceEvent projects one trip event under the source-version rules.
//
// The outcomes are the whole point of the method, and they are not
// interchangeable:
//
//   - no row yet: insert. Only trip.created.v1 legitimately reaches this, but
//     the check is on existence rather than event type so that a service which
//     starts emitting mid-trip still produces a usable projection instead of
//     silently dropping every event.
//   - version lower than held: an out-of-order or replayed delivery. Ignored,
//     because applying it would move the projection backwards.
//   - version equal, payload hash equal: a redelivery. Idempotent no-op, which
//     is what makes the producer's retry-after-unknown-outcome safe.
//   - version equal, payload hash DIFFERENT: two different events claim the
//     same version. One of them is wrong and we cannot tell which, so the
//     projection is marked GAP_DETECTED and a reconciliation read-back decides.
//     Overwriting would silently pick a winner.
//   - version exactly one greater: applied.
//   - version more than one greater: apply the newer state, since it is
//     genuinely more current, but mark GAP_DETECTED — fleet never saw the
//     intermediate history and must not act as though it did.
//
// A projection already gapped stays gapped until reconciliation clears it: one
// in-order event does not prove the earlier missing ones ever arrived.
func (r *TripRepository) ApplyTripSourceEvent(ctx context.Context, q database.Querier,
	event TripSourceEvent) (TripEventOutcome, *models.TripProjection, error) {
	existing, err := r.lockBySourceTrip(ctx, q, event.SourceService, event.PlatformTripID)
	if errors.Is(err, ErrNotFound) {
		created, insertErr := r.insertFromSource(ctx, q, event)
		if insertErr != nil {
			return "", nil, insertErr
		}
		return TripEventCreated, created, nil
	}
	if err != nil {
		return "", nil, err
	}

	// A projection belongs to exactly one organization. An event that names a
	// different one is either misrouted or an attempt to write across a tenant
	// boundary; neither is something to apply.
	if event.OrganizationID != "" && existing.OrganizationID != event.OrganizationID {
		return "", nil, fmt.Errorf(
			"marketplace: trip %s belongs to another organization", event.PlatformTripID)
	}

	switch {
	case event.SourceVersion < existing.SourceVersion:
		return TripEventStale, existing, nil

	case event.SourceVersion == existing.SourceVersion:
		if existing.LastPayloadHash == nil || *existing.LastPayloadHash == "" ||
			event.PayloadHash == "" || *existing.LastPayloadHash == event.PayloadHash {
			// Identical redelivery, or a hash we cannot compare. Neither is a
			// reason to change anything.
			return TripEventDuplicate, existing, nil
		}
		if err := r.markFreshness(ctx, q, existing.ID, models.FreshnessGapDetected); err != nil {
			return "", nil, err
		}
		return TripEventConflict, existing, nil
	}

	freshness := models.FreshnessCurrent
	outcome := TripEventApplied
	if event.SourceVersion > existing.SourceVersion+1 {
		freshness = models.FreshnessGapDetected
		outcome = TripEventGap
	}
	if existing.FreshnessState == models.FreshnessGapDetected {
		freshness = models.FreshnessGapDetected
	}

	updated, err := r.advanceFromSource(ctx, q, existing, event, freshness)
	if err != nil {
		return "", nil, err
	}
	return outcome, updated, nil
}

// lockBySourceTrip loads and locks a projection by its canonical identity.
//
// FOR NO KEY UPDATE rather than FOR UPDATE: this row is referenced by
// fleet_partner_trip_projection_history, and a child insert takes an implicit
// FOR KEY SHARE that deadlocks against FOR UPDATE. Same rule as WP1/WP2.
func (r *TripRepository) lockBySourceTrip(ctx context.Context, q database.Querier,
	sourceService string, platformTripID string) (*models.TripProjection, error) {
	return scanTripRow(q.QueryRow(ctx, `SELECT `+tripColumnsWithHash+`
		FROM fleet_partner_trip_projections
		WHERE source_service = $1 AND platform_trip_id = $2
		FOR NO KEY UPDATE`, sourceService, platformTripID))
}

// insertFromSource creates a projection from the first event seen for a trip.
func (r *TripRepository) insertFromSource(ctx context.Context, q database.Querier,
	event TripSourceEvent) (*models.TripProjection, error) {
	id := ids.New("trp")
	// An event that opens a projection at a version above 1 skipped history we
	// never saw, so the projection starts gapped rather than pretending to be
	// complete.
	freshness := models.FreshnessCurrent
	if event.SourceVersion > 1 {
		freshness = models.FreshnessGapDetected
	}
	row := q.QueryRow(ctx, `
		INSERT INTO fleet_partner_trip_projections
			(id, organization_id, source_service, platform_trip_id, platform_request_id,
			 platform_booking_id, executing_provider_id, organization_membership_id,
			 settlement_organization_id, assignment_id, service_type, trip_status,
			 source_version, last_event_id, last_event_at, last_payload_hash,
			 freshness_state, completed_at, cancelled_at, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), $7, NULLIF($8, ''),
			NULLIF($9, ''), NULLIF($10, ''), $11, $12, $13, NULLIF($14, ''), $15,
			NULLIF($16, ''), $17, $18, $19, 1, now(), now())
		RETURNING `+tripColumnsWithHash,
		id, event.OrganizationID, event.SourceService, event.PlatformTripID,
		event.PlatformRequestID, event.PlatformBookingID, event.ExecutingProviderID,
		event.OrganizationMembershipID, event.SettlementOrganizationID, event.AssignmentID,
		event.ServiceType, event.TripStatus, event.SourceVersion, event.EventID,
		event.OccurredAt, event.PayloadHash, freshness, event.CompletedAt, event.CancelledAt)
	return scanTripRow(row)
}

// advanceFromSource applies a newer event to an existing projection.
//
// COALESCE on the association and identity columns is deliberate: later events
// in a trip's life carry fewer fields than trip.created.v1 did, and a status
// change must not blank out the request association or the assignment link
// just because it did not repeat them.
func (r *TripRepository) advanceFromSource(ctx context.Context, q database.Querier,
	existing *models.TripProjection, event TripSourceEvent,
	freshness string) (*models.TripProjection, error) {
	row := q.QueryRow(ctx, `
		UPDATE fleet_partner_trip_projections SET
			trip_status = COALESCE(NULLIF($2, ''), trip_status),
			platform_request_id = COALESCE(platform_request_id, NULLIF($3, '')),
			platform_booking_id = COALESCE(platform_booking_id, NULLIF($4, '')),
			organization_membership_id = COALESCE(organization_membership_id, NULLIF($5, '')),
			settlement_organization_id = COALESCE(settlement_organization_id, NULLIF($6, '')),
			assignment_id = COALESCE(assignment_id, NULLIF($7, '')),
			service_type = COALESCE(NULLIF(service_type, ''), $8),
			source_version = $9,
			last_event_id = NULLIF($10, ''),
			last_event_at = $11,
			last_payload_hash = NULLIF($12, ''),
			freshness_state = $13,
			completed_at = COALESCE($14, completed_at),
			cancelled_at = COALESCE($15, cancelled_at),
			version = version + 1,
			updated_at = now()
		WHERE id = $1
		RETURNING `+tripColumnsWithHash,
		existing.ID, event.TripStatus, event.PlatformRequestID, event.PlatformBookingID,
		event.OrganizationMembershipID, event.SettlementOrganizationID, event.AssignmentID,
		event.ServiceType, event.SourceVersion, event.EventID, event.OccurredAt,
		event.PayloadHash, freshness, event.CompletedAt, event.CancelledAt)
	return scanTripRow(row)
}

// markFreshness sets a projection's freshness state.
func (r *TripRepository) markFreshness(ctx context.Context, q database.Querier,
	tripProjectionID string, freshness string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_trip_projections SET
			freshness_state = $2,
			last_reconciled_at = CASE WHEN $2 = 'CURRENT' THEN now() ELSE last_reconciled_at END,
			version = version + 1, updated_at = now()
		WHERE id = $1`, tripProjectionID, freshness)
	if err != nil {
		return fmt.Errorf("marketplace: set trip freshness: %w", err)
	}
	return nil
}

// SetTripFreshness exposes freshness changes to use cases.
func (r *TripRepository) SetTripFreshness(ctx context.Context, q database.Querier,
	tripProjectionID string, freshness string) error {
	return r.markFreshness(ctx, q, tripProjectionID, freshness)
}

// ReconcileTripFromSource overwrites a projection with authoritative state.
//
// Like the request-side equivalent, this does NOT follow the one-greater rule:
// a read-back is the source answering directly about present state, so there
// is no unseen history to guard. Versions still only move forward, because a
// lagging read replica must not drag a projection backwards and re-open the
// gap it was called to close.
func (r *TripRepository) ReconcileTripFromSource(ctx context.Context, q database.Querier,
	tripProjectionID string, sourceVersion int64, tripStatus string,
	completedAt *time.Time, cancelledAt *time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_trip_projections SET
			source_version = GREATEST(source_version, $2),
			trip_status = CASE WHEN $2 >= source_version THEN COALESCE(NULLIF($3, ''), trip_status)
				ELSE trip_status END,
			completed_at = CASE WHEN $2 >= source_version THEN COALESCE($4, completed_at)
				ELSE completed_at END,
			cancelled_at = CASE WHEN $2 >= source_version THEN COALESCE($5, cancelled_at)
				ELSE cancelled_at END,
			freshness_state = 'CURRENT',
			last_reconciled_at = now(),
			version = version + 1,
			updated_at = now()
		WHERE id = $1`, tripProjectionID, sourceVersion, tripStatus, completedAt, cancelledAt)
	if err != nil {
		return fmt.Errorf("marketplace: reconcile trip projection: %w", err)
	}
	return nil
}

// ListForReconciliation finds projections too uncertain to be trusted.
//
// STALE is included alongside GAP_DETECTED and UNKNOWN because a projection
// that has heard nothing for a long time is not evidence that nothing
// happened — it is equally consistent with delivery having silently stopped,
// which is exactly the case a read-back settles. Terminal trips are excluded:
// a completed or cancelled trip emits nothing further, so its silence is
// expected and opening a case for it would fill the queue with noise.
func (r *TripRepository) ListForReconciliation(ctx context.Context, limit int) ([]models.TripProjection, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `SELECT `+tripColumnsWithHash+`
		FROM fleet_partner_trip_projections
		WHERE freshness_state IN ('STALE', 'GAP_DETECTED', 'UNKNOWN')
		  AND completed_at IS NULL
		  AND cancelled_at IS NULL
		ORDER BY updated_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list trips for reconciliation: %w", err)
	}
	defer rows.Close()

	var trips []models.TripProjection
	for rows.Next() {
		trip, scanErr := scanTripRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		trips = append(trips, *trip)
	}
	return trips, rows.Err()
}

// GetBySourceTripAnyOrganization loads a projection by canonical identity.
func (r *TripRepository) GetBySourceTripAnyOrganization(ctx context.Context, q database.Querier,
	sourceService string, platformTripID string) (*models.TripProjection, error) {
	return scanTripRow(q.QueryRow(ctx, `SELECT `+tripColumnsWithHash+`
		FROM fleet_partner_trip_projections
		WHERE source_service = $1 AND platform_trip_id = $2`,
		sourceService, platformTripID))
}

// tripColumnsWithHash is tripColumns plus the payload hash used for conflict
// detection. Kept separate so the existing column list and its scanner stay
// exactly as they were.
const tripColumnsWithHash = tripColumns + `, last_payload_hash`

// scanTripRow is the shared scanner for the extended column list.
func scanTripRow(row pgx.Row) (*models.TripProjection, error) {
	var trip models.TripProjection
	err := row.Scan(&trip.ID, &trip.OrganizationID, &trip.SourceService, &trip.PlatformTripID,
		&trip.PlatformRequestID, &trip.PlatformBookingID, &trip.ExecutingProviderID,
		&trip.OrganizationMembershipID, &trip.SettlementOrganizationID, &trip.AssignmentID,
		&trip.ServiceType, &trip.TripStatus, &trip.SourceVersion, &trip.LastEventID,
		&trip.LastEventAt, &trip.LastReconciledAt, &trip.FreshnessState, &trip.CompletedAt,
		&trip.CancelledAt, &trip.Version, &trip.CreatedAt, &trip.UpdatedAt,
		&trip.LastPayloadHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan trip projection: %w", err)
	}
	return &trip, nil
}
