package retentionexec

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	retentionmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/models"
)

// redactedPlaceholder marks a field as already anonymized -- the same
// literal internal/platform/audit.Redact uses for a redacted log/audit
// value, reused here so a human reading either table recognizes the same
// convention rather than two different "this was scrubbed" spellings.
const redactedPlaceholder = "[REDACTED]"

// OrgContactExecutor implements retention for the ClassOperational class,
// scoped to exactly one real, explicitly safe target: an organization's own
// authorized-representative contact fields (full_name/email/phone --
// CONTACT_PII per docs/section28-sensitive-column-inventory.json), and only
// once the organization itself has been CLOSED (internal/features/
// organizations' closure workflow -- see that feature's own session notes)
// and the organization's effective OPERATIONAL retention window has
// elapsed since closure completed.
//
// This was the first of the four classes this package executes end to
// end; the other three Section 28 names (expired exports through Media
// Service, superseded cached projections, non-dispute support metadata)
// now each have their own ClassExecutor too -- see
// executor_expired_export.go, executor_superseded_projection.go and
// executor_support_metadata.go. See the package doc comment for why a
// class with no registered executor can never be executed regardless.
type OrgContactExecutor struct{}

// NewOrgContactExecutor builds the executor. Stateless -- every dependency
// (legal hold, retention days, the database handle itself) is supplied per
// call by Runner, not held here.
func NewOrgContactExecutor() *OrgContactExecutor { return &OrgContactExecutor{} }

// Class implements ClassExecutor.
func (e *OrgContactExecutor) Class() string { return retentionmodels.ClassOperational }

const orgContactRecordType = "organization_authorized_representative"

// FindCandidates returns authorized representatives of organizationID that
// are not yet anonymized, but ONLY when organizationID is itself CLOSED --
// an open organization's representatives are never eligible no matter how
// old the row is; this executor's eligibility window (EligibleSince) is
// anchored to closure, not row age.
func (e *OrgContactExecutor) FindCandidates(ctx context.Context, q database.Querier, organizationID string, limit int) ([]Candidate, error) {
	rows, err := q.Query(ctx, `
		SELECT r.id
		FROM fleet_partner_organization_authorized_representatives r
		JOIN fleet_partner_organizations o ON o.id = r.organization_id
		WHERE r.organization_id = $1
		  AND o.status = 'CLOSED'
		  AND r.email <> $2
		ORDER BY r.id
		LIMIT $3`, organizationID, redactedPlaceholder, limit)
	if err != nil {
		return nil, fmt.Errorf("retentionexec: org contact find candidates: %w", err)
	}
	defer rows.Close()
	var candidates []Candidate
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("retentionexec: org contact scan candidate: %w", err)
		}
		candidates = append(candidates, Candidate{RecordType: orgContactRecordType, RecordID: id})
	}
	return candidates, rows.Err()
}

// ScopeForLegalHold checks an ORGANIZATION-scoped hold (not a
// representative-scoped one -- legalhold's ValidScopeTypes has no
// representative-level granularity, and an org-wide hold is the correct,
// conservative veto for any of that organization's own contact data).
func (e *OrgContactExecutor) ScopeForLegalHold(candidate Candidate) (scopeType, scopeReference string) {
	// The organization ID is not on Candidate itself (only the
	// representative's own row ID is) -- Runner resolves it via the run's
	// own OrganizationID, which IS the correct scope reference for every
	// candidate this executor ever produces (FindCandidates is already
	// scoped to one organization). Returning "ORGANIZATION" with an empty
	// reference here is intentional: Runner substitutes its own
	// organizationID before calling IsUnderHold -- see runner.go.
	return "ORGANIZATION", ""
}

// EligibleSince returns the organization's most recent COMPLETED closure
// timestamp -- the retention clock for this class starts at closure, not
// at the representative row's own created_at.
func (e *OrgContactExecutor) EligibleSince(ctx context.Context, q database.Querier, candidate Candidate) (time.Time, error) {
	var organizationID string
	if err := q.QueryRow(ctx, `
		SELECT organization_id FROM fleet_partner_organization_authorized_representatives WHERE id = $1`,
		candidate.RecordID).Scan(&organizationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, fmt.Errorf("retentionexec: org contact %s no longer exists", candidate.RecordID)
		}
		return time.Time{}, fmt.Errorf("retentionexec: org contact resolve organization: %w", err)
	}
	var closedAt time.Time
	err := q.QueryRow(ctx, `
		SELECT completed_at FROM fleet_partner_organization_closure_requests
		WHERE organization_id = $1 AND status = 'COMPLETED' AND completed_at IS NOT NULL
		ORDER BY completed_at DESC LIMIT 1`, organizationID).Scan(&closedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// FindCandidates already required o.status = 'CLOSED'; a CLOSED
			// organization with no COMPLETED closure request row is an
			// inconsistent state this executor refuses to guess about,
			// rather than silently treating "unknown" as "long ago eligible".
			return time.Time{}, fmt.Errorf("retentionexec: organization %s is CLOSED but has no completed closure request", organizationID)
		}
		return time.Time{}, fmt.Errorf("retentionexec: org contact resolve closure time: %w", err)
	}
	return closedAt, nil
}

// Preview reports what EXECUTE would do, without writing anything.
func (e *OrgContactExecutor) Preview(ctx context.Context, q database.Querier, candidate Candidate) (string, string, error) {
	var fullName string
	if err := q.QueryRow(ctx, `
		SELECT full_name FROM fleet_partner_organization_authorized_representatives WHERE id = $1`,
		candidate.RecordID).Scan(&fullName); err != nil {
		return "", "", fmt.Errorf("retentionexec: org contact preview: %w", err)
	}
	// The detail string is operator-facing metadata (which fields would be
	// touched), never the actual PII value -- fullName is read only to
	// confirm the row still exists, never echoed.
	return OutcomeAnonymized, "would redact full_name, email and phone", nil
}

// Execute anonymizes full_name/email/phone in place. The representative row
// itself is retained (organization_id foreign key, role_title, dates,
// authority_type) -- only the CONTACT_PII fields are scrubbed, matching
// this executor's scope (anonymize, not archive/delete the row).
func (e *OrgContactExecutor) Execute(ctx context.Context, tx pgx.Tx, candidate Candidate) (string, string, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE fleet_partner_organization_authorized_representatives
		SET full_name = $2, email = $2, phone = $2, updated_at = now()
		WHERE id = $1 AND email <> $2`, candidate.RecordID, redactedPlaceholder)
	if err != nil {
		return "", "", fmt.Errorf("retentionexec: org contact execute: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", "", fmt.Errorf("retentionexec: org contact %s already anonymized or no longer exists", candidate.RecordID)
	}
	return OutcomeAnonymized, "redacted full_name, email and phone", nil
}
