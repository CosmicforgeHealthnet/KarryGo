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

// supportCaseTerminalStatuses are the only fleet_partner_support_case_
// projections.status values this executor will ever consider -- an open,
// in-progress or otherwise non-terminal case (see internal/features/
// engagement/support_quality.go's own valid-status set) is never eligible
// no matter how old its projection row is.
var supportCaseTerminalStatuses = map[string]bool{"RESOLVED": true, "CLOSED": true, "CANCELLED": true}

// supportCaseExcludedTypeFragments excludes case_type values that name a
// dispute, or overlap a category this executor has no business touching
// (safety, incident, compliance, financial, payment evidence all belong to
// their own owning features and their own, separate retention classes --
// see internal/features/safety, financeops). case_type is free text with
// no CHECK constraint (confirmed by reading migrations/000052), so this is
// a defensive substring denylist rather than an exhaustive enum match --
// deliberately erring toward excluding more than strictly necessary,
// consistent with this package's "operate only on explicitly eligible"
// framing (an unrecognized or ambiguous case_type is excluded, not
// assumed safe).
var supportCaseExcludedTypeFragments = []string{"DISPUTE", "SAFETY", "INCIDENT", "COMPLIAN", "FINANC", "PAYMENT"}

func supportCaseTypeExcluded(caseType string) bool {
	upper := toUpperASCII(caseType)
	for _, fragment := range supportCaseExcludedTypeFragments {
		if containsASCII(upper, fragment) {
			return true
		}
	}
	return false
}

// toUpperASCII/containsASCII avoid pulling in strings/unicode-aware
// casing for a small, fixed ASCII denylist check -- case_type values in
// this schema are always plain ASCII category codes (see the INSERT call
// sites in internal/features/engagement/support_quality.go), never
// user-authored free text.
func toUpperASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}
func containsASCII(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// SupportCaseAuthorityReader confirms a support case's resolution status
// against the authoritative Support Service (support-dispute-service),
// mirroring engagement.SupportPort.Read's shape without importing
// engagement's usecases package directly (this package's established
// port-isolation convention -- see ProjectionAuthorityReader's doc
// comment for the identical reasoning).
type SupportCaseAuthorityReader interface {
	// CaseStatus returns the authority's live status for
	// supportServiceCaseID. Any failed call must return an error -- "we
	// could not confirm" is never treated as "confirmed eligible".
	CaseStatus(ctx context.Context, organizationID, supportServiceCaseID string) (status string, err error)
}

// redactedSupportReference reuses the exact same placeholder
// OrgContactExecutor already established for "this field has been
// anonymized" (retentionexec/executor_org_contact.go) -- one convention
// for the whole package, not a second spelling of the same idea.
const redactedSupportReference = redactedPlaceholder

// SupportMetadataExecutor implements retention for the ClassSupport class,
// scoped to fleet_partner_support_case_projections rows that are (1)
// locally terminal and not a dispute/safety/incident/compliance/financial
// case, and (2) confirmed terminal by a live call to the authoritative
// Support Service -- Fleet's own cached status is never sufficient on its
// own, matching "never delete private support evidence unless the
// authoritative Support Service confirms eligibility" exactly.
//
// Only the three identity-linking reference fields (customer_reference,
// provider_reference, owner_reference) are ever anonymized -- the
// explicit allowlist this executor implements. support_service_case_id
// (the external Support Service's own reference, required to stay
// resolvable for any later cross-service audit), case_type, status,
// priority, trip/order/assignment references (operational correlation,
// not personal identity) and every row in fleet_partner_support_case_
// events_inbox (immutable ingestion history) are all preserved untouched.
type SupportMetadataExecutor struct {
	authority SupportCaseAuthorityReader
}

// NewSupportMetadataExecutor builds the executor. A nil authority reader
// makes FindCandidates return nothing, fail closed like every other
// external-confirmation-dependent executor in this package.
func NewSupportMetadataExecutor(authority SupportCaseAuthorityReader) *SupportMetadataExecutor {
	return &SupportMetadataExecutor{authority: authority}
}

// Class implements ClassExecutor.
func (e *SupportMetadataExecutor) Class() string { return retentionmodels.ClassSupport }

const supportMetadataRecordType = "support_case_projection"

// FindCandidates returns terminal, non-excluded support case projections
// of organizationID that are not yet anonymized, THEN confirms each one
// against the live Support Service before including it -- same
// authority-call-inside-FindCandidates shape as
// SupersededProjectionExecutor, for the same reason (eligibility itself
// requires the external confirmation, not merely a local staleness check).
func (e *SupportMetadataExecutor) FindCandidates(ctx context.Context, q database.Querier, organizationID string, limit int) ([]Candidate, error) {
	if e.authority == nil {
		return nil, nil
	}
	rows, err := q.Query(ctx, `
		SELECT id, support_service_case_id, case_type, status
		FROM fleet_partner_support_case_projections
		WHERE organization_id = $1
		  AND status IN ('RESOLVED', 'CLOSED', 'CANCELLED')
		  AND customer_reference <> $2
		  AND provider_reference <> $2
		  AND owner_reference <> $2
		ORDER BY id
		LIMIT $3`, organizationID, redactedSupportReference, limit)
	if err != nil {
		return nil, fmt.Errorf("retentionexec: support metadata find candidates: %w", err)
	}
	type row struct{ id, caseID, caseType, status string }
	var pending []row
	for rows.Next() {
		var r row
		if scanErr := rows.Scan(&r.id, &r.caseID, &r.caseType, &r.status); scanErr != nil {
			rows.Close()
			return nil, fmt.Errorf("retentionexec: support metadata scan candidate: %w", scanErr)
		}
		pending = append(pending, r)
	}
	closeErr := rows.Err()
	rows.Close()
	if closeErr != nil {
		return nil, closeErr
	}

	var candidates []Candidate
	for _, r := range pending {
		if !supportCaseTerminalStatuses[r.status] || supportCaseTypeExcluded(r.caseType) {
			continue
		}
		authoritative, err := e.authority.CaseStatus(ctx, organizationID, r.caseID)
		if err != nil || !supportCaseTerminalStatuses[authoritative] {
			// Could not confirm, or the authority disagrees (case reopened
			// since fleet's projection last synced) -- not a candidate.
			// Fleet's local cache is never sufficient on its own.
			continue
		}
		candidates = append(candidates, Candidate{RecordType: supportMetadataRecordType, RecordID: r.id})
	}
	return candidates, nil
}

// ScopeForLegalHold checks an ORGANIZATION-scoped hold -- there is no
// per-case hold concept in this schema's support projection today, and an
// org-wide hold is the correct, conservative veto.
func (e *SupportMetadataExecutor) ScopeForLegalHold(candidate Candidate) (scopeType, scopeReference string) {
	return "ORGANIZATION", ""
}

// EligibleSince returns the projection's own last update time -- the
// retention clock for support metadata runs from when the case was last
// synced (which, for a terminal case, is effectively when it settled into
// its terminal state), not from the case's original creation.
func (e *SupportMetadataExecutor) EligibleSince(ctx context.Context, q database.Querier, candidate Candidate) (time.Time, error) {
	var updatedAt time.Time
	if err := q.QueryRow(ctx, `SELECT updated_at FROM fleet_partner_support_case_projections WHERE id = $1`,
		candidate.RecordID).Scan(&updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, fmt.Errorf("retentionexec: support case projection %s no longer exists", candidate.RecordID)
		}
		return time.Time{}, fmt.Errorf("retentionexec: support metadata resolve updated_at: %w", err)
	}
	return updatedAt, nil
}

