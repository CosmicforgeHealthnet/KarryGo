package taxonomy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	commandCreateFacet = "CREATE_FACET"

	resultTypeFacet = "TAXONOMY_FACET"

	changeFacetCreated = "FACET_CREATED"

	aggregateFacet = "TAXONOMY_FACET"

	eventFacetCreated = "taxonomy.facet.created"
)

type CreateFacetInput struct {
	VersionID     string
	Code          string
	DisplayName   string
	Description   string
	StructureType FacetStructureType
	Position      int
	Metadata      json.RawMessage
	Actor         Actor
}

func (s *Service) CreateFacet(
	ctx context.Context,
	input CreateFacetInput,
) (Facet, error) {
	if s == nil ||
		s.repo == nil ||
		input.VersionID == "" ||
		input.Actor.ActorRef == "" ||
		input.Actor.CommandRef == "" {
		return Facet{}, ErrInvalidArgument
	}

	input.Code = strings.ToUpper(strings.TrimSpace(input.Code))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Description = strings.TrimSpace(input.Description)

	if input.Code == "" ||
		input.DisplayName == "" ||
		input.Position < 0 {
		return Facet{}, ErrInvalidArgument
	}

	if input.StructureType == "" {
		input.StructureType = FacetStructureFlat
	}

	if input.StructureType != FacetStructureFlat &&
		input.StructureType != FacetStructureHierarchical {
		return Facet{}, ErrInvalidArgument
	}

	metadata, err := normalizeMetadata(input.Metadata)
	if err != nil {
		return Facet{}, err
	}
	input.Metadata = json.RawMessage(metadata)

	fingerprint, err := requestFingerprint(struct {
		VersionID     string             `json:"version_id"`
		Code          string             `json:"code"`
		DisplayName   string             `json:"display_name"`
		Description   string             `json:"description"`
		StructureType FacetStructureType `json:"structure_type"`
		Position      int                `json:"position"`
		Metadata      json.RawMessage    `json:"metadata"`
	}{
		VersionID:     input.VersionID,
		Code:          input.Code,
		DisplayName:   input.DisplayName,
		Description:   input.Description,
		StructureType: input.StructureType,
		Position:      input.Position,
		Metadata:      input.Metadata,
	})
	if err != nil {
		return Facet{}, err
	}

	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return Facet{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	version, err := s.repo.GetVersionByIDTx(
		ctx,
		tx,
		input.VersionID,
		true,
	)
	if err != nil {
		return Facet{}, err
	}

	if version.Status != VersionStatusDraft {
		return Facet{}, ErrNotDraft
	}

	replayed, err := s.replayFacetCommand(
		ctx,
		tx,
		version.NamespaceID,
		input.Actor.CommandRef,
		commandCreateFacet,
		fingerprint,
	)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return Facet{}, err
		}
		return replayed, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Facet{}, err
	}

	facet, err := s.repo.InsertFacet(
		ctx,
		tx,
		Facet{
			ID:            "ctf_" + uuid.NewString(),
			NamespaceID:   version.NamespaceID,
			VersionID:     version.ID,
			Code:          input.Code,
			DisplayName:   input.DisplayName,
			Description:   input.Description,
			StructureType: input.StructureType,
			Position:      input.Position,
			Status:        RecordStatusActive,
			Metadata:      input.Metadata,
		},
	)
	if err != nil {
		return Facet{}, err
	}

	if err = s.repo.InsertChangeHistory(
		ctx,
		tx,
		ChangeHistoryEntry{
			ID:            "ctchg_" + uuid.NewString(),
			NamespaceID:   version.NamespaceID,
			VersionID:     version.ID,
			ActorRef:      input.Actor.ActorRef,
			CommandRef:    input.Actor.CommandRef,
			ChangeType:    changeFacetCreated,
			AggregateType: aggregateFacet,
			AggregateRef:  facet.ID,
			BeforeState:   nil,
			AfterState:    facet,
			CorrelationID: input.Actor.CorrelationID,
			CausationID:   input.Actor.CausationID,
		},
	); err != nil {
		return Facet{}, err
	}

	if s.publisher != nil {
		if err = s.publisher.Publish(
			ctx,
			tx,
			eventFacetCreated,
			aggregateFacet,
			facet.ID,
			facet.Revision,
			"",
			input.Actor.CorrelationID,
			input.Actor.CausationID,
			map[string]any{
				"namespace_id":   facet.NamespaceID,
				"version_id":     facet.VersionID,
				"facet_id":       facet.ID,
				"code":           facet.Code,
				"display_name":   facet.DisplayName,
				"structure_type": facet.StructureType,
				"status":         facet.Status,
			},
		); err != nil {
			return Facet{}, err
		}
	}

	if err = s.repo.InsertCommand(
		ctx,
		tx,
		CommandRecord{
			ID:                 "ctcmd_" + uuid.NewString(),
			NamespaceID:        version.NamespaceID,
			CommandRef:         input.Actor.CommandRef,
			CommandType:        commandCreateFacet,
			RequestFingerprint: fingerprint,
			ResultType:         resultTypeFacet,
			ResultRef:          facet.ID,
			ResultRevision:     facet.Revision,
		},
	); err != nil {
		return Facet{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Facet{}, err
	}

	return facet, nil
}

func (s *Service) GetFacet(
	ctx context.Context,
	facetID string,
) (Facet, error) {
	if s == nil || s.repo == nil || facetID == "" {
		return Facet{}, ErrInvalidArgument
	}

	return s.repo.GetFacetByID(ctx, facetID)
}

func (s *Service) ListFacets(
	ctx context.Context,
	versionID string,
) ([]Facet, error) {
	if s == nil || s.repo == nil || versionID == "" {
		return nil, ErrInvalidArgument
	}

	return s.repo.ListFacets(ctx, versionID)
}

func (s *Service) replayFacetCommand(
	ctx context.Context,
	tx pgx.Tx,
	namespaceID string,
	commandRef string,
	commandType string,
	fingerprint string,
) (Facet, error) {
	record, err := s.repo.GetCommandTx(
		ctx,
		tx,
		namespaceID,
		commandRef,
	)
	if err != nil {
		return Facet{}, err
	}

	if record.CommandType != commandType ||
		record.RequestFingerprint != fingerprint ||
		record.ResultType != resultTypeFacet {
		return Facet{}, ErrIdempotency
	}

	facet, err := s.repo.GetFacetByIDTx(
		ctx,
		tx,
		record.ResultRef,
		false,
	)
	if err != nil {
		return Facet{}, err
	}

	if facet.NamespaceID != namespaceID ||
		facet.Revision < record.ResultRevision {
		return Facet{}, ErrConflict
	}

	return facet, nil
}
