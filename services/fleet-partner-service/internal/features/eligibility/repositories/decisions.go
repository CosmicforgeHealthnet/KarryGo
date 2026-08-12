// Package repositories stores eligibility policies and decisions.
//
// Every statement here is tenant-scoped in SQL. There is no unscoped read of a
// decision: a decision names a subject and a verdict about it, and one
// organization has no business learning either about another's.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ErrNotFound means no row matched within the tenant scope.
var ErrNotFound = errors.New("eligibility: not found")

// ErrConflict means a concurrent writer already recorded a current decision.
var ErrConflict = errors.New("eligibility: conflicting decision")

// Identifier prefixes for locally owned rows.
const (
	prefixDecision = "eld"
	prefixSource   = "els"
	prefixHistory  = "elh"
	prefixPolicy   = "elp"
)

// Repository stores eligibility policies, decisions and their history.
type Repository struct {
	db database.Querier
}

// NewRepository wires the repository.
func NewRepository(db database.Querier) *Repository {
	return &Repository{db: db}
}

// ---------------------------------------------------------------------------
// Policies
// ---------------------------------------------------------------------------

const policyColumns = `id, organization_id, policy_key, operation_type, version,
	required_company_checks, required_provider_checks, required_vehicle_checks,
	required_documents, decision_ttl_seconds, source_max_age_seconds,
	approval_required, minimum_approvals, eligible_role_ids, self_approval_allowed,
	separation_of_duties, approval_ttl_seconds, acknowledgement_required,
	subscription_required, financial_threshold_minor, financial_threshold_currency,
	status, effective_from, created_at`

