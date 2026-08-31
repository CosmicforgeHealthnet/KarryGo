package section13security

// Section 13I nested-ownership and cross-namespace isolation certification.
//
// Section 13 introduces two independent axes along which a request can escape
// its bounds, and both are certified here:
//
//   - TENANT: vendor -> business -> branch -> source resource. A caller must
//     not reach another vendor's classification by guessing a source_ref,
//     mapping id, assignment id or branch id.
//   - NAMESPACE: FOOD and SHOP are separate vocabularies, separate projections
//     and separate generations. An identifier from one must never resolve
//     through the other, in either direction.
//
// The namespace axis is the one that is easy to get subtly wrong, because a
// FOOD version id and a SHOP version id are the same shape and both exist. A
// handler that looks a record up by id and only then reads its namespace has
// already leaked whether that id exists; a handler that never checks at all
// will happily map a Food dish onto a Shop category. Every case below attacks
// exactly that.
//
// Assertions are made at the domain and database layer, not only through HTTP,
// so a future handler refactor cannot quietly remove the protection.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/searchprojection"
	"cosmicforge/logistics/services/vendor-service/internal/features/taxonomy"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
)

// ---------------------------------------------------------------------------
// Taxonomy: cross-namespace
// ---------------------------------------------------------------------------

type namespaceFixture struct {
	service *taxonomy.Service

	foodVersion string
	shopVersion string

	foodFacet string
	shopFacet string

	foodNode string
	shopNode string
}

func actor(ref string) taxonomy.Actor {
	return taxonomy.Actor{
		ActorRef:      "service:section13-isolation",
		CommandRef:    ref + "-" + time.Now().Format("150405.000000000"),
		CorrelationID: "section13-isolation",
	}
}

// newNamespaceFixture builds a live draft in BOTH namespaces, each with one
// facet and one node, so every cross-namespace attack below has a real target
// to aim at rather than a fabricated identifier that would fail for the
// uninteresting reason that nothing exists.
func newNamespaceFixture(t *testing.T, pool *pgxpool.Pool, ctx context.Context) *namespaceFixture {
	t.Helper()

	service := taxonomy.NewService(taxonomy.NewRepository(pool), nil)
	fixture := &namespaceFixture{service: service}

	for _, namespace := range []taxonomy.NamespaceCode{taxonomy.NamespaceFood, taxonomy.NamespaceShop} {
		version, err := service.CreateDraft(ctx, taxonomy.CreateDraftInput{
			NamespaceCode: namespace,
			Actor:         actor("draft-" + string(namespace)),
		})
		if err != nil {
			t.Fatalf("create %s draft: %v", namespace, err)
		}

		facet, err := service.CreateFacet(ctx, taxonomy.CreateFacetInput{
			VersionID:     version.ID,
			Code:          "ISOLATION_" + string(namespace),
			DisplayName:   string(namespace) + " isolation facet",
			StructureType: taxonomy.FacetStructureFlat,
			Metadata:      json.RawMessage(`{}`),
			Actor:         actor("facet-" + string(namespace)),
		})
		if err != nil {
			t.Fatalf("create %s facet: %v", namespace, err)
		}

		node, err := service.CreateNode(ctx, taxonomy.CreateNodeInput{
			VersionID:     version.ID,
			FacetID:       facet.ID,
			CanonicalCode: "ISOLATION_NODE_" + string(namespace),
			DisplayName:   string(namespace) + " isolation node",
			Metadata:      json.RawMessage(`{}`),
			Actor:         actor("node-" + string(namespace)),
		})
		if err != nil {
			t.Fatalf("create %s node: %v", namespace, err)
		}

		// Publish, because Section 13C only permits a mapping to target a
		// PUBLISHED version. Attacking a draft would make the cross-namespace
		// mapping cases below pass for the wrong reason - refused because the
		// version is a draft, rather than refused because the node belongs to
		// the other namespace.
		current, err := service.GetVersion(ctx, version.ID)
		if err != nil {
			t.Fatalf("re-read %s version: %v", namespace, err)
		}
		published, err := service.PublishVersion(ctx, taxonomy.PublishVersionInput{
			VersionID:        current.ID,
			ExpectedRevision: current.Revision,
			Actor:            actor("publish-" + string(namespace)),
		})
		if err != nil {
			t.Fatalf("publish %s version: %v", namespace, err)
		}

		if namespace == taxonomy.NamespaceFood {
			fixture.foodVersion, fixture.foodFacet, fixture.foodNode = published.ID, facet.ID, node.ID
		} else {
			fixture.shopVersion, fixture.shopFacet, fixture.shopNode = published.ID, facet.ID, node.ID
		}
	}

	return fixture
}

