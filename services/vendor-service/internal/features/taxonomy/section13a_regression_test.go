package taxonomy_test

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/taxonomy"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSection13ADatabaseGuards(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping Section 13A database guards")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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
		t.Fatalf("apply Vendor migrations: %v", err)
	}

	if err = resetSection13ATestState(ctx, pool); err != nil {
		t.Fatalf("reset Section 13A state: %v", err)
	}

	defer func() {
		if cleanupErr := resetSection13ATestState(
			context.Background(),
			pool,
		); cleanupErr != nil {
			t.Logf("Section 13A cleanup warning: %v", cleanupErr)
		}
	}()

	repo := taxonomy.NewRepository(pool)
	publisher := events.NewDomainPublisher(
		events.NewOutboxRepository(pool),
	)
	service := taxonomy.NewService(repo, publisher)

	// ---------------------------------------------------------------
	// Build one valid FOOD draft.
	// ---------------------------------------------------------------

	version, err := service.CreateDraft(
		ctx,
		taxonomy.CreateDraftInput{
			NamespaceCode: taxonomy.NamespaceFood,
			Actor:         actor("13a-reg-create-version"),
		},
	)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	facet, err := service.CreateFacet(
		ctx,
		taxonomy.CreateFacetInput{
			VersionID:     version.ID,
			Code:          "REGRESSION_TREE",
			DisplayName:   "Regression Tree",
			StructureType: taxonomy.FacetStructureHierarchical,
			Position:      1,
			Metadata:      json.RawMessage(`{}`),
			Actor:         actor("13a-reg-create-facet"),
		},
	)
	if err != nil {
		t.Fatalf("create facet: %v", err)
	}

	root, err := service.CreateNode(
		ctx,
		taxonomy.CreateNodeInput{
			VersionID:     version.ID,
			FacetID:       facet.ID,
			CanonicalCode: "ROOT",
			DisplayName:   "Root",
			Position:      1,
			Metadata:      json.RawMessage(`{}`),
			Actor:         actor("13a-reg-root"),
		},
	)
	if err != nil {
		t.Fatalf("create root: %v", err)
	}

	// ---------------------------------------------------------------
	// Bounded hierarchy must reject excessive depth.
	// We intentionally do not couple the test to the exact configured
	// limit; Section 13A requires the hierarchy to be bounded.
	// ---------------------------------------------------------------

	parentID := root.ID
	depthRejected := false

	for i := 1; i <= 20; i++ {
		node, createErr := service.CreateNode(
			ctx,
			taxonomy.CreateNodeInput{
				VersionID:     version.ID,
				FacetID:       facet.ID,
				CanonicalCode: "DEPTH_" + strconv.Itoa(i),
				DisplayName:   "Depth " + strconv.Itoa(i),
				ParentID:      &parentID,
				Position:      i + 1,
				Metadata:      json.RawMessage(`{}`),
				Actor: actor(
					"13a-reg-depth-" + strconv.Itoa(i),
				),
			},
		)

		if createErr != nil {
			depthRejected = true
			break
		}

		parentID = node.ID
	}

	if !depthRejected {
		t.Fatal("taxonomy hierarchy accepted excessive unbounded depth")
	}

	// ---------------------------------------------------------------
	// Publish the valid version.
	// ---------------------------------------------------------------

	published, err := service.PublishVersion(
		ctx,
		taxonomy.PublishVersionInput{
			VersionID:        version.ID,
			ExpectedRevision: version.Revision,
			Actor:            actor("13a-reg-publish"),
		},
	)
	if err != nil {
		t.Fatalf("publish version: %v", err)
	}

	if published.Status != taxonomy.VersionStatusPublished {
		t.Fatalf(
			"published status=%s want=PUBLISHED",
			published.Status,
		)
	}

	// ---------------------------------------------------------------
	// Published taxonomy content must be immutable even via direct SQL.
	// ---------------------------------------------------------------

	if _, err = pool.Exec(ctx, `
        UPDATE canonical_taxonomy_facets
        SET display_name = 'ILLEGAL PUBLISHED MUTATION'
        WHERE id = $1
    `, facet.ID); err == nil {
		t.Fatal("direct SQL mutated facet on published version")
	}

	if _, err = pool.Exec(ctx, `
        UPDATE canonical_taxonomy_nodes
        SET display_name = 'ILLEGAL PUBLISHED MUTATION'
        WHERE id = $1
    `, root.ID); err == nil {
		t.Fatal("direct SQL mutated node on published version")
	}

	// ---------------------------------------------------------------
	// Version identity/history cannot be rewritten.
	// ---------------------------------------------------------------

	if _, err = pool.Exec(ctx, `
        UPDATE canonical_taxonomy_versions
        SET version_number = version_number + 100
        WHERE id = $1
    `, published.ID); err == nil {
		t.Fatal("direct SQL rewrote taxonomy version identity")
	}

	// ---------------------------------------------------------------
	// Namespace canonical code is immutable.
	// ---------------------------------------------------------------

	if _, err = pool.Exec(ctx, `
        UPDATE canonical_taxonomy_namespaces
        SET code = 'FOOD_REWRITTEN'
        WHERE code = 'FOOD'
    `); err == nil {
		t.Fatal("direct SQL rewrote canonical namespace code")
	}

	// ---------------------------------------------------------------
	// Domain idempotency evidence is append-only.
	// ---------------------------------------------------------------

	if _, err = pool.Exec(ctx, `
        UPDATE canonical_taxonomy_commands
        SET request_fingerprint =
            '0000000000000000000000000000000000000000000000000000000000000000'
        WHERE command_ref = '13a-reg-create-version'
    `); err == nil {
		t.Fatal("taxonomy command evidence was mutable")
	}

	if _, err = pool.Exec(ctx, `
        DELETE FROM canonical_taxonomy_commands
        WHERE command_ref = '13a-reg-create-version'
    `); err == nil {
		t.Fatal("taxonomy command evidence was deletable")
	}

	// ---------------------------------------------------------------
	// Change history is append-only.
	// ---------------------------------------------------------------

	if _, err = pool.Exec(ctx, `
        UPDATE canonical_taxonomy_change_history
        SET actor_ref = 'illegal-rewrite'
        WHERE command_ref = '13a-reg-create-version'
    `); err == nil {
		t.Fatal("taxonomy change history was mutable")
	}

	if _, err = pool.Exec(ctx, `
        DELETE FROM canonical_taxonomy_change_history
        WHERE command_ref = '13a-reg-create-version'
    `); err == nil {
		t.Fatal("taxonomy change history was deletable")
	}

	// ---------------------------------------------------------------
	// Publication history is append-only.
	// ---------------------------------------------------------------

	if _, err = pool.Exec(ctx, `
        UPDATE canonical_taxonomy_publication_history
        SET actor_ref = 'illegal-rewrite'
        WHERE command_ref = '13a-reg-publish'
    `); err == nil {
		t.Fatal("taxonomy publication history was mutable")
	}

	if _, err = pool.Exec(ctx, `
        DELETE FROM canonical_taxonomy_publication_history
        WHERE command_ref = '13a-reg-publish'
    `); err == nil {
		t.Fatal("taxonomy publication history was deletable")
	}

	// ---------------------------------------------------------------
	// Exactly one active published version for FOOD.
	// ---------------------------------------------------------------

	var publishedCount int

	if err = pool.QueryRow(ctx, `
        SELECT count(*)
        FROM canonical_taxonomy_versions v
        JOIN canonical_taxonomy_namespaces n
          ON n.id = v.namespace_id
        WHERE n.code = 'FOOD'
          AND v.status = 'PUBLISHED'
    `).Scan(&publishedCount); err != nil {
		t.Fatalf("count published FOOD versions: %v", err)
	}

	if publishedCount != 1 {
		t.Fatalf(
			"published FOOD versions=%d want=1",
			publishedCount,
		)
	}
}
