// Package repositories implements tenant-scoped persistence for marketplace
// projections, offers, assignments, responses and trip projections.
//
// Every organization-owned query filters organization_id. Source-owned facts
// are only ever written from source events or reconciliation.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ErrNotFound is returned when a tenant-scoped lookup matches nothing. It is
// deliberately indistinguishable from "belongs to another organization".
var ErrNotFound = errors.New("marketplace: not found")

// ProjectionRepository stores marketplace request projections.
type ProjectionRepository struct {
	db database.Querier
}

// NewProjectionRepository wires the repository.
func NewProjectionRepository(db database.Querier) *ProjectionRepository {
	return &ProjectionRepository{db: db}
}

const projectionColumns = `id, organization_id, source_service, platform_request_id,
	platform_booking_id, platform_trip_id, service_type, projection_status, source_status,
	pickup_summary, dropoff_summary, operational_requirements, settlement_organization_id,
	executing_provider_id, organization_membership_id, acceptance_source, assignment_id,
	offered_at, expires_at, source_version, last_event_id, last_source_event_at,
	last_reconciled_at, freshness_state, version, created_at, updated_at`

// Upsert inserts or advances a projection from a source event.
//
// It applies strict version ordering: an older or equal source version is
// ignored so a duplicate or out-of-order delivery can never regress the
// projection. The caller detects gaps separately.
func (r *ProjectionRepository) Upsert(ctx context.Context, q database.Querier, projection *models.RequestProjection) error {
	if projection.ID == "" {
		projection.ID = ids.New("mrq")
	}
	pickup, err := json.Marshal(orEmptyMap(projection.PickupSummary))
	if err != nil {
		return fmt.Errorf("marketplace: marshal pickup: %w", err)
	}
	dropoff, err := json.Marshal(orEmptyMap(projection.DropoffSummary))
	if err != nil {
		return fmt.Errorf("marketplace: marshal dropoff: %w", err)
	}
	requirements, err := json.Marshal(orEmptyMap(projection.OperationalRequirements))
	if err != nil {
		return fmt.Errorf("marketplace: marshal requirements: %w", err)
	}

	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_marketplace_request_projections
			(id, organization_id, source_service, platform_request_id, platform_booking_id,
			 service_type, projection_status, source_status, pickup_summary, dropoff_summary,
			 operational_requirements, offered_at, expires_at, source_version, last_event_id,
			 last_source_event_at, freshness_state, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, now(), $16, 1, now(), now())
		ON CONFLICT (organization_id, source_service, platform_request_id) DO UPDATE SET
			platform_booking_id = COALESCE(EXCLUDED.platform_booking_id, fleet_partner_marketplace_request_projections.platform_booking_id),
			source_status = EXCLUDED.source_status,
			pickup_summary = EXCLUDED.pickup_summary,
			dropoff_summary = EXCLUDED.dropoff_summary,
			operational_requirements = EXCLUDED.operational_requirements,
			expires_at = EXCLUDED.expires_at,
			source_version = EXCLUDED.source_version,
			last_event_id = EXCLUDED.last_event_id,
			last_source_event_at = now(),
			freshness_state = EXCLUDED.freshness_state,
			version = fleet_partner_marketplace_request_projections.version + 1,
			updated_at = now()
		WHERE fleet_partner_marketplace_request_projections.source_version < EXCLUDED.source_version`,
		projection.ID, projection.OrganizationID, projection.SourceService,
		projection.PlatformRequestID, projection.PlatformBookingID, projection.ServiceType,
		projection.ProjectionStatus, projection.SourceStatus, pickup, dropoff, requirements,
		projection.OfferedAt, projection.ExpiresAt, projection.SourceVersion,
		projection.LastEventID, projection.FreshnessState)
	if err != nil {
		return fmt.Errorf("marketplace: upsert projection: %w", err)
	}
	return nil
}

// GetByID loads a projection inside the organization scope.
func (r *ProjectionRepository) GetByID(ctx context.Context, organizationID string, projectionID string) (*models.RequestProjection, error) {
	row := r.db.QueryRow(ctx, `SELECT `+projectionColumns+`
		FROM fleet_partner_marketplace_request_projections
		WHERE organization_id = $1 AND id = $2`, organizationID, projectionID)
	return scanProjection(row)
}

// LockByID loads a projection FOR NO KEY UPDATE inside the caller's transaction.
//
// The lock strength is deliberate, and matches the dispatch reservation path.
// Assignments, offers and trip projections all carry foreign keys to this
// row, so inserting any of them takes an implicit FOR KEY SHARE lock here.
// FOR UPDATE conflicts with FOR KEY SHARE, which deadlocks the moment two
// providers race to accept the same offer: each holds KEY SHARE from its own
// child insert and then asks for FOR UPDATE. FOR NO KEY UPDATE excludes other
// writers identically but is compatible with KEY SHARE, and is honest that we
// never modify this row's key.
func (r *ProjectionRepository) LockByID(ctx context.Context, q database.Querier, organizationID string, projectionID string) (*models.RequestProjection, error) {
	row := q.QueryRow(ctx, `SELECT `+projectionColumns+`
		FROM fleet_partner_marketplace_request_projections
		WHERE organization_id = $1 AND id = $2 FOR NO KEY UPDATE`, organizationID, projectionID)
	return scanProjection(row)
}

// GetBySourceRequest loads a projection by its source identity.
func (r *ProjectionRepository) GetBySourceRequest(ctx context.Context, organizationID string, sourceService string, platformRequestID string) (*models.RequestProjection, error) {
	row := r.db.QueryRow(ctx, `SELECT `+projectionColumns+`
		FROM fleet_partner_marketplace_request_projections
		WHERE organization_id = $1 AND source_service = $2 AND platform_request_id = $3`,
		organizationID, sourceService, platformRequestID)
	return scanProjection(row)
}

// FindBySourceRequestAnyOrganization resolves a projection when the consuming
// side only knows the source identity. Used by the internal event endpoint,
// which is service-authenticated and therefore platform-scoped by design.
func (r *ProjectionRepository) FindBySourceRequestAnyOrganization(ctx context.Context, q database.Querier, sourceService string, platformRequestID string) (*models.RequestProjection, error) {
	if q == nil {
		q = r.db
	}
	row := q.QueryRow(ctx, `SELECT `+projectionColumns+`
		FROM fleet_partner_marketplace_request_projections
		WHERE source_service = $1 AND platform_request_id = $2
		ORDER BY created_at DESC LIMIT 1`, sourceService, platformRequestID)
	return scanProjection(row)
}

// List returns projections for a tenant query.
func (r *ProjectionRepository) List(ctx context.Context, query tenant.Query, status string) ([]models.RequestProjection, error) {
	if query.OrganizationID == "" {
		return nil, fmt.Errorf("marketplace: organization id is required")
	}
	limit := query.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `SELECT `+projectionColumns+`
		FROM fleet_partner_marketplace_request_projections
		WHERE organization_id = $1 AND ($2 = '' OR projection_status = $2)
		ORDER BY offered_at DESC, id
		LIMIT $3`, query.OrganizationID, status, limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list projections: %w", err)
	}
	defer rows.Close()

	var projections []models.RequestProjection
	for rows.Next() {
		projection, err := scanProjectionRows(rows)
		if err != nil {
			return nil, err
		}
		projections = append(projections, *projection)
	}
	return projections, rows.Err()
}

// SetStatus performs a compare-and-swap projection status change.
func (r *ProjectionRepository) SetStatus(ctx context.Context, q database.Querier, organizationID string, projectionID string, status string, expectedVersion int64) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_marketplace_request_projections SET
			projection_status = $3, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $4`,
		organizationID, projectionID, status, expectedVersion)
	if err != nil {
		return fmt.Errorf("marketplace: set projection status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// SetExecutionContext records who is executing and who settles once a
// provider has been confirmed.
//
// settlementOrganizationID and executingProviderID are deliberately separate
// fields: the organization settles, the provider executes, and conflating
// them would misroute money.
func (r *ProjectionRepository) SetExecutionContext(ctx context.Context, q database.Querier, organizationID string, projectionID string, membershipID string, executingProviderID string, assignmentID string, acceptanceSource string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_marketplace_request_projections SET
			settlement_organization_id = $1,
			organization_membership_id = $3,
			executing_provider_id = $4,
			assignment_id = $5,
			acceptance_source = $6,
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2`,
		organizationID, projectionID, membershipID, executingProviderID, assignmentID, acceptanceSource)
	if err != nil {
		return fmt.Errorf("marketplace: set execution context: %w", err)
	}
	return nil
}

// SetFreshness marks a projection's freshness state.
func (r *ProjectionRepository) SetFreshness(ctx context.Context, q database.Querier, projectionID string, freshness string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_marketplace_request_projections SET
			freshness_state = $2, updated_at = now(),
			last_reconciled_at = CASE WHEN $2 = 'CURRENT' THEN now() ELSE last_reconciled_at END
		WHERE id = $1`, projectionID, freshness)
	if err != nil {
		return fmt.Errorf("marketplace: set freshness: %w", err)
	}
	return nil
}

// MarkStale flags projections whose last source event is older than the
// configured threshold, so the API can label them and commands can refuse.
func (r *ProjectionRepository) MarkStale(ctx context.Context, olderThan time.Duration, limit int) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_marketplace_request_projections SET freshness_state = 'STALE', updated_at = now()
		WHERE id IN (
			SELECT id FROM fleet_partner_marketplace_request_projections
			WHERE freshness_state = 'CURRENT'
			  AND projection_status IN ('OFFERED', 'RESERVATION_PENDING', 'ORGANIZATION_ACCEPTED', 'ASSIGNMENT_PENDING')
			  AND last_source_event_at < now() - make_interval(secs => $1::double precision)
			LIMIT $2
		)`, olderThan.Seconds(), limit)
	if err != nil {
		return 0, fmt.Errorf("marketplace: mark stale: %w", err)
	}
	return tag.RowsAffected(), nil
}

func scanProjection(row pgx.Row) (*models.RequestProjection, error) {
	var projection models.RequestProjection
	var pickup, dropoff, requirements []byte
	err := row.Scan(&projection.ID, &projection.OrganizationID, &projection.SourceService,
		&projection.PlatformRequestID, &projection.PlatformBookingID, &projection.PlatformTripID,
		&projection.ServiceType, &projection.ProjectionStatus, &projection.SourceStatus,
		&pickup, &dropoff, &requirements, &projection.SettlementOrganizationID,
		&projection.ExecutingProviderID, &projection.OrganizationMembershipID,
		&projection.AcceptanceSource, &projection.AssignmentID, &projection.OfferedAt,
		&projection.ExpiresAt, &projection.SourceVersion, &projection.LastEventID,
		&projection.LastSourceEventAt, &projection.LastReconciledAt, &projection.FreshnessState,
		&projection.Version, &projection.CreatedAt, &projection.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan projection: %w", err)
	}
	_ = json.Unmarshal(pickup, &projection.PickupSummary)
	_ = json.Unmarshal(dropoff, &projection.DropoffSummary)
	_ = json.Unmarshal(requirements, &projection.OperationalRequirements)
	return &projection, nil
}

func scanProjectionRows(rows pgx.Rows) (*models.RequestProjection, error) {
	var projection models.RequestProjection
	var pickup, dropoff, requirements []byte
	err := rows.Scan(&projection.ID, &projection.OrganizationID, &projection.SourceService,
		&projection.PlatformRequestID, &projection.PlatformBookingID, &projection.PlatformTripID,
		&projection.ServiceType, &projection.ProjectionStatus, &projection.SourceStatus,
		&pickup, &dropoff, &requirements, &projection.SettlementOrganizationID,
		&projection.ExecutingProviderID, &projection.OrganizationMembershipID,
		&projection.AcceptanceSource, &projection.AssignmentID, &projection.OfferedAt,
		&projection.ExpiresAt, &projection.SourceVersion, &projection.LastEventID,
		&projection.LastSourceEventAt, &projection.LastReconciledAt, &projection.FreshnessState,
		&projection.Version, &projection.CreatedAt, &projection.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan projection: %w", err)
	}
	_ = json.Unmarshal(pickup, &projection.PickupSummary)
	_ = json.Unmarshal(dropoff, &projection.DropoffSummary)
	_ = json.Unmarshal(requirements, &projection.OperationalRequirements)
	return &projection, nil
}

func orEmptyMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func orEmptySlice(value []string) []string {
	if value == nil {
		return []string{}
	}
	return value
}

// SourceEventOutcome describes how a source event was applied.
type SourceEventOutcome string

const (
	// SourceEventApplied — the event was the next expected version.
	SourceEventApplied SourceEventOutcome = "APPLIED"
	// SourceEventStale — an older version than we already hold; ignored.
	SourceEventStale SourceEventOutcome = "STALE"
	// SourceEventGap — a newer version than expected; versions are missing.
	SourceEventGap SourceEventOutcome = "GAP"
)

// ApplySourceEvent advances a projection under the source-version rules.
//
// The three outcomes are the whole point of the method:
//
//   - equal or lower source version: the event is a redelivery or arrived out
//     of order behind one we already applied. Applying it would move the
//     projection BACKWARDS, so it is ignored — not an error.
//   - exactly one greater: the expected next event. Applied.
//   - more than one greater: at least one event never arrived. We apply the
//     newer state (it is genuinely more current) but mark GAP_DETECTED, which
//     blocks state-changing commands until reconciliation fills the hole.
//     Silently accepting the jump would leave fleet acting on a request whose
//     intermediate history it never saw.
func (r *ProjectionRepository) ApplySourceEvent(ctx context.Context, q database.Querier,
	projectionID string, sourceVersion int64, eventID string, occurredAt time.Time,
	apply func(freshness string) error) (SourceEventOutcome, error) {
	var currentVersion int64
	var currentFreshness string
	err := q.QueryRow(ctx, `
		SELECT source_version, freshness_state
		FROM fleet_partner_marketplace_request_projections
		WHERE id = $1
		FOR NO KEY UPDATE`, projectionID).Scan(&currentVersion, &currentFreshness)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("marketplace: lock projection for source event: %w", err)
	}

	if sourceVersion <= currentVersion {
		return SourceEventStale, nil
	}

	outcome := SourceEventApplied
	freshness := models.FreshnessCurrent
	if sourceVersion > currentVersion+1 {
		outcome = SourceEventGap
		freshness = models.FreshnessGapDetected
	}
	// A projection already known to be gapped stays gapped until the
	// reconciliation worker clears it: one in-order event does not prove the
	// earlier missing ones ever arrived.
	if currentFreshness == models.FreshnessGapDetected && outcome == SourceEventApplied {
		freshness = models.FreshnessGapDetected
	}

	if apply != nil {
		if err := apply(freshness); err != nil {
			return "", err
		}
	}

	_, err = q.Exec(ctx, `
		UPDATE fleet_partner_marketplace_request_projections SET
			source_version = $2, last_event_id = $3, last_source_event_at = $4,
			freshness_state = $5, version = version + 1, updated_at = now()
		WHERE id = $1`, projectionID, sourceVersion, eventID, occurredAt, freshness)
	if err != nil {
		return "", fmt.Errorf("marketplace: apply source event: %w", err)
	}
	return outcome, nil
}

// SetSourceStatus records the source service's own status string.
func (r *ProjectionRepository) SetSourceStatus(ctx context.Context, q database.Querier,
	projectionID string, sourceStatus string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_marketplace_request_projections SET
			source_status = $2, updated_at = now()
		WHERE id = $1`, projectionID, sourceStatus)
	if err != nil {
		return fmt.Errorf("marketplace: set source status: %w", err)
	}
	return nil
}

// SetProjectionStatusUnversioned advances projection status without a CAS.
//
// Used only from inside ApplySourceEvent, which already holds the row lock and
// has established that this event is newer than what we hold; a second
// optimistic check there would fail against its own increment.
func (r *ProjectionRepository) SetProjectionStatusUnversioned(ctx context.Context,
	q database.Querier, projectionID string, status string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_marketplace_request_projections SET
			projection_status = $2, updated_at = now()
		WHERE id = $1`, projectionID, status)
	if err != nil {
		return fmt.Errorf("marketplace: set projection status: %w", err)
	}
	return nil
}

// SetPlatformTripID records the canonical trip id reported by the source.
func (r *ProjectionRepository) SetPlatformTripID(ctx context.Context, q database.Querier,
	projectionID string, tripID string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_marketplace_request_projections SET
			platform_trip_id = $2, updated_at = now()
		WHERE id = $1`, projectionID, tripID)
	if err != nil {
		return fmt.Errorf("marketplace: set platform trip id: %w", err)
	}
	return nil
}
