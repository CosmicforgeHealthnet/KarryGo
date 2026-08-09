package retentionexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	retentionmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/models"
)

// clearedProjectionState is the placeholder safe_state carries once its
// cached payload has been archived -- distinct from '{}' (a projection can
// legitimately arrive with an empty safe_state from a real event) so
// FindCandidates can tell "genuinely never populated" from "was populated,
// now cleared by retention" if that distinction ever matters later; today
// it only needs SOME stable, recognizable value to filter on, matching
// OrgContactExecutor's redactedPlaceholder convention.
const clearedProjectionState = `{"retentionArchived":true}`

// ProjectionAuthorityReader confirms, against the SAME external source
// synchronization.Service's own Authority port would ask (see
// internal/features/synchronization/service.go's Authority interface),
// that a cached projection is genuinely superseded -- the authoritative
// service has moved to a source version newer than what fleet's cache
// still holds. This executor defines its own narrow copy of that shape
// rather than importing synchronization's usecases package directly
// (internal/architecture_test.go's port/adapter convention, the same
// reason model.go defines its own LegalHoldChecker/
// EffectiveRetentionDaysReader instead of importing legalhold/
// retentionpolicy usecases).
type ProjectionAuthorityReader interface {
	// CurrentSourceVersion returns the authority's live source_version for
	// (sourceService, projectionType, subjectReference). A caller with no
	// adapter for sourceService, or any failed call, must return an error
	// -- "we could not confirm" is never treated as "confirmed superseded".
	CurrentSourceVersion(ctx context.Context, organizationID, sourceService, projectionType, subjectReference string) (int64, error)
}

// SupersededProjectionExecutor implements retention for the ClassTemporary
// class, scoped to fleet_partner_synchronization_projections rows whose
// cached safe_state payload is confirmed -- by a live call to the SAME
// authority the synchronization feature itself reconciles against, never
// by local staleness alone -- to no longer reflect the source's current
// version.
//
// Only the payload (safe_state) is ever cleared; the projection ROW
// itself, and every row in fleet_partner_synchronization_cases/_attempts/
// _dead_letters, are untouched. This is deliberate, not incidental: the
// projection's bookkeeping columns (id, source_version AT THE TIME this
// cache was superseded, gap_conflict_state, timestamps) are what proves a
// reconciliation history occurred; the payload is the only part of the row
// that exists purely to mirror another service's current-ish state, and is
// therefore the only part this executor is scoped to touch.
type SupersededProjectionExecutor struct {
	authority ProjectionAuthorityReader
}

// NewSupersededProjectionExecutor builds the executor. A nil authority
// reader makes FindCandidates return nothing -- "requires a newer
// authoritative source version" cannot be satisfied without asking the
// authority, so an unconfigured adapter must never let a projection
// through by default.
func NewSupersededProjectionExecutor(authority ProjectionAuthorityReader) *SupersededProjectionExecutor {
	return &SupersededProjectionExecutor{authority: authority}
}

// Class implements ClassExecutor.
//
// ClassTemporary would have been the more literal semantic fit ("cached,
// superseded, non-authoritative data" reads a lot like "temporary"), but
// ExpiredExportExecutor already claims it, and Runner registers at most
// one executor per class (model.go's ClassExecutor doc comment, "a class
// with no registered ClassExecutor can never be executed" -- the
// converse, silently true from the map-keyed registry in runner.go's
// NewRunner, is that a SECOND executor for an already-claimed class would
// silently overwrite the first rather than erroring, so this is a real
// constraint, not a preference). ClassCompanyConfidential is the next best
// fit among the classes retentionpolicy/models.go actually defines:
// cross-service synchronization state is internal platform data with no
// external customer/provider/financial shape of its own.
func (e *SupersededProjectionExecutor) Class() string {
	return retentionmodels.ClassCompanyConfidential
}

const supersededProjectionRecordType = "synchronization_projection"

