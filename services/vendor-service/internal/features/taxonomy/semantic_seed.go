package taxonomy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

type SemanticSeedBundle struct {
	SchemaVersion int    `json:"schema_version"`
	SeedKey       string `json:"seed_key"`

	Tags []SemanticTagSeed `json:"tags"`

	DietaryClaims []DietaryClaimSeed `json:"dietary_claims"`
}

type SemanticTagSeed struct {
	Namespace   NamespaceCode   `json:"namespace"`
	Code        string          `json:"code"`
	DisplayName string          `json:"display_name"`
	TagType     SemanticTagType `json:"tag_type"`
}

type DietaryClaimSeed struct {
	Namespace NamespaceCode `json:"namespace"`
	Code      string        `json:"code"`

	DisplayName string `json:"display_name"`

	VerificationPolicy DietaryVerificationPolicy `json:"verification_policy"`
	RiskLevel          DietaryRiskLevel          `json:"risk_level"`
}

func DecodeSemanticSeed(raw []byte) (SemanticSeedBundle, error) {
	var bundle SemanticSeedBundle

	if err := json.Unmarshal(raw, &bundle); err != nil {
		return SemanticSeedBundle{}, err
	}

	if bundle.SchemaVersion != 1 ||
		strings.TrimSpace(bundle.SeedKey) == "" {
		return SemanticSeedBundle{}, ErrInvalidArgument
	}

	seenTags := map[string]struct{}{}

	for _, tag := range bundle.Tags {
		if tag.Namespace != NamespaceFood &&
			tag.Namespace != NamespaceShop {
			return SemanticSeedBundle{}, ErrInvalidArgument
		}

		if err := validateSemanticCode(tag.Code); err != nil {
			return SemanticSeedBundle{}, err
		}

		switch tag.TagType {
		case SemanticTagAttribute,
			SemanticTagDiscovery,
			SemanticTagPreparation,
			SemanticTagMerchandising:
		default:
			return SemanticSeedBundle{}, ErrInvalidArgument
		}

		key := string(tag.Namespace) + "|" + tag.Code

		if _, exists := seenTags[key]; exists {
			return SemanticSeedBundle{},
				fmt.Errorf("duplicate semantic tag %s", key)
		}

		seenTags[key] = struct{}{}
	}

	seenClaims := map[string]struct{}{}

	for _, claim := range bundle.DietaryClaims {
		if claim.Namespace != NamespaceFood &&
			claim.Namespace != NamespaceShop {
			return SemanticSeedBundle{}, ErrInvalidArgument
		}

		if err := validateSemanticCode(claim.Code); err != nil {
			return SemanticSeedBundle{}, err
		}

		switch claim.VerificationPolicy {
		case DietaryVerificationDeclaration,
			DietaryVerificationEvidenceRequired,
			DietaryVerificationPlatformVerified:
		default:
			return SemanticSeedBundle{}, ErrInvalidArgument
		}

		switch claim.RiskLevel {
		case DietaryRiskLow,
			DietaryRiskModerate,
			DietaryRiskHigh:
		default:
			return SemanticSeedBundle{}, ErrInvalidArgument
		}

		key := string(claim.Namespace) + "|" + claim.Code

		if _, exists := seenClaims[key]; exists {
			return SemanticSeedBundle{},
				fmt.Errorf("duplicate dietary claim %s", key)
		}

		seenClaims[key] = struct{}{}
	}

	return bundle, nil
}

func semanticObjectID(
	prefix string,
	values ...string,
) string {
	sum := sha256.Sum256(
		[]byte(strings.Join(values, "|")),
	)

	return prefix + hex.EncodeToString(sum[:16])
}

func (s *Service) ApplySemanticSeed(
	ctx context.Context,
	bundle SemanticSeedBundle,
	actor Actor,
) error {
	if s == nil ||
		s.repo == nil ||
		strings.TrimSpace(actor.ActorRef) == "" {
		return ErrInvalidArgument
	}

	tx, err := s.repo.pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	namespaceIDs := map[NamespaceCode]string{}

	for _, namespace := range []NamespaceCode{
		NamespaceFood,
		NamespaceShop,
	} {
		var id string

		if err = tx.QueryRow(
			ctx,
			`
            SELECT id
              FROM canonical_taxonomy_namespaces
             WHERE code=$1
               AND status='ACTIVE'
            `,
			namespace,
		).Scan(&id); err != nil {
			return err
		}

		namespaceIDs[namespace] = id
	}

	for _, tag := range bundle.Tags {
		namespaceID := namespaceIDs[tag.Namespace]

		id := semanticObjectID(
			"cttag_",
			string(tag.Namespace),
			tag.Code,
		)

		if _, err = tx.Exec(
			ctx,
			`
            INSERT INTO canonical_taxonomy_tags (
                id,
                namespace_id,
                code,
                display_name,
                tag_type,
                status,
                metadata,
                revision,
                created_by_actor_ref,
                updated_by_actor_ref
            )
            VALUES (
                $1,$2,$3,$4,$5,
                'ACTIVE','{}'::jsonb,1,$6,$6
            )
            ON CONFLICT (namespace_id, code)
            DO NOTHING
            `,
			id,
			namespaceID,
			tag.Code,
			tag.DisplayName,
			tag.TagType,
			actor.ActorRef,
		); err != nil {
			return err
		}
	}

	for _, claim := range bundle.DietaryClaims {
		namespaceID := namespaceIDs[claim.Namespace]

		id := semanticObjectID(
			"ctclaim_",
			string(claim.Namespace),
			claim.Code,
		)

		if _, err = tx.Exec(
			ctx,
			`
            INSERT INTO canonical_taxonomy_dietary_claims (
                id,
                namespace_id,
                code,
                display_name,
                verification_policy,
                risk_level,
                status,
                metadata,
                revision,
                created_by_actor_ref,
                updated_by_actor_ref
            )
            VALUES (
                $1,$2,$3,$4,$5,$6,
                'ACTIVE','{}'::jsonb,1,$7,$7
            )
            ON CONFLICT (namespace_id, code)
            DO NOTHING
            `,
			id,
			namespaceID,
			claim.Code,
			claim.DisplayName,
			claim.VerificationPolicy,
			claim.RiskLevel,
			actor.ActorRef,
		); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
