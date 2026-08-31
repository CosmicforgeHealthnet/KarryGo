package taxonomy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
)

type CreateMappingInput struct {
	NamespaceCode NamespaceCode

	VersionID string
	NodeID    string

	SourceType string
	SourceRef  string

	MappingType MappingType

	ConfidenceBasis MappingConfidenceBasis
	ConfidenceScore *int

	BusinessRef *string
	BranchRef   *string

	Metadata json.RawMessage

	Actor Actor
}

func deterministicMappingID(
	input CreateMappingInput,
) string {
	raw := strings.Join(
		[]string{
			string(input.NamespaceCode),
			input.VersionID,
			input.NodeID,
			strings.ToUpper(
				strings.TrimSpace(input.SourceType),
			),
			strings.TrimSpace(input.SourceRef),
			string(input.MappingType),
		},
		"|",
	)

	sum := sha256.Sum256([]byte(raw))

	return "ctmap_" +
		hex.EncodeToString(sum[:16])
}

func (s *Service) CreateMapping(
	ctx context.Context,
	input CreateMappingInput,
) (TaxonomyMapping, error) {
	if s == nil ||
		s.repo == nil ||
		strings.TrimSpace(input.VersionID) == "" ||
		strings.TrimSpace(input.NodeID) == "" ||
		strings.TrimSpace(input.SourceType) == "" ||
		strings.TrimSpace(input.SourceRef) == "" ||
		strings.TrimSpace(input.Actor.ActorRef) == "" ||
		strings.TrimSpace(input.Actor.CommandRef) == "" {
		return TaxonomyMapping{}, ErrInvalidArgument
	}

	switch input.MappingType {
	case MappingTypePrimary,
		MappingTypeSecondary,
		MappingTypeDiscovery:
	default:
		return TaxonomyMapping{}, ErrInvalidArgument
	}

	switch input.ConfidenceBasis {
	case MappingConfidenceManual,
		MappingConfidenceRule,
		MappingConfidenceImport,
		MappingConfidenceSystem:
	default:
		return TaxonomyMapping{}, ErrInvalidArgument
	}

	if input.ConfidenceScore != nil &&
		(*input.ConfidenceScore < 0 ||
			*input.ConfidenceScore > 10000) {
		return TaxonomyMapping{}, ErrInvalidArgument
	}

	tx, err := s.repo.pool.BeginTx(
		ctx,
		pgx.TxOptions{
			IsoLevel: pgx.Serializable,
		},
	)
	if err != nil {
		return TaxonomyMapping{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var namespaceID string

	err = tx.QueryRow(
		ctx,
		`
        SELECT id
          FROM canonical_taxonomy_namespaces
         WHERE code=$1
           AND status='ACTIVE'
        `,
		input.NamespaceCode,
	).Scan(&namespaceID)
	if err != nil {
		return TaxonomyMapping{}, err
	}

	id := deterministicMappingID(input)

	metadata := input.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}

	var mapping TaxonomyMapping

	err = tx.QueryRow(
		ctx,
		`
        INSERT INTO canonical_taxonomy_mappings (
            id,
            namespace_id,
            version_id,
            node_id,
            source_type,
            source_ref,
            mapping_type,
            status,
            confidence_basis,
            confidence_score,
            business_ref,
            branch_ref,
            metadata,
            revision,
            created_by_actor_ref,
            updated_by_actor_ref
        )
        VALUES (
            $1,$2,$3,$4,$5,$6,$7,
            'ACTIVE',$8,$9,
            NULLIF($10,''),
            NULLIF($11,''),
            $12,1,$13,$13
        )
        RETURNING
            id,
            namespace_id,
            version_id,
            node_id,
            source_type,
            source_ref,
            mapping_type,
            status,
            confidence_basis,
            confidence_score,
            business_ref,
            branch_ref,
            metadata,
            revision,
            created_by_actor_ref,
            updated_by_actor_ref,
            created_at,
            updated_at
        `,
		id,
		namespaceID,
		input.VersionID,
		input.NodeID,
		strings.ToUpper(
			strings.TrimSpace(input.SourceType),
		),
		strings.TrimSpace(input.SourceRef),
		input.MappingType,
		input.ConfidenceBasis,
		input.ConfidenceScore,
		valueOrEmpty(input.BusinessRef),
		valueOrEmpty(input.BranchRef),
		metadata,
		input.Actor.ActorRef,
	).Scan(
		&mapping.ID,
		&mapping.NamespaceID,
		&mapping.VersionID,
		&mapping.NodeID,
		&mapping.SourceType,
		&mapping.SourceRef,
		&mapping.MappingType,
		&mapping.Status,
		&mapping.ConfidenceBasis,
		&mapping.ConfidenceScore,
		&mapping.BusinessRef,
		&mapping.BranchRef,
		&mapping.Metadata,
		&mapping.Revision,
		&mapping.CreatedByActorRef,
		&mapping.UpdatedByActorRef,
		&mapping.CreatedAt,
		&mapping.UpdatedAt,
	)
	if err != nil {
		return TaxonomyMapping{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return TaxonomyMapping{}, err
	}

	return mapping, nil
}

func valueOrEmpty(
	value *string,
) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}
