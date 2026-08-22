package searchprojection

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

const searchRebuildBatchLimit = 200

type RebuildDocument struct {
	Food *FoodProjectionInput
	Shop *ShopProjectionInput

	FoodSignals *FoodOperationalSignals
	ShopSignals *ShopOperationalSignals
}

type SearchRebuildBatch struct {
	Documents []RebuildDocument

	NextCheckpoint string

	Done bool

	SourceRevisions json.RawMessage
}

// SearchFullRebuildSource reads authoritative current domain
// state. Search itself never becomes catalogue, availability or
// inventory authority.
type SearchFullRebuildSource interface {
	LoadSearchRebuildBatch(
		ctx context.Context,
		namespace SearchCacheNamespace,
		scope SearchRebuildScope,
		scopeRef string,
		checkpoint string,
		limit int,
	) (
		SearchRebuildBatch,
		error,
	)
}

type SearchRebuildGenerationStore interface {
	GetGeneration(
		ctx context.Context,
		namespace SearchCacheNamespace,
		generation int64,
	) (
		SearchProjectionGeneration,
		error,
	)

	RecordGenerationProgress(
		ctx context.Context,
		namespace SearchCacheNamespace,
		generation int64,
		processedDelta int64,
		workerID string,
		sourceRevisions json.RawMessage,
	) error

	CompleteGenerationBuild(
		ctx context.Context,
		namespace SearchCacheNamespace,
		generation int64,
	) error

	MarkGenerationReady(
		ctx context.Context,
		namespace SearchCacheNamespace,
		generation int64,
	) error

	MarkGenerationFailed(
		ctx context.Context,
		namespace SearchCacheNamespace,
		generation int64,
		failureCode string,
		failureReason string,
		workerID string,
	) error

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
}

type SearchRebuildApplier interface {
	ApplyRebuildDocument(
		ctx context.Context,
		namespace SearchCacheNamespace,
		generation int64,
		document RebuildDocument,
	) error
}

type DefaultSearchRebuildApplier struct {
	foodProjector *FoodProjector

	shopProjector *ShopProjector

	foodBackend *PostgresFoodSearchBackend

	shopBackend *PostgresShopSearchBackend
}

func NewDefaultSearchRebuildApplier(
	foodProjector *FoodProjector,
	shopProjector *ShopProjector,
	foodBackend *PostgresFoodSearchBackend,
	shopBackend *PostgresShopSearchBackend,
) *DefaultSearchRebuildApplier {
	return &DefaultSearchRebuildApplier{
		foodProjector: foodProjector,

		shopProjector: shopProjector,

		foodBackend: foodBackend,

		shopBackend: shopBackend,
	}
}

func (
	applier *DefaultSearchRebuildApplier,
) ApplyRebuildDocument(
	ctx context.Context,
	namespace SearchCacheNamespace,
	generation int64,
	document RebuildDocument,
) error {
	if applier == nil ||
		generation <= 0 {
		return ErrInvalidArgument
	}

	switch namespace {
	case SearchCacheNamespaceFood:
		if document.Food == nil ||
			document.Shop != nil ||
			document.ShopSignals != nil ||
			applier.foodProjector == nil {
			return ErrInvalidArgument
		}

		input :=
			*document.Food

		input.Generation =
			generation

		if _, err :=
			applier.foodProjector.
				Project(
					ctx,
					input,
				); err != nil {
			return err
		}

		if document.FoodSignals != nil {
			if applier.foodBackend == nil {
				return ErrInvalidArgument
			}

			signals :=
				*document.FoodSignals

			signals.Generation =
				generation

			if _, err :=
				applier.foodBackend.
					ApplyOperationalSignals(
						ctx,
						signals,
					); err != nil {
				return err
			}
		}

	case SearchCacheNamespaceShop:
		if document.Shop == nil ||
			document.Food != nil ||
			document.FoodSignals != nil ||
			applier.shopProjector == nil {
			return ErrInvalidArgument
		}

		input :=
			*document.Shop

		input.Generation =
			generation

		if _, err :=
			applier.shopProjector.
				Project(
					ctx,
					input,
				); err != nil {
			return err
		}

		if document.ShopSignals != nil {
			if applier.shopBackend == nil {
				return ErrInvalidArgument
			}

			signals :=
				*document.ShopSignals

			signals.Generation =
				generation

			if _, err :=
				applier.shopBackend.
					ApplyOperationalSignals(
						ctx,
						signals,
					); err != nil {
				return err
			}
		}

	default:
		return ErrInvalidArgument
	}

	return nil
}

type FullRebuildJobHandler struct {
	source SearchFullRebuildSource

	applier SearchRebuildApplier

	generations SearchRebuildGenerationStore
}

func NewFullRebuildJobHandler(
	source SearchFullRebuildSource,
	applier SearchRebuildApplier,
	generations SearchRebuildGenerationStore,
) *FullRebuildJobHandler {
	return &FullRebuildJobHandler{
		source: source,

		applier: applier,

		generations: generations,
	}
}

