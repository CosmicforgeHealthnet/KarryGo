// Package repositories stores fleet-side safety references.
//
// Every statement here is tenant-scoped in SQL. There is no unscoped read, and
// deliberately so: unlike an assignment, nothing outside this organization has
// a legitimate reason to enumerate its incident or dispute references.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ErrNotFound means no reference matched within the tenant scope.
var ErrNotFound = errors.New("safety: reference not found")

// ErrConflict means the organization already holds a record for this external
// case, or the idempotency key was reused.
var ErrConflict = errors.New("safety: conflicting safety reference")

// ReferenceRepository stores references and their append-only history.
type ReferenceRepository struct {
	db database.Querier
}

// NewReferenceRepository wires the repository.
func NewReferenceRepository(db database.Querier) *ReferenceRepository {
	return &ReferenceRepository{db: db}
}

const referenceColumns = `id, organization_id, reference_type, authority_service, external_reference,
	assignment_id, request_projection_id, trip_projection_id, platform_trip_id,
	executing_provider_id, vehicle_reference, verification_state, verified_at,
	status_projection, status_projection_at, created_by_actor_type, created_by_actor_id,
	correlation_id, causation_id, idempotency_key, request_fingerprint,
	version, created_at, updated_at`

// Create inserts a reference.
//
// The organization is taken from the argument, never from the model, so a
// caller cannot write into another tenant by populating a struct field.
func (r *ReferenceRepository) Create(ctx context.Context, q database.Querier,
	organizationID string, reference *models.Reference) error {
	if reference.ID == "" {
		reference.ID = ids.New("sfr")
	}
	reference.OrganizationID = organizationID
	reference.Version = 1
	if reference.AuthorityService == "" {
		reference.AuthorityService = models.AuthoritySupportDispute
	}

	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_safety_references
			(id, organization_id, reference_type, authority_service, external_reference,
			 assignment_id, request_projection_id, trip_projection_id, platform_trip_id,
			 executing_provider_id, vehicle_reference, verification_state, verified_at,
			 status_projection, status_projection_at, created_by_actor_type,
			 created_by_actor_id, correlation_id, causation_id, idempotency_key,
			 request_fingerprint, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
		        $16, $17, $18, $19, $20, $21, 1, now(), now())`,
		reference.ID, organizationID, reference.ReferenceType, reference.AuthorityService,
		reference.ExternalReference, reference.AssignmentID, reference.RequestProjectionID,
		reference.TripProjectionID, reference.PlatformTripID, reference.ExecutingProviderID,
		reference.VehicleReference, reference.VerificationState, reference.VerifiedAt,
		reference.StatusProjection, reference.StatusProjectionAt, reference.CreatedByActorType,
		reference.CreatedByActorID, reference.CorrelationID, reference.CausationID,
		reference.IdempotencyKey, reference.RequestFingerprint)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("safety: create reference: %w", err)
	}
	return nil
}

// GetByID loads one reference inside the organization scope.
func (r *ReferenceRepository) GetByID(ctx context.Context, organizationID string,
	referenceID string) (*models.Reference, error) {
	return scanReference(r.db.QueryRow(ctx, `SELECT `+referenceColumns+`
		FROM fleet_partner_safety_references
		WHERE organization_id = $1 AND id = $2`, organizationID, referenceID))
}

// FindByExternalReference resolves an existing record for a case.
func (r *ReferenceRepository) FindByExternalReference(ctx context.Context, q database.Querier,
	organizationID string, referenceType string, authority string,
	externalReference string) (*models.Reference, error) {
	if q == nil {
		q = r.db
	}
	return scanReference(q.QueryRow(ctx, `SELECT `+referenceColumns+`
		FROM fleet_partner_safety_references
		WHERE organization_id = $1 AND reference_type = $2
		  AND authority_service = $3 AND external_reference = $4`,
		organizationID, referenceType, authority, externalReference))
}

// FindByIdempotencyKey resolves a retried write within the organization.
func (r *ReferenceRepository) FindByIdempotencyKey(ctx context.Context, q database.Querier,
	organizationID string, idempotencyKey string) (*models.Reference, error) {
	if q == nil {
		q = r.db
	}
	return scanReference(q.QueryRow(ctx, `SELECT `+referenceColumns+`
		FROM fleet_partner_safety_references
		WHERE organization_id = $1 AND idempotency_key = $2`,
		organizationID, idempotencyKey))
}

// ListFilter narrows a reference listing.
type ListFilter struct {
	ReferenceType       string
	AssignmentID        string
	PlatformTripID      string
	ExecutingProviderID string
}

// List returns references for a tenant query.
//
// Branch scope is applied through the assignment's provider membership when
// the reference is attached to an assignment. A reference with no assignment
// is organization-wide and visible to any reader holding the permission —
// there is no branch to scope it to, and hiding it would silently lose it.
func (r *ReferenceRepository) List(ctx context.Context, query tenant.Query,
	filter ListFilter) ([]models.Reference, error) {
	if query.OrganizationID == "" {
		return nil, fmt.Errorf("safety: organization id is required")
	}
	limit := query.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	branches := query.PermittedBranchIDs
	if branches == nil {
		branches = []string{}
	}

	rows, err := r.db.Query(ctx, `
		SELECT s.id, s.organization_id, s.reference_type, s.authority_service,
			s.external_reference, s.assignment_id, s.request_projection_id,
			s.trip_projection_id, s.platform_trip_id, s.executing_provider_id,
			s.vehicle_reference, s.verification_state, s.verified_at,
			s.status_projection, s.status_projection_at, s.created_by_actor_type,
			s.created_by_actor_id, s.correlation_id, s.causation_id, s.idempotency_key,
			s.request_fingerprint, s.version, s.created_at, s.updated_at
		FROM fleet_partner_safety_references s
		LEFT JOIN fleet_partner_provider_assignments a ON a.id = s.assignment_id
		LEFT JOIN fleet_partner_organization_provider_memberships m ON m.id = a.membership_id
		WHERE s.organization_id = $1
		  AND ($2 = '' OR s.reference_type = $2)
		  AND ($3 = '' OR s.assignment_id = $3)
		  AND ($4 = '' OR s.platform_trip_id = $4)
		  AND ($5 = '' OR s.executing_provider_id = $5)
		  AND (cardinality($6::text[]) = 0 OR s.assignment_id IS NULL
		       OR m.home_branch_id IS NULL OR m.home_branch_id = ANY($6::text[]))
		ORDER BY s.created_at DESC, s.id
		LIMIT $7`,
		query.OrganizationID, filter.ReferenceType, filter.AssignmentID,
		filter.PlatformTripID, filter.ExecutingProviderID, branches, limit)
	if err != nil {
		return nil, fmt.Errorf("safety: list references: %w", err)
	}
	defer rows.Close()

	var references []models.Reference
	for rows.Next() {
		reference, scanErr := scanReferenceRows(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		references = append(references, *reference)
	}
	return references, rows.Err()
}

// SetStatusProjection records the authoritative status fleet last observed.
//
// Compare-and-swap on version: two concurrent projections cannot interleave
// into a row that reports one status with the other's timestamp.
func (r *ReferenceRepository) SetStatusProjection(ctx context.Context, q database.Querier,
	organizationID string, referenceID string, status string, expectedVersion int64) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_safety_references SET
			status_projection = $3,
			status_projection_at = now(),
			version = version + 1,
			updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $4`,
		organizationID, referenceID, status, expectedVersion)
	if err != nil {
		return fmt.Errorf("safety: set status projection: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// AppendHistory writes one immutable history row.
func (r *ReferenceRepository) AppendHistory(ctx context.Context, q database.Querier,
	entry *models.HistoryEntry) error {
	if entry.ID == "" {
		entry.ID = ids.New("sfh")
	}
	metadata, err := json.Marshal(orEmptyMap(entry.Metadata))
	if err != nil {
		return fmt.Errorf("safety: marshal history metadata: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_safety_reference_history
			(id, organization_id, reference_id, action, from_state, to_state,
			 actor_type, actor_id, metadata, correlation_id, causation_id, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())`,
		entry.ID, entry.OrganizationID, entry.ReferenceID, entry.Action,
		entry.FromState, entry.ToState, entry.ActorType, entry.ActorID, metadata,
		entry.CorrelationID, entry.CausationID)
	if err != nil {
		return fmt.Errorf("safety: append reference history: %w", err)
	}
	return nil
}

// ListHistory returns the append-only history for one reference.
func (r *ReferenceRepository) ListHistory(ctx context.Context, organizationID string,
	referenceID string) ([]models.HistoryEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, reference_id, action, from_state, to_state,
			actor_type, actor_id, metadata, correlation_id, causation_id, occurred_at
		FROM fleet_partner_safety_reference_history
		WHERE organization_id = $1 AND reference_id = $2
		ORDER BY occurred_at DESC, id`, organizationID, referenceID)
	if err != nil {
		return nil, fmt.Errorf("safety: list reference history: %w", err)
	}
	defer rows.Close()

	var entries []models.HistoryEntry
	for rows.Next() {
		var entry models.HistoryEntry
		var metadata []byte
		if err := rows.Scan(&entry.ID, &entry.OrganizationID, &entry.ReferenceID,
			&entry.Action, &entry.FromState, &entry.ToState, &entry.ActorType,
			&entry.ActorID, &metadata, &entry.CorrelationID, &entry.CausationID,
			&entry.OccurredAt); err != nil {
			return nil, fmt.Errorf("safety: scan reference history: %w", err)
		}
		_ = json.Unmarshal(metadata, &entry.Metadata)
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// AssignmentBelongsToOrganization reports whether an assignment is this
// organization's.
//
// Checked in SQL before a reference is attached, so a caller cannot bind their
// organization's reference to another tenant's assignment id.
func (r *ReferenceRepository) AssignmentBelongsToOrganization(ctx context.Context,
	q database.Querier, organizationID string, assignmentID string) (bool, error) {
	if q == nil {
		q = r.db
	}
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM fleet_partner_provider_assignments
			WHERE id = $1 AND organization_id = $2)`, assignmentID, organizationID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("safety: verify assignment ownership: %w", err)
	}
	return exists, nil
}

// TripProjectionForOrganization resolves a canonical trip id to this
// organization's projection, when one exists.
func (r *ReferenceRepository) TripProjectionForOrganization(ctx context.Context,
	q database.Querier, organizationID string, platformTripID string) (string, error) {
	if q == nil {
		q = r.db
	}
	var id string
	err := q.QueryRow(ctx, `
		SELECT id FROM fleet_partner_trip_projections
		WHERE organization_id = $1 AND platform_trip_id = $2`,
		organizationID, platformTripID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("safety: resolve trip projection: %w", err)
	}
	return id, nil
}

func scanReference(row pgx.Row) (*models.Reference, error) {
	var reference models.Reference
	err := row.Scan(&reference.ID, &reference.OrganizationID, &reference.ReferenceType,
		&reference.AuthorityService, &reference.ExternalReference, &reference.AssignmentID,
		&reference.RequestProjectionID, &reference.TripProjectionID, &reference.PlatformTripID,
		&reference.ExecutingProviderID, &reference.VehicleReference, &reference.VerificationState,
		&reference.VerifiedAt, &reference.StatusProjection, &reference.StatusProjectionAt,
		&reference.CreatedByActorType, &reference.CreatedByActorID, &reference.CorrelationID,
		&reference.CausationID, &reference.IdempotencyKey, &reference.RequestFingerprint,
		&reference.Version, &reference.CreatedAt, &reference.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("safety: scan reference: %w", err)
	}
	return &reference, nil
}

func scanReferenceRows(rows pgx.Rows) (*models.Reference, error) {
	var reference models.Reference
	err := rows.Scan(&reference.ID, &reference.OrganizationID, &reference.ReferenceType,
		&reference.AuthorityService, &reference.ExternalReference, &reference.AssignmentID,
		&reference.RequestProjectionID, &reference.TripProjectionID, &reference.PlatformTripID,
		&reference.ExecutingProviderID, &reference.VehicleReference, &reference.VerificationState,
		&reference.VerifiedAt, &reference.StatusProjection, &reference.StatusProjectionAt,
		&reference.CreatedByActorType, &reference.CreatedByActorID, &reference.CorrelationID,
		&reference.CausationID, &reference.IdempotencyKey, &reference.RequestFingerprint,
		&reference.Version, &reference.CreatedAt, &reference.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("safety: scan reference: %w", err)
	}
	return &reference, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func orEmptyMap(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	return values
}
