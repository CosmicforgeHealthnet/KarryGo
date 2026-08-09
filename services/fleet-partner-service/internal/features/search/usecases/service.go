// Package usecases implements the Section 25 search workflows.
//
// Search is a read-side projection over other features' aggregates. It is
// never authoritative, and a search failure must never affect an
// authoritative operation — nothing in this package is called from inside
// another feature's write transaction; indexing is a best-effort, separate
// call a feature makes after its own commit succeeds.
//
// Permission filtering happens here, inside the query path, before any
// result is handed to the HTTP layer — not as a handler-side afterthought.
package usecases

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/search/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/search/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Flags gates the Section 25 search surface.
type Flags struct {
	Enabled bool
}

// Options configure the search service.
type Options struct {
	Pool       *pgxpool.Pool
	Repository *repositories.Repository
	Authorizer *accesscontrol.Authorizer
	Metrics    *observability.Metrics
	Flags      Flags
}

// Service implements the Section 25 search workflows.
type Service struct{ opts Options }

// NewService builds the service.
func NewService(opts Options) *Service {
	if opts.Metrics == nil {
		opts.Metrics = observability.NewMetrics()
	}
	return &Service{opts: opts}
}

func (s *Service) requireEnabled() error {
	if !s.opts.Flags.Enabled {
		return errorcodes.NotFound(errorcodes.SearchNotAvailable, "Search is not available for this organization.")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Indexing (called by other features after their own commit succeeds)
// ---------------------------------------------------------------------------

// IndexInput is the validated shape of an index/reindex request.
type IndexInput struct {
	BranchID             string
	SubjectType          string
	SubjectReference     string
	Title                string
	SearchableText       string
	ExactReferenceFields models.JSONMap
	Status               string
	RequiredPermission   string
	Visibility           string
	SourceVersion        int64
}

// Index creates or refreshes a subject's search projection.
//
// A source_version lower than what is already stored is silently ignored by
// the repository — this call never errors on a stale reindex, because a
// feature that fires a background reindex has no reasonable way to know
// whether a newer one already landed, and treating that as a failure would
// make ordinary concurrency look like a bug.
func (s *Service) Index(ctx context.Context, organizationID string, in IndexInput) (models.Document, error) {
	if organizationID == "" {
		return models.Document{}, apperrors.BadRequest("organizationId is required.", nil)
	}
	doc := models.Document{
		OrganizationID: organizationID, BranchID: in.BranchID, SubjectType: strings.TrimSpace(in.SubjectType),
		SubjectReference: strings.TrimSpace(in.SubjectReference), Title: strings.TrimSpace(in.Title),
		SearchableText: in.SearchableText, ExactReferenceFields: in.ExactReferenceFields.Clone(),
		Status: in.Status, RequiredPermission: strings.TrimSpace(in.RequiredPermission),
		Visibility:    strings.TrimSpace(in.Visibility),
		SourceVersion: in.SourceVersion,
	}
	if doc.Visibility == "" {
		doc.Visibility = "INTERNAL"
	}
	if err := doc.Validate(); err != nil {
		return models.Document{}, apperrors.BadRequest(err.Error(), err)
	}
	if !doc.Bounded() {
		return models.Document{}, apperrors.BadRequest("search document fields exceed their bounds.", nil)
	}

	existing, err := s.opts.Repository.Get(ctx, nil, organizationID, doc.SubjectType, doc.SubjectReference)
	switch {
	case err == nil:
		doc.ID = existing.ID
	default:
		doc.ID = ids.New(repositories.PrefixDocument)
	}
	return s.opts.Repository.Upsert(ctx, nil, doc)
}

// RemoveFromIndex marks a subject unavailable to search without deleting its
// projection history.
func (s *Service) RemoveFromIndex(ctx context.Context, organizationID, subjectType, subjectReference string) error {
	return s.opts.Repository.MarkUnavailable(ctx, nil, organizationID, subjectType, subjectReference)
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

// SearchInput is the allowlisted search query.
type SearchInput struct {
	Text              string
	SubjectType       string
	ExactReference    string
	Status            string
	Limit             int
	CallerRoleVersion int64
}

const (
	defaultSearchLimit = 25
	maxSearchLimit     = 100
	candidateLimit     = 200
)

// Search returns tenant- and branch-scoped, permission-filtered results.
//
// The candidate set is fetched tenant- and branch-scoped directly in SQL;
// this method then narrows it to rows whose required_permission the caller
// actually holds, and only then truncates to the requested page size — so a
// caller can never learn how many matching-but-unauthorized rows exist from
// the result count alone changing based on their own permissions in a
// detectable way beyond "fewer results."
func (s *Service) Search(ctx context.Context, t tenant.Context, in SearchInput) ([]models.Document, error) {
	if err := s.requireEnabled(); err != nil {
		return nil, err
	}
	limit := in.Limit
	if limit <= 0 || limit > maxSearchLimit {
		limit = defaultSearchLimit
	}

	permissionFilter := repositories.QueryFilter{Text: strings.TrimSpace(in.Text), SubjectType: in.SubjectType, ExactRef: in.ExactReference, Status: in.Status, BranchIDs: t.PermittedBranchIDs, AllowedVisibilities: []string{"INTERNAL", "ORGANIZATION"}}
	permissions, err := s.opts.Repository.RequiredPermissions(ctx, t.OrganizationID, permissionFilter)
	if err != nil {
		return nil, err
	}
	allowedPermissions := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		if s.opts.Authorizer == nil {
			continue
		}
		decision, authErr := s.opts.Authorizer.Authorize(ctx, t.OrganizationID, t.OrganizationUserID, in.CallerRoleVersion, permission, accesscontrol.Target{})
		if authErr != nil {
			return nil, authErr
		}
		if decision.Allowed {
			allowedPermissions = append(allowedPermissions, permission)
		}
	}
	if len(allowedPermissions) == 0 {
		return []models.Document{}, nil
	}
	candidates, err := s.opts.Repository.Query(ctx, t.OrganizationID, repositories.QueryFilter{
		Text: strings.TrimSpace(in.Text), SubjectType: in.SubjectType, ExactRef: in.ExactReference,
		Status: in.Status, BranchIDs: t.PermittedBranchIDs, CandidateLimit: candidateLimit,
		AllowedPermissions: allowedPermissions, AllowedVisibilities: []string{"INTERNAL", "ORGANIZATION"},
	})
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return []models.Document{}, nil
	}

	results := make([]models.Document, 0, limit)
	for _, candidate := range candidates {
		results = append(results, candidate)
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}