func (
	handler *FullRebuildJobHandler,
) HandleProjectionJob(
	ctx context.Context,
	job ProjectionJob,
	runtime *ProjectionJobRuntime,
) error {
	if handler == nil ||
		handler.source == nil ||
		handler.applier == nil ||
		handler.generations == nil ||
		runtime == nil ||
		job.JobType !=
			ProjectionJobFullRebuild ||
		job.Generation <= 0 {
		return ErrInvalidArgument
	}

	generation, err :=
		handler.generations.
			GetGeneration(
				ctx,
				job.Namespace,
				job.Generation,
			)
	if err != nil {
		return err
	}

	if generation.Status ==
		SearchGenerationFailed {
		if err =
			handler.generations.
				PrepareFailedGenerationRetry(
					ctx,
					job.Namespace,
					job.Generation,
				); err != nil {
			return err
		}

		job.ProgressCheckpoint = ""

		if err =
			runtime.Checkpoint(
				ctx,
				"",
			); err != nil {
			return err
		}

		generation.Status =
			SearchGenerationBuilding
	}

	if generation.Status !=
		SearchGenerationBuilding {
		return ErrGenerationStateConflict
	}

	checkpoint :=
		job.ProgressCheckpoint

	for {
		if err =
			runtime.Heartbeat(
				ctx,
			); err != nil {
			return err
		}

		batch, batchErr :=
			handler.source.
				LoadSearchRebuildBatch(
					ctx,
					job.Namespace,
					SearchRebuildScopeFull,
					"",
					checkpoint,
					searchRebuildBatchLimit,
				)

		if batchErr != nil {
			_ =
				handler.generations.
					MarkGenerationFailed(
						ctx,
						job.Namespace,
						job.Generation,
						"REBUILD_SOURCE_FAILED",
						batchErr.Error(),
						job.LeaseOwner,
					)

			return batchErr
		}

		if len(batch.Documents) >
			searchRebuildBatchLimit {
			err =
				ErrInvalidArgument

			_ =
				handler.generations.
					MarkGenerationFailed(
						ctx,
						job.Namespace,
						job.Generation,
						"REBUILD_BATCH_TOO_LARGE",
						err.Error(),
						job.LeaseOwner,
					)

			return err
		}

		for _, document := range batch.Documents {
			if err =
				handler.applier.
					ApplyRebuildDocument(
						ctx,
						job.Namespace,
						job.Generation,
						document,
					); err != nil {
				_ =
					handler.generations.
						MarkGenerationFailed(
							ctx,
							job.Namespace,
							job.Generation,
							"REBUILD_APPLY_FAILED",
							err.Error(),
							job.LeaseOwner,
						)

				return err
			}
		}

		if err =
			handler.generations.
				RecordGenerationProgress(
					ctx,
					job.Namespace,
					job.Generation,
					int64(
						len(
							batch.Documents,
						),
					),
					job.LeaseOwner,
					batch.SourceRevisions,
				); err != nil {
			return err
		}

		if len(
			[]rune(
				batch.NextCheckpoint,
			),
		) > 2048 {
			return ErrInvalidArgument
		}

		checkpoint =
			batch.NextCheckpoint

		if err =
			runtime.Checkpoint(
				ctx,
				checkpoint,
			); err != nil {
			return err
		}

		if batch.Done {
			break
		}

		// Prevent a broken source from returning no progress
		// forever while keeping a worker lease alive.
		if len(batch.Documents) == 0 &&
			strings.TrimSpace(
				batch.NextCheckpoint,
			) ==
				strings.TrimSpace(
					job.ProgressCheckpoint,
				) {
			return errors.New(
				"rebuild source made no progress",
			)
		}

		job.ProgressCheckpoint =
			checkpoint
	}

	if err =
		handler.generations.
			CompleteGenerationBuild(
				ctx,
				job.Namespace,
				job.Generation,
			); err != nil {
		return err
	}

	return nil
}

type GenerationValidateActivateHandler struct {
	generations SearchRebuildGenerationStore
}

func NewGenerationValidateActivateHandler(
	generations SearchRebuildGenerationStore,
) *GenerationValidateActivateHandler {
	return &GenerationValidateActivateHandler{
		generations: generations,
	}
}

func (
	handler *GenerationValidateActivateHandler,
) HandleProjectionJob(
	ctx context.Context,
	job ProjectionJob,
	runtime *ProjectionJobRuntime,
) error {
	if handler == nil ||
		handler.generations == nil ||
		job.JobType !=
			ProjectionJobValidateActivate ||
		job.Generation <= 0 {
		return ErrInvalidArgument
	}

	if runtime != nil {
		if err :=
			runtime.Heartbeat(
				ctx,
			); err != nil {
			return err
		}
	}

	if err :=
		handler.generations.
			MarkGenerationReady(
				ctx,
				job.Namespace,
				job.Generation,
			); err != nil {
		return err
	}

	return handler.generations.
		ActivateGeneration(
			ctx,
			job.Namespace,
			job.Generation,
		)
}

var (
	_ SearchRebuildApplier = (*DefaultSearchRebuildApplier)(nil)

	_ ProjectionJobHandler = (*FullRebuildJobHandler)(nil)

	_ ProjectionJobHandler = (*GenerationValidateActivateHandler)(nil)
)