// TestCrossNamespaceVersionAccessIsRefused attacks the version axis in both
// directions.
func TestCrossNamespaceVersionAccessIsRefused(t *testing.T) {
	pool, ctx := testPool(t)
	fixture := newNamespaceFixture(t, pool, ctx)

	cases := []struct {
		name      string
		namespace taxonomy.NamespaceCode
		versionID string
	}{
		{"SHOP version through the FOOD namespace", taxonomy.NamespaceFood, fixture.shopVersion},
		{"FOOD version through the SHOP namespace", taxonomy.NamespaceShop, fixture.foodVersion},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// The vocabulary read is the one that would otherwise disclose the
			// other namespace's structure.
			if _, err := fixture.service.ListAuthorityVocabulary(ctx, testCase.namespace, testCase.versionID); err != taxonomy.ErrNamespaceMismatch {
				t.Errorf("ListAuthorityVocabulary: err=%v want=ErrNamespaceMismatch", err)
			}
		})
	}
}

// TestCrossNamespaceNodeAndTermAccessIsRefused attacks the node and vocabulary
// axis, including the combination that looks most plausible: a node addressed
// through its OWN namespace but the other namespace's version.
func TestCrossNamespaceNodeAndTermAccessIsRefused(t *testing.T) {
	pool, ctx := testPool(t)
	fixture := newNamespaceFixture(t, pool, ctx)

	// A SHOP node read through FOOD.
	if _, err := fixture.service.ListNodeTerms(
		ctx, taxonomy.NamespaceFood, fixture.foodVersion, fixture.shopNode,
	); err != taxonomy.ErrNamespaceMismatch {
		t.Errorf("SHOP node under FOOD version: err=%v want=ErrNamespaceMismatch", err)
	}

	// A FOOD node read through SHOP.
	if _, err := fixture.service.ListNodeTerms(
		ctx, taxonomy.NamespaceShop, fixture.shopVersion, fixture.foodNode,
	); err != taxonomy.ErrNamespaceMismatch {
		t.Errorf("FOOD node under SHOP version: err=%v want=ErrNamespaceMismatch", err)
	}

	// A FOOD node addressed with the FOOD namespace but the SHOP version.
	if _, err := fixture.service.ListNodeTerms(
		ctx, taxonomy.NamespaceFood, fixture.shopVersion, fixture.foodNode,
	); err != taxonomy.ErrNamespaceMismatch {
		t.Errorf("FOOD node under SHOP version id: err=%v want=ErrNamespaceMismatch", err)
	}

	// The node itself is reachable through its own namespace and version,
	// which proves the refusals above are about the mismatch rather than about
	// the fixture being unreachable.
	if _, err := fixture.service.ListNodeTerms(
		ctx, taxonomy.NamespaceFood, fixture.foodVersion, fixture.foodNode,
	); err != nil {
		t.Fatalf("FOOD node under its own version was refused: %v", err)
	}
}

