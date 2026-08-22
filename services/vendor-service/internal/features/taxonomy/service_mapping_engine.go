package taxonomy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Service) ReplaceFacetMappings(
	ctx context.Context,
	input ReplaceFacetMappingsInput,
) (ReplaceFacetMappingsResult, error) {
	if s == nil ||
		s.repo == nil ||
		strings.TrimSpace(input.Actor.ActorRef) == "" ||
		strings.TrimSpace(input.Actor.CommandRef) == "" {
		return ReplaceFacetMappingsResult{},
			ErrInvalidArgument
	}

	normalized, requestHash, err :=
		prepareReplaceFacetMappingsInput(input)
	if err != nil {
		return ReplaceFacetMappingsResult{},
			err
	}

	tx, err := s.repo.pool.BeginTx(
		ctx,
		pgx.TxOptions{
			IsoLevel: pgx.Serializable,
		},
	)
	if err != nil {
		return ReplaceFacetMappingsResult{},
			err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// Serialize identical command references.
	if _, err = tx.Exec(
		ctx,
		`
        SELECT pg_advisory_xact_lock(
            hashtext($1)
        )
        `,
		"taxonomy-command|"+
			strings.TrimSpace(
				input.Actor.CommandRef,
			),
	); err != nil {
		return ReplaceFacetMappingsResult{},
			err
	}

	var existingHash string
	var existingSnapshot []byte

	err = tx.QueryRow(
		ctx,
		`
        SELECT
            request_hash,
            result_snapshot
          FROM canonical_taxonomy_mapping_commands
         WHERE command_ref=$1
        `,
		strings.TrimSpace(
			input.Actor.CommandRef,
		),
	).Scan(
		&existingHash,
		&existingSnapshot,
	)

	if err == nil {
		if existingHash != requestHash {
			return ReplaceFacetMappingsResult{},
				fmt.Errorf(
					"%w: command_ref reused with different mapping request",
					ErrInvalidArgument,
				)
		}

		var replay ReplaceFacetMappingsResult

		if err = json.Unmarshal(
			existingSnapshot,
			&replay,
		); err != nil {
			return ReplaceFacetMappingsResult{},
				err
		}

		replay.Replayed = true

		return replay, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return ReplaceFacetMappingsResult{},
			err
	}

	var namespaceID string
	var versionID string

	err = tx.QueryRow(
		ctx,
		`
        SELECT
            ns.id,
            v.id
          FROM canonical_taxonomy_namespaces ns
          JOIN canonical_taxonomy_versions v
            ON v.namespace_id=ns.id
         WHERE ns.code=$1
           AND ns.status='ACTIVE'
           AND v.status='PUBLISHED'
         ORDER BY v.version_number DESC
         LIMIT 1
        `,
		normalized.NamespaceCode,
	).Scan(
		&namespaceID,
		&versionID,
	)
	if err != nil {
		return ReplaceFacetMappingsResult{},
			err
	}

	var facetID string

	err = tx.QueryRow(
		ctx,
		`
        SELECT id
          FROM canonical_taxonomy_facets
         WHERE namespace_id=$1
           AND version_id=$2
           AND code=$3
        `,
		namespaceID,
		versionID,
		normalized.FacetCode,
	).Scan(
		&facetID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ReplaceFacetMappingsResult{},
				ErrNotFound
		}

		return ReplaceFacetMappingsResult{},
			err
	}

	// Serialize mutation of one source/facet mapping set.
	if _, err = tx.Exec(
		ctx,
		`
        SELECT pg_advisory_xact_lock(
            hashtext($1)
        )
        `,
		strings.Join(
			[]string{
				"taxonomy-source-facet",
				namespaceID,
				versionID,
				facetID,
				normalized.SourceType,
				normalized.SourceRef,
			},
			"|",
		),
	); err != nil {
		return ReplaceFacetMappingsResult{},
			err
	}

	type resolvedCandidate struct {
		ID string

		NodeID string

		Candidate MappingCandidate
	}

	resolved := make(
		[]resolvedCandidate,
		0,
		len(normalized.Mappings),
	)

	for _, candidate := range normalized.Mappings {

		var nodeID string

		err = tx.QueryRow(
			ctx,
			`
            SELECT id
              FROM canonical_taxonomy_nodes
             WHERE namespace_id=$1
               AND version_id=$2
               AND facet_id=$3
               AND canonical_code=$4
               AND status='ACTIVE'
            `,
			namespaceID,
			versionID,
			facetID,
			candidate.NodeCode,
		).Scan(
			&nodeID,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ReplaceFacetMappingsResult{},
					fmt.Errorf(
						"%w: node %s is not active in facet %s",
						ErrNotFound,
						candidate.NodeCode,
						normalized.FacetCode,
					)
			}

			return ReplaceFacetMappingsResult{},
				err
		}

		mappingID := deterministicMappingID(
			CreateMappingInput{
				NamespaceCode: normalized.NamespaceCode,

				VersionID: versionID,
				NodeID:    nodeID,

				SourceType: normalized.SourceType,

				SourceRef: normalized.SourceRef,

				MappingType: candidate.MappingType,
			},
		)

		resolved = append(
			resolved,
			resolvedCandidate{
				ID:        mappingID,
				NodeID:    nodeID,
				Candidate: candidate,
			},
		)
	}

	// Replacement semantics:
	// archive the current active set for this source/facet,
	// then reactivate/upsert exactly the requested set.
	if _, err = tx.Exec(
		ctx,
		`
        UPDATE canonical_taxonomy_mappings
           SET status='ARCHIVED',
               updated_by_actor_ref=$1
         WHERE namespace_id=$2
           AND version_id=$3
           AND facet_id=$4
           AND source_type=$5
           AND source_ref=$6
           AND status='ACTIVE'
        `,
		input.Actor.ActorRef,
		namespaceID,
		versionID,
		facetID,
		normalized.SourceType,
		normalized.SourceRef,
	); err != nil {
		return ReplaceFacetMappingsResult{},
			err
	}

	mappingIDs := make(
		[]string,
		0,
		len(resolved),
	)

	for _, item := range resolved {
		_, err = tx.Exec(
			ctx,
			`
            INSERT INTO canonical_taxonomy_mappings (
                id,
                namespace_id,
                version_id,
                facet_id,
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
                $1,$2,$3,$4,$5,$6,$7,$8,
                'ACTIVE',$9,$10,
                NULLIF($11,''),
                NULLIF($12,''),
                $13,1,$14,$14
            )
            ON CONFLICT (
                namespace_id,
                version_id,
                source_type,
                source_ref,
                node_id,
                mapping_type
            )
            DO UPDATE SET
                status='ACTIVE',
                confidence_basis=
                    EXCLUDED.confidence_basis,
                confidence_score=
                    EXCLUDED.confidence_score,
                business_ref=
                    EXCLUDED.business_ref,
                branch_ref=
                    EXCLUDED.branch_ref,
                metadata=
                    EXCLUDED.metadata,
                updated_by_actor_ref=
                    EXCLUDED.updated_by_actor_ref
            `,
			item.ID,
			namespaceID,
			versionID,
			facetID,
			item.NodeID,
			normalized.SourceType,
			normalized.SourceRef,
			item.Candidate.MappingType,
			item.Candidate.ConfidenceBasis,
			item.Candidate.ConfidenceScore,
			normalized.BusinessRef,
			normalized.BranchRef,
			item.Candidate.Metadata,
			input.Actor.ActorRef,
		)
		if err != nil {
			return ReplaceFacetMappingsResult{},
				err
		}

		mappingIDs = append(
			mappingIDs,
			item.ID,
		)
	}

	result := ReplaceFacetMappingsResult{
		NamespaceCode: normalized.NamespaceCode,

		NamespaceID: namespaceID,

		VersionID: versionID,

		FacetID: facetID,

		FacetCode: normalized.FacetCode,

		SourceType: normalized.SourceType,

		SourceRef: normalized.SourceRef,

		MappingIDs: mappingIDs,

		Replayed: false,
	}

	snapshot, err := json.Marshal(result)
	if err != nil {
		return ReplaceFacetMappingsResult{},
			err
	}

	_, err = tx.Exec(
		ctx,
		`
        INSERT INTO canonical_taxonomy_mapping_commands (
            id,
            command_ref,
            actor_ref,
            operation,
            mapping_id,
            request_hash,
            result_snapshot
        )
        VALUES (
            $1,$2,$3,
            'REPLACE_FACET_MAPPINGS',
            NULL,
            $4,
            $5
        )
        `,
		semanticObjectID(
			"ctmapcmd_",
			strings.TrimSpace(
				input.Actor.CommandRef,
			),
		),
		strings.TrimSpace(
			input.Actor.CommandRef,
		),
		input.Actor.ActorRef,
		requestHash,
		snapshot,
	)
	if err != nil {
		return ReplaceFacetMappingsResult{},
			err
	}

	if err = tx.Commit(ctx); err != nil {
		return ReplaceFacetMappingsResult{},
			err
	}

	return result, nil
}
