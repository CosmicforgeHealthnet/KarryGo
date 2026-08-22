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
	commandCreateNode = "CREATE_NODE"

	resultTypeNode = "TAXONOMY_NODE"

	changeNodeCreated = "NODE_CREATED"

	aggregateNode = "TAXONOMY_NODE"

	eventNodeCreated = "taxonomy.node.created"
)

type CreateNodeInput struct {
	VersionID     string
	FacetID       string
	CanonicalCode string
	DisplayName   string
	Description   string
	ParentID      *string
	Position      int
	Metadata      json.RawMessage
	Actor         Actor
}

func (s *Service) CreateNode(
	ctx context.Context,
	input CreateNodeInput,
) (Node, error) {
	if s == nil ||
		s.repo == nil ||
		input.VersionID == "" ||
		input.FacetID == "" ||
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

	metadata, err := normalizeMetadata(input.Metadata)
	if err != nil {
		return Node{}, err
	}
	input.Metadata = json.RawMessage(metadata)

	fingerprint, err := requestFingerprint(struct {
		VersionID     string          `json:"version_id"`
		FacetID       string          `json:"facet_id"`
		CanonicalCode string          `json:"canonical_code"`
		DisplayName   string          `json:"display_name"`
		Description   string          `json:"description"`
		ParentID      *string         `json:"parent_id"`
		Position      int             `json:"position"`
		Metadata      json.RawMessage `json:"metadata"`
	}{
		VersionID:     input.VersionID,
		FacetID:       input.FacetID,
		CanonicalCode: input.CanonicalCode,
		DisplayName:   input.DisplayName,
		Description:   input.Description,
		ParentID:      input.ParentID,
		Position:      input.Position,
		Metadata:      input.Metadata,
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

	version, err := s.repo.GetVersionByIDTx(
		ctx,
		tx,
		input.VersionID,
		true,
	)
	if err != nil {
		return Node{}, err
	}

	if version.Status != VersionStatusDraft {
		return Node{}, ErrNotDraft
	}

	replayed, err := s.replayNodeCommand(
		ctx,
		tx,
		version.NamespaceID,
		input.Actor.CommandRef,
		commandCreateNode,
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

	facet, err := s.repo.GetFacetByIDTx(
		ctx,
		tx,
		input.FacetID,
		true,
	)
	if err != nil {
		return Node{}, err
	}

	if facet.NamespaceID != version.NamespaceID ||
		facet.VersionID != version.ID {
		return Node{}, ErrConflict
	}

	if facet.Status != RecordStatusActive {
		return Node{}, ErrConflict
	}

	if facet.StructureType == FacetStructureFlat && input.ParentID != nil {
		return Node{}, ErrInvalidArgument
	}

	if input.ParentID != nil {
		parent, err := s.repo.GetNodeByIDTx(
			ctx,
			tx,
			*input.ParentID,
			false,
		)
		if err != nil {
			return Node{}, err
		}

		if parent.NamespaceID != version.NamespaceID ||
			parent.VersionID != version.ID ||
			parent.FacetID != facet.ID {
			return Node{}, ErrConflict
		}

		if parent.Status != RecordStatusActive {
			return Node{}, ErrConflict
		}
	}

	node, err := s.repo.InsertNode(
		ctx,
		tx,
		Node{
			ID:            "ctn_" + uuid.NewString(),
			NamespaceID:   version.NamespaceID,
			VersionID:     version.ID,
			FacetID:       facet.ID,
			CanonicalCode: input.CanonicalCode,
			DisplayName:   input.DisplayName,
			Description:   input.Description,
			ParentID:      input.ParentID,
			Position:      input.Position,
			Status:        RecordStatusActive,
			Metadata:      input.Metadata,
		},
	)
	if err != nil {
		return Node{}, err
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
			ChangeType:    changeNodeCreated,
			AggregateType: aggregateNode,
			AggregateRef:  node.ID,
			BeforeState:   nil,
			AfterState:    node,
			CorrelationID: input.Actor.CorrelationID,
			CausationID:   input.Actor.CausationID,
		},
	); err != nil {
		return Node{}, err
	}

	if s.publisher != nil {
		if err = s.publisher.Publish(
			ctx,
			tx,
			eventNodeCreated,
			aggregateNode,
			node.ID,
			node.Revision,
			"",
			input.Actor.CorrelationID,
			input.Actor.CausationID,
			map[string]any{
				"namespace_id":   node.NamespaceID,
				"version_id":     node.VersionID,
				"facet_id":       node.FacetID,
				"node_id":        node.ID,
				"canonical_code": node.CanonicalCode,
				"display_name":   node.DisplayName,
				"parent_id":      node.ParentID,
				"status":         node.Status,
				"revision":       node.Revision,
			},
		); err != nil {
			return Node{}, err
		}
	}

	if err = s.repo.InsertCommand(
		ctx,
		tx,
		CommandRecord{
			ID:                 "ctcmd_" + uuid.NewString(),
			NamespaceID:        version.NamespaceID,
			CommandRef:         input.Actor.CommandRef,
			CommandType:        commandCreateNode,
			RequestFingerprint: fingerprint,
			ResultType:         resultTypeNode,
			ResultRef:          node.ID,
			ResultRevision:     node.Revision,
		},
	); err != nil {
		return Node{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Node{}, err
	}

	return node, nil
}

func (s *Service) GetNode(
	ctx context.Context,
	nodeID string,
) (Node, error) {
	if s == nil || s.repo == nil || nodeID == "" {
		return Node{}, ErrInvalidArgument
	}

	return s.repo.GetNodeByID(ctx, nodeID)
}

func (s *Service) ListNodes(
	ctx context.Context,
	versionID string,
) ([]Node, error) {
	if s == nil || s.repo == nil || versionID == "" {
		return nil, ErrInvalidArgument
	}

	return s.repo.ListNodes(ctx, versionID)
}

func (s *Service) ListNodesByFacet(
	ctx context.Context,
	versionID string,
	facetID string,
) ([]Node, error) {
	if s == nil ||
		s.repo == nil ||
		versionID == "" ||
		facetID == "" {
		return nil, ErrInvalidArgument
	}

	return s.repo.ListNodesByFacet(ctx, versionID, facetID)
}

func (s *Service) replayNodeCommand(
	ctx context.Context,
	tx pgx.Tx,
	namespaceID string,
	commandRef string,
	commandType string,
	fingerprint string,
) (Node, error) {
	record, err := s.repo.GetCommandTx(
		ctx,
		tx,
		namespaceID,
		commandRef,
	)
	if err != nil {
		return Node{}, err
	}

	if record.CommandType != commandType ||
		record.RequestFingerprint != fingerprint ||
		record.ResultType != resultTypeNode {
		return Node{}, ErrIdempotency
	}

	node, err := s.repo.GetNodeByIDTx(
		ctx,
		tx,
		record.ResultRef,
		false,
	)
	if err != nil {
		return Node{}, err
	}

	if node.NamespaceID != namespaceID ||
		node.Revision < record.ResultRevision {
		return Node{}, ErrConflict
	}

	return node, nil
}
