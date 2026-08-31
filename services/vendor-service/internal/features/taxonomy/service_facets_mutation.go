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
	commandUpdateFacet  = "UPDATE_FACET"
	commandArchiveFacet = "ARCHIVE_FACET"
	commandRestoreFacet = "RESTORE_FACET"

	changeFacetUpdated  = "FACET_UPDATED"
	changeFacetArchived = "FACET_ARCHIVED"
	changeFacetRestored = "FACET_RESTORED"

	eventFacetUpdated  = "taxonomy.facet.updated"
	eventFacetArchived = "taxonomy.facet.archived"
	eventFacetRestored = "taxonomy.facet.restored"
)

type UpdateFacetInput struct {
	FacetID          string
	Code             string
	DisplayName      string
	Description      string
	StructureType    FacetStructureType
	Position         int
	Metadata         json.RawMessage
	ExpectedRevision int64
	Actor            Actor
}

type FacetStatusInput struct {
	FacetID          string
	ExpectedRevision int64
	Actor            Actor
}

// UpdateFacet changes mutable facet content on a DRAFT taxonomy version.
func (s *Service) UpdateFacet(
	ctx context.Context,
	input UpdateFacetInput,
) (Facet, error) {
	if s == nil ||
		s.repo == nil ||
		input.FacetID == "" ||
		input.ExpectedRevision <= 0 ||
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
		FacetID          string             `json:"facet_id"`
		Code             string             `json:"code"`
		DisplayName      string             `json:"display_name"`
		Description      string             `json:"description"`
		StructureType    FacetStructureType `json:"structure_type"`
		Position         int                `json:"position"`
		Metadata         json.RawMessage    `json:"metadata"`
		ExpectedRevision int64              `json:"expected_revision"`
	}{
		FacetID:          input.FacetID,
		Code:             input.Code,
		DisplayName:      input.DisplayName,
		Description:      input.Description,
		StructureType:    input.StructureType,
		Position:         input.Position,
		Metadata:         input.Metadata,
		ExpectedRevision: input.ExpectedRevision,
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

	version, current, err := s.loadDraftFacetForMutation(
		ctx,
		tx,
		input.FacetID,
	)
	if err != nil {
		return Facet{}, err
	}

	replayed, err := s.replayFacetCommand(
		ctx,
		tx,
		version.NamespaceID,
		input.Actor.CommandRef,
		commandUpdateFacet,
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

	if current.Revision != input.ExpectedRevision {
		return Facet{}, ErrVersionMismatch
	}

	if current.Status != RecordStatusActive {
		return Facet{}, ErrConflict
	}

	next := current
	next.Code = input.Code
	next.DisplayName = input.DisplayName
	next.Description = input.Description
	next.StructureType = input.StructureType
	next.Position = input.Position
	next.Metadata = input.Metadata

	updated, err := s.repo.UpdateFacet(
		ctx,
		tx,
		next,
		input.ExpectedRevision,
	)
	if err != nil {
		return Facet{}, err
	}

	if err = s.recordFacetMutation(
		ctx,
		tx,
		input.Actor,
		version,
		current,
		updated,
		commandUpdateFacet,
		fingerprint,
		changeFacetUpdated,
		eventFacetUpdated,
	); err != nil {
		return Facet{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Facet{}, err
	}

	return updated, nil
}

// ArchiveFacet removes a facet from active discovery configuration while
// retaining it as immutable draft history.
func (s *Service) ArchiveFacet(
	ctx context.Context,
	input FacetStatusInput,
) (Facet, error) {
	return s.changeFacetStatus(
		ctx,
		input,
		RecordStatusArchived,
		commandArchiveFacet,
		changeFacetArchived,
		eventFacetArchived,
	)
}

// RestoreFacet reactivates an archived facet while the version remains DRAFT.
func (s *Service) RestoreFacet(
	ctx context.Context,
	input FacetStatusInput,
) (Facet, error) {
	return s.changeFacetStatus(
		ctx,
		input,
		RecordStatusActive,
		commandRestoreFacet,
		changeFacetRestored,
		eventFacetRestored,
	)
}

func (s *Service) changeFacetStatus(
	ctx context.Context,
	input FacetStatusInput,
	targetStatus RecordStatus,
	commandType string,
	changeType string,
	eventType string,
) (Facet, error) {
	if s == nil ||
		s.repo == nil ||
		input.FacetID == "" ||
		input.ExpectedRevision <= 0 ||
		input.Actor.ActorRef == "" ||
		input.Actor.CommandRef == "" {
		return Facet{}, ErrInvalidArgument
	}

	fingerprint, err := requestFingerprint(struct {
		FacetID          string       `json:"facet_id"`
		TargetStatus     RecordStatus `json:"target_status"`
		ExpectedRevision int64        `json:"expected_revision"`
	}{
		FacetID:          input.FacetID,
		TargetStatus:     targetStatus,
		ExpectedRevision: input.ExpectedRevision,
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

	version, current, err := s.loadDraftFacetForMutation(
		ctx,
		tx,
		input.FacetID,
	)
	if err != nil {
		return Facet{}, err
	}

	replayed, err := s.replayFacetCommand(
		ctx,
		tx,
		version.NamespaceID,
		input.Actor.CommandRef,
		commandType,
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

	if current.Revision != input.ExpectedRevision {
		return Facet{}, ErrVersionMismatch
	}

	if current.Status == targetStatus {
		return Facet{}, ErrConflict
	}

	next := current
	next.Status = targetStatus

	updated, err := s.repo.UpdateFacet(
		ctx,
		tx,
		next,
		input.ExpectedRevision,
	)
	if err != nil {
		return Facet{}, err
	}

	if err = s.recordFacetMutation(
		ctx,
		tx,
		input.Actor,
		version,
		current,
		updated,
		commandType,
		fingerprint,
		changeType,
		eventType,
	); err != nil {
		return Facet{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Facet{}, err
	}

	return updated, nil
}

func (s *Service) loadDraftFacetForMutation(
	ctx context.Context,
	tx pgx.Tx,
	facetID string,
) (Version, Facet, error) {
	initial, err := s.repo.GetFacetByIDTx(
		ctx,
		tx,
		facetID,
		false,
	)
	if err != nil {
		return Version{}, Facet{}, err
	}

	version, err := s.repo.GetVersionByIDTx(
		ctx,
		tx,
		initial.VersionID,
		true,
	)
	if err != nil {
		return Version{}, Facet{}, err
	}

	if version.Status != VersionStatusDraft {
		return Version{}, Facet{}, ErrNotDraft
	}

	facet, err := s.repo.GetFacetByIDTx(
		ctx,
		tx,
		facetID,
		true,
	)
	if err != nil {
		return Version{}, Facet{}, err
	}

	if facet.VersionID != version.ID ||
		facet.NamespaceID != version.NamespaceID {
		return Version{}, Facet{}, ErrConflict
	}

	return version, facet, nil
}

func (s *Service) recordFacetMutation(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	version Version,
	before Facet,
	after Facet,
	commandType string,
	fingerprint string,
	changeType string,
	eventType string,
) error {
	if err := s.repo.InsertChangeHistory(
		ctx,
		tx,
		ChangeHistoryEntry{
			ID:            "ctchg_" + uuid.NewString(),
			NamespaceID:   version.NamespaceID,
			VersionID:     version.ID,
			ActorRef:      actor.ActorRef,
			CommandRef:    actor.CommandRef,
			ChangeType:    changeType,
			AggregateType: aggregateFacet,
			AggregateRef:  after.ID,
			BeforeState:   before,
			AfterState:    after,
			CorrelationID: actor.CorrelationID,
			CausationID:   actor.CausationID,
		},
	); err != nil {
		return err
	}

	if s.publisher != nil {
		if err := s.publisher.Publish(
			ctx,
			tx,
			eventType,
			aggregateFacet,
			after.ID,
			after.Revision,
			"",
			actor.CorrelationID,
			actor.CausationID,
			map[string]any{
				"namespace_id":   after.NamespaceID,
				"version_id":     after.VersionID,
				"facet_id":       after.ID,
				"code":           after.Code,
				"display_name":   after.DisplayName,
				"structure_type": after.StructureType,
				"status":         after.Status,
				"revision":       after.Revision,
			},
		); err != nil {
			return err
		}
	}

	return s.repo.InsertCommand(
		ctx,
		tx,
		CommandRecord{
			ID:                 "ctcmd_" + uuid.NewString(),
			NamespaceID:        version.NamespaceID,
			CommandRef:         actor.CommandRef,
			CommandType:        commandType,
			RequestFingerprint: fingerprint,
			ResultType:         resultTypeFacet,
			ResultRef:          after.ID,
			ResultRevision:     after.Revision,
		},
	)
}
