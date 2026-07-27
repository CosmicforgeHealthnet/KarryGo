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

// TripRepository stores organization-facing trip projections.
//
// Canonical trip state belongs to the execution service. Nothing here is
// writable by a corporate user: every mutation arrives from a source event or
// reconciliation.
type TripRepository struct {
	db database.Querier
}

// NewTripRepository wires the repository.
func NewTripRepository(db database.Querier) *TripRepository {
	return &TripRepository{db: db}
}

const tripColumns = `id, organization_id, source_service, platform_trip_id, platform_request_id,
	platform_booking_id, executing_provider_id, organization_membership_id,
	settlement_organization_id, assignment_id, service_type, trip_status, source_version,
	last_event_id, last_event_at, last_reconciled_at, freshness_state, completed_at,
	cancelled_at, version, created_at, updated_at`

// UpsertFromSource applies a trip event.
//
// Older or equal source versions are ignored, so duplicate and out-of-order
// deliveries cannot regress the projection. Completion and cancellation are
// only ever set from the source payload — never inferred from elapsed time or
// local assignment state.
func (r *TripRepository) UpsertFromSource(ctx context.Context, q database.Querier, trip *models.TripProjection) (bool, error) {
	if trip.ID == "" {
		trip.ID = ids.New("trp")
	}
	tag, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_trip_projections
			(id, organization_id, source_service, platform_trip_id, platform_request_id,
			 platform_booking_id, executing_provider_id, organization_membership_id,
			 settlement_organization_id, assignment_id, service_type, trip_status,
			 source_version, last_event_id, last_event_at, freshness_state, completed_at,
			 cancelled_at, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, now(), 'CURRENT', $15, $16, 1, now(), now())
		ON CONFLICT (organization_id, source_service, platform_trip_id) DO UPDATE SET
			trip_status = EXCLUDED.trip_status,
			platform_request_id = COALESCE(EXCLUDED.platform_request_id, fleet_partner_trip_projections.platform_request_id),
			platform_booking_id = COALESCE(EXCLUDED.platform_booking_id, fleet_partner_trip_projections.platform_booking_id),
			assignment_id = COALESCE(EXCLUDED.assignment_id, fleet_partner_trip_projections.assignment_id),
			source_version = EXCLUDED.source_version,
			last_event_id = EXCLUDED.last_event_id,
			last_event_at = now(),
			freshness_state = 'CURRENT',
			completed_at = COALESCE(EXCLUDED.completed_at, fleet_partner_trip_projections.completed_at),
			cancelled_at = COALESCE(EXCLUDED.cancelled_at, fleet_partner_trip_projections.cancelled_at),
			version = fleet_partner_trip_projections.version + 1,
			updated_at = now()
		WHERE fleet_partner_trip_projections.source_version < EXCLUDED.source_version`,
		trip.ID, trip.OrganizationID, trip.SourceService, trip.PlatformTripID,
		trip.PlatformRequestID, trip.PlatformBookingID, trip.ExecutingProviderID,
		trip.OrganizationMembershipID, trip.SettlementOrganizationID, trip.AssignmentID,
		trip.ServiceType, trip.TripStatus, trip.SourceVersion, trip.LastEventID,
		trip.CompletedAt, trip.CancelledAt)
	if err != nil {
		return false, fmt.Errorf("marketplace: upsert trip projection: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// AppendHistory records an append-only trip status change.
func (r *TripRepository) AppendHistory(ctx context.Context, q database.Querier, organizationID string, tripProjectionID string, fromStatus string, toStatus string, sourceVersion int64, eventID string) error {
	var event *string
	if eventID != "" {
		event = &eventID
	}
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_trip_projection_history
			(id, organization_id, trip_projection_id, from_status, to_status, source_version, source_event_id, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())`,
		ids.New("tph"), organizationID, tripProjectionID, fromStatus, toStatus, sourceVersion, event)
	if err != nil {
		return fmt.Errorf("marketplace: append trip history: %w", err)
	}
	return nil
}

// GetByID loads a trip projection inside the organization scope.
func (r *TripRepository) GetByID(ctx context.Context, organizationID string, tripID string) (*models.TripProjection, error) {
	row := r.db.QueryRow(ctx, `SELECT `+tripColumns+`
		FROM fleet_partner_trip_projections
		WHERE organization_id = $1 AND id = $2`, organizationID, tripID)
	return scanTrip(row)
}

