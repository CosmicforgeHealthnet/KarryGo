package taxonomy

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	commandPublishVersion = "PUBLISH_VERSION"
	commandRetireVersion  = "RETIRE_VERSION"

	publicationPublished = "PUBLISHED"
	publicationRetired   = "RETIRED"

	eventVersionPublished = "taxonomy.version.published"
	eventVersionRetired   = "taxonomy.version.retired"
)

type PublishVersionInput struct {
	VersionID        string
	ExpectedRevision int64
	Actor            Actor
}

type RetireVersionInput struct {
	VersionID        string
	ExpectedRevision int64
	Actor            Actor
}

// PublishVersion atomically replaces the currently published taxonomy.
//
// Transactional guarantee:
//
//	validate draft
//	    ↓
//	retire current published version
//	    ↓
//	publish new version
//	    ↓
//	publication history
//	    ↓
//	outbox evidence
//	    ↓
//	idempotency evidence
//	    ↓
//	COMMIT
//
// Any failure rolls back the entire transaction, leaving the previous
// published version active.
func (s *Service) PublishVersion(
	ctx context.Context,
	input PublishVersionInput,
) (Version, error) {
	if s == nil ||
		s.repo == nil ||
		input.VersionID == "" ||
		input.ExpectedRevision <= 0 ||
		input.Actor.ActorRef == "" ||
		input.Actor.CommandRef == "" {
		return Version{}, ErrInvalidArgument
	}

	fingerprint, err := requestFingerprint(struct {
		VersionID        string `json:"version_id"`
		ExpectedRevision int64  `json:"expected_revision"`
	}{
		VersionID:        input.VersionID,
		ExpectedRevision: input.ExpectedRevision,
	})
	if err != nil {
		return Version{}, err
	}

	// The version-to-namespace relationship is immutable in PostgreSQL.
	initial, err := s.repo.GetVersionByID(ctx, input.VersionID)
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

	namespace, err := s.repo.LockNamespaceByIDTx(
		ctx,
		tx,
		initial.NamespaceID,
	)
	if err != nil {
		return Version{}, err
	}

	if namespace.Status != NamespaceStatusActive {
		return Version{}, ErrConflict
	}

	draft, err := s.repo.GetVersionByIDTx(
		ctx,
		tx,
		input.VersionID,
		true,
	)
	if err != nil {
		return Version{}, err
	}

	if draft.NamespaceID != namespace.ID {
		return Version{}, ErrConflict
	}

	// Replay is intentionally checked before lifecycle state validation.
	// A successful publish retried with the same command must return the
	// original published result even though it is no longer DRAFT.
	replayed, err := s.replayVersionCommand(
		ctx,
		tx,
		namespace.ID,
		input.Actor.CommandRef,
		commandPublishVersion,
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

	if draft.Status != VersionStatusDraft {
		return Version{}, ErrNotDraft
	}

	if draft.Revision != input.ExpectedRevision {
		return Version{}, ErrVersionMismatch
	}

	if err = s.validateDraftForPublication(
		ctx,
		tx,
		draft,
	); err != nil {
		return Version{}, err
	}

	var previous *Version

	current, currentErr := s.repo.GetPublishedVersionTx(
		ctx,
		tx,
		namespace.ID,
		true,
	)

	switch {
	case currentErr == nil:
		previous = &current

	case errors.Is(currentErr, ErrNotFound):
		previous = nil

	default:
		return Version{}, currentErr
	}

	if previous != nil {
		retired, retireErr := s.repo.RetirePublishedVersion(
			ctx,
			tx,
			previous.ID,
			input.Actor.ActorRef,
			previous.Revision,
		)
		if retireErr != nil {
			return Version{}, retireErr
		}

		if err = s.repo.InsertPublicationHistory(
			ctx,
			tx,
			PublicationHistoryEntry{
				ID:            "ctpub_" + uuid.NewString(),
				NamespaceID:   namespace.ID,
				VersionID:     retired.ID,
				EventType:     publicationRetired,
				ActorRef:      input.Actor.ActorRef,
				CommandRef:    input.Actor.CommandRef,
				CorrelationID: input.Actor.CorrelationID,
				CausationID:   input.Actor.CausationID,
				Metadata: map[string]any{
					"replacement_version_id":     draft.ID,
					"replacement_version_number": draft.VersionNumber,
					"retired_version_number":     retired.VersionNumber,
				},
			},
		); err != nil {
			return Version{}, err
		}

		if err = s.publishLifecycleEvent(
			ctx,
			tx,
			input.Actor,
			eventVersionRetired,
			retired,
			map[string]any{
				"replacement_version_id": draft.ID,
			},
		); err != nil {
			return Version{}, err
		}
	}

	published, err := s.repo.PublishDraftVersion(
		ctx,
		tx,
		draft.ID,
		input.Actor.ActorRef,
		draft.Revision,
	)
	if err != nil {
		return Version{}, err
	}

	var previousID *string
	if previous != nil {
		id := previous.ID
		previousID = &id
	}

	if err = s.repo.InsertPublicationHistory(
		ctx,
		tx,
		PublicationHistoryEntry{
			ID:                "ctpub_" + uuid.NewString(),
			NamespaceID:       namespace.ID,
			VersionID:         published.ID,
			PreviousVersionID: previousID,
			EventType:         publicationPublished,
			ActorRef:          input.Actor.ActorRef,
			CommandRef:        input.Actor.CommandRef,
			CorrelationID:     input.Actor.CorrelationID,
			CausationID:       input.Actor.CausationID,
			Metadata: map[string]any{
				"version_number": published.VersionNumber,
			},
		},
	); err != nil {
		return Version{}, err
	}

	if err = s.publishLifecycleEvent(
		ctx,
		tx,
		input.Actor,
		eventVersionPublished,
		published,
		map[string]any{
			"previous_version_id": previousID,
		},
	); err != nil {
		return Version{}, err
	}

	if err = s.repo.InsertCommand(
		ctx,
		tx,
		CommandRecord{
			ID:                 "ctcmd_" + uuid.NewString(),
			NamespaceID:        namespace.ID,
			CommandRef:         input.Actor.CommandRef,
			CommandType:        commandPublishVersion,
			RequestFingerprint: fingerprint,
			ResultType:         resultTypeVersion,
			ResultRef:          published.ID,
			ResultRevision:     published.Revision,
		},
	); err != nil {
		return Version{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Version{}, err
	}

	return published, nil
}

// RetireVersion explicitly retires the currently published version without
// replacing it. The namespace may temporarily have no published taxonomy.
func (s *Service) RetireVersion(
	ctx context.Context,
	input RetireVersionInput,
) (Version, error) {
	if s == nil ||
		s.repo == nil ||
		input.VersionID == "" ||
		input.ExpectedRevision <= 0 ||
		input.Actor.ActorRef == "" ||
		input.Actor.CommandRef == "" {
		return Version{}, ErrInvalidArgument
	}

	fingerprint, err := requestFingerprint(struct {
		VersionID        string `json:"version_id"`
		ExpectedRevision int64  `json:"expected_revision"`
	}{
		VersionID:        input.VersionID,
		ExpectedRevision: input.ExpectedRevision,
	})
	if err != nil {
		return Version{}, err
	}

	initial, err := s.repo.GetVersionByID(ctx, input.VersionID)
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

	namespace, err := s.repo.LockNamespaceByIDTx(
		ctx,
		tx,
		initial.NamespaceID,
	)
	if err != nil {
		return Version{}, err
	}

	version, err := s.repo.GetVersionByIDTx(
		ctx,
		tx,
		input.VersionID,
		true,
	)
	if err != nil {
		return Version{}, err
	}

	replayed, err := s.replayVersionCommand(
		ctx,
		tx,
		namespace.ID,
		input.Actor.CommandRef,
		commandRetireVersion,
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

	if version.Status != VersionStatusPublished {
		return Version{}, ErrConflict
	}

	if version.Revision != input.ExpectedRevision {
		return Version{}, ErrVersionMismatch
	}

	retired, err := s.repo.RetirePublishedVersion(
		ctx,
		tx,
		version.ID,
		input.Actor.ActorRef,
		version.Revision,
	)
	if err != nil {
		return Version{}, err
	}

	if err = s.repo.InsertPublicationHistory(
		ctx,
		tx,
		PublicationHistoryEntry{
			ID:            "ctpub_" + uuid.NewString(),
			NamespaceID:   namespace.ID,
			VersionID:     retired.ID,
			EventType:     publicationRetired,
			ActorRef:      input.Actor.ActorRef,
			CommandRef:    input.Actor.CommandRef,
			CorrelationID: input.Actor.CorrelationID,
			CausationID:   input.Actor.CausationID,
			Metadata: map[string]any{
				"version_number": retired.VersionNumber,
				"explicit":       true,
			},
		},
	); err != nil {
		return Version{}, err
	}

	if err = s.publishLifecycleEvent(
		ctx,
		tx,
		input.Actor,
		eventVersionRetired,
		retired,
		map[string]any{
			"explicit": true,
		},
	); err != nil {
		return Version{}, err
	}

	if err = s.repo.InsertCommand(
		ctx,
		tx,
		CommandRecord{
			ID:                 "ctcmd_" + uuid.NewString(),
			NamespaceID:        namespace.ID,
			CommandRef:         input.Actor.CommandRef,
			CommandType:        commandRetireVersion,
			RequestFingerprint: fingerprint,
			ResultType:         resultTypeVersion,
			ResultRef:          retired.ID,
			ResultRevision:     retired.Revision,
		},
	); err != nil {
		return Version{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Version{}, err
	}

	return retired, nil
}

// ListPublicationHistory returns immutable publish/retire evidence.
func (s *Service) ListPublicationHistory(
	ctx context.Context,
	code NamespaceCode,
) ([]PublicationHistoryRecord, error) {
	if s == nil || s.repo == nil || code == "" {
		return nil, ErrInvalidArgument
	}

	namespace, err := s.repo.GetNamespaceByCode(ctx, code)
	if err != nil {
		return nil, err
	}

	return s.repo.ListPublicationHistory(ctx, namespace.ID)
}

// validateDraftForPublication checks cross-record invariants immediately
// before lifecycle replacement.
//
// PostgreSQL remains the final authority for FK ownership, cycles, bounded
// hierarchy depth and lifecycle immutability.
func (s *Service) validateDraftForPublication(
	ctx context.Context,
	tx pgx.Tx,
	version Version,
) error {
	if version.Status != VersionStatusDraft {
		return ErrNotDraft
	}

	facets, err := s.repo.ListFacetsTx(ctx, tx, version.ID)
	if err != nil {
		return err
	}

	nodes, err := s.repo.ListNodesTx(ctx, tx, version.ID)
	if err != nil {
		return err
	}

	facetByID := make(map[string]Facet, len(facets))
	for _, facet := range facets {
		if facet.NamespaceID != version.NamespaceID ||
			facet.VersionID != version.ID {
			return ErrConflict
		}

		facetByID[facet.ID] = facet
	}

	nodeByID := make(map[string]Node, len(nodes))
	for _, node := range nodes {
		if node.NamespaceID != version.NamespaceID ||
			node.VersionID != version.ID {
			return ErrConflict
		}

		nodeByID[node.ID] = node
	}

	for _, node := range nodes {
		facet, exists := facetByID[node.FacetID]
		if !exists {
			return ErrConflict
		}

		if facet.StructureType == FacetStructureFlat &&
			node.ParentID != nil {
			return ErrConflict
		}

		if node.Status == RecordStatusActive &&
			facet.Status != RecordStatusActive {
			return ErrConflict
		}

		if node.ParentID == nil {
			continue
		}

		parent, exists := nodeByID[*node.ParentID]
		if !exists {
			return ErrConflict
		}

		if parent.FacetID != node.FacetID ||
			parent.NamespaceID != node.NamespaceID ||
			parent.VersionID != node.VersionID {
			return ErrConflict
		}

		if node.Status == RecordStatusActive &&
			parent.Status != RecordStatusActive {
			return ErrConflict
		}
	}

	return nil
}

func (s *Service) publishLifecycleEvent(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	eventType string,
	version Version,
	extra map[string]any,
) error {
	if s.publisher == nil {
		return nil
	}

	payload := map[string]any{
		"namespace_id":   version.NamespaceID,
		"version_id":     version.ID,
		"version_number": version.VersionNumber,
		"status":         version.Status,
		"revision":       version.Revision,
	}

	for key, value := range extra {
		payload[key] = value
	}

	return s.publisher.Publish(
		ctx,
		tx,
		eventType,
		aggregateVersion,
		version.ID,
		version.Revision,
		"",
		actor.CorrelationID,
		actor.CausationID,
		payload,
	)
}
