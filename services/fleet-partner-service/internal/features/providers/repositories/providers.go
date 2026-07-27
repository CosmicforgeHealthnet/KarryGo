// Package repositories implements tenant-scoped persistence for managed
// provider memberships, invitations, capabilities, policies and sagas.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ErrNotFound is returned when a tenant-scoped lookup matches nothing.
var ErrNotFound = errors.New("providers: not found")

// ErrAlreadyManaged means the provider already holds a live membership with
// some organization for this service category.
var ErrAlreadyManaged = errors.New("providers: provider already has a live organization membership")

// Repository stores managed provider records.
type Repository struct {
	db database.Querier
}

// NewRepository wires the repository.
func NewRepository(db database.Querier) *Repository {
	return &Repository{db: db}
}

const membershipColumns = `id, organization_id, provider_service, service_category, provider_id,
	provider_mode, onboarding_source, relationship_type, employment_type, employee_number,
	home_branch_id, status, association_accepted_at, joined_at, suspended_at, ending_at,
	removed_at, source_version, last_event_id, synced_at, version, created_at, updated_at`

// CreateMembership inserts a membership.
func (r *Repository) CreateMembership(ctx context.Context, q database.Querier, organizationID string, membership *models.Membership) error {
	if organizationID == "" {
		return fmt.Errorf("providers: organization id is required")
	}
	if membership.ID == "" {
		membership.ID = ids.New("pmb")
	}
	membership.OrganizationID = organizationID
	membership.ProviderMode = "ORGANIZATION_MANAGED"
	membership.Version = 1
	if membership.Status == "" {
		membership.Status = models.StatusInvited
	}

	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_organization_provider_memberships
			(id, organization_id, provider_service, service_category, provider_id, provider_mode,
			 onboarding_source, relationship_type, employment_type, employee_number, home_branch_id,
			 status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'ORGANIZATION_MANAGED', $6, $7, $8, $9, $10, $11, 1, now(), now())`,
		membership.ID, organizationID, membership.ProviderService, membership.ServiceCategory,
		membership.ProviderID, membership.OnboardingSource, membership.RelationshipType,
		membership.EmploymentType, membership.EmployeeNumber, membership.HomeBranchID, membership.Status)
	if err != nil {
		return fmt.Errorf("providers: create membership: %w", err)
	}
	return nil
}

// GetMembership loads one membership inside the organization scope.
func (r *Repository) GetMembership(ctx context.Context, organizationID string, membershipID string) (*models.Membership, error) {
	return r.getMembership(ctx, r.db, organizationID, membershipID)
}

// getMembership reads through the supplied querier so a caller inside a
// transaction sees its own uncommitted writes. Reading through the pool from
// within a transaction would show stale state and make a multi-step
// transition (ACTIVE -> ENDING -> REMOVED) fail its own state-machine check.
func (r *Repository) getMembership(ctx context.Context, q database.Querier, organizationID string, membershipID string) (*models.Membership, error) {
	if q == nil {
		q = r.db
	}
	row := q.QueryRow(ctx, `SELECT `+membershipColumns+`
		FROM fleet_partner_organization_provider_memberships
		WHERE organization_id = $1 AND id = $2`, organizationID, membershipID)
	return scanMembership(row)
}

// FindLiveByProvider reports whether a provider already holds a live
// membership anywhere. Used to enforce one settlement organization per
// provider per service category.
func (r *Repository) FindLiveByProvider(ctx context.Context, q database.Querier, providerService string, providerID string) (*models.Membership, error) {
	if q == nil {
		q = r.db
	}
	row := q.QueryRow(ctx, `SELECT `+membershipColumns+`
		FROM fleet_partner_organization_provider_memberships
		WHERE provider_service = $1 AND provider_id = $2
		  AND status IN ('PENDING_PROVIDER_ACCEPTANCE','PENDING_SETUP','ACTIVE','SUSPENDED','ENDING')`,
		providerService, providerID)
	return scanMembership(row)
}

// ListMemberships returns memberships for a tenant query, honouring permitted
// branch scope.
func (r *Repository) ListMemberships(ctx context.Context, query tenant.Query) ([]models.Membership, error) {
	if query.OrganizationID == "" {
		return nil, fmt.Errorf("providers: organization id is required")
	}
	limit := query.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	branches := query.PermittedBranchIDs
	if branches == nil {
		branches = []string{}
	}
	rows, err := r.db.Query(ctx, `SELECT `+membershipColumns+`
		FROM fleet_partner_organization_provider_memberships
		WHERE organization_id = $1
		  AND (cardinality($2::text[]) = 0 OR home_branch_id IS NULL OR home_branch_id = ANY($2::text[]))
		ORDER BY created_at DESC, id
		LIMIT $3`, query.OrganizationID, branches, limit)
	if err != nil {
		return nil, fmt.Errorf("providers: list memberships: %w", err)
	}
	defer rows.Close()

	var memberships []models.Membership
	for rows.Next() {
		membership, err := scanMembershipRows(rows)
		if err != nil {
			return nil, err
		}
		memberships = append(memberships, *membership)
	}
	return memberships, rows.Err()
}

// CountLiveMemberships counts live managed providers; used by closure.
func (r *Repository) CountLiveMemberships(ctx context.Context, organizationID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM fleet_partner_organization_provider_memberships
		WHERE organization_id = $1
		  AND status IN ('PENDING_PROVIDER_ACCEPTANCE','PENDING_SETUP','ACTIVE','SUSPENDED','ENDING')`,
		organizationID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("providers: count live memberships: %w", err)
	}
	return count, nil
}