// GetBySourceTrip loads a projection by its source identity.
func (r *TripRepository) GetBySourceTrip(ctx context.Context, q database.Querier, sourceService string, platformTripID string) (*models.TripProjection, error) {
	if q == nil {
		q = r.db
	}
	row := q.QueryRow(ctx, `SELECT `+tripColumns+`
		FROM fleet_partner_trip_projections
		WHERE source_service = $1 AND platform_trip_id = $2
		ORDER BY created_at DESC LIMIT 1`, sourceService, platformTripID)
	return scanTrip(row)
}

// List returns trip projections for a tenant query.
func (r *TripRepository) List(ctx context.Context, query tenant.Query, status string) ([]models.TripProjection, error) {
	if query.OrganizationID == "" {
		return nil, fmt.Errorf("marketplace: organization id is required")
	}
	limit := query.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `SELECT `+tripColumns+`
		FROM fleet_partner_trip_projections
		WHERE organization_id = $1 AND ($2 = '' OR trip_status = $2)
		ORDER BY updated_at DESC, id LIMIT $3`, query.OrganizationID, status, limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list trips: %w", err)
	}
	defer rows.Close()

	var trips []models.TripProjection
	for rows.Next() {
		trip, err := scanTripRows(rows)
		if err != nil {
			return nil, err
		}
		trips = append(trips, *trip)
	}
	return trips, rows.Err()
}

// ListForMembership returns a provider's own trip projections.
func (r *TripRepository) ListForMembership(ctx context.Context, organizationID string, membershipID string, limit int) ([]models.TripProjection, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `SELECT `+tripColumns+`
		FROM fleet_partner_trip_projections
		WHERE organization_id = $1 AND organization_membership_id = $2
		ORDER BY updated_at DESC, id LIMIT $3`, organizationID, membershipID, limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list provider trips: %w", err)
	}
	defer rows.Close()

	var trips []models.TripProjection
	for rows.Next() {
		trip, err := scanTripRows(rows)
		if err != nil {
			return nil, err
		}
		trips = append(trips, *trip)
	}
	return trips, rows.Err()
}

// MarkStaleTrips flags trip projections with no recent source event.
func (r *TripRepository) MarkStaleTrips(ctx context.Context, olderThan time.Duration, limit int) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_trip_projections SET freshness_state = 'STALE', updated_at = now()
		WHERE id IN (
			SELECT id FROM fleet_partner_trip_projections
			WHERE freshness_state = 'CURRENT'
			  AND completed_at IS NULL AND cancelled_at IS NULL
			  AND last_event_at < now() - make_interval(secs => $1::double precision)
			LIMIT $2
		)`, olderThan.Seconds(), limit)
	if err != nil {
		return 0, fmt.Errorf("marketplace: mark stale trips: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ---- Projection gaps ----

// RecordGap opens a source-version gap for an aggregate.
func (r *TripRepository) RecordGap(ctx context.Context, q database.Querier, organizationID string, sourceService string, aggregateType string, aggregateSourceID string, expectedVersion int64, receivedVersion int64) error {
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_projection_gaps
			(id, organization_id, source_service, aggregate_type, aggregate_source_id,
			 expected_version, received_version, status, detected_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'OPEN', now())
		ON CONFLICT (source_service, aggregate_type, aggregate_source_id, expected_version) DO NOTHING`,
		ids.New("gap"), organizationID, sourceService, aggregateType, aggregateSourceID,
		expectedVersion, receivedVersion)
	if err != nil {
		return fmt.Errorf("marketplace: record gap: %w", err)
	}
	return nil
}

// HasOpenGap reports whether a material gap blocks state-changing commands.
func (r *TripRepository) HasOpenGap(ctx context.Context, sourceService string, aggregateType string, aggregateSourceID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM fleet_partner_projection_gaps
			WHERE source_service = $1 AND aggregate_type = $2 AND aggregate_source_id = $3 AND status = 'OPEN'
		)`, sourceService, aggregateType, aggregateSourceID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("marketplace: check gap: %w", err)
	}
	return exists, nil
}

// ResolveGaps closes gaps for an aggregate once reconciliation caught up.
func (r *TripRepository) ResolveGaps(ctx context.Context, q database.Querier, sourceService string, aggregateType string, aggregateSourceID string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_projection_gaps SET status = 'RESOLVED', resolved_at = now()
		WHERE source_service = $1 AND aggregate_type = $2 AND aggregate_source_id = $3 AND status = 'OPEN'`,
		sourceService, aggregateType, aggregateSourceID)
	if err != nil {
		return fmt.Errorf("marketplace: resolve gaps: %w", err)
	}
	return nil
}

