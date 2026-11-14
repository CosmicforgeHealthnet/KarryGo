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
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Sections 11-12: the production entry point of the organization opportunity.
//
// A source execution service announces an open request with
// platform.request.offered.v1. These queries are what turn that announcement
// into one request projection and one organization offer per eligible
// organization, and what keep every organization's copy in step with the
// source afterwards. They are platform-scoped by design (the caller is the
// service-authenticated event endpoint), but every row they create or touch
// carries its organization_id, and every organization-facing read keeps
// filtering on it.

// OpportunityCandidate is one active managed provider who could carry an
// opportunity for an organization.
type OpportunityCandidate struct {
	OrganizationID string
	MembershipID   string
	ProviderID     string
}

// ListOpportunityCandidates returns active, association-accepted and
// provisioned memberships of ACTIVE organizations for one service category.
//
// organizationIDs and providerIDs are optional narrowing filters supplied by
// the source (an execution service that already knows which organizations or
// which providers its broadcast reached). An empty filter means "no narrowing",
// never "everyone and anything": the membership, organization status and
// category predicates always apply.
func (r *ProjectionRepository) ListOpportunityCandidates(ctx context.Context, q database.Querier,
	serviceCategory string, organizationIDs []string, providerIDs []string,
	limit int) ([]OpportunityCandidate, error) {
	if q == nil {
		q = r.db
	}
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	if organizationIDs == nil {
		organizationIDs = []string{}
	}
	if providerIDs == nil {
		providerIDs = []string{}
	}
	rows, err := q.Query(ctx, `
		SELECT m.organization_id, m.id, m.provider_id
		FROM fleet_partner_organization_provider_memberships m
		JOIN fleet_partner_organizations o ON o.id = m.organization_id
		WHERE m.service_category = $1
		  AND m.status = 'ACTIVE'
		  AND m.association_accepted_at IS NOT NULL
		  AND m.provider_id IS NOT NULL AND m.provider_id <> ''
		  AND o.status = 'ACTIVE'
		  AND (cardinality($2::text[]) = 0 OR m.organization_id = ANY($2::text[]))
		  AND (cardinality($3::text[]) = 0 OR m.provider_id = ANY($3::text[]))
		ORDER BY m.organization_id, m.created_at, m.id
		LIMIT $4`, serviceCategory, organizationIDs, providerIDs, limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list opportunity candidates: %w", err)
	}
	defer rows.Close()

	var candidates []OpportunityCandidate
	for rows.Next() {
		var candidate OpportunityCandidate
		if err := rows.Scan(&candidate.OrganizationID, &candidate.MembershipID,
			&candidate.ProviderID); err != nil {
			return nil, fmt.Errorf("marketplace: scan opportunity candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

// ListBySourceRequest returns every organization's projection of one source
// request, ordered by id so concurrent writers always lock in the same order.
func (r *ProjectionRepository) ListBySourceRequest(ctx context.Context, q database.Querier,
	sourceService string, platformRequestID string) ([]models.RequestProjection, error) {
	if q == nil {
		q = r.db
	}
	rows, err := q.Query(ctx, `SELECT `+projectionColumns+`
		FROM fleet_partner_marketplace_request_projections
		WHERE source_service = $1 AND platform_request_id = $2
		ORDER BY id`, sourceService, platformRequestID)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list projections by source request: %w", err)
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

// InsertOpportunity creates an organization's projection of a source request.
//
// It never updates: a second delivery of the opening event, or a concurrent
// one, finds the row already present and reports created=false, so exactly one
// offer is ever opened per organization and request.
func (r *ProjectionRepository) InsertOpportunity(ctx context.Context, q database.Querier,
	projection *models.RequestProjection) (bool, error) {
	if projection.ID == "" {
		projection.ID = ids.New("mrq")
	}
	pickup, err := json.Marshal(orEmptyMap(projection.PickupSummary))
	if err != nil {
		return false, fmt.Errorf("marketplace: marshal pickup: %w", err)
	}
	dropoff, err := json.Marshal(orEmptyMap(projection.DropoffSummary))
	if err != nil {
		return false, fmt.Errorf("marketplace: marshal dropoff: %w", err)
	}
	requirements, err := json.Marshal(orEmptyMap(projection.OperationalRequirements))
	if err != nil {
		return false, fmt.Errorf("marketplace: marshal requirements: %w", err)
	}
	var id string
	err = q.QueryRow(ctx, `
		INSERT INTO fleet_partner_marketplace_request_projections
			(id, organization_id, source_service, platform_request_id, platform_booking_id,
			 service_type, projection_status, source_status, pickup_summary, dropoff_summary,
			 operational_requirements, offered_at, expires_at, source_version, last_event_id,
			 last_source_event_at, freshness_state, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'OFFERED', $7, $8, $9, $10, $11, $12, $13, $14, $15,
		        'CURRENT', 1, now(), now())
		ON CONFLICT (organization_id, source_service, platform_request_id) DO NOTHING
		RETURNING id`,
		projection.ID, projection.OrganizationID, projection.SourceService,
		projection.PlatformRequestID, projection.PlatformBookingID, projection.ServiceType,
		projection.SourceStatus, pickup, dropoff, requirements, projection.OfferedAt,
		projection.ExpiresAt, projection.SourceVersion, projection.LastEventID,
		projection.LastSourceEventAt).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("marketplace: insert opportunity projection: %w", err)
	}
	projection.ID = id
	projection.ProjectionStatus = models.ProjectionOffered
	projection.FreshnessState = models.FreshnessCurrent
	projection.Version = 1
	return true, nil
}

// SetExpiresAt records the source's current offer deadline on a projection.
func (r *ProjectionRepository) SetExpiresAt(ctx context.Context, q database.Querier,
	projectionID string, expiresAt time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_marketplace_request_projections SET
			expires_at = $2, updated_at = now()
		WHERE id = $1`, projectionID, expiresAt)
	if err != nil {
		return fmt.Errorf("marketplace: set projection expiry: %w", err)
	}
	return nil
}

// ExtendOpenOffer moves an OFFERED offer's deadline later, together with its
// still-pending recipients. A deadline is never moved earlier here, and an
// offer that has already resolved is never reopened.
func (r *OfferRepository) ExtendOpenOffer(ctx context.Context, q database.Querier,
	organizationID string, offerID string, expiresAt time.Time) (bool, error) {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_request_offers SET
			expires_at = $3, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND status = 'OFFERED' AND expires_at < $3`,
		organizationID, offerID, expiresAt)
	if err != nil {
		return false, fmt.Errorf("marketplace: extend offer: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	_, err = q.Exec(ctx, `
		UPDATE fleet_partner_provider_offer_recipients SET
			expires_at = $3, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND offer_id = $2 AND status = 'PENDING' AND expires_at < $3`,
		organizationID, offerID, expiresAt)
	if err != nil {
		return false, fmt.Errorf("marketplace: extend recipients: %w", err)
	}
	return true, nil
}

// ClaimDueOffers locks OFFERED offers whose deadline has passed.
//
// SKIP LOCKED lets several worker processes sweep concurrently without ever
// expiring the same offer twice or waiting on an offer a dispatcher is in the
// middle of accepting (acceptance holds the same row lock).
func (r *OfferRepository) ClaimDueOffers(ctx context.Context, q database.Querier,
	now time.Time, limit int) ([]models.Offer, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := q.Query(ctx, `SELECT `+offerColumns+`
		FROM fleet_partner_organization_request_offers
		WHERE status = 'OFFERED' AND expires_at <= $1
		ORDER BY expires_at, id
		LIMIT $2
		FOR UPDATE SKIP LOCKED`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: claim due offers: %w", err)
	}
	defer rows.Close()

	var offers []models.Offer
	for rows.Next() {
		offer, err := scanOffer(rows)
		if err != nil {
			return nil, err
		}
		offers = append(offers, *offer)
	}
	return offers, rows.Err()
}

// CloseOpenRecipients resolves every still-pending recipient of an offer.
func (r *OfferRepository) CloseOpenRecipients(ctx context.Context, q database.Querier,
	organizationID string, offerID string, status string) (int64, error) {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_provider_offer_recipients SET
			status = $3, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND offer_id = $2 AND status = 'PENDING'`,
		organizationID, offerID, status)
	if err != nil {
		return 0, fmt.Errorf("marketplace: close recipients: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ---- Trip lifecycle and assignment synchronization ----

// AssignmentTripLink pairs a live assignment with the trip projection the
// execution service created for it.
type AssignmentTripLink struct {
	AssignmentID   string
	OrganizationID string
	TripID         string
}

// ListAssignmentsBehindTrips finds assignments whose trip projection has moved
// past them: the trip is running, finished or cancelled while the assignment
// still says READY_FOR_EXECUTION, IN_PROGRESS or RECONCILIATION_REQUIRED.
//
// This is the recovery half of trip-driven assignment progress. Inline event
// handling advances assignments the moment a trip event arrives; this sweep
// catches whatever that missed (a trip event that landed before execution
// start confirmed, a crash between the two) and is safe to run repeatedly.
func (r *TripRepository) ListAssignmentsBehindTrips(ctx context.Context, limit int) ([]AssignmentTripLink, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT a.id, a.organization_id, t.id
		FROM fleet_partner_provider_assignments a
		JOIN fleet_partner_trip_projections t
		  ON t.assignment_id = a.id AND t.organization_id = a.organization_id
		WHERE a.status IN ('READY_FOR_EXECUTION', 'IN_PROGRESS', 'RECONCILIATION_REQUIRED')
		  AND t.freshness_state = 'CURRENT'
		  AND (
		        t.completed_at IS NOT NULL
		     OR t.cancelled_at IS NOT NULL
		     OR (a.status <> 'IN_PROGRESS' AND lower(t.trip_status) NOT IN
		          ('', 'assigned', 'accepted', 'created', 'pending', 'scheduled', 'reserved'))
		  )
		ORDER BY a.updated_at, a.id
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list assignments behind trips: %w", err)
	}
	defer rows.Close()

	var links []AssignmentTripLink
	for rows.Next() {
		var link AssignmentTripLink
		if err := rows.Scan(&link.AssignmentID, &link.OrganizationID, &link.TripID); err != nil {
			return nil, fmt.Errorf("marketplace: scan assignment trip link: %w", err)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// LockTripByID loads a trip projection FOR SHARE inside the caller's
// transaction so the assignment decision is made against a stable trip state.
func (r *TripRepository) LockTripByID(ctx context.Context, q database.Querier,
	organizationID string, tripID string) (*models.TripProjection, error) {
	row := q.QueryRow(ctx, `SELECT `+tripColumns+`
		FROM fleet_partner_trip_projections
		WHERE organization_id = $1 AND id = $2 FOR SHARE`, organizationID, tripID)
	return scanTrip(row)
}

// MembershipHomeBranch returns the home branch of an organization's managed
// provider membership (nil when it has none), for branch-scoped reads.
func (r *AssignmentRepository) MembershipHomeBranch(ctx context.Context, organizationID string,
	membershipID string) (*string, error) {
	var branch *string
	err := r.db.QueryRow(ctx, `SELECT home_branch_id FROM fleet_partner_organization_provider_memberships
		WHERE organization_id = $1 AND id = $2`, organizationID, membershipID).Scan(&branch)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: membership home branch: %w", err)
	}
	return branch, nil
}

// TripHistoryEntry is one append-only trip status change.
type TripHistoryEntry struct {
	ID            string    `json:"id"`
	FromStatus    string    `json:"fromStatus"`
	ToStatus      string    `json:"toStatus"`
	SourceVersion int64     `json:"sourceVersion"`
	SourceEventID *string   `json:"sourceEventId,omitempty"`
	OccurredAt    time.Time `json:"occurredAt"`
}

// ListTripHistory returns one trip projection's status history, oldest first.
func (r *TripRepository) ListTripHistory(ctx context.Context, organizationID string,
	tripID string, limit int) ([]TripHistoryEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, from_status, to_status, source_version, source_event_id, occurred_at
		FROM fleet_partner_trip_projection_history
		WHERE organization_id = $1 AND trip_projection_id = $2
		ORDER BY occurred_at, source_version, id
		LIMIT $3`, organizationID, tripID, limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list trip history: %w", err)
	}
	defer rows.Close()

	history := []TripHistoryEntry{}
	for rows.Next() {
		var entry TripHistoryEntry
		if err := rows.Scan(&entry.ID, &entry.FromStatus, &entry.ToStatus,
			&entry.SourceVersion, &entry.SourceEventID, &entry.OccurredAt); err != nil {
			return nil, fmt.Errorf("marketplace: scan trip history: %w", err)
		}
		history = append(history, entry)
	}
	return history, rows.Err()
}

// TripPageFilter narrows the organization trip list.
type TripPageFilter struct {
	Status        string
	SourceService string
	MembershipID  string
	Limit         int
	Offset        int
}

// ListPage returns an organization's trip projections, newest activity first.
//
// Branch scope follows the executing provider's home branch: a trip run by a
// provider homed in a branch the caller cannot see is not returned. Trips
// whose membership has no home branch stay visible to every permitted caller,
// matching how the providers list treats unbranched memberships.
func (r *TripRepository) ListPage(ctx context.Context, query tenant.Query,
	filter TripPageFilter) ([]models.TripProjection, bool, error) {
	if query.OrganizationID == "" {
		return nil, false, fmt.Errorf("marketplace: organization id is required")
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	branches := query.PermittedBranchIDs
	if branches == nil {
		branches = []string{}
	}
	rows, err := r.db.Query(ctx, `SELECT `+tripColumnsQualified+`
		FROM fleet_partner_trip_projections t
		LEFT JOIN fleet_partner_provider_assignments a
		  ON a.id = t.assignment_id AND a.organization_id = t.organization_id
		LEFT JOIN fleet_partner_organization_provider_memberships m
		  ON m.id = COALESCE(t.organization_membership_id, a.membership_id) AND m.organization_id = t.organization_id
		WHERE t.organization_id = $1
		  AND ($2 = '' OR t.trip_status = $2)
		  AND ($3 = '' OR t.source_service = $3)
		  AND ($4 = '' OR t.organization_membership_id = $4)
		  AND (cardinality($5::text[]) = 0 OR m.home_branch_id IS NULL OR m.home_branch_id = ANY($5::text[]))
		ORDER BY t.updated_at DESC, t.id
		LIMIT $6 OFFSET $7`, query.OrganizationID, filter.Status, filter.SourceService,
		filter.MembershipID, branches, limit+1, offset)
	if err != nil {
		return nil, false, fmt.Errorf("marketplace: list trip page: %w", err)
	}
	defer rows.Close()

	trips := []models.TripProjection{}
	for rows.Next() {
		trip, err := scanTripRows(rows)
		if err != nil {
			return nil, false, err
		}
		trips = append(trips, *trip)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(trips) > limit
	if hasMore {
		trips = trips[:limit]
	}
	return trips, hasMore, nil
}

// GetScoped loads one trip projection inside the organization scope, applying
// the same branch rule as ListPage. Anything outside scope is ErrNotFound.
func (r *TripRepository) GetScoped(ctx context.Context, query tenant.Query,
	tripID string) (*models.TripProjection, error) {
	branches := query.PermittedBranchIDs
	if branches == nil {
		branches = []string{}
	}
	row := r.db.QueryRow(ctx, `SELECT `+tripColumnsQualified+`
		FROM fleet_partner_trip_projections t
		LEFT JOIN fleet_partner_provider_assignments a
		  ON a.id = t.assignment_id AND a.organization_id = t.organization_id
		LEFT JOIN fleet_partner_organization_provider_memberships m
		  ON m.id = COALESCE(t.organization_membership_id, a.membership_id) AND m.organization_id = t.organization_id
		WHERE t.organization_id = $1 AND t.id = $2
		  AND (cardinality($3::text[]) = 0 OR m.home_branch_id IS NULL OR m.home_branch_id = ANY($3::text[]))`,
		query.OrganizationID, tripID, branches)
	return scanTrip(row)
}

const tripColumnsQualified = `t.id, t.organization_id, t.source_service, t.platform_trip_id,
	t.platform_request_id, t.platform_booking_id, t.executing_provider_id,
	t.organization_membership_id, t.settlement_organization_id, t.assignment_id,
	t.service_type, t.trip_status, t.source_version, t.last_event_id, t.last_event_at,
	t.last_reconciled_at, t.freshness_state, t.completed_at, t.cancelled_at, t.version,
	t.created_at, t.updated_at`
