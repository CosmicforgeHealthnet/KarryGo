package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
)

// The fleet-owned half of an eligibility evaluation.
//
// Everything read here is a fact this service genuinely owns: the
// organization's lifecycle status, its verification submission, its documents,
// its branches, its capabilities, its relationship with a provider, and the
// financial projections it keeps. Nothing here reaches into another service's
// data, and nothing here is a substitute for asking an authority about the
// facts that authority owns.

// CompanyState is the fleet-side standing of an organization.
type CompanyState struct {
	Found      bool
	Status     string
	Version    int64
	ArchivedAt *time.Time
}

// CompanyState reads the organization row.
func (r *Repository) CompanyState(ctx context.Context, q database.Querier,
	organizationID string) (CompanyState, error) {
	if q == nil {
		q = r.db
	}
	var state CompanyState
	err := q.QueryRow(ctx, `
		SELECT status, version, archived_at
		FROM fleet_partner_organizations WHERE id = $1`, organizationID).
		Scan(&state.Status, &state.Version, &state.ArchivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CompanyState{}, nil
	}
	if err != nil {
		return CompanyState{}, fmt.Errorf("eligibility: company state: %w", err)
	}
	state.Found = true
	return state, nil
}

// VerificationState is the latest organization verification submission.
type VerificationState struct {
	Found         bool
	Status        string
	SourceVersion int64
	SyncedAt      *time.Time
}

// VerificationState reads the most recent verification request.
//
// "Most recent" is by submission version, not by timestamp: two rows written
// in the same millisecond would order arbitrarily by time, and the submission
// version is the authoritative sequence.
func (r *Repository) VerificationState(ctx context.Context, q database.Querier,
	organizationID string) (VerificationState, error) {
	if q == nil {
		q = r.db
	}
	var state VerificationState
	err := q.QueryRow(ctx, `
		SELECT status, source_version, synced_at
		FROM fleet_partner_organization_verification_requests
		WHERE organization_id = $1
		ORDER BY submission_version DESC LIMIT 1`, organizationID).
		Scan(&state.Status, &state.SourceVersion, &state.SyncedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return VerificationState{}, nil
	}
	if err != nil {
		return VerificationState{}, fmt.Errorf("eligibility: verification state: %w", err)
	}
	state.Found = true
	return state, nil
}

// OnboardingComplete reports whether every mandatory onboarding stage is
// complete and carries no blocking issue.
func (r *Repository) OnboardingComplete(ctx context.Context, q database.Querier,
	organizationID string) (bool, error) {
	if q == nil {
		q = r.db
	}
	var incomplete bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM fleet_partner_organization_onboarding_stages
			WHERE organization_id = $1
			  AND (status <> 'COMPLETE' OR jsonb_array_length(blocking_issues) > 0)
		)`, organizationID).Scan(&incomplete)
	if err != nil {
		return false, fmt.Errorf("eligibility: onboarding state: %w", err)
	}
	return !incomplete, nil
}

// RestrictionState reports live restrictions on an organization.
type RestrictionState struct {
	// ClosureInProgress means an open closure request exists.
	ClosureInProgress bool
	// BlockersPresent covers both confirmed blockers and blockers whose state
	// could not be established. UNKNOWN blocks exactly like BLOCKING: a
	// blocker nobody could check is not a cleared one.
	BlockersPresent bool
}

// Restricted reports whether anything blocks operations.
func (s RestrictionState) Restricted() bool {
	return s.ClosureInProgress || s.BlockersPresent
}

// RestrictionState reads closure requests and blockers.
func (r *Repository) RestrictionState(ctx context.Context, q database.Querier,
	organizationID string) (RestrictionState, error) {
	if q == nil {
		q = r.db
	}
	var state RestrictionState
	err := q.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT 1 FROM fleet_partner_organization_closure_requests
				WHERE organization_id = $1
				  AND status IN ('REQUESTED', 'ASSESSING', 'BLOCKED',
				                 'APPROVAL_REQUIRED', 'APPROVED', 'EXECUTING')
			),
			EXISTS (
				SELECT 1 FROM fleet_partner_organization_closure_blockers
				WHERE organization_id = $1 AND state IN ('BLOCKING', 'UNKNOWN')
			)`, organizationID).Scan(&state.ClosureInProgress, &state.BlockersPresent)
	if err != nil {
		return RestrictionState{}, fmt.Errorf("eligibility: restriction state: %w", err)
	}
	return state, nil
}