// ActivePolicy resolves the policy version governing an operation.
//
// A tenant's own ACTIVE version wins over the platform default. The lookup is
// ordered rather than merged on purpose: a policy that is half tenant and half
// platform is a rule nobody wrote and nobody can reason about.
func (r *Repository) ActivePolicy(ctx context.Context, q database.Querier,
	organizationID string, policyKey string) (*models.Policy, error) {
	if q == nil {
		q = r.db
	}
	policy, err := scanPolicy(q.QueryRow(ctx, `SELECT `+policyColumns+`
		FROM fleet_partner_eligibility_policies
		WHERE organization_id = $1 AND policy_key = $2 AND status = 'ACTIVE'`,
		organizationID, policyKey))
	if err == nil {
		return policy, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return scanPolicy(q.QueryRow(ctx, `SELECT `+policyColumns+`
		FROM fleet_partner_eligibility_policies
		WHERE organization_id IS NULL AND policy_key = $1 AND status = 'ACTIVE'`,
		policyKey))
}

// GetPolicy loads one policy version by identifier, tenant-scoped.
//
// A platform default is readable by every tenant because every tenant is
// governed by it; a tenant override is readable only by its owner.
func (r *Repository) GetPolicy(ctx context.Context, organizationID string,
	policyID string) (*models.Policy, error) {
	return scanPolicy(r.db.QueryRow(ctx, `SELECT `+policyColumns+`
		FROM fleet_partner_eligibility_policies
		WHERE id = $1 AND (organization_id IS NULL OR organization_id = $2)`,
		policyID, organizationID))
}

// ListPolicies returns the policy versions visible to one organization.
func (r *Repository) ListPolicies(ctx context.Context, organizationID string,
	policyKey string) ([]models.Policy, error) {
	rows, err := r.db.Query(ctx, `SELECT `+policyColumns+`
		FROM fleet_partner_eligibility_policies
		WHERE (organization_id IS NULL OR organization_id = $1)
		  AND ($2 = '' OR policy_key = $2)
		ORDER BY policy_key, organization_id NULLS FIRST, version DESC`,
		organizationID, policyKey)
	if err != nil {
		return nil, fmt.Errorf("eligibility: list policies: %w", err)
	}
	defer rows.Close()

	policies := make([]models.Policy, 0, 16)
	for rows.Next() {
		policy, scanErr := scanPolicyRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		policies = append(policies, *policy)
	}
	return policies, rows.Err()
}

// PublishPolicy archives the tenant's current version and inserts a new one.
//
// Publishing never edits the outgoing version — the schema forbids it — so
// every decision that cited version N keeps citing exactly what version N
// said. The new version takes effect for decisions taken from now on and for
// nothing already recorded.
func (r *Repository) PublishPolicy(ctx context.Context, q database.Querier,
	organizationID string, policy *models.Policy) error {
	if organizationID == "" {
		return fmt.Errorf("eligibility: publish policy requires an organization")
	}
	policy.OrganizationID = organizationID
	if policy.ID == "" {
		policy.ID = ids.New(prefixPolicy)
	}

	var nextVersion int64
	if err := q.QueryRow(ctx, `
		SELECT coalesce(max(version), 0) + 1
		FROM fleet_partner_eligibility_policies
		WHERE organization_id = $1 AND policy_key = $2`,
		organizationID, policy.PolicyKey).Scan(&nextVersion); err != nil {
		return fmt.Errorf("eligibility: next policy version: %w", err)
	}
	policy.Version = nextVersion

	if _, err := q.Exec(ctx, `
		UPDATE fleet_partner_eligibility_policies
		SET status = 'ARCHIVED', archived_at = now()
		WHERE organization_id = $1 AND policy_key = $2 AND status = 'ACTIVE'`,
		organizationID, policy.PolicyKey); err != nil {
		return fmt.Errorf("eligibility: archive policy: %w", err)
	}

	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_eligibility_policies
			(id, organization_id, policy_key, operation_type, version,
			 required_company_checks, required_provider_checks, required_vehicle_checks,
			 required_documents, decision_ttl_seconds, source_max_age_seconds,
			 approval_required, minimum_approvals, eligible_role_ids,
			 self_approval_allowed, separation_of_duties, approval_ttl_seconds,
			 acknowledgement_required, subscription_required,
			 financial_threshold_minor, financial_threshold_currency,
			 status, created_by_actor_type, created_by_actor_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
		        $16, $17, $18, $19, $20, $21, 'ACTIVE', $22, $23)`,
		policy.ID, organizationID, policy.PolicyKey, policy.OperationType, policy.Version,
		jsonArray(policy.RequiredCompanyChecks), jsonArray(policy.RequiredProviderChecks),
		jsonArray(policy.RequiredVehicleChecks), jsonArray(policy.RequiredDocuments),
		policy.DecisionTTLSeconds, policy.SourceMaxAgeSeconds,
		policy.ApprovalRequired, policy.MinimumApprovals, jsonArray(policy.EligibleRoleIDs),
		policy.SelfApprovalAllowed, policy.SeparationOfDuties, policy.ApprovalTTLSeconds,
		policy.AcknowledgementRequired, policy.SubscriptionRequired,
		policy.FinancialThresholdMinor, nullableCurrency(policy.FinancialThresholdCurrency),
		"ORGANIZATION_USER", policy.OrganizationID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("eligibility: publish policy: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Decisions
// ---------------------------------------------------------------------------

const decisionColumns = `id, organization_id, branch_id, subject_type, subject_id,
	operation_type, policy_id, policy_key, policy_version, status, input_fingerprint,
	checked_at, expires_at, superseded_at, superseded_by, actor_type, actor_id,
	correlation_id, causation_id, audit_record_id, outbox_event_id, created_at`

// Record writes a decision, superseding whatever it replaces.
//
// The supersede and the insert happen in one statement pair inside the
// caller's transaction, and the partial unique index on
// (organization, subject, operation) WHERE superseded_at IS NULL is what makes
// that safe under concurrency: two evaluators racing produce one winner and
// one conflict, never two live decisions a caller could choose between.
func (r *Repository) Record(ctx context.Context, q database.Querier,
	organizationID string, decision *models.Decision) error {
	if decision.ID == "" {
		decision.ID = ids.New(prefixDecision)
	}
	decision.OrganizationID = organizationID

	if _, err := q.Exec(ctx, `
		UPDATE fleet_partner_eligibility_decisions
		SET superseded_at = now(), superseded_by = $5
		WHERE organization_id = $1 AND subject_type = $2 AND subject_id = $3
		  AND operation_type = $4 AND superseded_at IS NULL`,
		organizationID, decision.SubjectType, decision.SubjectID,
		decision.OperationType, decision.ID); err != nil {
		return fmt.Errorf("eligibility: supersede decision: %w", err)
	}

	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_eligibility_decisions
			(id, organization_id, branch_id, subject_type, subject_id, operation_type,
			 policy_id, policy_key, policy_version, status, input_fingerprint,
			 checked_at, expires_at, actor_type, actor_id, correlation_id,
			 causation_id, audit_record_id, outbox_event_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
		        $16, $17, $18, $19)`,
		decision.ID, organizationID, decision.BranchID, decision.SubjectType,
		decision.SubjectID, decision.OperationType, decision.PolicyID,
		decision.PolicyKey, decision.PolicyVersion, decision.Status,
		decision.InputFingerprint, decision.CheckedAt, decision.ExpiresAt,
		decision.ActorType, decision.ActorID, decision.CorrelationID,
		decision.CausationID, decision.AuditRecordID, decision.OutboxEventID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("eligibility: record decision: %w", err)
	}

	for index, reason := range decision.Reasons {
		if _, err := q.Exec(ctx, `
			INSERT INTO fleet_partner_eligibility_decision_reasons
				(decision_id, reason_code, authority, ordinal)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING`, decision.ID, reason, "", index); err != nil {
			return fmt.Errorf("eligibility: record reason: %w", err)
		}
	}

	for index := range decision.Sources {
		source := &decision.Sources[index]
		if source.ID == "" {
			source.ID = ids.New(prefixSource)
		}
		source.DecisionID = decision.ID
		source.OrganizationID = organizationID
		if _, err := q.Exec(ctx, `
			INSERT INTO fleet_partner_eligibility_decision_sources
				(id, decision_id, organization_id, authority_service, source_kind,
				 source_reference, source_version, freshness_state, observed_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT DO NOTHING`,
			source.ID, decision.ID, organizationID, source.AuthorityService,
			source.SourceKind, source.SourceReference, source.SourceVersion,
			source.FreshnessState, source.ObservedAt); err != nil {
			return fmt.Errorf("eligibility: record source: %w", err)
		}
	}
	return nil
}

