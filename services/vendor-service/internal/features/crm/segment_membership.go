package crm

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// qualifiedProfileCols is the Section 8A profile column list aliased to the
// profiles table, for queries that join another table alongside it. It is kept
// beside profileCols so the two are edited together.
const qualifiedProfileCols = `p.id,p.business_id,p.subject_type,p.customer_ref,p.guest_ref,p.external_ref,
p.safe_display_label,p.status,p.privacy_state,p.merge_state,p.acquisition_source_type,p.acquisition_source_ref,
p.first_completed_order_at,p.last_completed_order_at,p.observed_order_count,p.completed_order_count,p.gross_spend_minor,p.net_spend_minor,
p.average_order_value_minor,p.commercial_currency,p.last_order_ref,p.last_branch_ref,p.preferred_branch_ref,
p.last_capability_type,p.commercial_projection_state,p.commercial_projection_version,p.last_commercial_source_observed_at,
p.consent_projection_version,p.consent_observed_at,p.redacted_at,p.merged_into_profile_id,p.created_at,p.updated_at,p.version`

// SegmentMember is one row of a segment's membership list.
type SegmentMember struct {
	Profile                   Profile
	Source                    string
	Generation                int64
	SegmentDefinitionVersion  *int64
	MatchedAt                 time.Time
	CommercialProjectionState ProjectionState
}

// MemberView is the safe wire projection of a member.
//
// It is a relationship reference and a projection state, never contact data:
// there is no email, phone or address field to populate because CRM does not
// hold one, so an audience export cannot become a contact-list export.
type MemberView struct {
	ProfileRef                string      `json:"profile_ref"`
	DisplayLabel              *string     `json:"display_label"`
	SubjectType               SubjectType `json:"subject_type"`
	CommercialProjectionState string      `json:"commercial_projection_state"`
	MembershipSource          string      `json:"membership_source"`
	Generation                int64       `json:"generation"`
	DefinitionVersion         *int64      `json:"definition_version,omitempty"`
	MatchedAt                 time.Time   `json:"matched_at"`
}

// ---------------------------------------------------------------------------
// Static membership
// ---------------------------------------------------------------------------