// ---- Reconciliation cases ----

// OpenReconciliation records that an aggregate needs authoritative repair.
func (r *TripRepository) OpenReconciliation(ctx context.Context, q database.Querier, organizationID string, sourceService string, aggregateType string, aggregateSourceID string, reason string, nextAttempt time.Time) error {
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_projection_reconciliation_cases
			(id, organization_id, source_service, aggregate_type, aggregate_source_id, reason,
			 status, next_attempt_at, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'OPEN', $7, 1, now(), now())`,
		ids.New("rec"), organizationID, sourceService, aggregateType, aggregateSourceID, reason, nextAttempt)
	if err != nil {
		return fmt.Errorf("marketplace: open reconciliation: %w", err)
	}
	return nil
}

// ClaimReconciliationCases leases due cases for the reconciliation worker.
func (r *TripRepository) ClaimReconciliationCases(ctx context.Context, q database.Querier, limit int) ([]ReconciliationCase, error) {
	rows, err := q.Query(ctx, `
		SELECT id, organization_id, source_service, aggregate_type, aggregate_source_id,
			reason, status, attempts, version
		FROM fleet_partner_projection_reconciliation_cases
		WHERE status IN ('OPEN', 'IN_PROGRESS')
		  AND (next_attempt_at IS NULL OR next_attempt_at <= now())
		ORDER BY updated_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: claim reconciliation cases: %w", err)
	}
	defer rows.Close()

	var cases []ReconciliationCase
	for rows.Next() {
		var item ReconciliationCase
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.SourceService,
			&item.AggregateType, &item.AggregateSourceID, &item.Reason, &item.Status,
			&item.Attempts, &item.Version); err != nil {
			return nil, fmt.Errorf("marketplace: scan reconciliation case: %w", err)
		}
		cases = append(cases, item)
	}
	return cases, rows.Err()
}

// ReconciliationCase is one aggregate awaiting authoritative repair.
type ReconciliationCase struct {
	ID                string
	OrganizationID    string
	SourceService     string
	AggregateType     string
	AggregateSourceID string
	Reason            string
	Status            string
	Attempts          int
	Version           int64
}

