package taxonomy_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/taxonomy"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errForcedPublicationFailure = errors.New("forced taxonomy publication failure")

type failingPublisher struct{}

func (failingPublisher) Publish(
	context.Context,
	pgx.Tx,
	string,
	string,
	string,
	int64,
	string,
	string,
	string,
	any,
) error {
	return errForcedPublicationFailure
}

func TestSection13AHardGate(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping Section 13A real-Postgres hard gate")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err = pool.Ping(ctx); err != nil {
		t.Fatalf("ping test PostgreSQL: %v", err)
	}

	if err = database.ApplyMigrations(
		ctx,
		pool,
		"../../../migrations",
	); err != nil {
		t.Fatalf("apply Vendor migrations: %v", err)
	}

	if err = resetSection13ATestState(ctx, pool); err != nil {
		t.Fatalf("reset Section 13A state before test: %v", err)
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

	outbox := events.NewOutboxRepository(pool)
	publisher := events.NewDomainPublisher(outbox)

	service := taxonomy.NewService(repo, publisher)

	// -----------------------------------------------------------------
	// 1. Create FOOD v1.
	// -----------------------------------------------------------------

	createV1 := taxonomy.CreateDraftInput{
		NamespaceCode: taxonomy.NamespaceFood,
		Actor:         actor("13a-create-v1"),
	}

	v1, err := service.CreateDraft(ctx, createV1)
	if err != nil {
		t.Fatalf("create FOOD v1: %v", err)
	}

	if v1.VersionNumber != 1 {
		t.Fatalf(
			"FOOD v1 version number=%d want=1",
			v1.VersionNumber,
		)
	}

	if v1.Status != taxonomy.VersionStatusDraft {
		t.Fatalf(
			"FOOD v1 status=%s want=DRAFT",
			v1.Status,
		)
	}

	// Same command + same request must replay rather than duplicate.
	v1Replay, err := service.CreateDraft(ctx, createV1)
	if err != nil {
		t.Fatalf("replay create FOOD v1: %v", err)
	}

	if v1Replay.ID != v1.ID {
		t.Fatalf(
			"idempotent replay returned version=%s want=%s",
			v1Replay.ID,
			v1.ID,
		)
	}

	// -----------------------------------------------------------------
	// 2. Give v1 one hierarchical facet and root node.
	// -----------------------------------------------------------------

	v1Facet, err := service.CreateFacet(
		ctx,
		taxonomy.CreateFacetInput{
			VersionID:     v1.ID,
			Code:          "DISH_TYPE",
			DisplayName:   "Dish Type",
			Description:   "Canonical food dish hierarchy.",
			StructureType: taxonomy.FacetStructureHierarchical,
			Position:      10,
			Metadata:      json.RawMessage(`{}`),
			Actor:         actor("13a-v1-facet"),
		},
	)
	if err != nil {
		t.Fatalf("create v1 facet: %v", err)
	}

	v1Root, err := service.CreateNode(
		ctx,
		taxonomy.CreateNodeInput{
			VersionID:     v1.ID,
			FacetID:       v1Facet.ID,
			CanonicalCode: "RICE_MEALS",
			DisplayName:   "Rice & Meals",
			Description:   "Canonical rice and meal family.",
			Position:      10,
			Metadata:      json.RawMessage(`{}`),
			Actor:         actor("13a-v1-root"),
		},
	)
	if err != nil {
		t.Fatalf("create v1 root node: %v", err)
	}

	// -----------------------------------------------------------------
	// 3. Publish v1.
	// -----------------------------------------------------------------

	v1Published, err := service.PublishVersion(
		ctx,
		taxonomy.PublishVersionInput{
			VersionID:        v1.ID,
			ExpectedRevision: v1.Revision,
			Actor:            actor("13a-publish-v1"),
		},
	)
	if err != nil {
		t.Fatalf("publish FOOD v1: %v", err)
	}

	if v1Published.Status != taxonomy.VersionStatusPublished {
		t.Fatalf(
			"FOOD v1 status=%s want=PUBLISHED",
			v1Published.Status,
		)
	}

	active, err := service.GetPublishedVersion(
		ctx,
		taxonomy.NamespaceFood,
	)
	if err != nil {
		t.Fatalf("get active FOOD after v1 publish: %v", err)
	}

	if active.ID != v1.ID {
		t.Fatalf(
			"active FOOD version=%s want v1=%s",
			active.ID,
			v1.ID,
		)
	}

	// -----------------------------------------------------------------
	// 4. Create FOOD v2 by cloning active v1.
	// -----------------------------------------------------------------

	v2, err := service.CloneActiveVersion(
		ctx,
		taxonomy.CloneActiveVersionInput{
			NamespaceCode: taxonomy.NamespaceFood,
			Actor:         actor("13a-clone-v2"),
		},
	)
	if err != nil {
		t.Fatalf("clone FOOD v2: %v", err)
	}

	if v2.VersionNumber != 2 {
		t.Fatalf(
			"FOOD v2 version number=%d want=2",
			v2.VersionNumber,
		)
	}

	if v2.Status != taxonomy.VersionStatusDraft {
		t.Fatalf(
			"FOOD v2 status=%s want=DRAFT",
			v2.Status,
		)
	}

	if v2.SourceVersionID == nil ||
		*v2.SourceVersionID != v1.ID {
		t.Fatalf(
			"FOOD v2 source version=%v want=%s",
			v2.SourceVersionID,
			v1.ID,
		)
	}

	inspection, err := service.InspectDraft(ctx, v2.ID)
	if err != nil {
		t.Fatalf("inspect FOOD v2: %v", err)
	}

	if len(inspection.Facets) != 1 {
		t.Fatalf(
			"cloned FOOD v2 facets=%d want=1",
			len(inspection.Facets),
		)
	}

	if len(inspection.Nodes) != 1 {
		t.Fatalf(
			"cloned FOOD v2 nodes=%d want=1",
			len(inspection.Nodes),
		)
	}

	v2Facet := inspection.Facets[0]
	v2Root := inspection.Nodes[0]

	if v2Facet.ID == v1Facet.ID {
		t.Fatal("clone reused v1 facet row ID")
	}

	if v2Root.ID == v1Root.ID {
		t.Fatal("clone reused v1 node row ID")
	}

	if v2Facet.Code != v1Facet.Code ||
		v2Root.CanonicalCode != v1Root.CanonicalCode {
		t.Fatal("clone did not preserve canonical identity")
	}

	// -----------------------------------------------------------------
	// 5. Add a new node to FOOD v2.
	// -----------------------------------------------------------------

	parentID := v2Root.ID

	v2Child, err := service.CreateNode(
		ctx,
		taxonomy.CreateNodeInput{
			VersionID:     v2.ID,
			FacetID:       v2Facet.ID,
			CanonicalCode: "JOLLOF_RICE",
			DisplayName:   "Jollof Rice",
			Description:   "Canonical Jollof rice node.",
			ParentID:      &parentID,
			Position:      20,
			Metadata:      json.RawMessage(`{}`),
			Actor:         actor("13a-v2-jollof"),
		},
	)
	if err != nil {
		t.Fatalf("add FOOD v2 node: %v", err)
	}

	// -----------------------------------------------------------------
	// 6. Invalid hierarchy must be rejected.
	// -----------------------------------------------------------------

	childID := v2Child.ID

	_, err = service.UpdateNode(
		ctx,
		taxonomy.UpdateNodeInput{
			NodeID:           v2Root.ID,
			CanonicalCode:    v2Root.CanonicalCode,
			DisplayName:      v2Root.DisplayName,
			Description:      v2Root.Description,
			ParentID:         &childID,
			Position:         v2Root.Position,
			Metadata:         v2Root.Metadata,
			ExpectedRevision: v2Root.Revision,
			Actor:            actor("13a-cycle-attempt"),
		},
	)
	if err == nil {
		t.Fatal("cyclic hierarchy update was accepted")
	}

	selfID := v2Child.ID

	_, err = service.UpdateNode(
		ctx,
		taxonomy.UpdateNodeInput{
			NodeID:           v2Child.ID,
			CanonicalCode:    v2Child.CanonicalCode,
			DisplayName:      v2Child.DisplayName,
			Description:      v2Child.Description,
			ParentID:         &selfID,
			Position:         v2Child.Position,
			Metadata:         v2Child.Metadata,
			ExpectedRevision: v2Child.Revision,
			Actor:            actor("13a-self-parent-attempt"),
		},
	)
	if !errors.Is(err, taxonomy.ErrInvalidArgument) {
		t.Fatalf(
			"self-parent error=%v want ErrInvalidArgument",
			err,
		)
	}

	// -----------------------------------------------------------------
	// 7. Publish v2; v1 must remain historical as RETIRED.
	// -----------------------------------------------------------------

	v2Published, err := service.PublishVersion(
		ctx,
		taxonomy.PublishVersionInput{
			VersionID:        v2.ID,
			ExpectedRevision: v2.Revision,
			Actor:            actor("13a-publish-v2"),
		},
	)
	if err != nil {
		t.Fatalf("publish FOOD v2: %v", err)
	}

	if v2Published.Status != taxonomy.VersionStatusPublished {
		t.Fatalf(
			"FOOD v2 status=%s want=PUBLISHED",
			v2Published.Status,
		)
	}

	active, err = service.GetPublishedVersion(
		ctx,
		taxonomy.NamespaceFood,
	)
	if err != nil {
		t.Fatalf("get active FOOD after v2 publish: %v", err)
	}

	if active.ID != v2.ID {
		t.Fatalf(
			"active FOOD version=%s want v2=%s",
			active.ID,
			v2.ID,
		)
	}

	v1Historical, err := service.GetVersion(ctx, v1.ID)
	if err != nil {
		t.Fatalf("load historical v1: %v", err)
	}

	if v1Historical.Status != taxonomy.VersionStatusRetired {
		t.Fatalf(
			"historical v1 status=%s want=RETIRED",
			v1Historical.Status,
		)
	}

	versions, err := service.ListVersions(
		ctx,
		taxonomy.NamespaceFood,
	)
	if err != nil {
		t.Fatalf("list FOOD version history: %v", err)
	}

	if len(versions) != 2 {
		t.Fatalf(
			"FOOD version history count=%d want=2",
			len(versions),
		)
	}

	// -----------------------------------------------------------------
	// 8. Version comparison must identify the new canonical node.
	// -----------------------------------------------------------------

	comparison, err := service.CompareVersions(
		ctx,
		v1.ID,
		v2.ID,
	)
	if err != nil {
		t.Fatalf("compare FOOD v1/v2: %v", err)
	}

	if !containsCanonicalNode(
		comparison.AddedNodes,
		"JOLLOF_RICE",
	) {
		t.Fatal("version comparison did not report JOLLOF_RICE as added")
	}

	// -----------------------------------------------------------------
	// 9. Domain idempotency conflict.
	// -----------------------------------------------------------------

	_, err = service.CloneActiveVersion(
		ctx,
		taxonomy.CloneActiveVersionInput{
			NamespaceCode: taxonomy.NamespaceFood,
			Actor:         actor("13a-create-v1"),
		},
	)
	if !errors.Is(err, taxonomy.ErrIdempotency) {
		t.Fatalf(
			"cross-command idempotency error=%v want ErrIdempotency",
			err,
		)
	}

	// -----------------------------------------------------------------
	// 10. Create v3 and verify optimistic concurrency.
	// -----------------------------------------------------------------

	v3, err := service.CloneActiveVersion(
		ctx,
		taxonomy.CloneActiveVersionInput{
			NamespaceCode: taxonomy.NamespaceFood,
			Actor:         actor("13a-clone-v3"),
		},
	)
	if err != nil {
		t.Fatalf("clone FOOD v3: %v", err)
	}

	v3Inspection, err := service.InspectDraft(ctx, v3.ID)
	if err != nil {
		t.Fatalf("inspect FOOD v3: %v", err)
	}

	if len(v3Inspection.Facets) == 0 {
		t.Fatal("FOOD v3 has no cloned facet")
	}

	v3Facet := v3Inspection.Facets[0]

	updatedFacet, err := service.UpdateFacet(
		ctx,
		taxonomy.UpdateFacetInput{
			FacetID:          v3Facet.ID,
			Code:             v3Facet.Code,
			DisplayName:      "Dish Type Updated",
			Description:      v3Facet.Description,
			StructureType:    v3Facet.StructureType,
			Position:         v3Facet.Position,
			Metadata:         v3Facet.Metadata,
			ExpectedRevision: v3Facet.Revision,
			Actor:            actor("13a-update-v3-facet"),
		},
	)
	if err != nil {
		t.Fatalf("update FOOD v3 facet: %v", err)
	}

	if updatedFacet.Revision != v3Facet.Revision+1 {
		t.Fatalf(
			"facet revision=%d want=%d",
			updatedFacet.Revision,
			v3Facet.Revision+1,
		)
	}

	_, err = service.UpdateFacet(
		ctx,
		taxonomy.UpdateFacetInput{
			FacetID:          v3Facet.ID,
			Code:             v3Facet.Code,
			DisplayName:      "Stale Update",
			Description:      v3Facet.Description,
			StructureType:    v3Facet.StructureType,
			Position:         v3Facet.Position,
			Metadata:         v3Facet.Metadata,
			ExpectedRevision: v3Facet.Revision,
			Actor:            actor("13a-stale-v3-facet"),
		},
	)
	if !errors.Is(err, taxonomy.ErrVersionMismatch) {
		t.Fatalf(
			"stale facet write error=%v want ErrVersionMismatch",
			err,
		)
	}

	// -----------------------------------------------------------------
	// 11. Failed publish must leave v2 active.
	//
	// The failing publisher is invoked after the transaction has already
	// retired v2. Returning an error must roll that retirement back.
	// -----------------------------------------------------------------

	failingService := taxonomy.NewService(
		repo,
		failingPublisher{},
	)

	_, err = failingService.PublishVersion(
		ctx,
		taxonomy.PublishVersionInput{
			VersionID:        v3.ID,
			ExpectedRevision: v3.Revision,
			Actor:            actor("13a-failed-publish-v3"),
		},
	)
	if !errors.Is(err, errForcedPublicationFailure) {
		t.Fatalf(
			"failed publication error=%v want forced failure",
			err,
		)
	}

	activeAfterFailure, err := service.GetPublishedVersion(
		ctx,
		taxonomy.NamespaceFood,
	)
	if err != nil {
		t.Fatalf(
			"get active FOOD after failed v3 publish: %v",
			err,
		)
	}

	if activeAfterFailure.ID != v2.ID {
		t.Fatalf(
			"failed publish changed active version=%s want v2=%s",
			activeAfterFailure.ID,
			v2.ID,
		)
	}

	v2AfterFailure, err := service.GetVersion(ctx, v2.ID)
	if err != nil {
		t.Fatalf("load v2 after failed publish: %v", err)
	}

	if v2AfterFailure.Status != taxonomy.VersionStatusPublished {
		t.Fatalf(
			"v2 status after failed publish=%s want=PUBLISHED",
			v2AfterFailure.Status,
		)
	}

	v3AfterFailure, err := service.GetVersion(ctx, v3.ID)
	if err != nil {
		t.Fatalf("load v3 after failed publish: %v", err)
	}

	if v3AfterFailure.Status != taxonomy.VersionStatusDraft {
		t.Fatalf(
			"v3 status after failed publish=%s want=DRAFT",
			v3AfterFailure.Status,
		)
	}

	// Failed publication must not leave publication evidence behind.
	history, err := service.ListPublicationHistory(
		ctx,
		taxonomy.NamespaceFood,
	)
	if err != nil {
		t.Fatalf("list publication history: %v", err)
	}

	if len(history) != 3 {
		t.Fatalf(
			"publication history count=%d want=3",
			len(history),
		)
	}

	// -----------------------------------------------------------------
	// 12. Outbox evidence must be platform-scoped.
	// -----------------------------------------------------------------

	var taxonomyEvents int
	var vendorScopedTaxonomyEvents int

	if err = pool.QueryRow(ctx, `
        SELECT
            count(*),
            count(*) FILTER (WHERE vendor_id IS NOT NULL)
        FROM vendor_outbox
        WHERE event_type LIKE 'taxonomy.%'
    `).Scan(
		&taxonomyEvents,
		&vendorScopedTaxonomyEvents,
	); err != nil {
		t.Fatalf("inspect taxonomy outbox: %v", err)
	}

	if taxonomyEvents == 0 {
		t.Fatal("no taxonomy outbox evidence was recorded")
	}

	if vendorScopedTaxonomyEvents != 0 {
		t.Fatalf(
			"vendor-scoped taxonomy outbox events=%d want=0",
			vendorScopedTaxonomyEvents,
		)
	}
}

func actor(commandRef string) taxonomy.Actor {
	return taxonomy.Actor{
		ActorRef:      "section13a-test-actor",
		CommandRef:    commandRef,
		CorrelationID: "section13a-hard-gate",
		CausationID:   "section13a-test",
	}
}

func containsCanonicalNode(
	nodes []taxonomy.Node,
	code string,
) bool {
	for _, node := range nodes {
		if node.CanonicalCode == code {
			return true
		}
	}

	return false
}

func resetSection13ATestState(
	ctx context.Context,
	pool *pgxpool.Pool,
) error {
	_, err := pool.Exec(ctx, `
        TRUNCATE TABLE
            canonical_taxonomy_publication_history,
            canonical_taxonomy_change_history,
            canonical_taxonomy_commands,
            canonical_taxonomy_nodes,
            canonical_taxonomy_facets,
            canonical_taxonomy_versions
        CASCADE
    `)
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx, `
        DELETE FROM vendor_outbox
        WHERE event_type LIKE 'taxonomy.%'
    `)
	return err
}