// FindCandidates returns projections of organizationID that are currently
// settled (reconciliation_status='IDLE', gap_conflict_state='CURRENT' --
// no open reconciliation work in flight, per "operate only on
// non-authoritative cached projections" excluding anything still being
// actively reconciled) and not yet archived, THEN confirms each one
// against the live authority before including it -- a bounded number of
// authority calls per pass (at most limit), the same "bounded batches"
// discipline every other executor gets from Runner's own batchSize, just
// applied one layer earlier here because determining eligibility itself
// requires the external call.
func (e *SupersededProjectionExecutor) FindCandidates(ctx context.Context, q database.Querier, organizationID string, limit int) ([]Candidate, error) {
	if e.authority == nil {
		return nil, nil
	}
	rows, err := q.Query(ctx, `
		SELECT id, source_service, projection_type, subject_reference, source_version
		FROM fleet_partner_synchronization_projections
		WHERE organization_id = $1
		  AND reconciliation_status = 'IDLE'
		  AND gap_conflict_state = 'CURRENT'
		  AND safe_state <> $2::jsonb
		ORDER BY id
		LIMIT $3`, organizationID, clearedProjectionState, limit)
	if err != nil {
		return nil, fmt.Errorf("retentionexec: superseded projection find candidates: %w", err)
	}
	type row struct {
		id, sourceService, projectionType, subjectReference string
		sourceVersion                                       int64
	}
	var pending []row
	for rows.Next() {
		var r row
		if scanErr := rows.Scan(&r.id, &r.sourceService, &r.projectionType, &r.subjectReference, &r.sourceVersion); scanErr != nil {
			rows.Close()
			return nil, fmt.Errorf("retentionexec: superseded projection scan candidate: %w", scanErr)
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
		authoritative, err := e.authority.CurrentSourceVersion(ctx, organizationID, r.sourceService, r.projectionType, r.subjectReference)
		if err != nil {
			// Could not confirm against the authority -- not a candidate
			// this pass. A transient authority outage must never widen
			// eligibility; it can only ever narrow it.
			continue
		}
		if authoritative > r.sourceVersion {
			candidates = append(candidates, Candidate{RecordType: supersededProjectionRecordType, RecordID: r.id})
		}
	}
	return candidates, nil
}

// ScopeForLegalHold checks an ORGANIZATION-scoped hold -- there is no
// finer-grained hold concept for synchronization projections (confirmed by
// reading legalhold's ValidScopeTypes), and an org-wide hold is the
// correct, conservative veto for any of that organization's cached
// cross-service state.
func (e *SupersededProjectionExecutor) ScopeForLegalHold(candidate Candidate) (scopeType, scopeReference string) {
	return "ORGANIZATION", ""
}

// EligibleSince returns the projection's own last update time -- the
// retention clock for "how long may a superseded cache payload linger"
// runs from when this cache was last legitimately refreshed, not from
// when the row was first created (an old projection_id that has been
// faithfully kept current until very recently must not be treated as
// though its payload had been stale for the row's entire lifetime).
func (e *SupersededProjectionExecutor) EligibleSince(ctx context.Context, q database.Querier, candidate Candidate) (time.Time, error) {
	var updatedAt time.Time
	if err := q.QueryRow(ctx, `SELECT updated_at FROM fleet_partner_synchronization_projections WHERE id = $1`,
		candidate.RecordID).Scan(&updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, fmt.Errorf("retentionexec: synchronization projection %s no longer exists", candidate.RecordID)
		}
		return time.Time{}, fmt.Errorf("retentionexec: superseded projection resolve updated_at: %w", err)
	}
	return updatedAt, nil
}

// Preview reports what EXECUTE would do, without writing anything and
// without calling the authority again (FindCandidates already confirmed
// eligibility for this batch; Preview trusts that same confirmation rather
// than spending a second external call per candidate on every DRY_RUN).
func (e *SupersededProjectionExecutor) Preview(ctx context.Context, q database.Querier, candidate Candidate) (string, string, error) {
	var exists bool
	if err := q.QueryRow(ctx, `SELECT true FROM fleet_partner_synchronization_projections WHERE id = $1`,
		candidate.RecordID).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", fmt.Errorf("retentionexec: synchronization projection %s no longer exists", candidate.RecordID)
		}
		return "", "", fmt.Errorf("retentionexec: superseded projection preview: %w", err)
	}
	return OutcomeArchived, "would clear the cached payload (safe_state); reconciliation history and the projection row itself are preserved", nil
}

// Execute clears the cached payload only. The projection row, and every
// row in fleet_partner_synchronization_cases/_attempts/_dead_letters,
// remain exactly as they were.
func (e *SupersededProjectionExecutor) Execute(ctx context.Context, tx pgx.Tx, candidate Candidate) (string, string, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE fleet_partner_synchronization_projections
		SET safe_state = $2::jsonb, updated_at = now()
		WHERE id = $1 AND safe_state <> $2::jsonb`,
		candidate.RecordID, json.RawMessage(clearedProjectionState))
	if err != nil {
		return "", "", fmt.Errorf("retentionexec: superseded projection execute: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", "", fmt.Errorf("retentionexec: synchronization projection %s already archived or no longer exists", candidate.RecordID)
	}
	return OutcomeArchived, "cleared the cached payload; the row, its source_version and reconciliation history are preserved", nil
}
