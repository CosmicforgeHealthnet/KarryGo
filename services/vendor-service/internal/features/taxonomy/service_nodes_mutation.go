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
	commandUpdateNode  = "UPDATE_NODE"
	commandArchiveNode = "ARCHIVE_NODE"
	commandRestoreNode = "RESTORE_NODE"

	changeNodeUpdated  = "NODE_UPDATED"
	changeNodeArchived = "NODE_ARCHIVED"
	changeNodeRestored = "NODE_RESTORED"

	eventNodeUpdated  = "taxonomy.node.updated"
	eventNodeArchived = "taxonomy.node.archived"
	eventNodeRestored = "taxonomy.node.restored"
)

type UpdateNodeInput struct {
	NodeID           string
	CanonicalCode    string
	DisplayName      string
	Description      string
	ParentID         *string
	Position         int
	Metadata         json.RawMessage
	ExpectedRevision int64
	Actor            Actor
}

type NodeStatusInput struct {
	NodeID           string
	ExpectedRevision int64
	Actor            Actor
}

// UpdateNode changes mutable canonical-node content while the owning taxonomy
// version remains DRAFT.
func (s *Service) UpdateNode(
	ctx context.Context,
	input UpdateNodeInput,
) (Node, error) {
	if s == nil ||
		s.repo == nil ||
		input.NodeID == "" ||
		input.ExpectedRevision <= 0 ||
		input.Actor.ActorRef == "" ||
		input.Actor.CommandRef == "" {
		return Node{}, ErrInvalidArgument
	}

	input.CanonicalCode = strings.ToUpper(strings.TrimSpace(input.CanonicalCode))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Description = strings.TrimSpace(input.Description)

	if input.CanonicalCode == "" ||
		input.DisplayName == "" ||
		input.Position < 0 {
		return Node{}, ErrInvalidArgument
	}

	if input.ParentID != nil {
		parent := strings.TrimSpace(*input.ParentID)

		if parent == "" {
			input.ParentID = nil
		} else {
			input.ParentID = &parent
		}
	}

	if input.ParentID != nil && *input.ParentID == input.NodeID {
		return Node{}, ErrInvalidArgument
	}

	metadata, err := normalizeMetadata(input.Metadata)
	if err != nil {
		return Node{}, err
	}
	input.Metadata = json.RawMessage(metadata)

	fingerprint, err := requestFingerprint(struct {
		NodeID           string          `json:"node_id"`
		CanonicalCode    string          `json:"canonical_code"`
		DisplayName      string          `json:"display_name"`
		Description      string          `json:"description"`
		ParentID         *string         `json:"parent_id"`
		Position         int             `json:"position"`
		Metadata         json.RawMessage `json:"metadata"`
		ExpectedRevision int64           `json:"expected_revision"`
	}{
		NodeID:           input.NodeID,
		CanonicalCode:    input.CanonicalCode,
		DisplayName:      input.DisplayName,
		Description:      input.Description,
		ParentID:         input.ParentID,
		Position:         input.Position,
		Metadata:         input.Metadata,
		ExpectedRevision: input.ExpectedRevision,
	})
	if err != nil {
		return Node{}, err
	}

	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return Node{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	version, facet, current, err := s.loadDraftNodeForMutation(
		ctx,
		tx,
		input.NodeID,
	)
	if err != nil {
		return Node{}, err
	}

	replayed, err := s.replayNodeCommand(
		ctx,
		tx,
		version.NamespaceID,
		input.Actor.CommandRef,
		commandUpdateNode,
		fingerprint,
	)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return Node{}, err
		}
		return replayed, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Node{}, err
	}

	if current.Revision != input.ExpectedRevision {
		return Node{}, ErrVersionMismatch
	}

	if current.Status != RecordStatusActive ||
		facet.Status != RecordStatusActive {
		return Node{}, ErrConflict
	}

	if err = s.validateNodeParent(
		ctx,
		tx,
		version,
		facet,
		current.ID,
		input.ParentID,
	); err != nil {
		return Node{}, err
	}

	next := current
	next.CanonicalCode = input.CanonicalCode
	next.DisplayName = input.DisplayName
	next.Description = input.Description
	next.ParentID = input.ParentID
	next.Position = input.Position
	next.Metadata = input.Metadata

	updated, err := s.repo.UpdateNode(
		ctx,
		tx,
		next,
		input.ExpectedRevision,
	)
	if err != nil {
		return Node{}, err
	}

	if err = s.recordNodeMutation(
		ctx,
		tx,
		input.Actor,
		version,
		current,
		updated,
		commandUpdateNode,
		fingerprint,
		changeNodeUpdated,
		eventNodeUpdated,
	); err != nil {
		return Node{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Node{}, err
	}

	return updated, nil
}

// ArchiveNode removes a canonical node from active discovery while preserving
// its draft history. Active children must be handled first.
func (s *Service) ArchiveNode(
	ctx context.Context,
	input NodeStatusInput,
) (Node, error) {
	return s.changeNodeStatus(
		ctx,
		input,
		RecordStatusArchived,
		commandArchiveNode,
		changeNodeArchived,
		eventNodeArchived,
	)
}

// RestoreNode reactivates an archived node. Its facet and parent, if any, must
// already be active.
func (s *Service) RestoreNode(
	ctx context.Context,
	input NodeStatusInput,
) (Node, error) {
	return s.changeNodeStatus(
		ctx,
		input,
		RecordStatusActive,
		commandRestoreNode,
		changeNodeRestored,
		eventNodeRestored,
	)
}

