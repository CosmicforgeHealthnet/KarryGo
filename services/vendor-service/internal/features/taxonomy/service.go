package taxonomy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	commandCreateDraft = "CREATE_DRAFT"

	resultTypeVersion = "TAXONOMY_VERSION"

	changeDraftCreated = "DRAFT_CREATED"

	aggregateVersion = "TAXONOMY_VERSION"

	eventDraftCreated = "taxonomy.version.draft_created"
)

type EventPublisher interface {
	Publish(
		ctx context.Context,
		tx pgx.Tx,
		eventType string,
		aggregateType string,
		aggregateID string,
		aggregateVersion int64,
		vendorID string,
		correlationID string,
		causationID string,
		payload any,
	) error
}

type Service struct {
	repo      *Repository
	publisher EventPublisher
}

func NewService(
	repo *Repository,
	publisher EventPublisher,
) *Service {
	return &Service{
		repo:      repo,
		publisher: publisher,
	}
}

type CreateDraftInput struct {
	NamespaceCode NamespaceCode
	Actor         Actor
}

type createDraftFingerprintPayload struct {
	NamespaceCode NamespaceCode `json:"namespace_code"`
}

// CreateDraft creates the next draft version for a canonical taxonomy namespace.
//
// Version allocation is serialized by locking the namespace row. The command,
// taxonomy mutation, change evidence and outbox event all commit atomically.
func (s *Service) CreateDraft(
	ctx context.Context,
	input CreateDraftInput,
) (Version, error) {
	if s == nil ||
		s.repo == nil ||
		input.NamespaceCode == "" ||
		input.Actor.ActorRef == "" ||
		input.Actor.CommandRef == "" {
		return Version{}, ErrInvalidArgument
	}

	fingerprint, err := requestFingerprint(createDraftFingerprintPayload{
		NamespaceCode: input.NamespaceCode,
	})
	if err != nil {
		return Version{}, err
	}

	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return Version{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	namespace, err := s.repo.LockNamespaceByCode(
		ctx,
		tx,
		input.NamespaceCode,
	)
	if err != nil {
		return Version{}, err
	}

	if namespace.Status != NamespaceStatusActive {
		return Version{}, ErrConflict
	}

	replayed, err := s.replayVersionCommand(
		ctx,
		tx,
		namespace.ID,
		input.Actor.CommandRef,
		commandCreateDraft,
		fingerprint,
	)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return Version{}, err
		}
		return replayed, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Version{}, err
	}

	versionNumber, err := s.repo.NextVersionNumber(
		ctx,
		tx,
		namespace.ID,
	)
	if err != nil {
		return Version{}, err
	}

	version, err := s.repo.InsertVersion(
		ctx,
		tx,
		Version{
			ID:                "ctv_" + uuid.NewString(),
			NamespaceID:       namespace.ID,
			VersionNumber:     versionNumber,
			Status:            VersionStatusDraft,
			CreatedByActorRef: input.Actor.ActorRef,
		},
	)
	if err != nil {
		return Version{}, err
	}

	if err = s.repo.InsertChangeHistory(
		ctx,
		tx,
		ChangeHistoryEntry{
			ID:            "ctchg_" + uuid.NewString(),
			NamespaceID:   namespace.ID,
			VersionID:     version.ID,
			ActorRef:      input.Actor.ActorRef,
			CommandRef:    input.Actor.CommandRef,
			ChangeType:    changeDraftCreated,
			AggregateType: aggregateVersion,
			AggregateRef:  version.ID,
			BeforeState:   nil,
			AfterState:    version,
			CorrelationID: input.Actor.CorrelationID,
			CausationID:   input.Actor.CausationID,
		},
	); err != nil {
		return Version{}, err
	}

	if s.publisher != nil {
		if err = s.publisher.Publish(
			ctx,
			tx,
			eventDraftCreated,
			aggregateVersion,
			version.ID,
			version.Revision,
			"",
			input.Actor.CorrelationID,
			input.Actor.CausationID,
			map[string]any{
				"namespace_code": input.NamespaceCode,
				"namespace_id":   namespace.ID,
				"version_id":     version.ID,
				"version_number": version.VersionNumber,
				"status":         version.Status,
			},
		); err != nil {
			return Version{}, err
		}
	}

	if err = s.repo.InsertCommand(
		ctx,
		tx,
		CommandRecord{
			ID:                 "ctcmd_" + uuid.NewString(),
			NamespaceID:        namespace.ID,
			CommandRef:         input.Actor.CommandRef,
			CommandType:        commandCreateDraft,
			RequestFingerprint: fingerprint,
			ResultType:         resultTypeVersion,
			ResultRef:          version.ID,
			ResultRevision:     version.Revision,
		},
	); err != nil {
		return Version{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Version{}, err
	}

	return version, nil
}

// GetVersion returns a canonical taxonomy version by its immutable identifier.
func (s *Service) GetVersion(
	ctx context.Context,
	versionID string,
) (Version, error) {
	if s == nil || s.repo == nil || versionID == "" {
		return Version{}, ErrInvalidArgument
	}

	return s.repo.GetVersionByID(ctx, versionID)
}

// GetPublishedVersion returns the currently published version for a namespace.
func (s *Service) GetPublishedVersion(
	ctx context.Context,
	code NamespaceCode,
) (Version, error) {
	if s == nil || s.repo == nil || code == "" {
		return Version{}, ErrInvalidArgument
	}

	namespace, err := s.repo.GetNamespaceByCode(ctx, code)
	if err != nil {
		return Version{}, err
	}

	return s.repo.GetPublishedVersion(ctx, namespace.ID)
}

// ListVersions returns deterministic version history for a namespace.
func (s *Service) ListVersions(
	ctx context.Context,
	code NamespaceCode,
) ([]Version, error) {
	if s == nil || s.repo == nil || code == "" {
		return nil, ErrInvalidArgument
	}

	namespace, err := s.repo.GetNamespaceByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	return s.repo.ListVersions(ctx, namespace.ID)
}

func (s *Service) replayVersionCommand(
	ctx context.Context,
	tx pgx.Tx,
	namespaceID string,
	commandRef string,
	commandType string,
	fingerprint string,
) (Version, error) {
	record, err := s.repo.GetCommandTx(
		ctx,
		tx,
		namespaceID,
		commandRef,
	)
	if err != nil {
		return Version{}, err
	}

	if record.CommandType != commandType ||
		record.RequestFingerprint != fingerprint ||
		record.ResultType != resultTypeVersion {
		return Version{}, ErrIdempotency
	}

	version, err := s.repo.GetVersionByIDTx(
		ctx,
		tx,
		record.ResultRef,
		false,
	)
	if err != nil {
		return Version{}, err
	}

	if version.NamespaceID != namespaceID {
		return Version{}, ErrConflict
	}

	return version, nil
}

func requestFingerprint(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", ErrInvalidArgument
	}

	sum := sha256.Sum256(raw)

	return hex.EncodeToString(sum[:]), nil
}