// ResolveReconciliation closes a case.
func (r *TripRepository) ResolveReconciliation(ctx context.Context, q database.Querier, caseID string, status string, errorCode string, nextAttempt *time.Time) error {
	var code *string
	if errorCode != "" {
		code = &errorCode
	}
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_projection_reconciliation_cases SET
			status = $2, last_error_code = $3, next_attempt_at = $4,
			attempts = attempts + 1,
			resolved_at = CASE WHEN $2 = 'RESOLVED' THEN now() ELSE resolved_at END,
			version = version + 1, updated_at = now()
		WHERE id = $1`, caseID, status, code, nextAttempt)
	if err != nil {
		return fmt.Errorf("marketplace: resolve reconciliation: %w", err)
	}
	return nil
}

func scanTrip(row pgx.Row) (*models.TripProjection, error) {
	var trip models.TripProjection
	err := row.Scan(&trip.ID, &trip.OrganizationID, &trip.SourceService, &trip.PlatformTripID,
		&trip.PlatformRequestID, &trip.PlatformBookingID, &trip.ExecutingProviderID,
		&trip.OrganizationMembershipID, &trip.SettlementOrganizationID, &trip.AssignmentID,
		&trip.ServiceType, &trip.TripStatus, &trip.SourceVersion, &trip.LastEventID,
		&trip.LastEventAt, &trip.LastReconciledAt, &trip.FreshnessState, &trip.CompletedAt,
		&trip.CancelledAt, &trip.Version, &trip.CreatedAt, &trip.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan trip: %w", err)
	}
	return &trip, nil
}

func scanTripRows(rows pgx.Rows) (*models.TripProjection, error) {
	var trip models.TripProjection
	err := rows.Scan(&trip.ID, &trip.OrganizationID, &trip.SourceService, &trip.PlatformTripID,
		&trip.PlatformRequestID, &trip.PlatformBookingID, &trip.ExecutingProviderID,
		&trip.OrganizationMembershipID, &trip.SettlementOrganizationID, &trip.AssignmentID,
		&trip.ServiceType, &trip.TripStatus, &trip.SourceVersion, &trip.LastEventID,
		&trip.LastEventAt, &trip.LastReconciledAt, &trip.FreshnessState, &trip.CompletedAt,
		&trip.CancelledAt, &trip.Version, &trip.CreatedAt, &trip.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan trip: %w", err)
	}
	return &trip, nil
}

// ---- Command operations ----

// CommandRepository stores durable records of external state-changing calls.
type CommandRepository struct {
	db database.Querier
}

// NewCommandRepository wires the repository.
func NewCommandRepository(db database.Querier) *CommandRepository {
	return &CommandRepository{db: db}
}

const commandColumns = `id, organization_id, operation_type, source_service, source_request_id,
	assignment_id, idempotency_key, request_fingerprint, status, external_reference, attempts,
	last_error_code, outcome_checked_at, next_attempt_at, version, created_at, updated_at`

// Begin claims an operation for an idempotency key.
//
// If the key already exists the stored operation is returned instead, so a
// replay re-uses the original external call rather than issuing a second one.
// A different fingerprint under the same key is an idempotency conflict.
func (r *CommandRepository) Begin(ctx context.Context, q database.Querier, operation *models.CommandOperation) (*models.CommandOperation, bool, error) {
	if operation.ID == "" {
		operation.ID = ids.New("cmd")
	}
	// ON CONFLICT DO NOTHING rather than catching the unique violation: a
	// raised constraint error aborts the surrounding transaction, and the
	// read-back below has to run inside that same transaction.
	tag, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_marketplace_command_operations
			(id, organization_id, operation_type, source_service, source_request_id, assignment_id,
			 idempotency_key, request_fingerprint, status, attempts, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'PENDING', 0, 1, now(), now())
		ON CONFLICT (operation_type, idempotency_key) DO NOTHING`,
		operation.ID, operation.OrganizationID, operation.OperationType, operation.SourceService,
		operation.SourceRequestID, operation.AssignmentID, operation.IdempotencyKey,
		operation.RequestFingerprint)
	if err != nil {
		return nil, false, fmt.Errorf("marketplace: begin command: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return operation, false, nil
	}

	existing, err := r.GetByIdempotencyKey(ctx, q, operation.OperationType, operation.IdempotencyKey)
	if err != nil {
		return nil, false, err
	}
	if existing.RequestFingerprint != operation.RequestFingerprint {
		return existing, true, ErrIdempotencyConflict
	}
	return existing, true, nil
}

// ErrIdempotencyConflict means the same key was reused with a different request.
var ErrIdempotencyConflict = errors.New("marketplace: idempotency key reused with a different request")

// GetByIdempotencyKey loads an operation by its key.
func (r *CommandRepository) GetByIdempotencyKey(ctx context.Context, q database.Querier, operationType string, key string) (*models.CommandOperation, error) {
	if q == nil {
		q = r.db
	}
	row := q.QueryRow(ctx, `SELECT `+commandColumns+`
		FROM fleet_partner_marketplace_command_operations
		WHERE operation_type = $1 AND idempotency_key = $2`, operationType, key)
	return scanCommand(row)
}

// SetStatus records the outcome of an external call.
func (r *CommandRepository) SetStatus(ctx context.Context, q database.Querier, operationID string, status string, externalReference string, errorCode string, nextAttempt *time.Time) error {
	var reference, code *string
	if externalReference != "" {
		reference = &externalReference
	}
	if errorCode != "" {
		code = &errorCode
	}
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_marketplace_command_operations SET
			status = $2,
			external_reference = COALESCE($3, external_reference),
			last_error_code = $4,
			next_attempt_at = $5,
			attempts = attempts + 1,
			outcome_checked_at = CASE WHEN $2 IN ('CONFIRMED','REJECTED') THEN now() ELSE outcome_checked_at END,
			version = version + 1, updated_at = now()
		WHERE id = $1`, operationID, status, reference, code, nextAttempt)
	if err != nil {
		return fmt.Errorf("marketplace: set command status: %w", err)
	}
	return nil
}

// ClaimDue leases operations needing a retry or read-back.
func (r *CommandRepository) ClaimDue(ctx context.Context, q database.Querier, limit int) ([]models.CommandOperation, error) {
	rows, err := q.Query(ctx, `SELECT `+commandColumns+`
		FROM fleet_partner_marketplace_command_operations
		WHERE status IN ('PENDING', 'SUBMITTED', 'UNKNOWN_OUTCOME', 'RECONCILIATION_REQUIRED')
		  AND (next_attempt_at IS NULL OR next_attempt_at <= now())
		ORDER BY updated_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: claim due commands: %w", err)
	}
	defer rows.Close()

	var operations []models.CommandOperation
	for rows.Next() {
		operation, err := scanCommandRows(rows)
		if err != nil {
			return nil, err
		}
		operations = append(operations, *operation)
	}
	return operations, rows.Err()
}

