// Package repositories persists pricing policies, their versions and activations.
package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/pricing-service/internal/database"
	"cosmicforge/logistics/services/pricing-service/internal/features/policies/models"
)

// Store reads and writes policies.
type Store struct{ db database.Querier }

// NewStore returns a store over db.
func NewStore(db database.Querier) *Store { return &Store{db: db} }

// WithQuerier binds the store to a transaction so a change, its lifecycle event
// and its audit row commit together.
func (s *Store) WithQuerier(q database.Querier) *Store { return &Store{db: q} }

// EnsurePolicy finds or creates the lineage for a kind and place.
func (s *Store) EnsurePolicy(ctx context.Context, policy models.Policy) (models.Policy, error) {
	var stored models.Policy
	err := s.db.QueryRow(ctx, `
		INSERT INTO pricing_policies (id, kind, geography_level, geography_ref, created_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (kind, geography_level, geography_ref) DO UPDATE
			SET kind = pricing_policies.kind
		RETURNING id, kind, geography_level, geography_ref, created_by, created_at`,
		policy.ID, policy.Kind, policy.GeographyLevel, policy.GeographyRef, policy.CreatedBy,
	).Scan(&stored.ID, &stored.Kind, &stored.GeographyLevel, &stored.GeographyRef,
		&stored.CreatedBy, &stored.CreatedAt)
	if err != nil {
		return models.Policy{}, fmt.Errorf("policies: ensure lineage: %w", err)
	}
	return stored, nil
}

// GetPolicy returns one lineage.
func (s *Store) GetPolicy(ctx context.Context, id string) (models.Policy, error) {
	var policy models.Policy
	err := s.db.QueryRow(ctx, `
		SELECT id, kind, geography_level, geography_ref, created_by, created_at
		FROM pricing_policies WHERE id = $1`, id,
	).Scan(&policy.ID, &policy.Kind, &policy.GeographyLevel, &policy.GeographyRef,
		&policy.CreatedBy, &policy.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Policy{}, fmt.Errorf("%w: policy %s", models.ErrNotFound, id)
	}
	if err != nil {
		return models.Policy{}, fmt.Errorf("policies: get lineage: %w", err)
	}
	return policy, nil
}

// NextVersionNo returns the next edition number for a lineage.
func (s *Store) NextVersionNo(ctx context.Context, policyID string) (int, error) {
	var next int
	if err := s.db.QueryRow(ctx,
		`SELECT COALESCE(MAX(version_no), 0) + 1 FROM pricing_policy_versions WHERE policy_id = $1`, policyID,
	).Scan(&next); err != nil {
		return 0, fmt.Errorf("policies: next version number: %w", err)
	}
	return next, nil
}

// CreateVersion inserts a new draft.
func (s *Store) CreateVersion(ctx context.Context, version models.Version) error {
	var clonedFrom any
	if version.ClonedFrom != "" {
		clonedFrom = version.ClonedFrom
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO pricing_policy_versions
			(id, policy_id, version_no, kind, schema_version, status, body, body_hash,
			 cloned_from, created_by, change_reason)
		VALUES ($1, $2, $3, $4, $5, 'DRAFT', $6, $7, $8, $9, $10)`,
		version.ID, version.PolicyID, version.VersionNo, version.Kind, version.SchemaVersion,
		version.Body, version.BodyHash, clonedFrom, version.CreatedBy, version.ChangeReason)
	if err != nil {
		return fmt.Errorf("policies: create version: %w", err)
	}
	return nil
}

// GetVersion returns one version.
func (s *Store) GetVersion(ctx context.Context, id string) (models.Version, error) {
	return s.scanVersion(s.db.QueryRow(ctx, `
		SELECT id, policy_id, version_no, kind, schema_version, status, body, body_hash,
		       COALESCE(cloned_from, ''), created_by, change_reason, created_at, updated_at
		FROM pricing_policy_versions WHERE id = $1`, id), id)
}

// UpdateDraftBody replaces a draft's body. The database refuses this once the
// version has left DRAFT, so a lost race cannot rewrite approved numbers.
func (s *Store) UpdateDraftBody(ctx context.Context, id string, body []byte, bodyHash, changeReason string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE pricing_policy_versions
		SET body = $2, body_hash = $3, change_reason = $4
		WHERE id = $1 AND status = 'DRAFT'`, id, body, bodyHash, changeReason)
	if err != nil {
		if strings.Contains(err.Error(), "cannot change once it is") {
			return fmt.Errorf("%w: %s", models.ErrNotDraft, id)
		}
		return fmt.Errorf("policies: update draft: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: %s", models.ErrNotDraft, id)
	}
	return nil
}