// TestCrossNamespaceMappingIsRefused is the case with the most operational
// consequence: a Food dish mapped onto a Shop grocery category, or the
// reverse, would corrupt discovery for both catalogues at once.
func TestCrossNamespaceMappingIsRefused(t *testing.T) {
	pool, ctx := testPool(t)
	fixture := newNamespaceFixture(t, pool, ctx)

	// FOOD mapping against a SHOP node.
	_, err := fixture.service.CreateMapping(ctx, taxonomy.CreateMappingInput{
		NamespaceCode:   taxonomy.NamespaceFood,
		VersionID:       fixture.foodVersion,
		NodeID:          fixture.shopNode,
		SourceType:      taxonomy.SourceTypeFoodItem,
		SourceRef:       "fi_isolation_probe",
		MappingType:     taxonomy.MappingTypePrimary,
		ConfidenceBasis: taxonomy.MappingConfidenceManual,
		Metadata:        json.RawMessage(`{}`),
		Actor:           actor("map-food-to-shop-node"),
	})
	if err == nil {
		t.Error("a FOOD mapping was accepted against a SHOP node")
	}

	// SHOP mapping against a FOOD node.
	_, err = fixture.service.CreateMapping(ctx, taxonomy.CreateMappingInput{
		NamespaceCode:   taxonomy.NamespaceShop,
		VersionID:       fixture.shopVersion,
		NodeID:          fixture.foodNode,
		SourceType:      taxonomy.SourceTypeShopProduct,
		SourceRef:       "sp_isolation_probe",
		MappingType:     taxonomy.MappingTypePrimary,
		ConfidenceBasis: taxonomy.MappingConfidenceManual,
		Metadata:        json.RawMessage(`{}`),
		Actor:           actor("map-shop-to-food-node"),
	})
	if err == nil {
		t.Error("a SHOP mapping was accepted against a FOOD node")
	}

	// The database itself must also refuse the combination, so the protection
	// survives any future code path that bypasses the service.
	var accepted int
	dbErr := pool.QueryRow(ctx, `
        INSERT INTO canonical_taxonomy_mappings
            (id, namespace_id, version_id, node_id, source_type, source_ref,
             mapping_type, confidence_basis, created_by_actor_ref, updated_by_actor_ref)
        VALUES ('ctm_isolation_probe', 'ctns_food', $1, $2, 'FOOD_ITEM', 'fi_direct_probe',
                'PRIMARY', 'MANUAL', 'test', 'test')
        RETURNING 1`,
		fixture.foodVersion, fixture.shopNode,
	).Scan(&accepted)
	if dbErr == nil {
		t.Error("PostgreSQL accepted a FOOD mapping onto a SHOP node; the schema invariant is missing")
	}
}

// TestCrossNamespaceSourceTypeBindingIsRefused proves the vendor surface binds
// each source type to exactly one namespace before it looks anything up. A
// Food item can never be classified inside SHOP, whatever the caller sends.
func TestCrossNamespaceSourceTypeBindingIsRefused(t *testing.T) {
	fixture, ctx := newIAMFixture(t)

	scope, err := fixture.access.Authorize(ctx, staffOwnerA, permTaxonomyMapping)
	if err != nil {
		t.Fatalf("owner should hold %s: %v", permTaxonomyMapping, err)
	}

	cases := []struct {
		name       string
		namespace  taxonomy.NamespaceCode
		sourceType string
		sourceRef  string
	}{
		{"Food item addressed through SHOP", taxonomy.NamespaceShop, taxonomy.SourceTypeFoodItem, foodItemA1},
		{"Shop product addressed through FOOD", taxonomy.NamespaceFood, taxonomy.SourceTypeShopProduct, shopProductA},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := fixture.access.ResolveSource(
				ctx, testCase.namespace, testCase.sourceType, testCase.sourceRef, businessA, scope,
			)
			if err != taxonomy.ErrNamespaceMismatch {
				t.Fatalf("err=%v want=ErrNamespaceMismatch", err)
			}
		})
	}

	// Each resource resolves correctly through its own namespace, so the
	// refusals above are about the binding and not about the fixture.
	if _, err = fixture.access.ResolveSource(
		ctx, taxonomy.NamespaceFood, taxonomy.SourceTypeFoodItem, foodItemA1, businessA, scope,
	); err != nil {
		t.Fatalf("Food item through FOOD was refused: %v", err)
	}
	if _, err = fixture.access.ResolveSource(
		ctx, taxonomy.NamespaceShop, taxonomy.SourceTypeShopProduct, shopProductA, businessA, scope,
	); err != nil {
		t.Fatalf("Shop product through SHOP was refused: %v", err)
	}
}