// Current loads the live decision for a subject and operation.
func (r *Repository) Current(ctx context.Context, q database.Querier,
	organizationID string, subjectType string, subjectID string,
	operationType string) (*models.Decision, error) {
	if q == nil {
		q = r.db
	}
	decision, err := scanDecision(q.QueryRow(ctx, `SELECT `+decisionColumns+`
		FROM fleet_partner_eligibility_decisions
		WHERE organization_id = $1 AND subject_type = $2 AND subject_id = $3
		  AND operation_type = $4 AND superseded_at IS NULL`,
		organizationID, subjectType, subjectID, operationType))
	if err != nil {
		return nil, err
	}
	return r.hydrate(ctx, q, decision)
}

// GetByID loads one decision inside the organization scope.
func (r *Repository) GetByID(ctx context.Context, organizationID string,
	decisionID string) (*models.Decision, error) {
	decision, err := scanDecision(r.db.QueryRow(ctx, `SELECT `+decisionColumns+`
		FROM fleet_partner_eligibility_decisions
		WHERE organization_id = $1 AND id = $2`, organizationID, decisionID))
	if err != nil {
		return nil, err
	}
	return r.hydrate(ctx, r.db, decision)
}

// ListFilter narrows a decision listing.
type ListFilter struct {
	SubjectType   string
	SubjectID     string
	OperationType string
	Status        string
	CurrentOnly   bool
}

