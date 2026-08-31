package taxonomy

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	commandCloneActiveVersion = "CLONE_ACTIVE_VERSION"

	changeDraftCloned = "DRAFT_CLONED"

	eventDraftCloned = "taxonomy.version.draft_cloned"
)

type CloneActiveVersionInput struct {
	NamespaceCode NamespaceCode
	Actor         Actor
}

// CloneActiveVersion creates a new DRAFT from the currently published version.
//
// Facets and nodes receive new row identifiers while canonical codes,
// hierarchy, statuses and metadata are preserved. Parent links are remapped
// to the cloned node identifiers.
//
// The namespace row serializes version allocation and publication lifecycle.
func (s *Service) CloneActiveVersion(
	ctx context.Context,
	input CloneActiveVersionInput,
) (Version, error) {
	if s == nil ||
		s.repo == nil ||
		input.NamespaceCode == "" ||
		input.Actor.ActorRef == "" ||
		input.Actor.CommandRef == "" {
		return Version{}, ErrInvalidArgument
	}

	fingerprint, err := requestFingerprint(struct {
		NamespaceCode NamespaceCode `json:"namespace_code"`
	}{
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

	// Idempotent replay is checked before looking at the current publication.
	// The same command reference must always resolve to its original result.
	replayed, err := s.replayVersionCommand(
		ctx,
		tx,
		namespace.ID,
		input.Actor.CommandRef,
		commandCloneActiveVersion,
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

	published, err := s.repo.GetPublishedVersionTx(
		ctx,
		tx,
		namespace.ID,
		false,
	)
	if err != nil {
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

	sourceVersionID := published.ID

	draft, err := s.repo.InsertVersion(
		ctx,
		tx,
		Version{
			ID:                "ctv_" + uuid.NewString(),
			NamespaceID:       namespace.ID,
			VersionNumber:     versionNumber,
			Status:            VersionStatusDraft,
			SourceVersionID:   &sourceVersionID,
			CreatedByActorRef: input.Actor.ActorRef,
		},
	)
	if err != nil {
		return Version{}, err
	}

	facetCount, nodeCount, err := s.cloneVersionContent(
		ctx,
		tx,
		published,
		draft,
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
			VersionID:     draft.ID,
			ActorRef:      input.Actor.ActorRef,
			CommandRef:    input.Actor.CommandRef,
			ChangeType:    changeDraftCloned,
			AggregateType: aggregateVersion,
			AggregateRef:  draft.ID,
			BeforeState:   published,
			AfterState:    draft,
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
			eventDraftCloned,
			aggregateVersion,
			draft.ID,
			draft.Revision,
			"",
			input.Actor.CorrelationID,
			input.Actor.CausationID,
			map[string]any{
				"namespace_code":    input.NamespaceCode,
				"namespace_id":      namespace.ID,
				"source_version_id": published.ID,
				"source_version":    published.VersionNumber,
				"draft_version_id":  draft.ID,
				"draft_version":     draft.VersionNumber,
				"facet_count":       facetCount,
				"node_count":        nodeCount,
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
			CommandType:        commandCloneActiveVersion,
			RequestFingerprint: fingerprint,
			ResultType:         resultTypeVersion,
			ResultRef:          draft.ID,
			ResultRevision:     draft.Revision,
		},
	); err != nil {
		return Version{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Version{}, err
	}

	return draft, nil
}

func (s *Service) cloneVersionContent(
	ctx context.Context,
	tx pgx.Tx,
	source Version,
	target Version,
) (int, int, error) {
	sourceFacets, err := s.repo.ListFacetsTx(
		ctx,
		tx,
		source.ID,
	)
	if err != nil {
		return 0, 0, err
	}

	facetIDMap := make(map[string]string, len(sourceFacets))

	for _, sourceFacet := range sourceFacets {
		newID := "ctf_" + uuid.NewString()
		facetIDMap[sourceFacet.ID] = newID

		_, err = s.repo.InsertFacet(
			ctx,
			tx,
			Facet{
				ID:            newID,
				NamespaceID:   target.NamespaceID,
				VersionID:     target.ID,
				Code:          sourceFacet.Code,
				DisplayName:   sourceFacet.DisplayName,
				Description:   sourceFacet.Description,
				StructureType: sourceFacet.StructureType,
				Position:      sourceFacet.Position,
				Status:        sourceFacet.Status,
				Metadata:      sourceFacet.Metadata,
			},
		)
		if err != nil {
			return 0, 0, err
		}
	}

	sourceNodes, err := s.repo.ListNodesTx(
		ctx,
		tx,
		source.ID,
	)
	if err != nil {
		return 0, 0, err
	}

	nodeIDMap := make(map[string]string, len(sourceNodes))

	// Allocate all cloned identifiers first so parent relationships can be
	// deterministically remapped before insertion.
	for _, sourceNode := range sourceNodes {
		nodeIDMap[sourceNode.ID] = "ctn_" + uuid.NewString()
	}

	inserted := make(map[string]bool, len(sourceNodes))
	remaining := len(sourceNodes)

	for remaining > 0 {
		progress := false

		for _, sourceNode := range sourceNodes {
			if inserted[sourceNode.ID] {
				continue
			}

			var clonedParentID *string

			if sourceNode.ParentID != nil {
				parentID, exists := nodeIDMap[*sourceNode.ParentID]
				if !exists {
					return 0, 0, ErrConflict
				}

				// Parent must already exist because the composite FK is
				// enforced immediately by PostgreSQL.
				if !inserted[*sourceNode.ParentID] {
					continue
				}

				clonedParentID = &parentID
			}

			clonedFacetID, exists := facetIDMap[sourceNode.FacetID]
			if !exists {
				return 0, 0, ErrConflict
			}

			_, err = s.repo.InsertNode(
				ctx,
				tx,
				Node{
					ID:            nodeIDMap[sourceNode.ID],
					NamespaceID:   target.NamespaceID,
					VersionID:     target.ID,
					FacetID:       clonedFacetID,
					CanonicalCode: sourceNode.CanonicalCode,
					DisplayName:   sourceNode.DisplayName,
					Description:   sourceNode.Description,
					ParentID:      clonedParentID,
					Position:      sourceNode.Position,
					Status:        sourceNode.Status,
					Metadata:      sourceNode.Metadata,
				},
			)
			if err != nil {
				return 0, 0, err
			}

			inserted[sourceNode.ID] = true
			remaining--
			progress = true
		}

		if !progress {
			// A healthy published taxonomy cannot reach this point. It means
			// the source hierarchy contains an unresolved parent or cycle.
			return 0, 0, ErrConflict
		}
	}

	if err := s.cloneVersionVocabulary(

		ctx,

		tx,

		source,

		target,

		nodeIDMap,
	); err != nil {

		return 0, 0, err

	}

	return len(sourceFacets), len(sourceNodes), nil
}
