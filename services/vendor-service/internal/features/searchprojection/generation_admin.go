package searchprojection

import (
	"context"
	"strings"
)

type SearchProjectionAdminAuthorizer interface {
	RequireSearchProjectionAdmin(
		ctx context.Context,
	) error
}

type SearchProjectionAdministrationStore interface {
	RequestFullRebuild(
		ctx context.Context,
		namespace SearchCacheNamespace,
		requestedBy string,
	) (
		SearchProjectionGeneration,
		error,
	)

	RequestScopedRebuild(
		ctx context.Context,
		namespace SearchCacheNamespace,
		scope SearchRebuildScope,
		scopeRef string,
		requestedBy string,
	) (
		string,
		error,
	)

	GetGeneration(
		ctx context.Context,
		namespace SearchCacheNamespace,
		generation int64,
	) (
		SearchProjectionGeneration,
		error,
	)

	CurrentActiveGeneration(
		ctx context.Context,
		namespace SearchCacheNamespace,
	) (
		int64,
		error,
	)

	PrepareFailedGenerationRetry(
		ctx context.Context,
		namespace SearchCacheNamespace,
		generation int64,
	) error

	ActivateGeneration(
		ctx context.Context,
		namespace SearchCacheNamespace,
		generation int64,
	) error

	RetireGeneration(
		ctx context.Context,
		namespace SearchCacheNamespace,
		generation int64,
	) error

	ReplayDeadLetterProjectionJob(
		ctx context.Context,
		jobID string,
	) error
}

type SearchProjectionAdminService struct {
	authorizer SearchProjectionAdminAuthorizer

	store SearchProjectionAdministrationStore
}

func NewSearchProjectionAdminService(
	authorizer SearchProjectionAdminAuthorizer,
	store SearchProjectionAdministrationStore,
) *SearchProjectionAdminService {
	return &SearchProjectionAdminService{
		authorizer: authorizer,

		store: store,
	}
}

func (
	service *SearchProjectionAdminService,
) authorize(
	ctx context.Context,
) error {
	if service == nil ||
		service.authorizer == nil ||
		service.store == nil {
		return ErrInvalidArgument
	}

	return service.authorizer.
		RequireSearchProjectionAdmin(
			ctx,
		)
}

func (
	service *SearchProjectionAdminService,
) RequestFullRebuild(
	ctx context.Context,
	namespace SearchCacheNamespace,
	requestedBy string,
) (
	SearchProjectionGeneration,
	error,
) {
	if err :=
		service.authorize(
			ctx,
		); err != nil {
		return SearchProjectionGeneration{},
			err
	}

	return service.store.
		RequestFullRebuild(
			ctx,
			namespace,
			strings.TrimSpace(
				requestedBy,
			),
		)
}

func (
	service *SearchProjectionAdminService,
) RequestScopedRebuild(
	ctx context.Context,
	namespace SearchCacheNamespace,
	scope SearchRebuildScope,
	scopeRef string,
	requestedBy string,
) (
	string,
	error,
) {
	if err :=
		service.authorize(
			ctx,
		); err != nil {
		return "", err
	}

	return service.store.
		RequestScopedRebuild(
			ctx,
			namespace,
			scope,
			scopeRef,
			requestedBy,
		)
}

func (
	service *SearchProjectionAdminService,
) InspectGeneration(
	ctx context.Context,
	namespace SearchCacheNamespace,
	generation int64,
) (
	SearchProjectionGeneration,
	error,
) {
	if err :=
		service.authorize(
			ctx,
		); err != nil {
		return SearchProjectionGeneration{},
			err
	}

	return service.store.
		GetGeneration(
			ctx,
			namespace,
			generation,
		)
}

func (
	service *SearchProjectionAdminService,
) InspectActiveGeneration(
	ctx context.Context,
	namespace SearchCacheNamespace,
) (
	int64,
	error,
) {
	if err :=
		service.authorize(
			ctx,
		); err != nil {
		return 0, err
	}

	return service.store.
		CurrentActiveGeneration(
			ctx,
			namespace,
		)
}

func (
	service *SearchProjectionAdminService,
) RetryFailedRebuild(
	ctx context.Context,
	namespace SearchCacheNamespace,
	generation int64,
) error {
	if err :=
		service.authorize(
			ctx,
		); err != nil {
		return err
	}

	return service.store.
		PrepareFailedGenerationRetry(
			ctx,
			namespace,
			generation,
		)
}

func (
	service *SearchProjectionAdminService,
) ActivateValidatedGeneration(
	ctx context.Context,
	namespace SearchCacheNamespace,
	generation int64,
) error {
	if err :=
		service.authorize(
			ctx,
		); err != nil {
		return err
	}

	return service.store.
		ActivateGeneration(
			ctx,
			namespace,
			generation,
		)
}

func (
	service *SearchProjectionAdminService,
) RetireOldGeneration(
	ctx context.Context,
	namespace SearchCacheNamespace,
	generation int64,
) error {
	if err :=
		service.authorize(
			ctx,
		); err != nil {
		return err
	}

	return service.store.
		RetireGeneration(
			ctx,
			namespace,
			generation,
		)
}

func (
	service *SearchProjectionAdminService,
) ReplayDeadLetterJob(
	ctx context.Context,
	jobID string,
) error {
	if err :=
		service.authorize(
			ctx,
		); err != nil {
		return err
	}

	if strings.TrimSpace(
		jobID,
	) == "" {
		return ErrInvalidArgument
	}

	return service.store.
		ReplayDeadLetterProjectionJob(
			ctx,
			jobID,
		)
}

var _ SearchProjectionAdministrationStore = (*PostgresSearchGenerationStore)(nil)
