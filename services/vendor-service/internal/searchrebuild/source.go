package searchrebuild

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/searchprojection"
)

// Source implements searchprojection.SearchFullRebuildSource over the
// authoritative Vendor domain.
type Source struct {
	pool *pgxpool.Pool
}

func NewSource(pool *pgxpool.Pool) *Source {
	return &Source{pool: pool}
}

// LoadSearchRebuildBatch returns the next bounded batch for a namespace.
//
// The namespace decides which authoritative domain is read; there is no shared
// path that could accidentally emit a Food document into a Shop rebuild. The
// handler above enforces the same separation when it applies the batch, so a
// crossover would have to defeat both.
func (s *Source) LoadSearchRebuildBatch(
	ctx context.Context,
	namespace searchprojection.SearchCacheNamespace,
	scope searchprojection.SearchRebuildScope,
	scopeRef string,
	checkpoint string,
	limit int,
) (searchprojection.SearchRebuildBatch, error) {
	if s == nil || s.pool == nil {
		return searchprojection.SearchRebuildBatch{}, searchprojection.ErrInvalidArgument
	}

	limit = boundedLimit(limit)
	scopeRef = strings.TrimSpace(scopeRef)
	checkpoint = strings.TrimSpace(checkpoint)

	switch namespace {
	case searchprojection.SearchCacheNamespaceFood:
		return s.loadFoodBatch(ctx, scope, scopeRef, checkpoint, limit)
	case searchprojection.SearchCacheNamespaceShop:
		return s.loadShopBatch(ctx, scope, scopeRef, checkpoint, limit)
	default:
		return searchprojection.SearchRebuildBatch{}, searchprojection.ErrInvalidArgument
	}
}

// MaxBatch is the certified rebuild batch ceiling. The handler refuses a batch
// larger than this, so the source never exceeds it.
const MaxBatch = 200

func boundedLimit(limit int) int {
	switch {
	case limit <= 0:
		return MaxBatch
	case limit > MaxBatch:
		return MaxBatch
	default:
		return limit
	}
}

// scopeFilter turns a rebuild scope into a SQL predicate fragment plus its
// argument.
//
// A scoped rebuild narrows to one business or one branch. The scope value is
// never interpolated into SQL — it is bound as a parameter, and an unknown
// scope is refused rather than silently widened to a full rebuild, because
// silently rebuilding the entire marketplace when an operator asked for one
// branch is the more expensive mistake.
func scopeFilter(
	scope searchprojection.SearchRebuildScope,
	scopeRef string,
	businessColumn string,
	branchColumn string,
) (string, any, error) {
	switch scope {
	case "", searchprojection.SearchRebuildScopeFull:
		return "", nil, nil
	case searchprojection.SearchRebuildScopeBusiness:
		if scopeRef == "" {
			return "", nil, searchprojection.ErrInvalidArgument
		}
		return " AND " + businessColumn + " = $%d", scopeRef, nil
	case searchprojection.SearchRebuildScopeBranch:
		if scopeRef == "" {
			return "", nil, searchprojection.ErrInvalidArgument
		}
		return " AND " + branchColumn + " = $%d", scopeRef, nil
	default:
		return "", nil, searchprojection.ErrInvalidArgument
	}
}

// jsonObject marshals a map, returning an empty object rather than nil so the
// projection's NOT NULL jsonb columns always receive a value.
func jsonObject(value map[string]any) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}

// dedupeStrings returns the distinct non-empty values, preserving order.
//
// Vocabulary arrives from several joins (a node's own terms, its aliases, its
// synonyms) and duplicates would inflate the search text without adding a
// single new match.
func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

var _ searchprojection.SearchFullRebuildSource = (*Source)(nil)