// TransitionStatus moves a version between statuses, refusing a move from an
// unexpected current status so two operators cannot both "approve" a version.
func (s *Store) TransitionStatus(ctx context.Context, id, from, to string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE pricing_policy_versions SET status = $3
		WHERE id = $1 AND status = $2`, id, from, to)
	if err != nil {
		return fmt.Errorf("policies: transition %s -> %s: %w", from, to, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("policies: version %s is not %s, so it cannot become %s", id, from, to)
	}
	return nil
}

// RecordEvent appends to a version's lifecycle history.
func (s *Store) RecordEvent(ctx context.Context, versionID, event, from, to, performedBy, reason string) error {
	var fromValue, toValue any
	if from != "" {
		fromValue = from
	}
	if to != "" {
		toValue = to
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO pricing_policy_version_events (version_id, event, from_status, to_status, performed_by, reason)
		VALUES ($1, $2, $3, $4, $5, $6)`, versionID, event, fromValue, toValue, performedBy, reason)
	if err != nil {
		return fmt.Errorf("policies: record event: %w", err)
	}
	return nil
}

// Activate records that a version becomes live from a moment, and supersedes the
// version it replaces.
func (s *Store) Activate(ctx context.Context, activation models.Activation) error {
	var approval, supersedes any
	if activation.ApprovalRequestID != "" {
		approval = activation.ApprovalRequestID
	}
	if activation.Supersedes != "" {
		supersedes = activation.Supersedes
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO pricing_policy_activations
			(id, policy_id, version_id, effective_from, action, activated_by,
			 approval_request_id, supersedes_activation, reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		activation.ID, activation.PolicyID, activation.VersionID, activation.EffectiveFrom,
		activation.Action, activation.ActivatedBy, approval, supersedes, activation.Reason)
	if err != nil {
		return fmt.Errorf("policies: activate: %w", err)
	}
	return nil
}

// LatestActivation returns the activation in force for a lineage at asOf.
func (s *Store) LatestActivation(ctx context.Context, policyID string, asOf time.Time) (models.Activation, error) {
	var activation models.Activation
	err := s.db.QueryRow(ctx, `
		SELECT id, policy_id, version_id, effective_from, action, activated_by,
		       COALESCE(approval_request_id, ''), COALESCE(supersedes_activation, ''), reason, created_at
		FROM pricing_policy_activations
		WHERE policy_id = $1 AND effective_from <= $2
		ORDER BY effective_from DESC, created_at DESC
		LIMIT 1`, policyID, asOf,
	).Scan(&activation.ID, &activation.PolicyID, &activation.VersionID, &activation.EffectiveFrom,
		&activation.Action, &activation.ActivatedBy, &activation.ApprovalRequestID,
		&activation.Supersedes, &activation.Reason, &activation.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Activation{}, fmt.Errorf("%w: no activation for %s at %s", models.ErrNoActivePolicy, policyID, asOf.Format(time.RFC3339))
	}
	if err != nil {
		return models.Activation{}, fmt.Errorf("policies: latest activation: %w", err)
	}
	return activation, nil
}

// Resolve finds the version that prices a kind for a place at a moment.
//
// It asks the database for the MOST SPECIFIC lineage that has an activation in
// force, in one query: zone beats market beats state beats country. Two lineages
// at the same level for the same place are impossible (a unique constraint), so
// the ordering can never be arbitrary.
func (s *Store) Resolve(ctx context.Context, kind string, chain models.GeographyChain, asOf time.Time) (models.Resolution, error) {
	if err := chain.Validate(); err != nil {
		return models.Resolution{}, err
	}

	// Parallel arrays rather than a row-vs-array comparison: Postgres has no
	// `record = text` operator, and unnest of two arrays is robust whatever a
	// geography reference happens to contain.
	candidates := chain.Candidates()
	levels := make([]string, 0, len(candidates))
	refs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		levels = append(levels, candidate[0])
		refs = append(refs, candidate[1])
	}

	rows, err := s.db.Query(ctx, `
		SELECT p.id, p.kind, p.geography_level, p.geography_ref, p.created_by, p.created_at,
		       v.id, v.policy_id, v.version_no, v.kind, v.schema_version, v.status, v.body, v.body_hash,
		       COALESCE(v.cloned_from, ''), v.created_by, v.change_reason, v.created_at, v.updated_at,
		       a.id, a.policy_id, a.version_id, a.effective_from, a.action, a.activated_by,
		       COALESCE(a.approval_request_id, ''), COALESCE(a.supersedes_activation, ''), a.reason, a.created_at
		FROM pricing_policies p
		JOIN LATERAL (
			SELECT * FROM pricing_policy_activations inner_a
			WHERE inner_a.policy_id = p.id AND inner_a.effective_from <= $2
			ORDER BY inner_a.effective_from DESC, inner_a.created_at DESC
			LIMIT 1
		) a ON true
		JOIN pricing_policy_versions v ON v.id = a.version_id
		WHERE p.kind = $1
		  AND EXISTS (
			SELECT 1 FROM unnest($3::text[], $4::text[]) AS candidate(level, ref)
			WHERE candidate.level = p.geography_level AND candidate.ref = p.geography_ref
		  )
		ORDER BY CASE p.geography_level
			WHEN 'ZONE' THEN 1 WHEN 'MARKET' THEN 2 WHEN 'STATE' THEN 3 ELSE 4 END
		LIMIT 1`,
		kind, asOf, levels, refs)
	if err != nil {
		return models.Resolution{}, fmt.Errorf("policies: resolve %s: %w", kind, err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return models.Resolution{}, fmt.Errorf("policies: resolve %s: %w", kind, err)
		}
		return models.Resolution{}, fmt.Errorf("%w: %s for %v at %s",
			models.ErrNoActivePolicy, kind, chain.Candidates(), asOf.Format(time.RFC3339))
	}

	var resolution models.Resolution
	if err := rows.Scan(
		&resolution.Policy.ID, &resolution.Policy.Kind, &resolution.Policy.GeographyLevel,
		&resolution.Policy.GeographyRef, &resolution.Policy.CreatedBy, &resolution.Policy.CreatedAt,
		&resolution.Version.ID, &resolution.Version.PolicyID, &resolution.Version.VersionNo,
		&resolution.Version.Kind, &resolution.Version.SchemaVersion, &resolution.Version.Status,
		&resolution.Version.Body, &resolution.Version.BodyHash, &resolution.Version.ClonedFrom,
		&resolution.Version.CreatedBy, &resolution.Version.ChangeReason,
		&resolution.Version.CreatedAt, &resolution.Version.UpdatedAt,
		&resolution.Activation.ID, &resolution.Activation.PolicyID, &resolution.Activation.VersionID,
		&resolution.Activation.EffectiveFrom, &resolution.Activation.Action, &resolution.Activation.ActivatedBy,
		&resolution.Activation.ApprovalRequestID, &resolution.Activation.Supersedes,
		&resolution.Activation.Reason, &resolution.Activation.CreatedAt,
	); err != nil {
		return models.Resolution{}, fmt.Errorf("policies: scan resolution: %w", err)
	}
	return resolution, nil
}

