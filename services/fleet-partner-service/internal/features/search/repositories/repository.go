// Package repositories implements the PostgreSQL full-text search projection
// for Section 25.
//
// Every read carries organization_id in SQL, and the branch predicate is
// applied in the same query — never as a second in-Go filter after rows have
// already left the database — because "the tenant condition is part of the
// query" is the property that makes a projection safe to query directly.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/search/models"
)

// ErrNotFound covers "no such row" and "not yours" identically.
var ErrNotFound = errors.New("search: record not found")

// PrefixDocument is the opaque ID prefix for search projection rows.
const PrefixDocument = "sdc"

// Repository persists the Section 25 search projection.
type Repository struct{ db database.Querier }

// NewRepository builds the repository.
func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q == nil {
		return r.db
	}
	return q
}

func classify(err error, operation string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	default:
		return fmt.Errorf("search: %s: %w", operation, err)
	}
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func encodeJSONObject(value models.JSONMap) ([]byte, error) {
	if value == nil {
		return []byte("{}"), nil
	}
	encoded, err := json.Marshal(map[string]any(value))
	if err != nil {
		return nil, fmt.Errorf("search: encode json: %w", err)
	}
	return encoded, nil
}

func decodeJSONObject(raw []byte) models.JSONMap {
	if len(raw) == 0 {
		return models.JSONMap{}
	}
	var out models.JSONMap
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return models.JSONMap{}
	}
	return out
}

const documentColumns = `id, organization_id, COALESCE(branch_id, ''), subject_type, subject_reference,
	title, searchable_text, exact_reference_fields, status, required_permission, source_version,
	visibility, indexed_at, available`

func scanDocument(row pgx.Row) (models.Document, error) {
	var d models.Document
	var exactRaw []byte
	err := row.Scan(&d.ID, &d.OrganizationID, &d.BranchID, &d.SubjectType, &d.SubjectReference,
		&d.Title, &d.SearchableText, &exactRaw, &d.Status, &d.RequiredPermission, &d.SourceVersion,
		&d.Visibility, &d.IndexedAt, &d.Available)
	if err != nil {
		return d, err
	}
	d.ExactReferenceFields = decodeJSONObject(exactRaw)
	return d, nil
}

// Upsert indexes or re-indexes one subject.
//
// The WHERE clause on the ON CONFLICT branch is the whole guarantee: a
// reindex carrying a source_version lower than what is already stored is
// silently ignored rather than clobbering a newer projection with a stale
// one. It is not an error — out-of-order indexing calls are expected under
// concurrency — it is simply a no-op.
func (r *Repository) Upsert(ctx context.Context, q database.Querier, d models.Document) (models.Document, error) {
	exact, err := encodeJSONObject(d.ExactReferenceFields)
	if err != nil {
		return models.Document{}, err
	}
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_search_documents
			(id, organization_id, branch_id, subject_type, subject_reference, title,
			 searchable_text, exact_reference_fields, status, required_permission, source_version, visibility,
			 available)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, true)
		ON CONFLICT (organization_id, subject_type, subject_reference) DO UPDATE SET
			branch_id = EXCLUDED.branch_id, title = EXCLUDED.title,
			searchable_text = EXCLUDED.searchable_text,
			exact_reference_fields = EXCLUDED.exact_reference_fields, status = EXCLUDED.status,
			required_permission = EXCLUDED.required_permission, source_version = EXCLUDED.source_version,
			visibility = EXCLUDED.visibility,
			indexed_at = now(), available = true
		WHERE EXCLUDED.source_version >= fleet_partner_search_documents.source_version
		RETURNING `+documentColumns,
		d.ID, d.OrganizationID, nullable(d.BranchID), d.SubjectType, d.SubjectReference, d.Title,
		d.SearchableText, exact, d.Status, d.RequiredPermission, d.SourceVersion, d.Visibility)
	updated, err := scanDocument(row)
	if err != nil {
		// A refused (stale) upsert also lands here as ErrNoRows, since the
		// UPDATE's WHERE excluded it and there was nothing to RETURNING. That
		// is the correct, silent outcome for a stale reindex, not a caller
		// error, so translate it back to the current stored row instead of a
		// not-found.
		if errors.Is(err, pgx.ErrNoRows) {
			return r.Get(ctx, q, d.OrganizationID, d.SubjectType, d.SubjectReference)
		}
		return updated, classify(err, "upsert search document")
	}
	return updated, nil
}

// Get reads one indexed document by its subject identity.
func (r *Repository) Get(ctx context.Context, q database.Querier, organizationID, subjectType, subjectReference string) (models.Document, error) {
	doc, err := scanDocument(r.q(q).QueryRow(ctx, `SELECT `+documentColumns+`
		FROM fleet_partner_search_documents
		WHERE organization_id = $1 AND subject_type = $2 AND subject_reference = $3`,
		organizationID, subjectType, subjectReference))
	if err != nil {
		return doc, classify(err, "get search document")
	}
	return doc, nil
}

// MarkUnavailable removes a subject from search results without deleting its
// history — a deleted or archived subject stops being findable, but the
// projection row itself is not destroyed.
func (r *Repository) MarkUnavailable(ctx context.Context, q database.Querier, organizationID, subjectType, subjectReference string) error {
	_, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_search_documents SET available = false, indexed_at = now()
		WHERE organization_id = $1 AND subject_type = $2 AND subject_reference = $3`,
		organizationID, subjectType, subjectReference)
	if err != nil {
		return fmt.Errorf("search: mark unavailable: %w", err)
	}
	return nil
}