// TestUnknownNamespaceIsRefusedRatherThanDefaulted guards the quiet failure
// mode: a typo that falls through to FOOD would classify a resource into a
// namespace the caller never named.
func TestUnknownNamespaceIsRefusedRatherThanDefaulted(t *testing.T) {
	pool, ctx := testPool(t)
	service := taxonomy.NewService(taxonomy.NewRepository(pool), nil)

	for _, code := range []string{"", "GROCERY", "FOODS", "shop;", "*", "FOOD SHOP"} {
		if _, err := service.Namespace(ctx, taxonomy.NamespaceCode(code)); err != taxonomy.ErrInvalidArgument {
			t.Errorf("namespace %q: err=%v want=ErrInvalidArgument", code, err)
		}
	}

	// Case-insensitivity and surrounding whitespace are normalised on purpose;
	// that is canonicalisation of a value that IS one of the two namespaces,
	// not a fallthrough from a value that is neither.
	for _, code := range []string{"FOOD", "food", " Shop ", "food "} {
		if _, err := service.Namespace(ctx, taxonomy.NamespaceCode(code)); err != nil {
			t.Errorf("namespace %q was refused: %v", code, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Taxonomy: cross-tenant
// ---------------------------------------------------------------------------

// TestCrossVendorSourceAccessIsRefused is the tenant axis: business B's Owner
// holds every vendor-scoped capability at global scope inside their OWN
// business, and still cannot reach business A's catalogue.
func TestCrossVendorSourceAccessIsRefused(t *testing.T) {
	fixture, ctx := newIAMFixture(t)

	scopeB, err := fixture.access.Authorize(ctx, staffOwnerB, permTaxonomyMapping)
	if err != nil {
		t.Fatalf("owner B should hold %s: %v", permTaxonomyMapping, err)
	}
	if !scopeB.Global {
		t.Fatal("owner B should hold a business-wide scope")
	}

	// Business B's own resources resolve.
	if _, err = fixture.access.ResolveSource(
		ctx, taxonomy.NamespaceFood, taxonomy.SourceTypeFoodItem, foodItemB1, businessB, scopeB,
	); err != nil {
		t.Fatalf("own Food item refused: %v", err)
	}

	// Business A's resources do not resolve for business B, in either
	// namespace. The refusal is ErrNotFound rather than ErrForbidden so the
	// caller cannot use the status code to discover that the resource exists.
	for _, attempt := range []struct {
		name       string
		namespace  taxonomy.NamespaceCode
		sourceType string
		sourceRef  string
	}{
		{"Food item of another vendor", taxonomy.NamespaceFood, taxonomy.SourceTypeFoodItem, foodItemA1},
		{"Food item in another vendor's second branch", taxonomy.NamespaceFood, taxonomy.SourceTypeFoodItem, foodItemA2},
		{"Shop product of another vendor", taxonomy.NamespaceShop, taxonomy.SourceTypeShopProduct, shopProductA},
	} {
		t.Run(attempt.name, func(t *testing.T) {
			if _, err := fixture.access.ResolveSource(
				ctx, attempt.namespace, attempt.sourceType, attempt.sourceRef, businessB, scopeB,
			); err != taxonomy.ErrNotFound {
				t.Fatalf("err=%v want=ErrNotFound", err)
			}
		})
	}
}

// TestVendorMappingCommandRejectsCallerSuppliedScope is the HTTP-level half of
// the same rule, and the one that matters in production: the business and
// branch a classification is written under come from the resolved session and
// the owning catalogue row, never from the request.
//
// A body carrying business_ref or branch_ref is refused outright rather than
// silently ignored. Silently ignoring it would be safe but dishonest - the
// caller would believe it had written into another scope and the API would
// have quietly disagreed.
func TestVendorMappingCommandRejectsCallerSuppliedScope(t *testing.T) {
	fixture, ctx := newIAMFixture(t)
	_ = ctx

	gin.SetMode(gin.TestMode)
	router := gin.New()

	flags := taxonomy.Flags{TaxonomyEnabled: true, WritesEnabled: true}
	service := taxonomy.NewService(taxonomy.NewRepository(fixture.pool), nil)

	group := router.Group("/api/v1/vendor")
	// Stand in for Section 5's RequireAuth: the production middleware resolves
	// this same context server-side from the session.
	group.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{
			StaffID: staffOwnerA, VendorID: businessA, SessionID: "vsess_section13",
		})
		c.Next()
	})
	taxonomy.RegisterRoutes(group, taxonomy.NewHandler(service, fixture.access, flags), flags)

	target := "/api/v1/vendor/taxonomy/namespaces/FOOD/mappings"

	cases := []struct {
		name string
		body string
	}{
		{
			name: "business_ref smuggled into the body",
			body: `{"facet_code":"ISOLATION_FOOD","source_type":"FOOD_ITEM","source_ref":"` +
				foodItemA1 + `","business_ref":"` + businessB + `","mappings":[]}`,
		},
		{
			name: "branch_ref smuggled into the body",
			body: `{"facet_code":"ISOLATION_FOOD","source_type":"FOOD_ITEM","source_ref":"` +
				foodItemA1 + `","branch_ref":"` + branchB1 + `","mappings":[]}`,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPut, target, strings.NewReader(testCase.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "section13-scope-probe")

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want=400 body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

// TestVendorClassificationReadSucceedsForAnOwnedResource is the positive
// control for the whole vendor surface.
//
// Almost every other assertion in this package is a refusal, and a surface that
// refused everything - a broken route, a mis-wired handler, an always-empty
// scope - would satisfy all of them. This proves the surface actually works for
// the caller it is built for, so the refusals elsewhere mean something.
func TestVendorClassificationReadSucceedsForAnOwnedResource(t *testing.T) {
	fixture, _ := newIAMFixture(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	flags := taxonomy.Flags{TaxonomyEnabled: true, WritesEnabled: true}
	service := taxonomy.NewService(taxonomy.NewRepository(fixture.pool), nil)

	group := router.Group("/api/v1/vendor")
	group.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{
			StaffID: staffOwnerA, VendorID: businessA, SessionID: "vsess_section13_ok",
		})
		c.Next()
	})
	taxonomy.RegisterRoutes(group, taxonomy.NewHandler(service, fixture.access, flags), flags)

	for _, route := range []struct {
		name   string
		target string
	}{
		{
			name: "classification of an owned Food item",
			target: "/api/v1/vendor/taxonomy/namespaces/FOOD/classifications" +
				"?source_type=FOOD_ITEM&source_ref=" + foodItemA1,
		},
		{
			name: "mappings of an owned Food item",
			target: "/api/v1/vendor/taxonomy/namespaces/FOOD/mappings" +
				"?source_type=FOOD_ITEM&source_ref=" + foodItemA1,
		},
		{
			name: "dietary claim assignments of an owned Food item",
			target: "/api/v1/vendor/taxonomy/namespaces/FOOD/dietary-claim-assignments" +
				"?source_type=FOOD_ITEM&source_ref=" + foodItemA1,
		},
		{
			name: "classification of an owned Shop product",
			target: "/api/v1/vendor/taxonomy/namespaces/SHOP/classifications" +
				"?source_type=SHOP_PRODUCT&source_ref=" + shopProductA,
		},
		{
			name:   "dietary claim catalogue",
			target: "/api/v1/vendor/taxonomy/namespaces/FOOD/dietary-claims",
		},
	} {
		t.Run(route.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, route.target, nil)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d want=200 body=%s", recorder.Code, recorder.Body.String())
			}

			var envelope struct {
				Success bool `json:"success"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !envelope.Success {
				t.Fatalf("response envelope reports failure: %s", recorder.Body.String())
			}
		})
	}
}

// TestOwnedResourceReadsAreScopedToTheOwningBusiness proves the second gate:
// even once a source is resolved, the mapping/tag/claim reads filter on the
// owning business, so a row another tenant attached to the same source_ref is
// never returned.
func TestOwnedResourceReadsAreScopedToTheOwningBusiness(t *testing.T) {
	fixture, ctx := newIAMFixture(t)
	service := taxonomy.NewService(taxonomy.NewRepository(fixture.pool), nil)

	scopeA, err := fixture.access.Authorize(ctx, staffOwnerA, permTaxonomyRead)
	if err != nil {
		t.Fatal(err)
	}
	ownership, err := fixture.access.ResolveSource(
		ctx, taxonomy.NamespaceFood, taxonomy.SourceTypeFoodItem, foodItemA1, businessA, scopeA,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Plant a mapping that business B attached to the SAME source ref. This is
	// the row that must never leak.
	nsFixture := newNamespaceFixture(t, fixture.pool, ctx)
	if _, err = fixture.pool.Exec(ctx, `
        INSERT INTO canonical_taxonomy_mappings
            (id, namespace_id, version_id, node_id, source_type, source_ref,
             mapping_type, confidence_basis, business_ref, created_by_actor_ref, updated_by_actor_ref)
        VALUES ('ctm_other_tenant', 'ctns_food', $1, $2, 'FOOD_ITEM', $3,
                'PRIMARY', 'MANUAL', $4, 'test', 'test')`,
		nsFixture.foodVersion, nsFixture.foodNode, foodItemA1, businessB,
	); err != nil {
		t.Fatalf("plant foreign mapping: %v", err)
	}

	mappings, err := service.ListMappingsForOwnedSource(ctx, taxonomy.NamespaceFood, ownership)
	if err != nil {
		t.Fatal(err)
	}
	for _, mapping := range mappings {
		if mapping.BusinessRef != nil && *mapping.BusinessRef == businessB {
			t.Fatalf("business A read business B's mapping %s on a shared source ref", mapping.ID)
		}
	}
}

// ---------------------------------------------------------------------------
// Search: cross-namespace
// ---------------------------------------------------------------------------

// TestSearchNamespacesNeverCrossOver is the Section 13D claim re-certified at
// the API layer: the two engines read different tables, so a text match in one
// namespace can never surface a document from the other.
func TestSearchNamespacesNeverCrossOver(t *testing.T) {
	pool, ctx := testPool(t)

	if _, err := pool.Exec(ctx,
		`TRUNCATE TABLE food_search_documents, shop_search_documents CASCADE`); err != nil {
		t.Fatalf("reset projections: %v", err)
	}

	foodProjector := searchprojection.NewFoodProjector(searchprojection.NewPostgresFoodSearchBackend(pool))
	shopProjector := searchprojection.NewShopProjector(searchprojection.NewPostgresShopSearchBackend(pool))

	// Both documents contain "rice". Only the namespace separates them.
	if _, err := foodProjector.Project(ctx, searchprojection.FoodProjectionInput{
		Generation: 1, BusinessRef: businessA, BranchRef: branchA1, FoodItemRef: "fi_iso_jollof",
		Name: "Jollof Rice", Active: true, CatalogueRevision: 1,
	}); err != nil {
		t.Fatalf("project Food document: %v", err)
	}
	if _, err := shopProjector.Project(ctx, searchprojection.ShopProjectionInput{
		Generation: 1, BusinessRef: businessA, BranchRef: branchA1,
		ProductRef: "sp_iso_rice", VariantRef: "sv_iso_rice", ListingRef: "sl_iso_rice",
		Name: "Raw Rice 50kg", Active: true, CatalogueRevision: 1, InventoryRevision: 1,
	}); err != nil {
		t.Fatalf("project Shop document: %v", err)
	}

	foodEngine := searchprojection.NewFoodSearchEngine(searchprojection.NewPostgresFoodSearchBackend(pool))
	shopEngine := searchprojection.NewShopSearchEngine(searchprojection.NewPostgresShopSearchBackend(pool))

	foodResponse, err := foodEngine.Search(ctx, searchprojection.FoodEngineSearchRequest{
		Query: "rice", Generation: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("Food search: %v", err)
	}
	if len(foodResponse.Hits) == 0 {
		t.Fatal("Food search returned nothing; the fixture never became discoverable")
	}
	for _, hit := range foodResponse.Hits {
		if hit.FoodItemRef != "fi_iso_jollof" {
			t.Errorf("Food search returned a foreign document %s", hit.DocumentID)
		}
	}

	shopResponse, err := shopEngine.Search(ctx, searchprojection.ShopEngineSearchRequest{
		Query: "rice", Generation: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("Shop search: %v", err)
	}
	if len(shopResponse.Hits) == 0 {
		t.Fatal("Shop search returned nothing; the fixture never became discoverable")
	}
	for _, hit := range shopResponse.Hits {
		if hit.ProductRef != "sp_iso_rice" {
			t.Errorf("Shop search returned a foreign document %s", hit.DocumentID)
		}
	}
}

// TestSearchGenerationsAreNamespaceScoped attacks the generation axis: a FOOD
// generation number used on a SHOP administration request, and the reverse.
func TestSearchGenerationsAreNamespaceScoped(t *testing.T) {
	pool, ctx := testPool(t)

	if _, err := pool.Exec(ctx, `
        TRUNCATE TABLE search_projection_jobs, search_projection_generation_history,
                       search_projection_generations CASCADE`); err != nil {
		t.Fatalf("reset generations: %v", err)
	}

	// A FOOD generation that exists, and no SHOP generation with that number.
	if _, err := pool.Exec(ctx, `
        INSERT INTO search_projection_generations
            (namespace_code, generation, status, base_generation, rebuild_scope, requested_by,
             document_count, processed_count)
        VALUES ('FOOD', 4241, 'READY', 0, 'FULL', 'test', 1, 1)`); err != nil {
		t.Fatalf("seed FOOD generation: %v", err)
	}

	store := searchprojection.NewPostgresSearchGenerationStore(pool)

	if _, err := store.GetGeneration(ctx, searchprojection.SearchCacheNamespaceFood, 4241); err != nil {
		t.Fatalf("FOOD generation is not readable through FOOD: %v", err)
	}

	// The same number through SHOP must not resolve.
	if _, err := store.GetGeneration(ctx, searchprojection.SearchCacheNamespaceShop, 4241); err == nil {
		t.Error("a FOOD generation was readable through the SHOP namespace")
	}

	// Nor may a SHOP administration command act on it. Activation is the one
	// with real consequence: activating a Food projection as Shop's live
	// generation would empty the Shop catalogue from discovery.
	if err := store.ActivateGeneration(ctx, searchprojection.SearchCacheNamespaceShop, 4241); err == nil {
		t.Error("a FOOD generation was activated through the SHOP namespace")
	}
	if err := store.RetireGeneration(ctx, searchprojection.SearchCacheNamespaceShop, 4241); err == nil {
		t.Error("a FOOD generation was retired through the SHOP namespace")
	}

	// The FOOD generation is still READY: nothing the SHOP path did touched it.
	record, err := store.GetGeneration(ctx, searchprojection.SearchCacheNamespaceFood, 4241)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != searchprojection.SearchGenerationReady {
		t.Fatalf("FOOD generation status=%s after cross-namespace attempts; want READY", record.Status)
	}
}

// TestRankingProfilesAreNamespaceScoped attacks the ranking axis.
func TestRankingProfilesAreNamespaceScoped(t *testing.T) {
	pool, ctx := testPool(t)
	store := searchprojection.NewPostgresRankingProfileStore(pool)

	food, err := store.GetRankingProfile(ctx, searchprojection.SearchCacheNamespaceFood,
		searchprojection.DefaultFoodRankingProfile)
	if err != nil {
		t.Fatalf("read the seeded FOOD profile: %v", err)
	}
	if food.NamespaceCode != string(searchprojection.SearchCacheNamespaceFood) {
		t.Fatalf("profile namespace=%s want=FOOD", food.NamespaceCode)
	}

	// The FOOD profile code through SHOP, and the reverse.
	if _, err = store.GetRankingProfile(ctx, searchprojection.SearchCacheNamespaceShop,
		searchprojection.DefaultFoodRankingProfile); err != searchprojection.ErrRankingProfileNotFound {
		t.Errorf("FOOD profile through SHOP: err=%v want=ErrRankingProfileNotFound", err)
	}
	if _, err = store.GetRankingProfile(ctx, searchprojection.SearchCacheNamespaceFood,
		searchprojection.DefaultShopRankingProfile); err != searchprojection.ErrRankingProfileNotFound {
		t.Errorf("SHOP profile through FOOD: err=%v want=ErrRankingProfileNotFound", err)
	}

	// An update aimed through the wrong namespace must not write.
	boost := 9000
	if _, err = store.UpdateRankingProfile(ctx, searchprojection.SearchCacheNamespaceShop,
		searchprojection.DefaultFoodRankingProfile,
		searchprojection.RankingWeightUpdate{PromotionWeight: &boost},
	); err != searchprojection.ErrRankingProfileNotFound {
		t.Errorf("cross-namespace ranking update: err=%v want=ErrRankingProfileNotFound", err)
	}

	after, err := store.GetRankingProfile(ctx, searchprojection.SearchCacheNamespaceFood,
		searchprojection.DefaultFoodRankingProfile)
	if err != nil {
		t.Fatal(err)
	}
	if after.PromotionWeight != food.PromotionWeight {
		t.Fatalf("FOOD promotion weight changed from %d to %d via a SHOP-addressed update",
			food.PromotionWeight, after.PromotionWeight)
	}
}

// TestCommercialBoostCannotDefeatTextualRelevance certifies the ranking bound
// that keeps search honest. It is enforced at the store, so no administrator -
// however privileged - can configure a promoted result above an exact name
// match.
func TestCommercialBoostCannotDefeatTextualRelevance(t *testing.T) {
	pool, ctx := testPool(t)
	store := searchprojection.NewPostgresRankingProfileStore(pool)

	before, err := store.GetRankingProfile(ctx, searchprojection.SearchCacheNamespaceFood,
		searchprojection.DefaultFoodRankingProfile)
	if err != nil {
		t.Fatal(err)
	}

	overExactMatch := before.ExactNameWeight + 1
	aboveCeiling := before.MaxNonTextBoost + 1

	for _, attempt := range []struct {
		name   string
		update searchprojection.RankingWeightUpdate
	}{
		{"promotion above the non-text ceiling",
			searchprojection.RankingWeightUpdate{PromotionWeight: &aboveCeiling}},
		{"commercial above the non-text ceiling",
			searchprojection.RankingWeightUpdate{CommercialWeight: &aboveCeiling}},
		{"non-text ceiling raised to exact-name relevance",
			searchprojection.RankingWeightUpdate{MaxNonTextBoost: &overExactMatch}},
	} {
		t.Run(attempt.name, func(t *testing.T) {
			if _, err := store.UpdateRankingProfile(ctx, searchprojection.SearchCacheNamespaceFood,
				searchprojection.DefaultFoodRankingProfile, attempt.update,
			); err != searchprojection.ErrRankingProfileInvalid {
				t.Fatalf("err=%v want=ErrRankingProfileInvalid", err)
			}
		})
	}

	after, err := store.GetRankingProfile(ctx, searchprojection.SearchCacheNamespaceFood,
		searchprojection.DefaultFoodRankingProfile)
	if err != nil {
		t.Fatal(err)
	}
	if after.PromotionWeight != before.PromotionWeight ||
		after.CommercialWeight != before.CommercialWeight ||
		after.MaxNonTextBoost != before.MaxNonTextBoost {
		t.Fatal("a refused ranking update still changed the stored profile")
	}

	// A legitimate adjustment inside the bounds is accepted, proving the
	// refusals above are about the bound rather than about updates failing.
	within := before.MaxNonTextBoost - 1
	if within < 0 {
		within = 0
	}
	if _, err = store.UpdateRankingProfile(ctx, searchprojection.SearchCacheNamespaceFood,
		searchprojection.DefaultFoodRankingProfile,
		searchprojection.RankingWeightUpdate{PromotionWeight: &within},
	); err != nil {
		t.Fatalf("in-bounds ranking update was refused: %v", err)
	}
	// Restore.
	if _, err = store.UpdateRankingProfile(ctx, searchprojection.SearchCacheNamespaceFood,
		searchprojection.DefaultFoodRankingProfile,
		searchprojection.RankingWeightUpdate{PromotionWeight: &before.PromotionWeight},
	); err != nil {
		t.Fatalf("restore ranking profile: %v", err)
	}
}