// DocumentState is one organization document type's standing.
type DocumentState struct {
	DocumentType string
	Present      bool
	Rejected     bool
	Expired      bool
	ExpiresAt    *time.Time
}

// DocumentStates reports the standing of each required document type.
//
// A document is only present when it reached READY. ATTACHED means a file was
// linked and nothing has confirmed it; treating that as satisfied would let an
// organization satisfy a compliance requirement by uploading anything.
func (r *Repository) DocumentStates(ctx context.Context, q database.Querier,
	organizationID string, requiredTypes []string) ([]DocumentState, error) {
	if q == nil {
		q = r.db
	}
	if len(requiredTypes) == 0 {
		return nil, nil
	}

	rows, err := q.Query(ctx, `
		SELECT document_type,
		       bool_or(status = 'READY') AS ready,
		       bool_or(status = 'REJECTED') AS rejected,
		       max(expires_at) FILTER (WHERE status = 'READY') AS ready_expiry
		FROM fleet_partner_organization_documents
		WHERE organization_id = $1 AND document_type = ANY($2::text[])
		  AND status <> 'REMOVED'
		GROUP BY document_type`, organizationID, requiredTypes)
	if err != nil {
		return nil, fmt.Errorf("eligibility: document states: %w", err)
	}
	defer rows.Close()

	held := map[string]DocumentState{}
	for rows.Next() {
		var documentType string
		var ready, rejected *bool
		var expiry *time.Time
		if scanErr := rows.Scan(&documentType, &ready, &rejected, &expiry); scanErr != nil {
			return nil, scanErr
		}
		state := DocumentState{
			DocumentType: documentType,
			Present:      ready != nil && *ready,
			Rejected:     rejected != nil && *rejected,
			ExpiresAt:    expiry,
		}
		if expiry != nil {
			state.Expired = !expiry.After(time.Now().UTC())
		}
		held[documentType] = state
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	states := make([]DocumentState, 0, len(requiredTypes))
	for _, documentType := range requiredTypes {
		state, ok := held[documentType]
		if !ok {
			state = DocumentState{DocumentType: documentType}
		}
		states = append(states, state)
	}
	return states, nil
}

// ReferenceDocumentState is one required document type's standing in the WP10
// document domain.
//
// Distinct from DocumentState above, which describes the legacy onboarding
// attachment. This one is about a reference to an authoritative media object,
// so "present" is not enough: the file must also be verified by a human,
// confirmed safe by media, unexpired, not revoked, not superseded, not deleted,
// and backed by a projection nobody is currently reconciling.
type ReferenceDocumentState struct {
	DocumentType string
	Present      bool
	Satisfied    bool
	// Reason is the controlled eligibility reason code when Satisfied is false.
	Reason string
}

// ReferenceDocumentStates reports whether each required document type is
// satisfied for one owner.
//
// Every rule that could block is evaluated in one place — in SQL, against the
// newest live document per type — so a second gate cannot be written that
// forgets one of them. An unresolved reconciliation case outranks everything
// else: the projection is known to disagree with the authority, so no answer
// derived from it can be trusted.
func (r *Repository) ReferenceDocumentStates(ctx context.Context, q database.Querier,
	organizationID string, ownerType string, ownerID string,
	requiredTypes []string) ([]ReferenceDocumentState, error) {
	if q == nil {
		q = r.db
	}
	if len(requiredTypes) == 0 || ownerID == "" {
		return nil, nil
	}

	rows, err := q.Query(ctx, `
		SELECT DISTINCT ON (d.document_type)
			d.document_type,
			d.lifecycle_state,
			d.verification_state,
			d.safety_projection,
			d.media_deletion_state,
			d.projection_freshness,
			d.expiry_date,
			d.revoked_at IS NOT NULL AS revoked,
			d.deleted_at IS NOT NULL AS deleted,
			EXISTS (
				SELECT 1 FROM fleet_partner_document_reconciliation_cases c
				WHERE c.document_id = d.id AND c.status IN ('OPEN', 'IN_PROGRESS')
			) AS reconciling
		FROM fleet_partner_documents d
		WHERE d.organization_id = $1
		  AND d.owner_type = $2
		  AND d.owner_id = $3
		  AND d.document_type = ANY($4::text[])
		ORDER BY d.document_type,
			-- Prefer a live document over a superseded one, then newest first.
			(d.lifecycle_state IN ('REPLACED', 'DELETED', 'REVOKED')) ASC,
			d.created_at DESC`,
		organizationID, ownerType, ownerID, requiredTypes)
	if err != nil {
		return nil, fmt.Errorf("eligibility: reference document states: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	held := map[string]ReferenceDocumentState{}
	for rows.Next() {
		var documentType, lifecycle, verification, safety, deletion, freshness string
		var expiry *time.Time
		var revoked, deleted, reconciling bool
		if scanErr := rows.Scan(&documentType, &lifecycle, &verification, &safety,
			&deletion, &freshness, &expiry, &revoked, &deleted, &reconciling); scanErr != nil {
			return nil, fmt.Errorf("eligibility: scan reference document state: %w", scanErr)
		}

		state := ReferenceDocumentState{DocumentType: documentType, Present: true}
		switch {
		case reconciling:
			state.Reason = models.ReasonDocumentReconciling
		case deleted || lifecycle == "DELETED" || deletion == "DELETED":
			state.Reason = models.ReasonDocumentDeleted
		case revoked || lifecycle == "REVOKED":
			state.Reason = models.ReasonDocumentRevoked
		case lifecycle == "REPLACED":
			state.Reason = models.ReasonDocumentReplaced
		case deletion != "ACTIVE":
			state.Reason = models.ReasonDocumentDeleted
		case safety != "SAFE":
			state.Reason = models.ReasonDocumentNotSafe
		case verification != "VERIFIED" || lifecycle != "VERIFIED":
			state.Reason = models.ReasonDocumentNotVerified
		case expiry != nil && !expiry.After(now):
			state.Reason = models.ReasonDocumentExpired
		case freshness != models.FreshnessCurrent:
			if freshness == models.FreshnessGapDetected || freshness == models.FreshnessReconciling {
				state.Reason = models.ReasonDocumentReconciling
			} else {
				state.Reason = models.ReasonDocumentProjectionStale
			}
		default:
			state.Satisfied = true
		}
		held[documentType] = state
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	states := make([]ReferenceDocumentState, 0, len(requiredTypes))
	for _, documentType := range requiredTypes {
		state, ok := held[documentType]
		if !ok {
			state = ReferenceDocumentState{
				DocumentType: documentType, Reason: models.ReasonDocumentMissing,
			}
		}
		states = append(states, state)
	}
	return states, nil
}

// BranchState is a branch's standing inside the organization.
type BranchState struct {
	Found  bool
	Status string
}

// BranchState reads one branch inside the tenant scope.
func (r *Repository) BranchState(ctx context.Context, q database.Querier,
	organizationID string, branchID string) (BranchState, error) {
	if q == nil {
		q = r.db
	}
	var state BranchState
	err := q.QueryRow(ctx, `
		SELECT status FROM fleet_partner_branches
		WHERE organization_id = $1 AND id = $2`, organizationID, branchID).Scan(&state.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return BranchState{}, nil
	}
	if err != nil {
		return BranchState{}, fmt.Errorf("eligibility: branch state: %w", err)
	}
	state.Found = true
	return state, nil
}

// CapabilityApproved reports whether the organization holds an approved
// capability for a service.
func (r *Repository) CapabilityApproved(ctx context.Context, q database.Querier,
	organizationID string, capability string) (bool, error) {
	if q == nil {
		q = r.db
	}
	var approved bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM fleet_partner_organization_capabilities
			WHERE organization_id = $1 AND capability = $2 AND status = 'APPROVED'
		)`, organizationID, capability).Scan(&approved)
	if err != nil {
		return false, fmt.Errorf("eligibility: capability state: %w", err)
	}
	return approved, nil
}

// SubscriptionState is the fleet-side projection of the authoritative
// subscription.
type SubscriptionState struct {
	Found                bool
	SafeStatus           string
	FreshnessState       string
	ReconciliationStatus string
	SourceVersion        int64
	LastSyncedAt         time.Time
}

// Usable reports whether the projection may be relied on.
//
// Both conditions are required: a projection that is CURRENT but has an open
// reconciliation case is a value Fleet and Wallet disagreed about, and acting
// on it means acting on a number nobody currently vouches for.
func (s SubscriptionState) Usable() bool {
	return s.Found && s.FreshnessState == "CURRENT" && s.ReconciliationStatus == "IDLE"
}

// SubscriptionState reads the newest subscription projection.
func (r *Repository) SubscriptionState(ctx context.Context, q database.Querier,
	organizationID string) (SubscriptionState, error) {
	if q == nil {
		q = r.db
	}
	var state SubscriptionState
	err := q.QueryRow(ctx, `
		SELECT safe_status, freshness_state, reconciliation_status,
		       source_version, last_synced_at
		FROM fleet_partner_finance_projections
		WHERE organization_id = $1 AND projection_type = 'SUBSCRIPTION'
		ORDER BY last_synced_at DESC LIMIT 1`, organizationID).
		Scan(&state.SafeStatus, &state.FreshnessState, &state.ReconciliationStatus,
			&state.SourceVersion, &state.LastSyncedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubscriptionState{}, nil
	}
	if err != nil {
		return SubscriptionState{}, fmt.Errorf("eligibility: subscription state: %w", err)
	}
	state.Found = true
	return state, nil
}

// MembershipState is the organization's relationship with a provider.
type MembershipState struct {
	Found                 bool
	ID                    string
	Status                string
	ProviderID            string
	ProviderService       string
	ServiceCategory       string
	HomeBranchID          *string
	AssociationAcceptedAt *time.Time
	SourceVersion         int64
	SyncedAt              *time.Time
	Version               int64
}

// MembershipState reads one provider membership inside the tenant scope.
func (r *Repository) MembershipState(ctx context.Context, q database.Querier,
	organizationID string, membershipID string) (MembershipState, error) {
	if q == nil {
		q = r.db
	}
	var state MembershipState
	var providerID *string
	err := q.QueryRow(ctx, `
		SELECT id, status, provider_id, provider_service, service_category, home_branch_id,
		       association_accepted_at, source_version, synced_at, version
		FROM fleet_partner_organization_provider_memberships
		WHERE organization_id = $1 AND id = $2`, organizationID, membershipID).
		Scan(&state.ID, &state.Status, &providerID, &state.ProviderService, &state.ServiceCategory,
			&state.HomeBranchID, &state.AssociationAcceptedAt, &state.SourceVersion,
			&state.SyncedAt, &state.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return MembershipState{}, nil
	}
	if err != nil {
		return MembershipState{}, fmt.Errorf("eligibility: membership state: %w", err)
	}
	if providerID != nil {
		state.ProviderID = *providerID
	}
	state.Found = true
	return state, nil
}

// MembershipHasLiveAssignment reports whether a membership already holds work
// that has not resolved.
//
// The status list matches the partial unique index that enforces one live
// assignment per request, so the two never disagree about what "live" means.
func (r *Repository) MembershipHasLiveAssignment(ctx context.Context, q database.Querier,
	organizationID string, membershipID string, excludeAssignmentID string) (bool, error) {
	if q == nil {
		q = r.db
	}
	var live bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM fleet_partner_provider_assignments
			WHERE organization_id = $1 AND membership_id = $2
			  AND ($3 = '' OR id <> $3)
			  AND status IN ('CREATED', 'PENDING_PROVIDER_RESPONSE', 'ACCEPTED',
			                 'READY_FOR_EXECUTION', 'IN_PROGRESS', 'EXECUTION_START_PENDING')
		)`, organizationID, membershipID, excludeAssignmentID).Scan(&live)
	if err != nil {
		return false, fmt.Errorf("eligibility: live assignment: %w", err)
	}
	return live, nil
}

// AssetHasLiveAssignment reports whether a vehicle reference is already
// committed to live organization work.
//
// Fleet records the asset reference it was told to use; this is the fleet-side
// half of the vehicle conflict question, and it is combined with the
// authority's answer rather than replacing it.
func (r *Repository) AssetHasLiveAssignment(ctx context.Context, q database.Querier,
	organizationID string, assetReference string, excludeAssignmentID string) (bool, error) {
	if q == nil {
		q = r.db
	}
	if assetReference == "" {
		return false, nil
	}
	var live bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM fleet_partner_provider_assignments
			WHERE organization_id = $1 AND asset_reference = $2
			  AND ($3 = '' OR id <> $3)
			  AND status IN ('CREATED', 'PENDING_PROVIDER_RESPONSE', 'ACCEPTED',
			                 'READY_FOR_EXECUTION', 'IN_PROGRESS', 'EXECUTION_START_PENDING')
		)`, organizationID, assetReference, excludeAssignmentID).Scan(&live)
	if err != nil {
		return false, fmt.Errorf("eligibility: asset live assignment: %w", err)
	}
	return live, nil
}
