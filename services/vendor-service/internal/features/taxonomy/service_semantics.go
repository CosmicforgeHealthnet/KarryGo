package taxonomy

import (
	"context"
	"encoding/json"
	"strings"
)

type AssignTagInput struct {
	NamespaceCode NamespaceCode
	TagCode       string

	SourceType string
	SourceRef  string

	BusinessRef *string
	BranchRef   *string

	Metadata json.RawMessage

	Actor Actor
}

func (s *Service) AssignTag(
	ctx context.Context,
	input AssignTagInput,
) (string, error) {
	if s == nil ||
		s.repo == nil ||
		strings.TrimSpace(input.SourceType) == "" ||
		strings.TrimSpace(input.SourceRef) == "" ||
		strings.TrimSpace(input.Actor.ActorRef) == "" {
		return "", ErrInvalidArgument
	}

	if err := validateSemanticCode(input.TagCode); err != nil {
		return "", err
	}

	var namespaceID string
	var tagID string

	err := s.repo.pool.QueryRow(
		ctx,
		`
        SELECT ns.id, tag.id
          FROM canonical_taxonomy_namespaces ns
          JOIN canonical_taxonomy_tags tag
            ON tag.namespace_id=ns.id
         WHERE ns.code=$1
           AND ns.status='ACTIVE'
           AND tag.code=$2
           AND tag.status='ACTIVE'
        `,
		input.NamespaceCode,
		input.TagCode,
	).Scan(
		&namespaceID,
		&tagID,
	)
	if err != nil {
		return "", err
	}

	id := semanticObjectID(
		"cttagas_",
		string(input.NamespaceCode),
		tagID,
		strings.ToUpper(
			strings.TrimSpace(input.SourceType),
		),
		strings.TrimSpace(input.SourceRef),
	)

	metadata := input.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}

	_, err = s.repo.pool.Exec(
		ctx,
		`
        INSERT INTO canonical_taxonomy_tag_assignments (
            id,
            namespace_id,
            tag_id,
            source_type,
            source_ref,
            business_ref,
            branch_ref,
            status,
            metadata,
            revision,
            created_by_actor_ref,
            updated_by_actor_ref
        )
        VALUES (
            $1,$2,$3,$4,$5,
            NULLIF($6,''),
            NULLIF($7,''),
            'ACTIVE',$8,1,$9,$9
        )
        ON CONFLICT (
            namespace_id,
            tag_id,
            source_type,
            source_ref
        )
        DO NOTHING
        `,
		id,
		namespaceID,
		tagID,
		strings.ToUpper(
			strings.TrimSpace(input.SourceType),
		),
		strings.TrimSpace(input.SourceRef),
		valueOrEmpty(input.BusinessRef),
		valueOrEmpty(input.BranchRef),
		metadata,
		input.Actor.ActorRef,
	)

	if err != nil {
		return "", err
	}

	return id, nil
}

type AssignDietaryClaimInput struct {
	NamespaceCode NamespaceCode
	ClaimCode     string

	SourceType string
	SourceRef  string

	BusinessRef *string
	BranchRef   *string

	Status DietaryClaimAssignmentStatus

	EvidenceRef *string

	VerifiedByActorRef *string

	Metadata json.RawMessage

	Actor Actor
}

func (s *Service) AssignDietaryClaim(
	ctx context.Context,
	input AssignDietaryClaimInput,
) (string, error) {
	if s == nil ||
		s.repo == nil ||
		strings.TrimSpace(input.SourceType) == "" ||
		strings.TrimSpace(input.SourceRef) == "" ||
		strings.TrimSpace(input.Actor.ActorRef) == "" {
		return "", ErrInvalidArgument
	}

	if err := validateSemanticCode(input.ClaimCode); err != nil {
		return "", err
	}

	switch input.Status {
	case DietaryAssignmentDeclared,
		DietaryAssignmentPendingVerification,
		DietaryAssignmentVerified,
		DietaryAssignmentRejected,
		DietaryAssignmentExpired:
	default:
		return "", ErrInvalidArgument
	}

	var namespaceID string
	var claimID string

	err := s.repo.pool.QueryRow(
		ctx,
		`
        SELECT ns.id, claim.id
          FROM canonical_taxonomy_namespaces ns
          JOIN canonical_taxonomy_dietary_claims claim
            ON claim.namespace_id=ns.id
         WHERE ns.code=$1
           AND ns.status='ACTIVE'
           AND claim.code=$2
           AND claim.status='ACTIVE'
        `,
		input.NamespaceCode,
		input.ClaimCode,
	).Scan(
		&namespaceID,
		&claimID,
	)
	if err != nil {
		return "", err
	}

	id := semanticObjectID(
		"ctclaimas_",
		string(input.NamespaceCode),
		claimID,
		strings.ToUpper(
			strings.TrimSpace(input.SourceType),
		),
		strings.TrimSpace(input.SourceRef),
	)

	metadata := input.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}

	_, err = s.repo.pool.Exec(
		ctx,
		`
        INSERT INTO canonical_taxonomy_dietary_claim_assignments (
            id,
            namespace_id,
            claim_id,
            source_type,
            source_ref,
            business_ref,
            branch_ref,
            status,
            evidence_ref,
            verified_by_actor_ref,
            metadata,
            revision,
            created_by_actor_ref,
            updated_by_actor_ref
        )
        VALUES (
            $1,$2,$3,$4,$5,
            NULLIF($6,''),
            NULLIF($7,''),
            $8,
            NULLIF($9,''),
            NULLIF($10,''),
            $11,1,$12,$12
        )
        ON CONFLICT (
            namespace_id,
            claim_id,
            source_type,
            source_ref
        )
        DO NOTHING
        `,
		id,
		namespaceID,
		claimID,
		strings.ToUpper(
			strings.TrimSpace(input.SourceType),
		),
		strings.TrimSpace(input.SourceRef),
		valueOrEmpty(input.BusinessRef),
		valueOrEmpty(input.BranchRef),
		input.Status,
		valueOrEmpty(input.EvidenceRef),
		valueOrEmpty(input.VerifiedByActorRef),
		metadata,
		input.Actor.ActorRef,
	)

	if err != nil {
		return "", err
	}

	return id, nil
}