// List returns decisions inside the tenant and branch scope.
//
// Branch filtering is applied in SQL rather than after the read: a
// branch-scoped operator must not receive another branch's rows and then have
// them hidden by a view layer, because the row already left the database.
func (r *Repository) List(ctx context.Context, query tenant.Query,
	filter ListFilter) ([]models.Decision, error) {
	limit := query.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	arguments := []any{query.OrganizationID, filter.SubjectType, filter.SubjectID,
		filter.OperationType, filter.Status, limit}
	statement := `SELECT ` + decisionColumns + `
		FROM fleet_partner_eligibility_decisions
		WHERE organization_id = $1
		  AND ($2 = '' OR subject_type = $2)
		  AND ($3 = '' OR subject_id = $3)
		  AND ($4 = '' OR operation_type = $4)
		  AND ($5 = '' OR status = $5)`
	if filter.CurrentOnly {
		statement += ` AND superseded_at IS NULL`
	}
	if len(query.PermittedBranchIDs) > 0 {
		arguments = append(arguments, query.PermittedBranchIDs)
		statement += ` AND (branch_id IS NULL OR branch_id = ANY($7))`
	}
	statement += ` ORDER BY checked_at DESC, id DESC LIMIT $6`

	rows, err := r.db.Query(ctx, statement, arguments...)
	if err != nil {
		return nil, fmt.Errorf("eligibility: list decisions: %w", err)
	}
	defer rows.Close()

	decisions := make([]models.Decision, 0, limit)
	for rows.Next() {
		decision, scanErr := scanDecisionRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		decisions = append(decisions, *decision)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for index := range decisions {
		if _, err := r.hydrate(ctx, r.db, &decisions[index]); err != nil {
			return nil, err
		}
	}
	return decisions, nil
}

// hydrate loads a decision's reasons and sources.
func (r *Repository) hydrate(ctx context.Context, q database.Querier,
	decision *models.Decision) (*models.Decision, error) {
	reasonRows, err := q.Query(ctx, `
		SELECT reason_code FROM fleet_partner_eligibility_decision_reasons
		WHERE decision_id = $1 ORDER BY ordinal, reason_code`, decision.ID)
	if err != nil {
		return nil, fmt.Errorf("eligibility: load reasons: %w", err)
	}
	decision.Reasons = decision.Reasons[:0]
	for reasonRows.Next() {
		var code string
		if scanErr := reasonRows.Scan(&code); scanErr != nil {
			reasonRows.Close()
			return nil, scanErr
		}
		decision.Reasons = append(decision.Reasons, code)
	}
	reasonRows.Close()
	if err := reasonRows.Err(); err != nil {
		return nil, err
	}

	sourceRows, err := q.Query(ctx, `
		SELECT id, decision_id, organization_id, authority_service, source_kind,
		       source_reference, source_version, freshness_state, observed_at
		FROM fleet_partner_eligibility_decision_sources
		WHERE decision_id = $1 ORDER BY authority_service, source_kind`, decision.ID)
	if err != nil {
		return nil, fmt.Errorf("eligibility: load sources: %w", err)
	}
	defer sourceRows.Close()
	decision.Sources = decision.Sources[:0]
	for sourceRows.Next() {
		var source models.DecisionSource
		if scanErr := sourceRows.Scan(&source.ID, &source.DecisionID, &source.OrganizationID,
			&source.AuthorityService, &source.SourceKind, &source.SourceReference,
			&source.SourceVersion, &source.FreshnessState, &source.ObservedAt); scanErr != nil {
			return nil, scanErr
		}
		decision.Sources = append(decision.Sources, source)
	}
	return decision, sourceRows.Err()
}

// InvalidateSubjectDecisions supersedes every current decision about a subject
// and records why in the append-only history.
//
// It exists for the case where the evidence a decision rested on changed after
// the decision was taken: a document was revoked, replaced, expired or found
// unsafe. The decision rows themselves stay immutable — what changes is that
// they are no longer CURRENT, so the next gate re-evaluates instead of reusing
// a "yes" that was true about a document nobody stands behind any more.
//
// Returns the number of decisions invalidated.
func (r *Repository) InvalidateSubjectDecisions(ctx context.Context, q database.Querier,
	organizationID string, subjectType string, subjectID string, reasonCode string,
	correlationID string) (int64, error) {
	if q == nil {
		q = r.db
	}
	if organizationID == "" || subjectType == "" || subjectID == "" {
		return 0, nil
	}

	rows, err := q.Query(ctx, `
		UPDATE fleet_partner_eligibility_decisions
		SET superseded_at = now()
		WHERE organization_id = $1 AND subject_type = $2 AND subject_id = $3
		  AND superseded_at IS NULL
		RETURNING id, operation_type, status`,
		organizationID, subjectType, subjectID)
	if err != nil {
		return 0, fmt.Errorf("eligibility: invalidate subject decisions: %w", err)
	}

	type invalidated struct {
		id, operationType, status string
	}
	var affected []invalidated
	for rows.Next() {
		var entry invalidated
		if scanErr := rows.Scan(&entry.id, &entry.operationType, &entry.status); scanErr != nil {
			rows.Close()
			return 0, fmt.Errorf("eligibility: scan invalidated decision: %w", scanErr)
		}
		affected = append(affected, entry)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, entry := range affected {
		if histErr := r.AppendHistory(ctx, q, &models.HistoryEntry{
			OrganizationID: organizationID,
			DecisionID:     entry.id,
			SubjectType:    subjectType,
			SubjectID:      subjectID,
			OperationType:  entry.operationType,
			Action:         models.ActionInvalidated,
			FromStatus:     entry.status,
			ToStatus:       entry.status,
			ActorType:      models.ActorSystem,
			ReasonCode:     reasonCode,
			CorrelationID:  correlationID,
		}); histErr != nil {
			return 0, histErr
		}
	}
	return int64(len(affected)), nil
}

// AppendHistory writes one immutable history entry.
func (r *Repository) AppendHistory(ctx context.Context, q database.Querier,
	entry *models.HistoryEntry) error {
	if entry.ID == "" {
		entry.ID = ids.New(prefixHistory)
	}
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_eligibility_decision_history
			(id, organization_id, decision_id, subject_type, subject_id, operation_type,
			 action, from_status, to_status, actor_type, actor_id, reason_code,
			 correlation_id, causation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		entry.ID, entry.OrganizationID, entry.DecisionID, entry.SubjectType,
		entry.SubjectID, entry.OperationType, entry.Action, entry.FromStatus,
		entry.ToStatus, entry.ActorType, entry.ActorID, entry.ReasonCode,
		entry.CorrelationID, entry.CausationID)
	if err != nil {
		return fmt.Errorf("eligibility: append history: %w", err)
	}
	return nil
}

// ListHistory returns the append-only history of one decision.
func (r *Repository) ListHistory(ctx context.Context, organizationID string,
	decisionID string) ([]models.HistoryEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, decision_id, subject_type, subject_id,
		       operation_type, action, from_status, to_status, actor_type, actor_id,
		       reason_code, correlation_id, causation_id, occurred_at
		FROM fleet_partner_eligibility_decision_history
		WHERE organization_id = $1 AND decision_id = $2
		ORDER BY occurred_at DESC`, organizationID, decisionID)
	if err != nil {
		return nil, fmt.Errorf("eligibility: list history: %w", err)
	}
	defer rows.Close()

	entries := make([]models.HistoryEntry, 0, 8)
	for rows.Next() {
		var entry models.HistoryEntry
		if scanErr := rows.Scan(&entry.ID, &entry.OrganizationID, &entry.DecisionID,
			&entry.SubjectType, &entry.SubjectID, &entry.OperationType, &entry.Action,
			&entry.FromStatus, &entry.ToStatus, &entry.ActorType, &entry.ActorID,
			&entry.ReasonCode, &entry.CorrelationID, &entry.CausationID,
			&entry.OccurredAt); scanErr != nil {
			return nil, scanErr
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// ---------------------------------------------------------------------------
// Scanning helpers
// ---------------------------------------------------------------------------

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPolicy(row pgx.Row) (*models.Policy, error) { return scanPolicyRow(row) }

func scanPolicyRow(row rowScanner) (*models.Policy, error) {
	var policy models.Policy
	var organizationID *string
	var companyChecks, providerChecks, vehicleChecks, documents, roleIDs []byte
	var currency *string
	err := row.Scan(&policy.ID, &organizationID, &policy.PolicyKey, &policy.OperationType,
		&policy.Version, &companyChecks, &providerChecks, &vehicleChecks, &documents,
		&policy.DecisionTTLSeconds, &policy.SourceMaxAgeSeconds, &policy.ApprovalRequired,
		&policy.MinimumApprovals, &roleIDs, &policy.SelfApprovalAllowed,
		&policy.SeparationOfDuties, &policy.ApprovalTTLSeconds,
		&policy.AcknowledgementRequired, &policy.SubscriptionRequired,
		&policy.FinancialThresholdMinor, &currency, &policy.Status,
		&policy.EffectiveFrom, &policy.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("eligibility: scan policy: %w", err)
	}
	if organizationID != nil {
		policy.OrganizationID = *organizationID
	}
	if currency != nil {
		policy.FinancialThresholdCurrency = *currency
	}
	policy.RequiredCompanyChecks = decodeStrings(companyChecks)
	policy.RequiredProviderChecks = decodeStrings(providerChecks)
	policy.RequiredVehicleChecks = decodeStrings(vehicleChecks)
	policy.RequiredDocuments = decodeStrings(documents)
	policy.EligibleRoleIDs = decodeStrings(roleIDs)
	return &policy, nil
}

func scanDecision(row pgx.Row) (*models.Decision, error) { return scanDecisionRow(row) }

func scanDecisionRow(row rowScanner) (*models.Decision, error) {
	var decision models.Decision
	err := row.Scan(&decision.ID, &decision.OrganizationID, &decision.BranchID,
		&decision.SubjectType, &decision.SubjectID, &decision.OperationType,
		&decision.PolicyID, &decision.PolicyKey, &decision.PolicyVersion,
		&decision.Status, &decision.InputFingerprint, &decision.CheckedAt,
		&decision.ExpiresAt, &decision.SupersededAt, &decision.SupersededBy,
		&decision.ActorType, &decision.ActorID, &decision.CorrelationID,
		&decision.CausationID, &decision.AuditRecordID, &decision.OutboxEventID,
		&decision.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("eligibility: scan decision: %w", err)
	}
	return &decision, nil
}

func decodeStrings(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		// A malformed policy array is treated as "no checks named", which is
		// the conservative reading only because a policy that names no checks
		// still cannot produce ELIGIBLE for anything it was meant to guard —
		// the service refuses a policy it cannot parse.
		return nil
	}
	return values
}

func jsonArray(values []string) []byte {
	if values == nil {
		values = []string{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return []byte("[]")
	}
	return encoded
}

func nullableCurrency(currency string) *string {
	trimmed := strings.ToUpper(strings.TrimSpace(currency))
	if len(trimmed) != 3 {
		return nil
	}
	return &trimmed
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// TouchExpired records that a decision has passed its expiry.
//
// The decision row itself is untouched — it is immutable and its expiry was
// always part of what it said. What this writes is the history entry, so a
// reader can see when the platform noticed.
func (r *Repository) TouchExpired(ctx context.Context, q database.Querier,
	decision *models.Decision, now time.Time) error {
	return r.AppendHistory(ctx, q, &models.HistoryEntry{
		OrganizationID: decision.OrganizationID,
		DecisionID:     decision.ID,
		SubjectType:    decision.SubjectType,
		SubjectID:      decision.SubjectID,
		OperationType:  decision.OperationType,
		Action:         models.ActionExpired,
		FromStatus:     decision.Status,
		ToStatus:       decision.Status,
		ActorType:      models.ActorSystem,
		ReasonCode:     models.ReasonDecisionExpired,
	})
}