// SetProviderID records the resolved or provisioned provider identity.
func (r *Repository) SetProviderID(ctx context.Context, q database.Querier, organizationID string, membershipID string, providerID string, sourceVersion int64) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_provider_memberships SET
			provider_id = $3, source_version = GREATEST(source_version, $4),
			synced_at = now(), version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2`,
		organizationID, membershipID, providerID, sourceVersion)
	if err != nil {
		return fmt.Errorf("providers: set provider id: %w", err)
	}
	return nil
}

// TransitionMembership applies a validated state-machine move with
// compare-and-swap and appends append-only history.
func (r *Repository) TransitionMembership(ctx context.Context, q database.Querier, organizationID string, membershipID string, toStatus string, expectedVersion int64, entry models.HistoryEntry) error {
	current, err := r.getMembership(ctx, q, organizationID, membershipID)
	if err != nil {
		return err
	}
	if !models.CanTransition(current.Status, toStatus) {
		return fmt.Errorf("providers: transition %s -> %s is not allowed", current.Status, toStatus)
	}

	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_provider_memberships SET
			status = $3,
			association_accepted_at = CASE WHEN $3 = 'PENDING_SETUP' THEN COALESCE(association_accepted_at, now()) ELSE association_accepted_at END,
			joined_at = CASE WHEN $3 = 'ACTIVE' THEN COALESCE(joined_at, now()) ELSE joined_at END,
			suspended_at = CASE WHEN $3 = 'SUSPENDED' THEN now() ELSE suspended_at END,
			ending_at = CASE WHEN $3 = 'ENDING' THEN now() ELSE ending_at END,
			removed_at = CASE WHEN $3 = 'REMOVED' THEN now() ELSE removed_at END,
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $4`,
		organizationID, membershipID, toStatus, expectedVersion)
	if err != nil {
		return fmt.Errorf("providers: transition membership: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}

	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_provider_membership_history
			(id, organization_id, membership_id, from_status, to_status, actor_type, actor_id,
			 reason_code, reason_text, request_id, correlation_id, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())`,
		ids.New("pmh"), organizationID, membershipID, current.Status, toStatus,
		entry.ActorType, entry.ActorID, entry.ReasonCode, entry.ReasonText,
		entry.RequestID, entry.CorrelationID)
	if err != nil {
		return fmt.Errorf("providers: append membership history: %w", err)
	}
	return nil
}

// ListHistory returns the append-only membership history.
func (r *Repository) ListHistory(ctx context.Context, organizationID string, membershipID string) ([]models.HistoryEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, membership_id, from_status, to_status, actor_type, actor_id,
			reason_code, reason_text, request_id, correlation_id, occurred_at
		FROM fleet_partner_provider_membership_history
		WHERE organization_id = $1 AND membership_id = $2
		ORDER BY occurred_at DESC, id`, organizationID, membershipID)
	if err != nil {
		return nil, fmt.Errorf("providers: list history: %w", err)
	}
	defer rows.Close()

	var entries []models.HistoryEntry
	for rows.Next() {
		var entry models.HistoryEntry
		if err := rows.Scan(&entry.ID, &entry.OrganizationID, &entry.MembershipID, &entry.FromStatus,
			&entry.ToStatus, &entry.ActorType, &entry.ActorID, &entry.ReasonCode, &entry.ReasonText,
			&entry.RequestID, &entry.CorrelationID, &entry.OccurredAt); err != nil {
			return nil, fmt.Errorf("providers: scan history: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// UpsertPolicy stores the provider policy.
//
// ProviderDeclineRequired is always written as true; the database CHECK
// constraint rejects anything else.
func (r *Repository) UpsertPolicy(ctx context.Context, q database.Querier, organizationID string, policy *models.Policy) error {
	shifts, err := json.Marshal(orEmptyMap(policy.ShiftRestrictions))
	if err != nil {
		return fmt.Errorf("providers: marshal shift restrictions: %w", err)
	}
	zones, err := json.Marshal(orEmptyMap(policy.ZoneRestrictions))
	if err != nil {
		return fmt.Errorf("providers: marshal zone restrictions: %w", err)
	}
	workload, err := json.Marshal(orEmptyMap(policy.WorkloadRestrictions))
	if err != nil {
		return fmt.Errorf("providers: marshal workload restrictions: %w", err)
	}
	if policy.AssignmentMode == "" {
		policy.AssignmentMode = "OFFER"
	}
	if policy.VisibilityPolicy == "" {
		policy.VisibilityPolicy = "ORGANIZATION"
	}

	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_provider_policies
			(membership_id, organization_id, assignment_mode, provider_decline_required,
			 visibility_policy, shift_restrictions, zone_restrictions, workload_restrictions,
			 version, created_at, updated_at)
		VALUES ($1, $2, $3, true, $4, $5, $6, $7, 1, now(), now())
		ON CONFLICT (membership_id) DO UPDATE SET
			assignment_mode = EXCLUDED.assignment_mode,
			provider_decline_required = true,
			visibility_policy = EXCLUDED.visibility_policy,
			shift_restrictions = EXCLUDED.shift_restrictions,
			zone_restrictions = EXCLUDED.zone_restrictions,
			workload_restrictions = EXCLUDED.workload_restrictions,
			version = fleet_partner_provider_policies.version + 1,
			updated_at = now()`,
		policy.MembershipID, organizationID, policy.AssignmentMode, policy.VisibilityPolicy,
		shifts, zones, workload)
	if err != nil {
		return fmt.Errorf("providers: upsert policy: %w", err)
	}
	return nil
}

// GetPolicy loads a provider policy.
func (r *Repository) GetPolicy(ctx context.Context, organizationID string, membershipID string) (*models.Policy, error) {
	var policy models.Policy
	var shifts, zones, workload []byte
	err := r.db.QueryRow(ctx, `
		SELECT membership_id, organization_id, assignment_mode, provider_decline_required,
			visibility_policy, shift_restrictions, zone_restrictions, workload_restrictions,
			version, created_at, updated_at
		FROM fleet_partner_provider_policies
		WHERE organization_id = $1 AND membership_id = $2`, organizationID, membershipID).
		Scan(&policy.MembershipID, &policy.OrganizationID, &policy.AssignmentMode,
			&policy.ProviderDeclineRequired, &policy.VisibilityPolicy, &shifts, &zones, &workload,
			&policy.Version, &policy.CreatedAt, &policy.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("providers: scan policy: %w", err)
	}
	_ = json.Unmarshal(shifts, &policy.ShiftRestrictions)
	_ = json.Unmarshal(zones, &policy.ZoneRestrictions)
	_ = json.Unmarshal(workload, &policy.WorkloadRestrictions)
	return &policy, nil
}

// ReplaceCapabilities sets the provider's capability rows.
func (r *Repository) ReplaceCapabilities(ctx context.Context, q database.Querier, organizationID string, membershipID string, capabilities []models.Capability) error {
	if _, err := q.Exec(ctx, `
		UPDATE fleet_partner_provider_capabilities SET status = 'EXPIRED', updated_at = now()
		WHERE organization_id = $1 AND membership_id = $2 AND status = 'ACTIVE'`,
		organizationID, membershipID); err != nil {
		return fmt.Errorf("providers: expire capabilities: %w", err)
	}
	for _, capability := range capabilities {
		vehicles, err := json.Marshal(orEmptySlice(capability.VehicleClasses))
		if err != nil {
			return fmt.Errorf("providers: marshal vehicle classes: %w", err)
		}
		zones, err := json.Marshal(orEmptySlice(capability.OperatingZones))
		if err != nil {
			return fmt.Errorf("providers: marshal operating zones: %w", err)
		}
		qualifications, err := json.Marshal(orEmptySlice(capability.Qualifications))
		if err != nil {
			return fmt.Errorf("providers: marshal qualifications: %w", err)
		}
		if _, err := q.Exec(ctx, `
			INSERT INTO fleet_partner_provider_capabilities
				(id, organization_id, membership_id, service_type, vehicle_classes, operating_zones,
				 qualifications, valid_from, valid_until, status, version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, now(), $8, 'ACTIVE', 1, now(), now())`,
			ids.New("pcap"), organizationID, membershipID, capability.ServiceType,
			vehicles, zones, qualifications, capability.ValidUntil); err != nil {
			return fmt.Errorf("providers: insert capability: %w", err)
		}
	}
	return nil
}

// ListCapabilities returns active capabilities.
func (r *Repository) ListCapabilities(ctx context.Context, organizationID string, membershipID string) ([]models.Capability, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, membership_id, service_type, vehicle_classes, operating_zones,
			qualifications, valid_from, valid_until, status, version, created_at, updated_at
		FROM fleet_partner_provider_capabilities
		WHERE organization_id = $1 AND membership_id = $2 AND status = 'ACTIVE'
		ORDER BY created_at, id`, organizationID, membershipID)
	if err != nil {
		return nil, fmt.Errorf("providers: list capabilities: %w", err)
	}
	defer rows.Close()

	var capabilities []models.Capability
	for rows.Next() {
		var capability models.Capability
		var vehicles, zones, qualifications []byte
		if err := rows.Scan(&capability.ID, &capability.OrganizationID, &capability.MembershipID,
			&capability.ServiceType, &vehicles, &zones, &qualifications, &capability.ValidFrom,
			&capability.ValidUntil, &capability.Status, &capability.Version,
			&capability.CreatedAt, &capability.UpdatedAt); err != nil {
			return nil, fmt.Errorf("providers: scan capability: %w", err)
		}
		_ = json.Unmarshal(vehicles, &capability.VehicleClasses)
		_ = json.Unmarshal(zones, &capability.OperatingZones)
		_ = json.Unmarshal(qualifications, &capability.Qualifications)
		capabilities = append(capabilities, capability)
	}
	return capabilities, rows.Err()
}

func scanMembership(row pgx.Row) (*models.Membership, error) {
	var membership models.Membership
	err := row.Scan(
		&membership.ID, &membership.OrganizationID, &membership.ProviderService,
		&membership.ServiceCategory, &membership.ProviderID, &membership.ProviderMode,
		&membership.OnboardingSource, &membership.RelationshipType, &membership.EmploymentType,
		&membership.EmployeeNumber, &membership.HomeBranchID, &membership.Status,
		&membership.AssociationAcceptedAt, &membership.JoinedAt, &membership.SuspendedAt,
		&membership.EndingAt, &membership.RemovedAt, &membership.SourceVersion,
		&membership.LastEventID, &membership.SyncedAt, &membership.Version,
		&membership.CreatedAt, &membership.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("providers: scan membership: %w", err)
	}
	return &membership, nil
}

func scanMembershipRows(rows pgx.Rows) (*models.Membership, error) {
	var membership models.Membership
	err := rows.Scan(
		&membership.ID, &membership.OrganizationID, &membership.ProviderService,
		&membership.ServiceCategory, &membership.ProviderID, &membership.ProviderMode,
		&membership.OnboardingSource, &membership.RelationshipType, &membership.EmploymentType,
		&membership.EmployeeNumber, &membership.HomeBranchID, &membership.Status,
		&membership.AssociationAcceptedAt, &membership.JoinedAt, &membership.SuspendedAt,
		&membership.EndingAt, &membership.RemovedAt, &membership.SourceVersion,
		&membership.LastEventID, &membership.SyncedAt, &membership.Version,
		&membership.CreatedAt, &membership.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("providers: scan membership: %w", err)
	}
	return &membership, nil
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

var _ = time.Now