func scanCommand(row pgx.Row) (*models.CommandOperation, error) {
	var operation models.CommandOperation
	err := row.Scan(&operation.ID, &operation.OrganizationID, &operation.OperationType,
		&operation.SourceService, &operation.SourceRequestID, &operation.AssignmentID,
		&operation.IdempotencyKey, &operation.RequestFingerprint, &operation.Status,
		&operation.ExternalReference, &operation.Attempts, &operation.LastErrorCode,
		&operation.OutcomeCheckedAt, &operation.NextAttemptAt, &operation.Version,
		&operation.CreatedAt, &operation.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan command: %w", err)
	}
	return &operation, nil
}

func scanCommandRows(rows pgx.Rows) (*models.CommandOperation, error) {
	var operation models.CommandOperation
	err := rows.Scan(&operation.ID, &operation.OrganizationID, &operation.OperationType,
		&operation.SourceService, &operation.SourceRequestID, &operation.AssignmentID,
		&operation.IdempotencyKey, &operation.RequestFingerprint, &operation.Status,
		&operation.ExternalReference, &operation.Attempts, &operation.LastErrorCode,
		&operation.OutcomeCheckedAt, &operation.NextAttemptAt, &operation.Version,
		&operation.CreatedAt, &operation.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan command: %w", err)
	}
	return &operation, nil
}

// ---- Reassignment cases and decline follow-ups ----

// CreateReassignmentCase opens a case for a request needing another provider.
func (r *AssignmentRepository) CreateReassignmentCase(ctx context.Context, q database.Querier, organizationID string, item *models.ReassignmentCase) error {
	if item.ID == "" {
		item.ID = ids.New("rea")
	}
	excluded, err := json.Marshal(orEmptySlice(item.ExcludedMembershipIDs))
	if err != nil {
		return fmt.Errorf("marketplace: marshal excluded memberships: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_reassignment_cases
			(id, organization_id, request_projection_id, origin_assignment_id, trigger,
			 reason_code, requires_human_review, status, attempts, excluded_membership_ids,
			 version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'OPEN', 0, $8, 1, now(), now())`,
		item.ID, organizationID, item.RequestProjectionID, item.OriginAssignmentID,
		item.Trigger, item.ReasonCode, item.RequiresHumanReview, excluded)
	if err != nil {
		return fmt.Errorf("marketplace: create reassignment case: %w", err)
	}
	return nil
}

// GetOpenReassignmentCase returns the open case for a request.
func (r *AssignmentRepository) GetOpenReassignmentCase(ctx context.Context, organizationID string, projectionID string) (*models.ReassignmentCase, error) {
	var item models.ReassignmentCase
	var excluded []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, organization_id, request_projection_id, origin_assignment_id, trigger,
			reason_code, requires_human_review, status, attempts, excluded_membership_ids,
			resolved_assignment_id, version, created_at, updated_at
		FROM fleet_partner_reassignment_cases
		WHERE organization_id = $1 AND request_projection_id = $2 AND status IN ('OPEN', 'IN_PROGRESS')
		ORDER BY created_at DESC LIMIT 1`, organizationID, projectionID).
		Scan(&item.ID, &item.OrganizationID, &item.RequestProjectionID, &item.OriginAssignmentID,
			&item.Trigger, &item.ReasonCode, &item.RequiresHumanReview, &item.Status,
			&item.Attempts, &excluded, &item.ResolvedAssignmentID, &item.Version,
			&item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan reassignment case: %w", err)
	}
	_ = json.Unmarshal(excluded, &item.ExcludedMembershipIDs)
	return &item, nil
}

// ResolveReassignmentCase closes a case with the replacement assignment.
func (r *AssignmentRepository) ResolveReassignmentCase(ctx context.Context, q database.Querier, caseID string, replacementAssignmentID string, expectedVersion int64) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_reassignment_cases SET
			status = 'RESOLVED', resolved_assignment_id = $2, attempts = attempts + 1,
			version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $3`, caseID, replacementAssignmentID, expectedVersion)
	if err != nil {
		return fmt.Errorf("marketplace: resolve reassignment case: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// CreateFollowup records a durable decline follow-up action.
func (r *AssignmentRepository) CreateFollowup(ctx context.Context, q database.Querier, action *models.FollowupAction) error {
	if action.ID == "" {
		action.ID = ids.New("dfu")
	}
	payload, err := json.Marshal(orEmptyMap(action.Payload))
	if err != nil {
		return fmt.Errorf("marketplace: marshal followup payload: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_decline_followup_actions
			(id, organization_id, response_id, membership_id, action_type, reason_code,
			 safety_related, payload, status, attempts, next_attempt_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'PENDING', 0, now(), now(), now())`,
		action.ID, action.OrganizationID, action.ResponseID, action.MembershipID,
		action.ActionType, action.ReasonCode, action.SafetyRelated, payload)
	if err != nil {
		return fmt.Errorf("marketplace: create followup: %w", err)
	}
	return nil
}

