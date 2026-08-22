package taxonomy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SeedApplyResult struct {
	Version                  Version
	Replayed                 bool
	FacetCount               int
	NodeCount                int
	TermCount                int
	AuthorityVocabularyCount int
}

type SeedApplier struct {
	pool      *pgxpool.Pool
	publisher EventPublisher
}

func NewSeedApplier(
	pool *pgxpool.Pool,
	publisher EventPublisher,
) *SeedApplier {
	return &SeedApplier{
		pool:      pool,
		publisher: publisher,
	}
}

func seedBundleFingerprint(bundle SeedBundle) (string, error) {
	raw, err := json.Marshal(bundle)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(raw)

	return hex.EncodeToString(sum[:]), nil
}

func deterministicSeedObjectID(
	prefix string,
	values ...string,
) string {
	raw := strings.Join(values, "|")
	sum := sha256.Sum256([]byte(raw))

	return prefix + hex.EncodeToString(sum[:16])
}

func seedActor(
	base Actor,
	commandRef string,
) Actor {
	base.CommandRef = commandRef
	return base
}

func (a *SeedApplier) Apply(
	ctx context.Context,
	bundle SeedBundle,
	actor Actor,
) (SeedApplyResult, error) {
	if a == nil ||
		a.pool == nil ||
		strings.TrimSpace(actor.ActorRef) == "" {
		return SeedApplyResult{}, ErrInvalidArgument
	}

	if err := bundle.Validate(); err != nil {
		return SeedApplyResult{}, err
	}

	fingerprint, err := seedBundleFingerprint(bundle)
	if err != nil {
		return SeedApplyResult{}, err
	}

	version, replayed, err := a.prepareVersion(
		ctx,
		bundle,
		fingerprint,
		actor,
	)
	if err != nil {
		return SeedApplyResult{}, err
	}

	result := SeedApplyResult{
		Version:                  version,
		Replayed:                 replayed,
		FacetCount:               len(bundle.Facets),
		NodeCount:                len(bundle.Nodes),
		TermCount:                len(bundle.Terms),
		AuthorityVocabularyCount: len(bundle.AuthorityVocabulary),
	}

	if replayed {
		return result, nil
	}

	service := &Service{
		repo:      NewRepository(a.pool),
		publisher: a.publisher,
	}

	shortFingerprint := fingerprint[:16]

	facets, err := service.ListFacets(ctx, version.ID)
	if err != nil {
		return SeedApplyResult{}, err
	}

	facetByCode := make(
		map[string]Facet,
		len(facets),
	)

	for _, facet := range facets {
		facetByCode[facet.Code] = facet
	}

	for _, definition := range bundle.Facets {
		if _, exists := facetByCode[definition.Code]; exists {
			continue
		}

		metadata, marshalErr := json.Marshal(definition.Metadata)
		if marshalErr != nil {
			return SeedApplyResult{}, marshalErr
		}

		facet, createErr := service.CreateFacet(
			ctx,
			CreateFacetInput{
				VersionID:     version.ID,
				Code:          definition.Code,
				DisplayName:   definition.DisplayName,
				Description:   definition.Description,
				StructureType: definition.StructureType,
				Position:      definition.Position,
				Metadata:      metadata,
				Actor: seedActor(
					actor,
					fmt.Sprintf(
						"seed:%s:%s:facet:%s",
						bundle.NamespaceCode,
						shortFingerprint,
						definition.Code,
					),
				),
			},
		)
		if createErr != nil {
			return SeedApplyResult{}, createErr
		}

		facetByCode[facet.Code] = facet
	}

	nodes, err := service.ListNodes(ctx, version.ID)
	if err != nil {
		return SeedApplyResult{}, err
	}

	nodeByCode := make(
		map[string]Node,
		len(nodes),
	)

	for _, node := range nodes {
		nodeByCode[node.CanonicalCode] = node
	}

	for {
		if len(nodeByCode) >= len(bundle.Nodes) {
			break
		}

		progress := false

		for _, definition := range bundle.Nodes {
			if _, exists := nodeByCode[definition.CanonicalCode]; exists {
				continue
			}

			facet, exists := facetByCode[definition.FacetCode]
			if !exists {
				return SeedApplyResult{}, ErrConflict
			}

			var parentID *string

			if strings.TrimSpace(definition.ParentCode) != "" {
				parent, parentExists :=
					nodeByCode[definition.ParentCode]

				if !parentExists {
					continue
				}

				id := parent.ID
				parentID = &id
			}

			metadata, marshalErr :=
				json.Marshal(definition.Metadata)
			if marshalErr != nil {
				return SeedApplyResult{}, marshalErr
			}

			node, createErr := service.CreateNode(
				ctx,
				CreateNodeInput{
					VersionID:     version.ID,
					FacetID:       facet.ID,
					CanonicalCode: definition.CanonicalCode,
					DisplayName:   definition.DisplayName,
					Description:   definition.Description,
					ParentID:      parentID,
					Position:      definition.Position,
					Metadata:      metadata,
					Actor: seedActor(
						actor,
						fmt.Sprintf(
							"seed:%s:%s:node:%s",
							bundle.NamespaceCode,
							shortFingerprint,
							definition.CanonicalCode,
						),
					),
				},
			)
			if createErr != nil {
				return SeedApplyResult{}, createErr
			}

			nodeByCode[node.CanonicalCode] = node
			progress = true
		}

		if !progress {
			return SeedApplyResult{}, ErrConflict
		}
	}

	for _, term := range bundle.Terms {
		node, exists := nodeByCode[term.NodeCode]
		if !exists {
			return SeedApplyResult{}, ErrConflict
		}

		if err = a.insertTerm(
			ctx,
			bundle.NamespaceCode,
			version,
			node,
			term,
			actor,
		); err != nil {
			return SeedApplyResult{}, err
		}
	}

	for _, vocabulary := range bundle.AuthorityVocabulary {
		if err = a.insertAuthorityVocabulary(
			ctx,
			bundle.NamespaceCode,
			version,
			vocabulary,
			actor,
		); err != nil {
			return SeedApplyResult{}, err
		}
	}

	published, err := service.PublishVersion(
		ctx,
		PublishVersionInput{
			VersionID:        version.ID,
			ExpectedRevision: version.Revision,
			Actor: seedActor(
				actor,
				fmt.Sprintf(
					"seed:%s:%s:publish",
					bundle.NamespaceCode,
					shortFingerprint,
				),
			),
		},
	)
	if err != nil {
		return SeedApplyResult{}, err
	}

	_, err = a.pool.Exec(
		ctx,
		`
        UPDATE canonical_taxonomy_seed_runs
           SET status='PUBLISHED',
               published_at=COALESCE(published_at, now())
         WHERE namespace_id=$1
           AND seed_key=$2
           AND fingerprint=$3
        `,
		published.NamespaceID,
		bundle.SeedKey,
		fingerprint,
	)
	if err != nil {
		return SeedApplyResult{}, err
	}

	result.Version = published

	return result, nil
}

