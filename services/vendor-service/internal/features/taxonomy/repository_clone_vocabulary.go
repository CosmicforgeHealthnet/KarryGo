package taxonomy

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

func (s *Service) cloneVersionVocabulary(
	ctx context.Context,
	tx pgx.Tx,
	source Version,
	target Version,
	nodeIDMap map[string]string,
) error {
	rows, err := tx.Query(
		ctx,
		`
        SELECT
            node_id,
            term,
            normalized_term,
            relationship_type,
            locale,
            COALESCE(region,''),
            status,
            metadata
          FROM canonical_taxonomy_node_terms
         WHERE version_id=$1
         ORDER BY id
        `,
		source.ID,
	)
	if err != nil {
		return err
	}

	type sourceTerm struct {
		NodeID       string
		Term         string
		Normalized   string
		Relationship string
		Locale       string
		Region       string
		Status       string
		Metadata     json.RawMessage
	}

	var terms []sourceTerm

	for rows.Next() {
		var item sourceTerm

		if err = rows.Scan(
			&item.NodeID,
			&item.Term,
			&item.Normalized,
			&item.Relationship,
			&item.Locale,
			&item.Region,
			&item.Status,
			&item.Metadata,
		); err != nil {
			rows.Close()
			return err
		}

		terms = append(terms, item)
	}

	rows.Close()

	if err = rows.Err(); err != nil {
		return err
	}

	for _, item := range terms {
		targetNodeID, exists := nodeIDMap[item.NodeID]
		if !exists {
			return ErrConflict
		}

		id := deterministicSeedObjectID(
			"cterm_clone_",
			target.ID,
			targetNodeID,
			item.Relationship,
			item.Normalized,
			item.Locale,
			item.Region,
		)

		if _, err = tx.Exec(
			ctx,
			`
            INSERT INTO canonical_taxonomy_node_terms (
                id,
                namespace_id,
                version_id,
                node_id,
                term,
                normalized_term,
                relationship_type,
                locale,
                region,
                status,
                metadata,
                revision
            )
            VALUES (
                $1,$2,$3,$4,$5,$6,$7,$8,
                NULLIF($9,''),$10,$11,1
            )
            `,
			id,
			target.NamespaceID,
			target.ID,
			targetNodeID,
			item.Term,
			item.Normalized,
			item.Relationship,
			item.Locale,
			item.Region,
			item.Status,
			item.Metadata,
		); err != nil {
			return err
		}
	}

	authorityRows, err := tx.Query(
		ctx,
		`
        SELECT
            authority_type,
            authority_ref,
            authority_code,
            term,
            normalized_term,
            locale,
            COALESCE(region,''),
            metadata
          FROM canonical_taxonomy_authority_vocabulary
         WHERE version_id=$1
         ORDER BY id
        `,
		source.ID,
	)
	if err != nil {
		return err
	}

	type sourceAuthority struct {
		Type       string
		Ref        string
		Code       string
		Term       string
		Normalized string
		Locale     string
		Region     string
		Metadata   json.RawMessage
	}

	var authority []sourceAuthority

	for authorityRows.Next() {
		var item sourceAuthority

		if err = authorityRows.Scan(
			&item.Type,
			&item.Ref,
			&item.Code,
			&item.Term,
			&item.Normalized,
			&item.Locale,
			&item.Region,
			&item.Metadata,
		); err != nil {
			authorityRows.Close()
			return err
		}

		authority = append(authority, item)
	}

	authorityRows.Close()

	if err = authorityRows.Err(); err != nil {
		return err
	}

	for _, item := range authority {
		id := deterministicSeedObjectID(
			"ctav_clone_",
			target.ID,
			item.Type,
			item.Ref,
			item.Code,
			item.Normalized,
			item.Locale,
			item.Region,
		)

		if _, err = tx.Exec(
			ctx,
			`
            INSERT INTO canonical_taxonomy_authority_vocabulary (
                id,
                namespace_id,
                version_id,
                authority_type,
                authority_ref,
                authority_code,
                term,
                normalized_term,
                locale,
                region,
                metadata
            )
            VALUES (
                $1,$2,$3,$4,$5,$6,$7,$8,$9,
                NULLIF($10,''),$11
            )
            `,
			id,
			target.NamespaceID,
			target.ID,
			item.Type,
			item.Ref,
			item.Code,
			item.Term,
			item.Normalized,
			item.Locale,
			item.Region,
			item.Metadata,
		); err != nil {
			return err
		}
	}

	return nil
}