// ListFollowups returns follow-ups for a response.
func (r *AssignmentRepository) ListFollowups(ctx context.Context, organizationID string, responseID string) ([]models.FollowupAction, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, response_id, membership_id, action_type, reason_code,
			safety_related, payload, status, attempts, last_error_code
		FROM fleet_partner_decline_followup_actions
		WHERE organization_id = $1 AND response_id = $2
		ORDER BY created_at, id`, organizationID, responseID)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list followups: %w", err)
	}
	defer rows.Close()

	var actions []models.FollowupAction
	for rows.Next() {
		var action models.FollowupAction
		var payload []byte
		if err := rows.Scan(&action.ID, &action.OrganizationID, &action.ResponseID,
			&action.MembershipID, &action.ActionType, &action.ReasonCode, &action.SafetyRelated,
			&payload, &action.Status, &action.Attempts, &action.LastErrorCode); err != nil {
			return nil, fmt.Errorf("marketplace: scan followup: %w", err)
		}
		_ = json.Unmarshal(payload, &action.Payload)
		actions = append(actions, action)
	}
	return actions, rows.Err()
}

// ClaimDueFollowups leases follow-ups for the processor.
func (r *AssignmentRepository) ClaimDueFollowups(ctx context.Context, q database.Querier, limit int) ([]models.FollowupAction, error) {
	rows, err := q.Query(ctx, `
		SELECT id, organization_id, response_id, membership_id, action_type, reason_code,
			safety_related, payload, status, attempts, last_error_code
		FROM fleet_partner_decline_followup_actions
		WHERE status IN ('PENDING', 'FAILED') AND (next_attempt_at IS NULL OR next_attempt_at <= now())
		ORDER BY updated_at LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: claim followups: %w", err)
	}
	defer rows.Close()

	var actions []models.FollowupAction
	for rows.Next() {
		var action models.FollowupAction
		var payload []byte
		if err := rows.Scan(&action.ID, &action.OrganizationID, &action.ResponseID,
			&action.MembershipID, &action.ActionType, &action.ReasonCode, &action.SafetyRelated,
			&payload, &action.Status, &action.Attempts, &action.LastErrorCode); err != nil {
			return nil, fmt.Errorf("marketplace: scan followup: %w", err)
		}
		_ = json.Unmarshal(payload, &action.Payload)
		actions = append(actions, action)
	}
	return actions, rows.Err()
}

// SetFollowupStatus records follow-up progress. A follow-up failure never
// rolls back the provider's decline.
func (r *AssignmentRepository) SetFollowupStatus(ctx context.Context, q database.Querier, actionID string, status string, errorCode string, nextAttempt *time.Time) error {
	var code *string
	if errorCode != "" {
		code = &errorCode
	}
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_decline_followup_actions SET
			status = $2, last_error_code = $3, next_attempt_at = $4,
			attempts = attempts + 1, updated_at = now()
		WHERE id = $1`, actionID, status, code, nextAttempt)
	if err != nil {
		return fmt.Errorf("marketplace: set followup status: %w", err)
	}
	return nil
}