// BulkStaticMembership adds or removes explicit members of a static segment.
//
// Dynamic segments refuse this outright: a hand-added member would be silently
// discarded at the next materialization, so accepting the write would be a lie.
func (s *Service) BulkStaticMembership(ctx context.Context, actor Actor, scope Scope, segmentID string, profileIDs []string, add bool) (BulkTagResult, error) {
	if err := s.assertSegmentationWrites(); err != nil {
		return BulkTagResult{}, err
	}
	ids, err := normalizeProfileRefs(profileIDs)
	if err != nil {
		return BulkTagResult{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BulkTagResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	segment, err := s.loadSegmentForUpdate(ctx, tx, actor.VendorID, segmentID)
	if err != nil {
		return BulkTagResult{}, err
	}
	if err = s.assertSegmentVisible(scope, segment); err != nil {
		return BulkTagResult{}, err
	}
	if err = authorizeObjectScope(scope, segment.ScopeType, segment.BranchID, CodeSegmentScopeDenied); err != nil {
		return BulkTagResult{}, err
	}
	if segment.Dynamic() {
		return BulkTagResult{}, errStaticOnly()
	}
	if segment.Status == SegmentArchived {
		return BulkTagResult{}, errSegmentState("An archived segment cannot be changed.")
	}
	if add {
		// Locking and re-checking the profiles inside this transaction is what
		// makes redaction win the race: a REDACT committing first is visible
		// here and refuses the add, and one committing later finds the row and
		// removes it.
		if err = s.assertProfilesAssignable(ctx, tx, actor, scope, ids, segment.BranchID); err != nil {
			return BulkTagResult{}, err
		}
	}

	result := BulkTagResult{TagRef: segmentID, Requested: len(ids)}
	if add {
		membershipIDs := make([]string, len(ids))
		for index := range ids {
			membershipIDs[index] = newID("crmsm_")
		}
		inserted, insertErr := tx.Exec(ctx, `INSERT INTO crm_segment_memberships
			(id,business_id,segment_id,profile_id,membership_source,generation,segment_definition_version,added_by_staff_ref)
			SELECT m.id, $1, $2, m.profile_id, 'STATIC', 0, $3, NULLIF($4,'')
			FROM unnest($5::text[], $6::text[]) AS m(id, profile_id)
			ON CONFLICT (business_id, segment_id, profile_id, generation) DO NOTHING`,
			actor.VendorID, segmentID, segment.CurrentDefinitionVersion, actor.StaffID, membershipIDs, ids)
		if insertErr != nil {
			return BulkTagResult{}, insertErr
		}
		result.Applied = int(inserted.RowsAffected())
	} else {
		removed, removeErr := tx.Exec(ctx, `DELETE FROM crm_segment_memberships
			WHERE business_id=$1 AND segment_id=$2 AND membership_source='STATIC' AND profile_id = ANY($3)`,
			actor.VendorID, segmentID, ids)
		if removeErr != nil {
			return BulkTagResult{}, removeErr
		}
		result.Applied = int(removed.RowsAffected())
	}
	result.Skipped = result.Requested - result.Applied

	// The count is maintained transactionally from the rows themselves rather
	// than incremented, so it cannot drift from the membership it describes.
	count, err := s.recountStaticMembership(ctx, tx, actor.VendorID, segmentID)
	if err != nil {
		return BulkTagResult{}, err
	}
	action := "vendor.crm.segment.members_removed"
	if add {
		action = "vendor.crm.segment.members_added"
	}
	if err = s.auditTx(ctx, tx, actor, action, "crm_segment", segmentID, derefOr(segment.BranchID), nil,
		map[string]any{"requested": result.Requested, "applied": result.Applied, "member_count": count}, ""); err != nil {
		return BulkTagResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return BulkTagResult{}, err
	}
	s.metrics.Add("crm_segment_members_current", float64(count), "source", MembershipStatic)
	return result, nil
}

func (s *Service) recountStaticMembership(ctx context.Context, tx pgx.Tx, businessID, segmentID string) (int, error) {
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM crm_segment_memberships
		WHERE business_id=$1 AND segment_id=$2 AND membership_source='STATIC'`, businessID, segmentID).Scan(&count); err != nil {
		return 0, err
	}
	_, err := tx.Exec(ctx, `UPDATE crm_segments SET current_member_count=$1, evaluation_status='CURRENT',
		last_evaluated_at=now(), stale_since=NULL, updated_at=now(), version=version+1
		WHERE business_id=$2 AND id=$3 AND segment_type='STATIC'`, count, businessID, segmentID)
	return count, err
}

// ---------------------------------------------------------------------------
// Manual exclusions
// ---------------------------------------------------------------------------

// BulkExclusion adds or removes manual exclusions from a dynamic segment.
//
// An exclusion is a standing decision about a customer, so it is stored
// separately from membership and re-applied on every evaluation. It survives
// rule changes and rematerializations, which is the point: "never market to
// this customer from this audience" must not be undone by an edit.
func (s *Service) BulkExclusion(ctx context.Context, actor Actor, scope Scope, segmentID string, profileIDs []string, add bool) (BulkTagResult, error) {
	if err := s.assertSegmentationWrites(); err != nil {
		return BulkTagResult{}, err
	}
	ids, err := normalizeProfileRefs(profileIDs)
	if err != nil {
		return BulkTagResult{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BulkTagResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	segment, err := s.loadSegmentForUpdate(ctx, tx, actor.VendorID, segmentID)
	if err != nil {
		return BulkTagResult{}, err
	}
	if err = s.assertSegmentVisible(scope, segment); err != nil {
		return BulkTagResult{}, err
	}
	if err = authorizeObjectScope(scope, segment.ScopeType, segment.BranchID, CodeSegmentScopeDenied); err != nil {
		return BulkTagResult{}, err
	}
	if !segment.Dynamic() {
		return BulkTagResult{}, errDynamicOnly()
	}
	if segment.Status == SegmentArchived {
		return BulkTagResult{}, errSegmentState("An archived segment cannot be changed.")
	}
	if add {
		if err = s.assertProfilesAssignable(ctx, tx, actor, scope, ids, segment.BranchID); err != nil {
			return BulkTagResult{}, err
		}
	}

	result := BulkTagResult{TagRef: segmentID, Requested: len(ids)}
	if add {
		exclusionIDs := make([]string, len(ids))
		for index := range ids {
			exclusionIDs[index] = newID("crmse_")
		}
		inserted, insertErr := tx.Exec(ctx, `INSERT INTO crm_segment_exclusions
			(id,business_id,segment_id,profile_id,created_by_staff_ref)
			SELECT e.id, $1, $2, e.profile_id, NULLIF($3,'')
			FROM unnest($4::text[], $5::text[]) AS e(id, profile_id)
			ON CONFLICT (business_id, segment_id, profile_id) DO NOTHING`,
			actor.VendorID, segmentID, actor.StaffID, exclusionIDs, ids)
		if insertErr != nil {
			return BulkTagResult{}, insertErr
		}
		result.Applied = int(inserted.RowsAffected())
		// An excluded customer must stop being a member immediately, not at the
		// next scheduled run.
		if _, err = tx.Exec(ctx, `DELETE FROM crm_segment_memberships
			WHERE business_id=$1 AND segment_id=$2 AND profile_id = ANY($3)`, actor.VendorID, segmentID, ids); err != nil {
			return BulkTagResult{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE crm_segments SET current_member_count=(
			SELECT count(*) FROM crm_segment_memberships m WHERE m.business_id=crm_segments.business_id
			  AND m.segment_id=crm_segments.id AND m.generation = COALESCE((
				SELECT e.generation FROM crm_segment_evaluations e
				WHERE e.business_id=crm_segments.business_id AND e.id=crm_segments.current_evaluation_id), 0))
			WHERE business_id=$1 AND id=$2`, actor.VendorID, segmentID); err != nil {
			return BulkTagResult{}, err
		}
	} else {
		removed, removeErr := tx.Exec(ctx, `DELETE FROM crm_segment_exclusions
			WHERE business_id=$1 AND segment_id=$2 AND profile_id = ANY($3)`, actor.VendorID, segmentID, ids)
		if removeErr != nil {
			return BulkTagResult{}, removeErr
		}
		result.Applied = int(removed.RowsAffected())
	}
	result.Skipped = result.Requested - result.Applied

	// Membership no longer matches the definition until it is rebuilt.
	if err = s.markSegmentStaleTx(ctx, tx, actor.VendorID, segmentID); err != nil {
		return BulkTagResult{}, err
	}
	action := "vendor.crm.segment.exclusions_removed"
	if add {
		action = "vendor.crm.segment.exclusions_added"
	}
	if err = s.auditTx(ctx, tx, actor, action, "crm_segment", segmentID, derefOr(segment.BranchID), nil,
		map[string]any{"requested": result.Requested, "applied": result.Applied}, ""); err != nil {
		return BulkTagResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return BulkTagResult{}, err
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Member listing
// ---------------------------------------------------------------------------

// ListSegmentMembers pages a segment's current membership.
//
// It reads the current generation only, so a rematerialization in flight is
// invisible: a caller never sees a half-built audience, and never sees the
// previous generation's members mixed with the new one's.
func (s *Service) ListSegmentMembers(ctx context.Context, actor Actor, scope Scope, segment Segment, limit, offset int) ([]SegmentMember, bool, error) {
	if limit <= 0 {
		limit = DefaultPageSize
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}
	if offset < 0 {
		offset = 0
	}

	generation := int64(0)
	if segment.Dynamic() {
		var err error
		if generation, err = s.currentGeneration(ctx, segment); err != nil {
			return nil, false, err
		}
		if generation == 0 {
			return nil, false, nil
		}
	}

	args := []any{actor.VendorID, segment.ID, generation}
	// Redacted and merged-away relationships are filtered here as well as being
	// removed by the privacy handler: defence in depth, so a membership row that
	// somehow outlived its redaction still cannot be read back.
	// The profile column list is qualified because the join brings a table with
	// overlapping column names (id, business_id, created_at) into scope.
	query := `SELECT ` + qualifiedProfileCols + `, m.membership_source, m.generation, m.segment_definition_version, m.matched_at
		FROM crm_segment_memberships m
		JOIN vendor_customer_profiles p ON p.id = m.profile_id AND p.business_id = m.business_id
		WHERE m.business_id=$1 AND m.segment_id=$2 AND m.generation=$3
		  AND p.privacy_state <> 'REDACTED' AND p.merge_state <> 'MERGED_SOURCE'`
	if !scope.Global {
		query += ` AND EXISTS (SELECT 1 FROM crm_customer_branch_relations br
			WHERE br.business_id=p.business_id AND br.profile_id=p.id AND br.branch_id = ANY($4))`
		args = append(args, scope.BranchIDs)
	}
	args = append(args, limit+1, offset)
	query += ` ORDER BY p.id ASC LIMIT $` + itoa(len(args)-1) + ` OFFSET $` + itoa(len(args))

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var members []SegmentMember
	for rows.Next() {
		member, scanErr := scanSegmentMemberRow(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		members = append(members, member)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(members) > limit
	if hasMore {
		members = members[:limit]
	}
	return members, hasMore, nil
}

// scanSegmentMemberRow scans the shared profile column list followed by the
// four membership columns, in the same order the query selects them.
func scanSegmentMemberRow(rows pgx.Rows) (SegmentMember, error) {
	var member SegmentMember
	p := &member.Profile
	err := rows.Scan(&p.ID, &p.BusinessID, &p.SubjectType, &p.CustomerRef, &p.GuestRef, &p.ExternalRef,
		&p.SafeDisplayLabel, &p.Status, &p.PrivacyState, &p.MergeState,
		&p.AcquisitionSourceType, &p.AcquisitionSourceRef,
		&p.FirstCompletedOrderAt, &p.LastCompletedOrderAt, &p.ObservedOrderCount, &p.CompletedOrderCount,
		&p.GrossSpendMinor, &p.NetSpendMinor, &p.AverageOrderValueMinor, &p.CommercialCurrency,
		&p.LastOrderRef, &p.LastBranchRef, &p.PreferredBranchRef, &p.LastCapabilityType,
		&p.CommercialProjectionState, &p.CommercialProjectionVersion, &p.LastCommercialSourceObservedAt,
		&p.ConsentProjectionVersion, &p.ConsentObservedAt, &p.RedactedAt, &p.MergedIntoProfileID,
		&p.CreatedAt, &p.UpdatedAt, &p.Version,
		&member.Source, &member.Generation, &member.SegmentDefinitionVersion, &member.MatchedAt)
	if err != nil {
		return SegmentMember{}, err
	}
	member.CommercialProjectionState = p.CommercialProjectionState
	return member, nil
}

// MemberViewOf renders a member for the wire.
func MemberViewOf(member SegmentMember) MemberView {
	return MemberView{
		ProfileRef:                member.Profile.ID,
		DisplayLabel:              member.Profile.SafeDisplayLabel,
		SubjectType:               member.Profile.SubjectType,
		CommercialProjectionState: string(member.CommercialProjectionState),
		MembershipSource:          member.Source,
		Generation:                member.Generation,
		DefinitionVersion:         member.SegmentDefinitionVersion,
		MatchedAt:                 member.MatchedAt,
	}
}

func (s *Service) currentGeneration(ctx context.Context, segment Segment) (int64, error) {
	if segment.CurrentEvaluationID == nil {
		return 0, nil
	}
	var generation int64
	err := s.db.QueryRow(ctx, `SELECT generation FROM crm_segment_evaluations WHERE business_id=$1 AND id=$2`,
		segment.BusinessID, *segment.CurrentEvaluationID).Scan(&generation)
	return generation, err
}

// ---------------------------------------------------------------------------
// Segment membership for one profile (Customer 360)
// ---------------------------------------------------------------------------

// SegmentMembershipSummary is one entry in a customer's segment list.
type SegmentMembershipSummary struct {
	SegmentRef       string `json:"segment_ref"`
	Name             string `json:"name"`
	SegmentType      string `json:"segment_type"`
	ScopeType        string `json:"scope_type"`
	MembershipSource string `json:"membership_source"`
	EvaluationStatus string `json:"evaluation_status"`
}

// SegmentsForProfile lists the segments one relationship currently belongs to,
// filtered to what the caller may see.
//
// A branch-scoped caller is shown branch segments only. Telling a branch
// manager that a customer is in "Top 1% nationally" would disclose a
// business-wide ranking they have no visibility of.
func (s *Service) SegmentsForProfile(ctx context.Context, actor Actor, scope Scope, profileID string) ([]SegmentMembershipSummary, error) {
	args := []any{actor.VendorID, profileID}
	query := `SELECT g.id, g.name, g.segment_type, g.scope_type, m.membership_source, g.evaluation_status
		FROM crm_segment_memberships m
		JOIN crm_segments g ON g.id = m.segment_id AND g.business_id = m.business_id
		WHERE m.business_id=$1 AND m.profile_id=$2 AND g.status <> 'ARCHIVED'
		  AND m.generation = CASE WHEN g.segment_type = 'STATIC' THEN 0 ELSE COALESCE(
			(SELECT e.generation FROM crm_segment_evaluations e
			 WHERE e.business_id=g.business_id AND e.id=g.current_evaluation_id), -1) END`
	if !scope.Global {
		query += ` AND g.scope_type = 'BRANCH' AND g.branch_id = ANY($3)`
		args = append(args, scope.BranchIDs)
	}
	query += ` ORDER BY g.normalized_name ASC LIMIT ` + itoa(MaxPageSize)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SegmentMembershipSummary
	for rows.Next() {
		var entry SegmentMembershipSummary
		if err = rows.Scan(&entry.SegmentRef, &entry.Name, &entry.SegmentType, &entry.ScopeType,
			&entry.MembershipSource, &entry.EvaluationStatus); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// segmentDependsOn reports whether a dependency key is one a rule reads.
func segmentDependsOn(dependencies []string, key string) bool {
	for _, dependency := range dependencies {
		if dependency == key || strings.HasPrefix(dependency, key+".") {
			return true
		}
	}
	return false
}