// Preview reports what EXECUTE would do, without writing anything and
// without a second authority call (FindCandidates already confirmed
// eligibility for this batch).
func (e *SupportMetadataExecutor) Preview(ctx context.Context, q database.Querier, candidate Candidate) (string, string, error) {
	var exists bool
	if err := q.QueryRow(ctx, `SELECT true FROM fleet_partner_support_case_projections WHERE id = $1`,
		candidate.RecordID).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", fmt.Errorf("retentionexec: support case projection %s no longer exists", candidate.RecordID)
		}
		return "", "", fmt.Errorf("retentionexec: support metadata preview: %w", err)
	}
	return OutcomeAnonymized, "would redact customer_reference, provider_reference and owner_reference; support_service_case_id and case history are preserved", nil
}

// Execute anonymizes exactly the three allowlisted identity-linking
// fields. support_service_case_id, case_type, status, priority and every
// operational correlation reference are untouched, as is every row in
// fleet_partner_support_case_events_inbox (immutable ingestion history).
func (e *SupportMetadataExecutor) Execute(ctx context.Context, tx pgx.Tx, candidate Candidate) (string, string, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE fleet_partner_support_case_projections
		SET customer_reference = $2, provider_reference = $2, owner_reference = $2, updated_at = now()
		WHERE id = $1
		  AND (customer_reference <> $2 OR provider_reference <> $2 OR owner_reference <> $2)`,
		candidate.RecordID, redactedSupportReference)
	if err != nil {
		return "", "", fmt.Errorf("retentionexec: support metadata execute: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", "", fmt.Errorf("retentionexec: support case projection %s already anonymized or no longer exists", candidate.RecordID)
	}
	return OutcomeAnonymized, "redacted customer_reference, provider_reference and owner_reference", nil
}