func (s *Service) changeNodeStatus(
	ctx context.Context,
	input NodeStatusInput,
	targetStatus RecordStatus,
	commandType string,
	changeType string,
	eventType string,
) (Node, error) {
	if s == nil ||
		s.repo == nil ||
		input.NodeID == "" ||
		input.ExpectedRevision <= 0 ||
		input.Actor.ActorRef == "" ||
		input.Actor.CommandRef == "" {
		return Node{}, ErrInvalidArgument
	}

	fingerprint, err := requestFingerprint(struct {
		NodeID           string       `json:"node_id"`
		TargetStatus     RecordStatus `json:"target_status"`
		ExpectedRevision int64        `json:"expected_revision"`
	}{
		NodeID:           input.NodeID,
		TargetStatus:     targetStatus,
		ExpectedRevision: input.ExpectedRevision,
	})
	if err != nil {
		return Node{}, err
	}

	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return Node{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	version, facet, current, err := s.loadDraftNodeForMutation(
		ctx,
		tx,
		input.NodeID,
	)
	if err != nil {
		return Node{}, err
	}

	replayed, err := s.replayNodeCommand(
		ctx,
		tx,
		version.NamespaceID,
		input.Actor.CommandRef,
		commandType,
		fingerprint,
	)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return Node{}, err
		}
		return replayed, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Node{}, err
	}

	if current.Revision != input.ExpectedRevision {
		return Node{}, ErrVersionMismatch
	}

	if current.Status == targetStatus {
		return Node{}, ErrConflict
	}

	switch targetStatus {
	case RecordStatusArchived:
		hasChildren, checkErr := s.repo.HasActiveChildrenTx(
			ctx,
			tx,
			current.ID,
		)
		if checkErr != nil {
			return Node{}, checkErr
		}

		if hasChildren {
			return Node{}, ErrConflict
		}

	case RecordStatusActive:
		if facet.Status != RecordStatusActive {
			return Node{}, ErrConflict
		}

		if err = s.validateNodeParent(
			ctx,
			tx,
			version,
			facet,
			current.ID,
			current.ParentID,
		); err != nil {
			return Node{}, err
		}

	default:
		return Node{}, ErrInvalidArgument
	}

	next := current
	next.Status = targetStatus

	updated, err := s.repo.UpdateNode(
		ctx,
		tx,
		next,
		input.ExpectedRevision,
	)
	if err != nil {
		return Node{}, err
	}

	if err = s.recordNodeMutation(
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
		return Node{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Node{}, err
	}

	return updated, nil
}

func (s *Service) loadDraftNodeForMutation(
	ctx context.Context,
	tx pgx.Tx,
	nodeID string,
) (Version, Facet, Node, error) {
	initial, err := s.repo.GetNodeByIDTx(
		ctx,
		tx,
		nodeID,
		false,
	)
	if err != nil {
		return Version{}, Facet{}, Node{}, err
	}

	version, err := s.repo.GetVersionByIDTx(
		ctx,
		tx,
		initial.VersionID,
		true,
	)
	if err != nil {
		return Version{}, Facet{}, Node{}, err
	}

	if version.Status != VersionStatusDraft {
		return Version{}, Facet{}, Node{}, ErrNotDraft
	}

	node, err := s.repo.GetNodeByIDTx(
		ctx,
		tx,
		nodeID,
		true,
	)
	if err != nil {
		return Version{}, Facet{}, Node{}, err
	}

	if node.VersionID != version.ID ||
		node.NamespaceID != version.NamespaceID {
		return Version{}, Facet{}, Node{}, ErrConflict
	}

	facet, err := s.repo.GetFacetByIDTx(
		ctx,
		tx,
		node.FacetID,
		false,
	)
	if err != nil {
		return Version{}, Facet{}, Node{}, err
	}

	if facet.NamespaceID != version.NamespaceID ||
		facet.VersionID != version.ID ||
		facet.ID != node.FacetID {
		return Version{}, Facet{}, Node{}, ErrConflict
	}

	return version, facet, node, nil
}

func (s *Service) validateNodeParent(
	ctx context.Context,
	tx pgx.Tx,
	version Version,
	facet Facet,
	nodeID string,
	parentID *string,
) error {
	if facet.StructureType == FacetStructureFlat {
		if parentID != nil {
			return ErrInvalidArgument
		}
		return nil
	}

	if parentID == nil {
		return nil
	}

	if *parentID == nodeID {
		return ErrInvalidArgument
	}

	parent, err := s.repo.GetNodeByIDTx(
		ctx,
		tx,
		*parentID,
		false,
	)
	if err != nil {
		return err
	}

	if parent.NamespaceID != version.NamespaceID ||
		parent.VersionID != version.ID ||
		parent.FacetID != facet.ID {
		return ErrConflict
	}

	if parent.Status != RecordStatusActive {
		return ErrConflict
	}

	// Full ancestry cycle and maximum-depth validation remains enforced by
	// canonical_taxonomy_node_guard in PostgreSQL.
	return nil
}

func (s *Service) recordNodeMutation(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	version Version,
	before Node,
	after Node,
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
			AggregateType: aggregateNode,
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
			aggregateNode,
			after.ID,
			after.Revision,
			"",
			actor.CorrelationID,
			actor.CausationID,
			map[string]any{
				"namespace_id":   after.NamespaceID,
				"version_id":     after.VersionID,
				"facet_id":       after.FacetID,
				"node_id":        after.ID,
				"canonical_code": after.CanonicalCode,
				"display_name":   after.DisplayName,
				"parent_id":      after.ParentID,
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
			ResultType:         resultTypeNode,
			ResultRef:          after.ID,
			ResultRevision:     after.Revision,
		},
	)
}
