package taxonomy

import (
	"context"
	"os"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSection13B4SeedPublicationIdempotency(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Minute,
	)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err = database.ApplyMigrations(
		ctx,
		pool,
		"../../../migrations",
	); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	food := loadFoodSeedForTest(t)
	shop := loadShopSeedForTest(t)

	var measuresBefore int

	if err = pool.QueryRow(
		ctx,
		`SELECT count(*) FROM shop_measure_definitions`,
	).Scan(&measuresBefore); err != nil {
		t.Fatal(err)
	}

	applier := NewSeedApplier(pool, nil)

	actor := Actor{
		ActorRef:      "system:section13b-seed-test",
		CorrelationID: "section13b-hard-gate",
	}

	foodFirst, err := applier.Apply(ctx, food, actor)
	if err != nil {
		t.Fatalf("first Food seed: %v", err)
	}

	foodSecond, err := applier.Apply(ctx, food, actor)
	if err != nil {
		t.Fatalf("second Food seed: %v", err)
	}

	if foodFirst.Version.ID != foodSecond.Version.ID {
		t.Fatal("Food replay created another version")
	}

	if !foodSecond.Replayed {
		t.Fatal("second Food seed was not replayed")
	}

	shopFirst, err := applier.Apply(ctx, shop, actor)
	if err != nil {
		t.Fatalf("first Shop seed: %v", err)
	}

	shopSecond, err := applier.Apply(ctx, shop, actor)
	if err != nil {
		t.Fatalf("second Shop seed: %v", err)
	}

	if shopFirst.Version.ID != shopSecond.Version.ID {
		t.Fatal("Shop replay created another version")
	}

	if !shopSecond.Replayed {
		t.Fatal("second Shop seed was not replayed")
	}

	expected := []struct {
		versionID string
		facets    int
		nodes     int
		terms     int
		authority int
	}{
		{
			versionID: foodFirst.Version.ID,
			facets:    5,
			nodes:     60,
			terms:     7,
			authority: 0,
		},
		{
			versionID: shopFirst.Version.ID,
			facets:    1,
			nodes:     32,
			terms:     5,
			authority: 17,
		},
	}

	for _, item := range expected {
		var facets int
		var nodes int
		var terms int
		var authority int

		if err = pool.QueryRow(
			ctx,
			`
            SELECT count(*)
              FROM canonical_taxonomy_facets
             WHERE version_id=$1
            `,
			item.versionID,
		).Scan(&facets); err != nil {
			t.Fatal(err)
		}

		if err = pool.QueryRow(
			ctx,
			`
            SELECT count(*)
              FROM canonical_taxonomy_nodes
             WHERE version_id=$1
            `,
			item.versionID,
		).Scan(&nodes); err != nil {
			t.Fatal(err)
		}

		if err = pool.QueryRow(
			ctx,
			`
            SELECT count(*)
              FROM canonical_taxonomy_node_terms
             WHERE version_id=$1
            `,
			item.versionID,
		).Scan(&terms); err != nil {
			t.Fatal(err)
		}

		if err = pool.QueryRow(
			ctx,
			`
            SELECT count(*)
              FROM canonical_taxonomy_authority_vocabulary
             WHERE version_id=$1
            `,
			item.versionID,
		).Scan(&authority); err != nil {
			t.Fatal(err)
		}

		if facets != item.facets ||
			nodes != item.nodes ||
			terms != item.terms ||
			authority != item.authority {
			t.Fatalf(
				"seed counts mismatch version=%s facets=%d nodes=%d terms=%d authority=%d",
				item.versionID,
				facets,
				nodes,
				terms,
				authority,
			)
		}
	}

	var foodPublished int
	var shopPublished int

	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM canonical_taxonomy_versions v
          JOIN canonical_taxonomy_namespaces n
            ON n.id=v.namespace_id
         WHERE n.code='FOOD'
           AND v.status='PUBLISHED'
        `,
	).Scan(&foodPublished); err != nil {
		t.Fatal(err)
	}

	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM canonical_taxonomy_versions v
          JOIN canonical_taxonomy_namespaces n
            ON n.id=v.namespace_id
         WHERE n.code='SHOP'
           AND v.status='PUBLISHED'
        `,
	).Scan(&shopPublished); err != nil {
		t.Fatal(err)
	}

	if foodPublished != 1 || shopPublished != 1 {
		t.Fatalf(
			"published versions Food=%d Shop=%d",
			foodPublished,
			shopPublished,
		)
	}

	var duplicateFacets int
	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM (
            SELECT version_id, code
              FROM canonical_taxonomy_facets
             GROUP BY version_id, code
            HAVING count(*) > 1
          ) d
        `,
	).Scan(&duplicateFacets); err != nil {
		t.Fatal(err)
	}

	var duplicateNodes int
	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM (
            SELECT version_id, canonical_code
              FROM canonical_taxonomy_nodes
             GROUP BY version_id, canonical_code
            HAVING count(*) > 1
          ) d
        `,
	).Scan(&duplicateNodes); err != nil {
		t.Fatal(err)
	}

	var duplicateTerms int
	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM (
            SELECT
                namespace_id,
                version_id,
                normalized_term,
                relationship_type
              FROM canonical_taxonomy_node_terms
             GROUP BY
                namespace_id,
                version_id,
                normalized_term,
                relationship_type
            HAVING count(*) > 1
          ) d
        `,
	).Scan(&duplicateTerms); err != nil {
		t.Fatal(err)
	}

	if duplicateFacets != 0 ||
		duplicateNodes != 0 ||
		duplicateTerms != 0 {
		t.Fatalf(
			"duplicates facets=%d nodes=%d terms=%d",
			duplicateFacets,
			duplicateNodes,
			duplicateTerms,
		)
	}

	var foodGariLeak int

	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM canonical_taxonomy_node_terms t
          JOIN canonical_taxonomy_versions v
            ON v.id=t.version_id
          JOIN canonical_taxonomy_namespaces ns
            ON ns.id=v.namespace_id
         WHERE ns.code='FOOD'
           AND t.normalized_term IN ('gari','garri')
        `,
	).Scan(&foodGariLeak); err != nil {
		t.Fatal(err)
	}

	if foodGariLeak != 0 {
		t.Fatal("SHOP Garri/Gari vocabulary leaked into FOOD")
	}

	var shopMoiLeak int

	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM canonical_taxonomy_node_terms t
          JOIN canonical_taxonomy_versions v
            ON v.id=t.version_id
          JOIN canonical_taxonomy_namespaces ns
            ON ns.id=v.namespace_id
         WHERE ns.code='SHOP'
           AND t.normalized_term IN (
               'moin moin',
               'moimoi',
               'moi-moi'
           )
        `,
	).Scan(&shopMoiLeak); err != nil {
		t.Fatal(err)
	}

	if shopMoiLeak != 0 {
		t.Fatal("FOOD Moi Moi aliases leaked into SHOP")
	}

	var garriShop int
	var ebaFood int

	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM canonical_taxonomy_nodes
         WHERE version_id=$1
           AND canonical_code=
               'SHOP_CATEGORY_FOODSTUFF_GARRI_CASSAVA_GARRI'
        `,
		shopFirst.Version.ID,
	).Scan(&garriShop); err != nil {
		t.Fatal(err)
	}

	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM canonical_taxonomy_nodes
         WHERE version_id=$1
           AND canonical_code=
               'FOOD_DISH_TYPE_SOUPS_SWALLOW_EBA'
        `,
		foodFirst.Version.ID,
	).Scan(&ebaFood); err != nil {
		t.Fatal(err)
	}

	if garriShop != 1 || ebaFood != 1 {
		t.Fatal("Garri/Eba canonical isolation missing")
	}

	var ebaGarriLeak int

	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM canonical_taxonomy_node_terms t
          JOIN canonical_taxonomy_nodes n
            ON n.id=t.node_id
         WHERE t.version_id=$1
           AND n.canonical_code=
               'FOOD_DISH_TYPE_SOUPS_SWALLOW_EBA'
           AND t.normalized_term IN ('garri','gari')
        `,
		foodFirst.Version.ID,
	).Scan(&ebaGarriLeak); err != nil {
		t.Fatal(err)
	}

	if ebaGarriLeak != 0 {
		t.Fatal("Garri alias classified Eba as SHOP-equivalent")
	}

	var relatedType string

	if err = pool.QueryRow(
		ctx,
		`
        SELECT relationship_type
          FROM canonical_taxonomy_node_terms
         WHERE version_id=$1
           AND normalized_term='grilled meat'
        `,
		foodFirst.Version.ID,
	).Scan(&relatedType); err != nil {
		t.Fatal(err)
	}

	if relatedType != "RELATED_TERM" {
		t.Fatalf(
			"related term degraded to %s",
			relatedType,
		)
	}

	var measuresAfter int

	if err = pool.QueryRow(
		ctx,
		`SELECT count(*) FROM shop_measure_definitions`,
	).Scan(&measuresAfter); err != nil {
		t.Fatal(err)
	}

	if measuresAfter != measuresBefore {
		t.Fatalf(
			"Section 12 measurement authority changed: before=%d after=%d",
			measuresBefore,
			measuresAfter,
		)
	}

	// Clone the active SHOP taxonomy and prove vocabulary survives cloning.
	service := &Service{
		repo: NewRepository(pool),
	}

	cloned, err := service.CloneActiveVersion(
		ctx,
		CloneActiveVersionInput{
			NamespaceCode: NamespaceShop,
			Actor: Actor{
				ActorRef:   "system:section13b-clone-test",
				CommandRef: "section13b-clone-vocabulary",
			},
		},
	)
	if err != nil {
		t.Fatalf("clone active Shop: %v", err)
	}

	var clonedTerms int
	var clonedAuthority int

	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM canonical_taxonomy_node_terms
         WHERE version_id=$1
        `,
		cloned.ID,
	).Scan(&clonedTerms); err != nil {
		t.Fatal(err)
	}

	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM canonical_taxonomy_authority_vocabulary
         WHERE version_id=$1
        `,
		cloned.ID,
	).Scan(&clonedAuthority); err != nil {
		t.Fatal(err)
	}

	if clonedTerms != 5 || clonedAuthority != 17 {
		t.Fatalf(
			"cloned Shop vocabulary terms=%d authority=%d",
			clonedTerms,
			clonedAuthority,
		)
	}
}