// ListVersions returns a lineage's editions, newest first.
func (s *Store) ListVersions(ctx context.Context, policyID string, limit int) ([]models.Version, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, policy_id, version_no, kind, schema_version, status, body, body_hash,
		       COALESCE(cloned_from, ''), created_by, change_reason, created_at, updated_at
		FROM pricing_policy_versions
		WHERE policy_id = $1
		ORDER BY version_no DESC
		LIMIT $2`, policyID, limit)
	if err != nil {
		return nil, fmt.Errorf("policies: list versions: %w", err)
	}
	defer rows.Close()

	var versions []models.Version
	for rows.Next() {
		version, err := s.scanVersionRow(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func (s *Store) scanVersion(row scannable, id string) (models.Version, error) {
	version, err := s.scanVersionRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Version{}, fmt.Errorf("%w: version %s", models.ErrNotFound, id)
	}
	return version, err
}

func (s *Store) scanVersionRow(row scannable) (models.Version, error) {
	var version models.Version
	if err := row.Scan(&version.ID, &version.PolicyID, &version.VersionNo, &version.Kind,
		&version.SchemaVersion, &version.Status, &version.Body, &version.BodyHash,
		&version.ClonedFrom, &version.CreatedBy, &version.ChangeReason,
		&version.CreatedAt, &version.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Version{}, err
		}
		return models.Version{}, fmt.Errorf("policies: scan version: %w", err)
	}
	return version, nil
}