// QueryFilter is the allowlisted search query. There is no free-form filter
// field and no raw sort input: a filter cannot become a query fragment.
type QueryFilter struct {
	Text        string
	SubjectType string
	ExactRef    string
	Status      string
	// BranchIDs restricts results to organization-wide rows plus rows in
	// these branches. Empty means organization-wide authority (no filter).
	BranchIDs           []string
	AllowedPermissions  []string
	AllowedVisibilities []string
	// CandidateLimit bounds how many tenant/branch/query-matching rows come
	// back before the usecase applies the per-row permission filter, since
	// that filter can only be applied after knowing which distinct
	// permissions are present in this exact result set.
	CandidateLimit int
}

// RequiredPermissions returns only the permission codes attached to matching
// tenant/branch/visibility-scoped rows. No result title, reference or count is
// exposed before authorization; the subsequent result query still enforces
// the allowed set in SQL.
func (r *Repository) RequiredPermissions(ctx context.Context, organizationID string, filter QueryFilter) ([]string, error) {
	args := []any{organizationID, filter.SubjectType, filter.ExactRef, filter.Status, filter.Text, filter.AllowedVisibilities}
	where := `organization_id=$1 AND available=true AND ($2='' OR subject_type=$2) AND ($3='' OR subject_reference=$3) AND ($4='' OR status=$4) AND ($5='' OR search_vector @@ websearch_to_tsquery('english',$5)) AND visibility=ANY($6::text[])`
	if len(filter.BranchIDs) > 0 {
		args = append(args, filter.BranchIDs)
		where += fmt.Sprintf(" AND (branch_id IS NULL OR branch_id=ANY($%d))", len(args))
	}
	rows, err := r.db.Query(ctx, `SELECT DISTINCT required_permission FROM fleet_partner_search_documents WHERE `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("search: required permissions: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var permission string
		if err = rows.Scan(&permission); err != nil {
			return nil, err
		}
		out = append(out, permission)
	}
	return out, rows.Err()
}

// Query returns tenant- and branch-scoped candidates, ranked by relevance
// when a text query is supplied. Permission and visibility filtering happen
// one layer up, after this candidate set is known.
func (r *Repository) Query(ctx context.Context, organizationID string, filter QueryFilter) ([]models.Document, error) {
	limit := filter.CandidateLimit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	args := []any{organizationID, filter.SubjectType, filter.ExactRef, filter.Status, filter.Text, filter.AllowedPermissions, filter.AllowedVisibilities}
	where := `organization_id = $1 AND available = true
		AND ($2 = '' OR subject_type = $2)
		AND ($3 = '' OR subject_reference = $3)
		AND ($4 = '' OR status = $4)
		AND ($5 = '' OR search_vector @@ websearch_to_tsquery('english', $5))
		AND required_permission = ANY($6::text[])
		AND visibility = ANY($7::text[])`

	if len(filter.BranchIDs) > 0 {
		args = append(args, filter.BranchIDs)
		where += fmt.Sprintf(" AND (branch_id IS NULL OR branch_id = ANY($%d))", len(args))
	}
	args = append(args, limit)

	order := "indexed_at DESC"
	if strings.TrimSpace(filter.Text) != "" {
		order = "ts_rank(search_vector, websearch_to_tsquery('english', $5)) DESC, indexed_at DESC"
	}

	rows, err := r.db.Query(ctx, `SELECT `+documentColumns+`
		FROM fleet_partner_search_documents
		WHERE `+where+`
		ORDER BY `+order+`
		LIMIT $`+fmt.Sprintf("%d", len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("search: query: %w", err)
	}
	defer rows.Close()

	var docs []models.Document
	for rows.Next() {
		doc, err := scanDocument(rows)
		if err != nil {
			return nil, fmt.Errorf("search: query scan: %w", err)
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}