func (a *SeedApplier) prepareVersion(
	ctx context.Context,
	bundle SeedBundle,
	fingerprint string,
	actor Actor,
) (Version, bool, error) {
	tx, err := a.pool.BeginTx(
		ctx,
		pgx.TxOptions{
			IsoLevel: pgx.Serializable,
		},
	)
	if err != nil {
		return Version{}, false, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var namespaceID string

	if err = tx.QueryRow(
		ctx,
		`
        SELECT id
          FROM canonical_taxonomy_namespaces
         WHERE code=$1
           AND status='ACTIVE'
         FOR UPDATE
        `,
		bundle.NamespaceCode,
	).Scan(&namespaceID); err != nil {
		return Version{}, false, err
	}

	var existingVersionID string

	err = tx.QueryRow(
		ctx,
		`
        SELECT version_id
          FROM canonical_taxonomy_seed_runs
         WHERE namespace_id=$1
           AND seed_key=$2
           AND fingerprint=$3
        `,
		namespaceID,
		bundle.SeedKey,
		fingerprint,
	).Scan(&existingVersionID)

	if err == nil {
		var status string

		if err = tx.QueryRow(
			ctx,
			`
            SELECT status
              FROM canonical_taxonomy_versions
             WHERE id=$1
            `,
			existingVersionID,
		).Scan(&status); err != nil {
			return Version{}, false, err
		}

		if status == string(VersionStatusPublished) {
			if _, err = tx.Exec(
				ctx,
				`
                UPDATE canonical_taxonomy_seed_runs
                   SET status='PUBLISHED',
                       published_at=COALESCE(published_at, now())
                 WHERE version_id=$1
                `,
				existingVersionID,
			); err != nil {
				return Version{}, false, err
			}

			if err = tx.Commit(ctx); err != nil {
				return Version{}, false, err
			}

			version, getErr :=
				NewRepository(a.pool).
					GetVersionByID(
						ctx,
						existingVersionID,
					)

			return version, true, getErr
		}

		if status != string(VersionStatusDraft) {
			return Version{}, false, ErrConflict
		}

		if err = tx.Commit(ctx); err != nil {
			return Version{}, false, err
		}

		version, getErr :=
			NewRepository(a.pool).
				GetVersionByID(
					ctx,
					existingVersionID,
				)

		return version, false, getErr
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return Version{}, false, err
	}

	var nextVersion int64

	if err = tx.QueryRow(
		ctx,
		`
        SELECT COALESCE(MAX(version_number),0)+1
          FROM canonical_taxonomy_versions
         WHERE namespace_id=$1
        `,
		namespaceID,
	).Scan(&nextVersion); err != nil {
		return Version{}, false, err
	}

	versionID := deterministicSeedObjectID(
		"ctv_seed_",
		string(bundle.NamespaceCode),
		bundle.SeedKey,
		fingerprint,
	)

	runID := deterministicSeedObjectID(
		"ctsr_",
		string(bundle.NamespaceCode),
		bundle.SeedKey,
		fingerprint,
	)

	if _, err = tx.Exec(
		ctx,
		`
        INSERT INTO canonical_taxonomy_versions (
            id,
            namespace_id,
            version_number,
            status,
            revision,
            created_by_actor_ref
        )
        VALUES ($1,$2,$3,'DRAFT',1,$4)
        `,
		versionID,
		namespaceID,
		nextVersion,
		actor.ActorRef,
	); err != nil {
		return Version{}, false, err
	}

	if _, err = tx.Exec(
		ctx,
		`
        INSERT INTO canonical_taxonomy_seed_runs (
            id,
            namespace_id,
            seed_key,
            fingerprint,
            version_id,
            status,
            facet_count,
            node_count,
            term_count,
            authority_vocabulary_count,
            actor_ref
        )
        VALUES (
            $1,$2,$3,$4,$5,'PREPARED',$6,$7,$8,$9,$10
        )
        `,
		runID,
		namespaceID,
		bundle.SeedKey,
		fingerprint,
		versionID,
		len(bundle.Facets),
		len(bundle.Nodes),
		len(bundle.Terms),
		len(bundle.AuthorityVocabulary),
		actor.ActorRef,
	); err != nil {
		return Version{}, false, err
	}

	if a.publisher != nil {
		payload := map[string]any{
			"namespace":   bundle.NamespaceCode,
			"seed_key":    bundle.SeedKey,
			"fingerprint": fingerprint,
			"version_id":  versionID,
		}

		if err = a.publisher.Publish(
			ctx,
			tx,
			"taxonomy.seed.prepared",
			"TAXONOMY_VERSION",
			versionID,
			1,
			"",
			actor.CorrelationID,
			actor.CausationID,
			payload,
		); err != nil {
			return Version{}, false, err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return Version{}, false, err
	}

	version, err :=
		NewRepository(a.pool).
			GetVersionByID(
				ctx,
				versionID,
			)

	return version, false, err
}

func (a *SeedApplier) insertTerm(
	ctx context.Context,
	namespaceCode NamespaceCode,
	version Version,
	node Node,
	definition SeedNodeTerm,
	actor Actor,
) error {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	definition.VersionKey = version.ID

	id := DeterministicTermID(definition)

	metadata, err := json.Marshal(definition.Metadata)
	if err != nil {
		return err
	}

	tag, err := tx.Exec(
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
            $1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),
            'ACTIVE',$10,1
        )
        ON CONFLICT (id) DO NOTHING
        `,
		id,
		version.NamespaceID,
		version.ID,
		node.ID,
		strings.TrimSpace(definition.Term),
		NormalizeVocabularyTerm(definition.Term),
		definition.RelationshipType,
		definition.Locale,
		definition.Region,
		metadata,
	)
	if err != nil {
		return err
	}

	if tag.RowsAffected() > 0 && a.publisher != nil {
		payload := map[string]any{
			"namespace":         namespaceCode,
			"version_id":        version.ID,
			"node_code":         node.CanonicalCode,
			"term":              definition.Term,
			"relationship_type": definition.RelationshipType,
		}

		if err = a.publisher.Publish(
			ctx,
			tx,
			"taxonomy.term.seeded",
			"TAXONOMY_NODE_TERM",
			id,
			1,
			"",
			actor.CorrelationID,
			actor.CausationID,
			payload,
		); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (a *SeedApplier) insertAuthorityVocabulary(
	ctx context.Context,
	namespaceCode NamespaceCode,
	version Version,
	definition SeedAuthorityVocabularyDefinition,
	actor Actor,
) error {
	if definition.AuthorityType == "SECTION12_MEASUREMENT" {
		if namespaceCode != NamespaceShop {
			return ErrConflict
		}

		var exists bool

		if err := a.pool.QueryRow(
			ctx,
			`
            SELECT EXISTS (
                SELECT 1
                  FROM shop_measure_definitions
                 WHERE id=$1
                   AND code=$2
                   AND business_id IS NULL
                   AND status='ACTIVE'
            )
            `,
			definition.AuthorityRef,
			definition.AuthorityCode,
		).Scan(&exists); err != nil {
			return err
		}

		if !exists {
			return ErrConflict
		}
	}

	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	id := deterministicSeedObjectID(
		"ctav_",
		version.ID,
		definition.AuthorityType,
		definition.AuthorityRef,
		definition.AuthorityCode,
		definition.NormalizedTerm(),
		definition.Locale,
		definition.Region,
	)

	metadata, err := json.Marshal(definition.Metadata)
	if err != nil {
		return err
	}

	tag, err := tx.Exec(
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
            $1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),$11
        )
        ON CONFLICT (id) DO NOTHING
        `,
		id,
		version.NamespaceID,
		version.ID,
		definition.AuthorityType,
		definition.AuthorityRef,
		definition.AuthorityCode,
		strings.TrimSpace(definition.Term),
		definition.NormalizedTerm(),
		definition.Locale,
		definition.Region,
		metadata,
	)
	if err != nil {
		return err
	}

	if tag.RowsAffected() > 0 && a.publisher != nil {
		payload := map[string]any{
			"namespace":      namespaceCode,
			"version_id":     version.ID,
			"authority_type": definition.AuthorityType,
			"authority_ref":  definition.AuthorityRef,
			"authority_code": definition.AuthorityCode,
			"term":           definition.Term,
		}

		if err = a.publisher.Publish(
			ctx,
			tx,
			"taxonomy.authority_vocabulary.seeded",
			"TAXONOMY_AUTHORITY_VOCABULARY",
			id,
			1,
			"",
			actor.CorrelationID,
			actor.CausationID,
			payload,
		); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
