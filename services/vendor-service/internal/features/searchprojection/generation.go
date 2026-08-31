package searchprojection

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrGenerationNotFound = errors.New(
		"search projection generation not found",
	)

	ErrGenerationStateConflict = errors.New(
		"search projection generation state conflict",
	)

	ErrGenerationActivationConflict = errors.New(
		"search projection generation activation conflict",
	)

	ErrGenerationValidationFailed = errors.New(
		"search projection generation validation failed",
	)

	ErrProjectionJobLeaseLost = errors.New(
		"search projection worker lease lost",
	)
)

type SearchGenerationStatus string

const (
	SearchGenerationBuilding SearchGenerationStatus = "BUILDING"

	SearchGenerationReady SearchGenerationStatus = "READY"

	SearchGenerationActive SearchGenerationStatus = "ACTIVE"

	SearchGenerationFailed SearchGenerationStatus = "FAILED"

	SearchGenerationRetired SearchGenerationStatus = "RETIRED"
)

type SearchRebuildScope string

const (
	SearchRebuildScopeFull SearchRebuildScope = "FULL"

	SearchRebuildScopeBusiness SearchRebuildScope = "BUSINESS"

	SearchRebuildScopeBranch SearchRebuildScope = "BRANCH"
)

type SearchProjectionGeneration struct {
	Namespace SearchCacheNamespace

	Generation int64

	Status SearchGenerationStatus

	BaseGeneration int64

	Scope    SearchRebuildScope
	ScopeRef string

	SourceRevisions json.RawMessage

	DocumentCount  int64
	ProcessedCount int64

	FailureCode   string
	FailureReason string

	RequestedBy string

	StartedAt   *time.Time
	CompletedAt *time.Time
	ValidatedAt *time.Time
	ActivatedAt *time.Time
	RetiredAt   *time.Time

	WorkerID       string
	LeaseToken     string
	LeaseExpiresAt *time.Time
	HeartbeatAt    *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

type SearchProjectionGenerationProvider interface {
	CurrentActiveGeneration(
		ctx context.Context,
		namespace SearchCacheNamespace,
	) (
		int64,
		error,
	)
}

func validSearchNamespace(
	namespace SearchCacheNamespace,
) bool {
	return namespace ==
		SearchCacheNamespaceFood ||
		namespace ==
			SearchCacheNamespaceShop
}

func validRebuildScope(
	scope SearchRebuildScope,
	scopeRef string,
) bool {
	scopeRef =
		strings.TrimSpace(
			scopeRef,
		)

	switch scope {
	case SearchRebuildScopeFull:
		return scopeRef == ""

	case SearchRebuildScopeBusiness,
		SearchRebuildScopeBranch:
		return scopeRef != ""

	default:
		return false
	}
}

type GenerationAwareFoodSearch struct {
	origin FoodSearchOrigin

	generations SearchProjectionGenerationProvider
}

func NewGenerationAwareFoodSearch(
	origin FoodSearchOrigin,
	generations SearchProjectionGenerationProvider,
) *GenerationAwareFoodSearch {
	return &GenerationAwareFoodSearch{
		origin: origin,

		generations: generations,
	}
}

func (
	engine *GenerationAwareFoodSearch,
) Search(
	ctx context.Context,
	request FoodEngineSearchRequest,
) (
	FoodEngineSearchResponse,
	error,
) {
	if engine == nil ||
		engine.origin == nil ||
		engine.generations == nil {
		return FoodEngineSearchResponse{},
			ErrInvalidArgument
	}

	generation, err :=
		engine.generations.
			CurrentActiveGeneration(
				ctx,
				SearchCacheNamespaceFood,
			)
	if err != nil {
		return FoodEngineSearchResponse{},
			err
	}

	request.Generation =
		generation

	return engine.origin.Search(
		ctx,
		request,
	)
}

func (
	engine *GenerationAwareFoodSearch,
) Suggest(
	ctx context.Context,
	request SuggestionRequest,
) (
	[]SearchSuggestion,
	error,
) {
	if engine == nil ||
		engine.origin == nil ||
		engine.generations == nil {
		return nil,
			ErrInvalidArgument
	}

	generation, err :=
		engine.generations.
			CurrentActiveGeneration(
				ctx,
				SearchCacheNamespaceFood,
			)
	if err != nil {
		return nil, err
	}

	request.Generation =
		generation

	return engine.origin.Suggest(
		ctx,
		request,
	)
}

type GenerationAwareShopSearch struct {
	origin ShopSearchOrigin

	generations SearchProjectionGenerationProvider
}

func NewGenerationAwareShopSearch(
	origin ShopSearchOrigin,
	generations SearchProjectionGenerationProvider,
) *GenerationAwareShopSearch {
	return &GenerationAwareShopSearch{
		origin: origin,

		generations: generations,
	}
}

func (
	engine *GenerationAwareShopSearch,
) Search(
	ctx context.Context,
	request ShopEngineSearchRequest,
) (
	ShopEngineSearchResponse,
	error,
) {
	if engine == nil ||
		engine.origin == nil ||
		engine.generations == nil {
		return ShopEngineSearchResponse{},
			ErrInvalidArgument
	}

	generation, err :=
		engine.generations.
			CurrentActiveGeneration(
				ctx,
				SearchCacheNamespaceShop,
			)
	if err != nil {
		return ShopEngineSearchResponse{},
			err
	}

	request.Generation =
		generation

	return engine.origin.Search(
		ctx,
		request,
	)
}

func (
	engine *GenerationAwareShopSearch,
) Suggest(
	ctx context.Context,
	request SuggestionRequest,
) (
	[]SearchSuggestion,
	error,
) {
	if engine == nil ||
		engine.origin == nil ||
		engine.generations == nil {
		return nil,
			ErrInvalidArgument
	}

	generation, err :=
		engine.generations.
			CurrentActiveGeneration(
				ctx,
				SearchCacheNamespaceShop,
			)
	if err != nil {
		return nil, err
	}

	request.Generation =
		generation

	return engine.origin.Suggest(
		ctx,
		request,
	)
}

var (
	_ FoodSearchOrigin = (*GenerationAwareFoodSearch)(nil)

	_ ShopSearchOrigin = (*GenerationAwareShopSearch)(nil)
)
