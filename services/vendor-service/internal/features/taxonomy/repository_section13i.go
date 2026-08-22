package taxonomy

// Section 13I read queries.
//
// Every list here is bounded. A taxonomy read surface that can return an
// unbounded vocabulary dump is a denial-of-service surface and, on the vendor
// side, a cross-tenant enumeration surface; both are refused structurally by
// capping the row count rather than by trusting a caller-supplied limit.

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Bounded page sizes for Section 13I reads.
const (
	// MaxAssignmentRows caps mapping/tag/claim assignment reads for one
	// source resource. A single catalogue item classified more than this many
	// ways is a governance problem, not a paging problem.
	MaxAssignmentRows = 200

	// MaxVocabularyRows caps term, claim and authority-vocabulary listings.
	MaxVocabularyRows = 500

	// MaxSeedRunRows caps seed evidence listings.
	MaxSeedRunRows = 100
)

// ---------------------------------------------------------------------------
// Mappings
// ---------------------------------------------------------------------------

// ListMappingsForSourceScoped returns the active mappings for one source
// resource, restricted to rows that are either platform-owned (business_ref
// IS NULL) or owned by the calling business.
//
// The caller has already established authoritatively that the source resource
// belongs to that business (see VendorAccess.ResolveSource); this query is the
// second gate, so a mapping row another vendor attached to the same source ref
// still never leaves this service through the vendor surface.
func (r *Repository) ListMappingsForSourceScoped(
	ctx context.Context,
	namespaceID string,
	sourceType string,
	sourceRef string,
	businessRef string,
) ([]TaxonomyMapping, error) {
	if r == nil || r.pool == nil ||
		namespaceID == "" || sourceType == "" || sourceRef == "" || businessRef == "" {
		return nil, ErrInvalidArgument
	}

	rows, err := r.pool.Query(
		ctx,
		`
        SELECT
            id, namespace_id, version_id, node_id,
            source_type, source_ref,
            mapping_type, status,
            confidence_basis, confidence_score,
            business_ref, branch_ref,
            metadata, revision,
            created_by_actor_ref, updated_by_actor_ref,
            created_at, updated_at
          FROM canonical_taxonomy_mappings
         WHERE namespace_id=$1
           AND source_type=$2
           AND source_ref=$3
           AND (business_ref IS NULL OR business_ref=$4)
         ORDER BY mapping_type, id
         LIMIT $5
        `,
		namespaceID, sourceType, sourceRef, businessRef, MaxAssignmentRows,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TaxonomyMapping
	for rows.Next() {
		var m TaxonomyMapping
		if err = rows.Scan(
			&m.ID, &m.NamespaceID, &m.VersionID, &m.NodeID,
			&m.SourceType, &m.SourceRef,
			&m.MappingType, &m.Status,
			&m.ConfidenceBasis, &m.ConfidenceScore,
			&m.BusinessRef, &m.BranchRef,
			&m.Metadata, &m.Revision,
			&m.CreatedByActorRef, &m.UpdatedByActorRef,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Semantic tag assignments
// ---------------------------------------------------------------------------

// TagAssignmentView is an assignment joined to the tag it names, so a vendor
// reading their own classification does not have to resolve tag ids itself.
type TagAssignmentView struct {
	ID          string  `json:"id"`
	NamespaceID string  `json:"namespace_id"`
	TagID       string  `json:"tag_id"`
	TagCode     string  `json:"tag_code"`
	TagType     string  `json:"tag_type"`
	DisplayName string  `json:"display_name"`
	SourceType  string  `json:"source_type"`
	SourceRef   string  `json:"source_ref"`
	BusinessRef *string `json:"business_ref"`
	BranchRef   *string `json:"branch_ref"`
	Status      string  `json:"status"`
	Revision    int64   `json:"revision"`
}

func (r *Repository) ListTagAssignmentsForSourceScoped(
	ctx context.Context,
	namespaceID string,
	sourceType string,
	sourceRef string,
	businessRef string,
) ([]TagAssignmentView, error) {
	if r == nil || r.pool == nil ||
		namespaceID == "" || sourceType == "" || sourceRef == "" || businessRef == "" {
		return nil, ErrInvalidArgument
	}

	rows, err := r.pool.Query(
		ctx,
		`
        SELECT
            a.id, a.namespace_id, a.tag_id,
            t.code, t.tag_type, t.display_name,
            a.source_type, a.source_ref,
            a.business_ref, a.branch_ref,
            a.status, a.revision
          FROM canonical_taxonomy_tag_assignments a
          JOIN canonical_taxonomy_tags t
            ON t.id=a.tag_id
           AND t.namespace_id=a.namespace_id
         WHERE a.namespace_id=$1
           AND a.source_type=$2
           AND a.source_ref=$3
           AND (a.business_ref IS NULL OR a.business_ref=$4)
         ORDER BY t.code
         LIMIT $5
        `,
		namespaceID, sourceType, sourceRef, businessRef, MaxAssignmentRows,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TagAssignmentView
	for rows.Next() {
		var v TagAssignmentView
		if err = rows.Scan(
			&v.ID, &v.NamespaceID, &v.TagID,
			&v.TagCode, &v.TagType, &v.DisplayName,
			&v.SourceType, &v.SourceRef,
			&v.BusinessRef, &v.BranchRef,
			&v.Status, &v.Revision,
		); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Dietary claim assignments
// ---------------------------------------------------------------------------

// DietaryClaimAssignmentView is an assignment joined to the claim it names,
// carrying the verification policy and status a vendor needs to understand
// what evidence their claim still requires.
//
// It deliberately exposes the verifying actor as a boolean-ish presence only
// via VerifiedAt: which platform reviewer verified a claim is operator
// information, not vendor information.
type DietaryClaimAssignmentView struct {
	ID                 string     `json:"id"`
	NamespaceID        string     `json:"namespace_id"`
	ClaimID            string     `json:"claim_id"`
	ClaimCode          string     `json:"claim_code"`
	DisplayName        string     `json:"display_name"`
	VerificationPolicy string     `json:"verification_policy"`
	RiskLevel          string     `json:"risk_level"`
	SourceType         string     `json:"source_type"`
	SourceRef          string     `json:"source_ref"`
	BusinessRef        *string    `json:"business_ref"`
	BranchRef          *string    `json:"branch_ref"`
	Status             string     `json:"status"`
	EvidenceRef        *string    `json:"evidence_ref"`
	VerifiedAt         *time.Time `json:"verified_at"`
	ExpiresAt          *time.Time `json:"expires_at"`
	Revision           int64      `json:"revision"`
}

func (r *Repository) ListDietaryClaimAssignmentsForSourceScoped(
	ctx context.Context,
	namespaceID string,
	sourceType string,
	sourceRef string,
	businessRef string,
) ([]DietaryClaimAssignmentView, error) {
	if r == nil || r.pool == nil ||
		namespaceID == "" || sourceType == "" || sourceRef == "" || businessRef == "" {
		return nil, ErrInvalidArgument
	}

	rows, err := r.pool.Query(
		ctx,
		`
        SELECT
            a.id, a.namespace_id, a.claim_id,
            c.code, c.display_name, c.verification_policy, c.risk_level,
            a.source_type, a.source_ref,
            a.business_ref, a.branch_ref,
            a.status, a.evidence_ref, a.verified_at, a.expires_at,
            a.revision
          FROM canonical_taxonomy_dietary_claim_assignments a
          JOIN canonical_taxonomy_dietary_claims c
            ON c.id=a.claim_id
           AND c.namespace_id=a.namespace_id
         WHERE a.namespace_id=$1
           AND a.source_type=$2
           AND a.source_ref=$3
           AND (a.business_ref IS NULL OR a.business_ref=$4)
         ORDER BY c.code
         LIMIT $5
        `,
		namespaceID, sourceType, sourceRef, businessRef, MaxAssignmentRows,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DietaryClaimAssignmentView
	for rows.Next() {
		var v DietaryClaimAssignmentView
		if err = rows.Scan(
			&v.ID, &v.NamespaceID, &v.ClaimID,
			&v.ClaimCode, &v.DisplayName, &v.VerificationPolicy, &v.RiskLevel,
			&v.SourceType, &v.SourceRef,
			&v.BusinessRef, &v.BranchRef,
			&v.Status, &v.EvidenceRef, &v.VerifiedAt, &v.ExpiresAt,
			&v.Revision,
		); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListDietaryClaims returns the namespace's active dietary claim catalogue
// with each claim's verification policy, so a vendor can see what a claim will
// require before declaring it.
func (r *Repository) ListDietaryClaims(
	ctx context.Context,
	namespaceID string,
) ([]DietaryClaim, error) {
	if r == nil || r.pool == nil || namespaceID == "" {
		return nil, ErrInvalidArgument
	}

	rows, err := r.pool.Query(
		ctx,
		`
        SELECT
            id, namespace_id, code, display_name, description,
            verification_policy, risk_level, status, metadata, revision
          FROM canonical_taxonomy_dietary_claims
         WHERE namespace_id=$1
           AND status='ACTIVE'
         ORDER BY code
         LIMIT $2
        `,
		namespaceID, MaxVocabularyRows,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DietaryClaim
	for rows.Next() {
		var c DietaryClaim
		if err = rows.Scan(
			&c.ID, &c.NamespaceID, &c.Code, &c.DisplayName, &c.Description,
			&c.VerificationPolicy, &c.RiskLevel, &c.Status, &c.Metadata, &c.Revision,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Vocabulary, governance and seed evidence (platform reads)
// ---------------------------------------------------------------------------

// ListNodeTerms returns the aliases, synonyms and related terms attached to
// one node of one version.
func (r *Repository) ListNodeTerms(
	ctx context.Context,
	versionID string,
	nodeID string,
) ([]TaxonomyNodeTerm, error) {
	if r == nil || r.pool == nil || versionID == "" || nodeID == "" {
		return nil, ErrInvalidArgument
	}

	rows, err := r.pool.Query(
		ctx,
		`
        SELECT
            id, namespace_id, version_id, node_id,
            term, normalized_term, relationship_type,
            locale, COALESCE(region,''), status, revision
          FROM canonical_taxonomy_node_terms
         WHERE version_id=$1
           AND node_id=$2
         ORDER BY relationship_type, normalized_term
         LIMIT $3
        `,
		versionID, nodeID, MaxVocabularyRows,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TaxonomyNodeTerm
	for rows.Next() {
		var term TaxonomyNodeTerm
		if err = rows.Scan(
			&term.ID, &term.NamespaceID, &term.VersionID, &term.NodeID,
			&term.Term, &term.NormalizedTerm, &term.RelationshipType,
			&term.Locale, &term.Region, &term.Status, &term.Revision,
		); err != nil {
			return nil, err
		}
		out = append(out, term)
	}
	return out, rows.Err()
}

// GetGovernanceCase reads one governance case, scoped to its namespace so a
// case id from one namespace cannot be read through another.
func (r *Repository) GetGovernanceCase(
	ctx context.Context,
	namespaceID string,
	caseID string,
) (GovernanceCase, error) {
	if r == nil || r.pool == nil || namespaceID == "" || caseID == "" {
		return GovernanceCase{}, ErrInvalidArgument
	}

	var c GovernanceCase
	err := r.pool.QueryRow(
		ctx,
		`
        SELECT
            id, namespace_id, version_id,
            subject_type, subject_ref,
            case_type, status, priority,
            reason_code, description,
            resolution_code, resolution_notes,
            assigned_actor_ref, opened_by_actor_ref, resolved_by_actor_ref,
            metadata, revision,
            created_at, updated_at, resolved_at
          FROM canonical_taxonomy_governance_cases
         WHERE id=$1
           AND namespace_id=$2
        `,
		caseID, namespaceID,
	).Scan(
		&c.ID, &c.NamespaceID, &c.VersionID,
		&c.SubjectType, &c.SubjectRef,
		&c.CaseType, &c.Status, &c.Priority,
		&c.ReasonCode, &c.Description,
		&c.ResolutionCode, &c.ResolutionNotes,
		&c.AssignedActorRef, &c.OpenedByActorRef, &c.ResolvedByActorRef,
		&c.Metadata, &c.Revision,
		&c.CreatedAt, &c.UpdatedAt, &c.ResolvedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return GovernanceCase{}, ErrNotFound
		}
		return GovernanceCase{}, err
	}
	return c, nil
}

// SeedRunRecord is append-only seed evidence: what was applied, from which
// bundle fingerprint, into which version.
type SeedRunRecord struct {
	ID                       string     `json:"id"`
	NamespaceID              string     `json:"namespace_id"`
	SeedKey                  string     `json:"seed_key"`
	Fingerprint              string     `json:"fingerprint"`
	VersionID                string     `json:"version_id"`
	Status                   string     `json:"status"`
	FacetCount               int        `json:"facet_count"`
	NodeCount                int        `json:"node_count"`
	TermCount                int        `json:"term_count"`
	AuthorityVocabularyCount int        `json:"authority_vocabulary_count"`
	ActorRef                 string     `json:"actor_ref"`
	CreatedAt                time.Time  `json:"created_at"`
	PublishedAt              *time.Time `json:"published_at"`
}

// ListSeedRuns returns seed evidence newest-first. It is a read: nothing here
// re-runs a seed, because re-running one is a destructive publication action
// and belongs to the seed applier's own command path.
func (r *Repository) ListSeedRuns(
	ctx context.Context,
	namespaceID string,
) ([]SeedRunRecord, error) {
	if r == nil || r.pool == nil || namespaceID == "" {
		return nil, ErrInvalidArgument
	}

	rows, err := r.pool.Query(
		ctx,
		`
        SELECT
            id, namespace_id, seed_key, fingerprint, version_id, status,
            facet_count, node_count, term_count, authority_vocabulary_count,
            actor_ref, created_at, published_at
          FROM canonical_taxonomy_seed_runs
         WHERE namespace_id=$1
         ORDER BY created_at DESC, id DESC
         LIMIT $2
        `,
		namespaceID, MaxSeedRunRows,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SeedRunRecord
	for rows.Next() {
		var record SeedRunRecord
		if err = rows.Scan(
			&record.ID, &record.NamespaceID, &record.SeedKey, &record.Fingerprint,
			&record.VersionID, &record.Status,
			&record.FacetCount, &record.NodeCount, &record.TermCount,
			&record.AuthorityVocabularyCount,
			&record.ActorRef, &record.CreatedAt, &record.PublishedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

// AuthorityVocabularyRecord is one governed authority term for a version.
type AuthorityVocabularyRecord struct {
	ID             string `json:"id"`
	NamespaceID    string `json:"namespace_id"`
	VersionID      string `json:"version_id"`
	AuthorityType  string `json:"authority_type"`
	AuthorityRef   string `json:"authority_ref"`
	AuthorityCode  string `json:"authority_code"`
	Term           string `json:"term"`
	NormalizedTerm string `json:"normalized_term"`
	Locale         string `json:"locale"`
	Region         string `json:"region"`
}

func (r *Repository) ListAuthorityVocabulary(
	ctx context.Context,
	namespaceID string,
	versionID string,
) ([]AuthorityVocabularyRecord, error) {
	if r == nil || r.pool == nil || namespaceID == "" || versionID == "" {
		return nil, ErrInvalidArgument
	}

	rows, err := r.pool.Query(
		ctx,
		`
        SELECT
            id, namespace_id, version_id,
            authority_type, authority_ref, authority_code,
            term, normalized_term, locale, COALESCE(region,'')
          FROM canonical_taxonomy_authority_vocabulary
         WHERE namespace_id=$1
           AND version_id=$2
         ORDER BY authority_type, normalized_term
         LIMIT $3
        `,
		namespaceID, versionID, MaxVocabularyRows,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AuthorityVocabularyRecord
	for rows.Next() {
		var record AuthorityVocabularyRecord
		if err = rows.Scan(
			&record.ID, &record.NamespaceID, &record.VersionID,
			&record.AuthorityType, &record.AuthorityRef, &record.AuthorityCode,
			&record.Term, &record.NormalizedTerm, &record.Locale, &record.Region,
		); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

// normalizeNamespaceCode accepts the namespace as it arrives on a path and
// returns it only if it names a real canonical namespace. An unknown value is
// rejected rather than defaulted, so a typo can never silently address FOOD.
func normalizeNamespaceCode(value string) (NamespaceCode, bool) {
	switch NamespaceCode(strings.ToUpper(strings.TrimSpace(value))) {
	case NamespaceFood:
		return NamespaceFood, true
	case NamespaceShop:
		return NamespaceShop, true
	default:
		return "", false
	}
}
