package taxonomy

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSection13BFinalStaticContract(t *testing.T) {
	food := loadFoodSeedForTest(t)
	shop := loadShopSeedForTest(t)

	if food.NamespaceCode != NamespaceFood {
		t.Fatalf(
			"Food namespace=%s want=%s",
			food.NamespaceCode,
			NamespaceFood,
		)
	}

	if shop.NamespaceCode != NamespaceShop {
		t.Fatalf(
			"Shop namespace=%s want=%s",
			shop.NamespaceCode,
			NamespaceShop,
		)
	}

	if food.SeedKey != "KARRYGO_FOOD_V1" {
		t.Fatalf(
			"Food seed key=%s",
			food.SeedKey,
		)
	}

	if shop.SeedKey != "KARRYGO_SHOP_V1" {
		t.Fatalf(
			"Shop seed key=%s",
			shop.SeedKey,
		)
	}

	if len(food.Facets) != 5 {
		t.Fatalf(
			"Food facets=%d want=5",
			len(food.Facets),
		)
	}

	if len(food.Nodes) != 60 {
		t.Fatalf(
			"Food nodes=%d want=60",
			len(food.Nodes),
		)
	}

	if len(food.Terms) != 7 {
		t.Fatalf(
			"Food terms=%d want=7",
			len(food.Terms),
		)
	}

	if len(shop.Facets) != 1 {
		t.Fatalf(
			"Shop facets=%d want=1",
			len(shop.Facets),
		)
	}

	if len(shop.Nodes) != 32 {
		t.Fatalf(
			"Shop nodes=%d want=32",
			len(shop.Nodes),
		)
	}

	if len(shop.Terms) != 5 {
		t.Fatalf(
			"Shop terms=%d want=5",
			len(shop.Terms),
		)
	}

	if len(shop.AuthorityVocabulary) != 17 {
		t.Fatalf(
			"Shop authority vocabulary=%d want=17",
			len(shop.AuthorityVocabulary),
		)
	}

	requiredFoodFacets := map[string]FacetStructureType{
		"PROVIDER_TYPE": FacetStructureFlat,
		"CUISINE":       FacetStructureHierarchical,
		"MEAL_PERIOD":   FacetStructureFlat,
		"DISH_TYPE":     FacetStructureHierarchical,
		"OCCASION":      FacetStructureFlat,
	}

	for _, facet := range food.Facets {
		want, exists := requiredFoodFacets[facet.Code]
		if !exists {
			t.Fatalf(
				"unexpected Food facet %s",
				facet.Code,
			)
		}

		if facet.StructureType != want {
			t.Fatalf(
				"Food facet %s structure=%s want=%s",
				facet.Code,
				facet.StructureType,
				want,
			)
		}

		delete(requiredFoodFacets, facet.Code)
	}

	if len(requiredFoodFacets) != 0 {
		t.Fatalf(
			"missing Food facets: %v",
			requiredFoodFacets,
		)
	}

	if len(shop.Facets) != 1 ||
		shop.Facets[0].Code != "CATEGORY" ||
		shop.Facets[0].StructureType !=
			FacetStructureHierarchical {
		t.Fatal(
			"Shop CATEGORY hierarchical facet contract invalid",
		)
	}
}

func TestSection13BFinalRequiredFoodVocabulary(t *testing.T) {
	food := loadFoodSeedForTest(t)

	requiredDisplayNames := []string{
		"Restaurants",
		"Fast Food",
		"Cloud Kitchens",
		"Pastry & Bakery",

		"African Food",
		"Nigerian / Local Dishes",
		"Continental / International",

		"Breakfast & Brunch",
		"Lunch",
		"Dinner",
		"Late Night",

		"Rice & Meals",
		"Soups & Swallow",
		"Grills & BBQ",
		"Pizza",
		"Burgers",
		"Shawarma",
		"Cakes & Celebrations",
		"Pastries",
		"Small Chops",
		"Snacks",
		"Desserts",
		"Ice Cream",
		"Drinks",
		"Juices",
		"Smoothies",
		"Healthy Meals",

		"Family Meals",
		"Group Orders",
		"Bulk Food",
		"Corporate Meals",
		"Scheduled Food",
		"Celebration Food",
		"Party Packs",

		"Jollof Rice",
		"Fried Rice",
		"Ofada Rice",
		"Native Rice",

		"Egusi Soup",
		"Ogbono Soup",
		"Afang",
		"Edikang Ikong",
		"Banga",
		"Oha",
		"Nsala",
		"Fisherman Soup",

		"Eba",
		"Amala",
		"Pounded Yam",
		"Semo",
		"Fufu",
		"Tuwo",

		"Suya",
		"Kilishi",
		"Akara",
		"Moi Moi",
		"Puff Puff",
		"Chin Chin",
	}

	actual := make(
		map[string]struct{},
		len(food.Nodes),
	)

	for _, node := range food.Nodes {
		actual[node.DisplayName] = struct{}{}

		if !strings.HasPrefix(
			node.CanonicalCode,
			"FOOD_",
		) {
			t.Fatalf(
				"Food node escaped FOOD namespace: %s",
				node.CanonicalCode,
			)
		}
	}

	for _, name := range requiredDisplayNames {
		if _, exists := actual[name]; !exists {
			t.Fatalf(
				"required Food vocabulary missing: %s",
				name,
			)
		}
	}
}

func TestSection13BFinalRequiredShopVocabulary(t *testing.T) {
	shop := loadShopSeedForTest(t)

	requiredDisplayNames := []string{
		"Foodstuff",
		"Fresh Produce",
		"Protein",
		"Cooking Essentials",
		"Frozen & Chilled",
		"Beverages",
		"Dairy & Eggs",
		"Packaged Grocery",
		"Bulk / Wholesale",

		"Rice & Grains",
		"Beans & Pulses",
		"Garri & Cassava",
		"Yam & Tubers",
		"Plantain",

		"Meat & Poultry",
		"Fish & Seafood",

		"Vegetables",
		"Fruits",

		"Cooking Oils",
		"Spices & Seasoning",
		"Flour & Baking",

		"Frozen Foods",
		"Drinks & Water",
		"Dairy",
		"Eggs",
		"Canned Foods",
		"Packaged Groceries",

		"Garri",
		"Okra",
		"Ponmo",
		"Groundnut",
	}

	actual := make(
		map[string]struct{},
		len(shop.Nodes),
	)

	for _, node := range shop.Nodes {
		actual[node.DisplayName] = struct{}{}

		if !strings.HasPrefix(
			node.CanonicalCode,
			"SHOP_",
		) {
			t.Fatalf(
				"Shop node escaped SHOP namespace: %s",
				node.CanonicalCode,
			)
		}
	}

	for _, name := range requiredDisplayNames {
		if _, exists := actual[name]; !exists {
			t.Fatalf(
				"required Shop vocabulary missing: %s",
				name,
			)
		}
	}
}

func TestSection13BFinalNigerianRelationshipVocabulary(
	t *testing.T,
) {
	food := loadFoodSeedForTest(t)
	shop := loadShopSeedForTest(t)

	type expectedTerm struct {
		namespace    NamespaceCode
		nodeCode     string
		relationship TermRelationshipType
	}

	required := map[string]expectedTerm{
		"moin moin": {
			namespace:    NamespaceFood,
			nodeCode:     "FOOD_DISH_TYPE_SNACKS_MOI_MOI",
			relationship: TermRelationshipAlias,
		},
		"moimoi": {
			namespace:    NamespaceFood,
			nodeCode:     "FOOD_DISH_TYPE_SNACKS_MOI_MOI",
			relationship: TermRelationshipAlias,
		},
		"moi-moi": {
			namespace:    NamespaceFood,
			nodeCode:     "FOOD_DISH_TYPE_SNACKS_MOI_MOI",
			relationship: TermRelationshipAlias,
		},
		"puff-puff": {
			namespace:    NamespaceFood,
			nodeCode:     "FOOD_DISH_TYPE_SNACKS_PUFF_PUFF",
			relationship: TermRelationshipAlias,
		},
		"pap": {
			namespace:    NamespaceFood,
			nodeCode:     "FOOD_DISH_TYPE_BREAKFAST_MEALS_AKAMU",
			relationship: TermRelationshipSynonym,
		},
		"ogi": {
			namespace:    NamespaceFood,
			nodeCode:     "FOOD_DISH_TYPE_BREAKFAST_MEALS_AKAMU",
			relationship: TermRelationshipSynonym,
		},
		"gari": {
			namespace:    NamespaceShop,
			nodeCode:     "SHOP_CATEGORY_FOODSTUFF_GARRI_CASSAVA_GARRI",
			relationship: TermRelationshipAlias,
		},
		"okro": {
			namespace:    NamespaceShop,
			nodeCode:     "SHOP_CATEGORY_FRESH_PRODUCE_VEGETABLES_OKRA",
			relationship: TermRelationshipAlias,
		},
		"kpomo": {
			namespace:    NamespaceShop,
			nodeCode:     "SHOP_CATEGORY_PROTEIN_MEAT_POULTRY_PONMO",
			relationship: TermRelationshipAlias,
		},
		"kanda": {
			namespace:    NamespaceShop,
			nodeCode:     "SHOP_CATEGORY_PROTEIN_MEAT_POULTRY_PONMO",
			relationship: TermRelationshipAlias,
		},
		"peanut": {
			namespace:    NamespaceShop,
			nodeCode:     "SHOP_CATEGORY_FOODSTUFF_BEANS_PULSES_GROUNDNUT",
			relationship: TermRelationshipSynonym,
		},
	}

	seenRelationships :=
		map[TermRelationshipType]bool{}

	for _, bundle := range []SeedBundle{food, shop} {
		for _, term := range bundle.Terms {
			normalized :=
				NormalizeVocabularyTerm(term.Term)

			seenRelationships[term.RelationshipType] =
				true

			expected, exists := required[normalized]
			if !exists {
				continue
			}

			if term.NamespaceCode !=
				expected.namespace {
				t.Fatalf(
					"%s namespace=%s want=%s",
					term.Term,
					term.NamespaceCode,
					expected.namespace,
				)
			}

			if term.NodeCode != expected.nodeCode {
				t.Fatalf(
					"%s node=%s want=%s",
					term.Term,
					term.NodeCode,
					expected.nodeCode,
				)
			}

			if term.RelationshipType !=
				expected.relationship {
				t.Fatalf(
					"%s relationship=%s want=%s",
					term.Term,
					term.RelationshipType,
					expected.relationship,
				)
			}

			delete(required, normalized)
		}
	}

	if len(required) != 0 {
		t.Fatalf(
			"required Nigerian vocabulary missing: %v",
			required,
		)
	}

	for _, relationship := range []TermRelationshipType{
		TermRelationshipAlias,
		TermRelationshipSynonym,
		TermRelationshipRelatedTerm,
	} {
		if !seenRelationships[relationship] {
			t.Fatalf(
				"relationship type %s not represented",
				relationship,
			)
		}
	}
}

func TestSection13BFinalGarriAndEbaIsolation(t *testing.T) {
	food := loadFoodSeedForTest(t)
	shop := loadShopSeedForTest(t)

	var foodEba bool
	var shopGarri bool

	for _, node := range food.Nodes {
		if node.CanonicalCode ==
			"FOOD_DISH_TYPE_SOUPS_SWALLOW_EBA" {
			foodEba = true
		}

		if node.CanonicalCode ==
			"SHOP_CATEGORY_FOODSTUFF_GARRI_CASSAVA_GARRI" {
			t.Fatal(
				"SHOP Garri canonical node leaked into FOOD",
			)
		}
	}

	for _, node := range shop.Nodes {
		if node.CanonicalCode ==
			"SHOP_CATEGORY_FOODSTUFF_GARRI_CASSAVA_GARRI" {
			shopGarri = true
		}

		if node.CanonicalCode ==
			"FOOD_DISH_TYPE_SOUPS_SWALLOW_EBA" {
			t.Fatal(
				"FOOD Eba canonical node leaked into SHOP",
			)
		}
	}

	if !foodEba || !shopGarri {
		t.Fatal(
			"Garri/Eba canonical isolation fixture missing",
		)
	}

	for _, term := range food.Terms {
		normalized :=
			NormalizeVocabularyTerm(term.Term)

		if normalized == "garri" ||
			normalized == "gari" {
			t.Fatalf(
				"Food taxonomy incorrectly maps %s",
				term.Term,
			)
		}
	}
}

func TestSection13BFinalMeasurementBridgeContract(t *testing.T) {
	shop := loadShopSeedForTest(t)

	expected := map[string]string{
		"smd_paint":        "PAINT",
		"smd_paint_rubber": "PAINT_RUBBER",
		"smd_derica":       "DERICA",
		"smd_mudu":         "MUDU",
		"smd_congo":        "CONGO",
		"smd_tiya":         "TIYA",
		"smd_kobiowu":      "KOBIOWU",
		"smd_basket":       "BASKET",
		"smd_bowl":         "BOWL",
		"smd_basin":        "BASIN",
		"smd_bunch":        "BUNCH",
		"smd_tuber":        "TUBER",
		"smd_bag":          "BAG",
		"smd_sack":         "SACK",
		"smd_carton":       "CARTON",
		"smd_crate":        "CRATE",
		"smd_jerrycan":     "JERRYCAN",
	}

	if len(shop.AuthorityVocabulary) != len(expected) {
		t.Fatalf(
			"measurement refs=%d want=%d",
			len(shop.AuthorityVocabulary),
			len(expected),
		)
	}

	seenTerms := make(map[string]struct{})

	for _, item := range shop.AuthorityVocabulary {
		if item.AuthorityType !=
			"SECTION12_MEASUREMENT" {
			t.Fatalf(
				"unexpected authority type %s",
				item.AuthorityType,
			)
		}

		wantCode, exists :=
			expected[item.AuthorityRef]

		if !exists {
			t.Fatalf(
				"unexpected Section 12 ref %s",
				item.AuthorityRef,
			)
		}

		if item.AuthorityCode != wantCode {
			t.Fatalf(
				"%s code=%s want=%s",
				item.AuthorityRef,
				item.AuthorityCode,
				wantCode,
			)
		}

		if item.Metadata["read_only_reference"] != true {
			t.Fatalf(
				"%s is not reference-only",
				item.AuthorityRef,
			)
		}

		normalized :=
			NormalizeVocabularyTerm(item.Term)

		if _, exists := seenTerms[normalized]; exists {
			t.Fatalf(
				"duplicate measurement vocabulary %s",
				normalized,
			)
		}

		seenTerms[normalized] = struct{}{}

		delete(expected, item.AuthorityRef)
	}

	if len(expected) != 0 {
		t.Fatalf(
			"missing Section 12 refs: %v",
			expected,
		)
	}
}

func TestSection13BFinalMealPeriodAuthorityIsolation(
	t *testing.T,
) {
	food := loadFoodSeedForTest(t)

	required := map[string]bool{
		"Breakfast & Brunch": false,
		"Lunch":              false,
		"Dinner":             false,
		"Late Night":         false,
	}

	for _, node := range food.Nodes {
		if _, exists := required[node.DisplayName]; !exists {
			continue
		}

		if node.Metadata["discovery_only"] != true {
			t.Fatalf(
				"%s must remain discovery_only",
				node.DisplayName,
			)
		}

		if node.Metadata["branch_opening_authority"] != false {
			t.Fatalf(
				"%s must not become branch-opening authority",
				node.DisplayName,
			)
		}

		required[node.DisplayName] = true
	}

	for name, found := range required {
		if !found {
			t.Fatalf(
				"meal period %s missing",
				name,
			)
		}
	}
}

func TestSection13BFinalCanonicalCodesAreDBSafe(
	t *testing.T,
) {
	for _, bundle := range []SeedBundle{
		loadFoodSeedForTest(t),
		loadShopSeedForTest(t),
	} {
		seen := map[string]struct{}{}

		for _, node := range bundle.Nodes {
			if strings.Contains(
				node.CanonicalCode,
				".",
			) {
				t.Fatalf(
					"dotted canonical code remains: %s",
					node.CanonicalCode,
				)
			}

			if _, exists :=
				seen[node.CanonicalCode]; exists {
				t.Fatalf(
					"duplicate canonical code: %s",
					node.CanonicalCode,
				)
			}

			seen[node.CanonicalCode] =
				struct{}{}
		}
	}
}

func TestSection13BFinalPostgresHardGate(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")

	if dsn == "" {
		t.Skip(
			"VENDOR_SERVICE_TEST_DATABASE_URL not set",
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		4*time.Minute,
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
		t.Fatalf(
			"apply migrations: %v",
			err,
		)
	}

	food := loadFoodSeedForTest(t)
	shop := loadShopSeedForTest(t)

	applier := NewSeedApplier(pool, nil)

	actor := Actor{
		ActorRef:      "system:section13b-final",
		CorrelationID: "section13b-final-certification",
	}

	foodFirst, err :=
		applier.Apply(ctx, food, actor)

	if err != nil {
		t.Fatalf(
			"Food first seed: %v",
			err,
		)
	}

	foodSecond, err :=
		applier.Apply(ctx, food, actor)

	if err != nil {
		t.Fatalf(
			"Food replay: %v",
			err,
		)
	}

	shopFirst, err :=
		applier.Apply(ctx, shop, actor)

	if err != nil {
		t.Fatalf(
			"Shop first seed: %v",
			err,
		)
	}

	shopSecond, err :=
		applier.Apply(ctx, shop, actor)

	if err != nil {
		t.Fatalf(
			"Shop replay: %v",
			err,
		)
	}

	if foodFirst.Version.ID !=
		foodSecond.Version.ID ||
		!foodSecond.Replayed {
		t.Fatal(
			"Food seed is not idempotent",
		)
	}

	if shopFirst.Version.ID !=
		shopSecond.Version.ID ||
		!shopSecond.Replayed {
		t.Fatal(
			"Shop seed is not idempotent",
		)
	}

	expectedPublished := map[string]int{
		"FOOD": 1,
		"SHOP": 1,
	}

	rows, err := pool.Query(
		ctx,
		`
        SELECT ns.code, count(*)
          FROM canonical_taxonomy_versions v
          JOIN canonical_taxonomy_namespaces ns
            ON ns.id=v.namespace_id
         WHERE v.status='PUBLISHED'
         GROUP BY ns.code
         ORDER BY ns.code
        `,
	)
	if err != nil {
		t.Fatal(err)
	}

	for rows.Next() {
		var namespace string
		var count int

		if err = rows.Scan(
			&namespace,
			&count,
		); err != nil {
			rows.Close()
			t.Fatal(err)
		}

		want, exists :=
			expectedPublished[namespace]

		if !exists {
			rows.Close()
			t.Fatalf(
				"unexpected published namespace %s",
				namespace,
			)
		}

		if count != want {
			rows.Close()
			t.Fatalf(
				"%s published versions=%d want=%d",
				namespace,
				count,
				want,
			)
		}

		delete(
			expectedPublished,
			namespace,
		)
	}

	rows.Close()

	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}

	if len(expectedPublished) != 0 {
		t.Fatalf(
			"published namespaces missing: %v",
			expectedPublished,
		)
	}

	type counts struct {
		facets    int
		nodes     int
		terms     int
		authority int
	}

	expectedCounts := map[string]counts{
		"FOOD": {
			facets:    5,
			nodes:     60,
			terms:     7,
			authority: 0,
		},
		"SHOP": {
			facets:    1,
			nodes:     32,
			terms:     5,
			authority: 17,
		},
	}

	for namespace, expected := range expectedCounts {
		var facetCount int
		var nodeCount int
		var termCount int
		var authorityCount int

		err = pool.QueryRow(
			ctx,
			`
            WITH active AS (
                SELECT v.id
                  FROM canonical_taxonomy_versions v
                  JOIN canonical_taxonomy_namespaces ns
                    ON ns.id=v.namespace_id
                 WHERE ns.code=$1
                   AND v.status='PUBLISHED'
            )
            SELECT
                (
                    SELECT count(*)
                      FROM canonical_taxonomy_facets
                     WHERE version_id=(
                         SELECT id FROM active
                     )
                ),
                (
                    SELECT count(*)
                      FROM canonical_taxonomy_nodes
                     WHERE version_id=(
                         SELECT id FROM active
                     )
                ),
                (
                    SELECT count(*)
                      FROM canonical_taxonomy_node_terms
                     WHERE version_id=(
                         SELECT id FROM active
                     )
                ),
                (
                    SELECT count(*)
                      FROM canonical_taxonomy_authority_vocabulary
                     WHERE version_id=(
                         SELECT id FROM active
                     )
                )
            `,
			namespace,
		).Scan(
			&facetCount,
			&nodeCount,
			&termCount,
			&authorityCount,
		)

		if err != nil {
			t.Fatal(err)
		}

		actual := counts{
			facets:    facetCount,
			nodes:     nodeCount,
			terms:     termCount,
			authority: authorityCount,
		}

		if actual != expected {
			t.Fatalf(
				"%s counts=%+v want=%+v",
				namespace,
				actual,
				expected,
			)
		}
	}

	var duplicateFacets int
	var duplicateNodes int
	var duplicateTerms int
	var duplicateAuthority int

	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM (
            SELECT version_id, code
              FROM canonical_taxonomy_facets
             GROUP BY version_id, code
            HAVING count(*) > 1
          ) x
        `,
	).Scan(&duplicateFacets); err != nil {
		t.Fatal(err)
	}

	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM (
            SELECT version_id, canonical_code
              FROM canonical_taxonomy_nodes
             GROUP BY version_id, canonical_code
            HAVING count(*) > 1
          ) x
        `,
	).Scan(&duplicateNodes); err != nil {
		t.Fatal(err)
	}

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
          ) x
        `,
	).Scan(&duplicateTerms); err != nil {
		t.Fatal(err)
	}

	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM (
            SELECT
                namespace_id,
                version_id,
                authority_type,
                authority_ref,
                normalized_term
              FROM canonical_taxonomy_authority_vocabulary
             GROUP BY
                namespace_id,
                version_id,
                authority_type,
                authority_ref,
                normalized_term
            HAVING count(*) > 1
          ) x
        `,
	).Scan(&duplicateAuthority); err != nil {
		t.Fatal(err)
	}

	if duplicateFacets != 0 ||
		duplicateNodes != 0 ||
		duplicateTerms != 0 ||
		duplicateAuthority != 0 {
		t.Fatalf(
			"duplicates facets=%d nodes=%d terms=%d authority=%d",
			duplicateFacets,
			duplicateNodes,
			duplicateTerms,
			duplicateAuthority,
		)
	}

	var foodGarriLeak int
	var shopEbaLeak int

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
	).Scan(&foodGarriLeak); err != nil {
		t.Fatal(err)
	}

	if err = pool.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM canonical_taxonomy_nodes n
          JOIN canonical_taxonomy_versions v
            ON v.id=n.version_id
          JOIN canonical_taxonomy_namespaces ns
            ON ns.id=v.namespace_id
         WHERE ns.code='SHOP'
           AND n.canonical_code=
               'FOOD_DISH_TYPE_SOUPS_SWALLOW_EBA'
        `,
	).Scan(&shopEbaLeak); err != nil {
		t.Fatal(err)
	}

	if foodGarriLeak != 0 ||
		shopEbaLeak != 0 {
		t.Fatalf(
			"namespace leakage foodGarri=%d shopEba=%d",
			foodGarriLeak,
			shopEbaLeak,
		)
	}

	var relationshipTypes []string

	rows, err = pool.Query(
		ctx,
		`
        SELECT DISTINCT relationship_type
          FROM canonical_taxonomy_node_terms
         ORDER BY relationship_type
        `,
	)
	if err != nil {
		t.Fatal(err)
	}

	for rows.Next() {
		var relationship string

		if err = rows.Scan(
			&relationship,
		); err != nil {
			rows.Close()
			t.Fatal(err)
		}

		relationshipTypes =
			append(
				relationshipTypes,
				relationship,
			)
	}

	rows.Close()

	sort.Strings(relationshipTypes)

	expectedRelationships := []string{
		"ALIAS",
		"RELATED_TERM",
		"SYNONYM",
	}

	if strings.Join(
		relationshipTypes,
		",",
	) != strings.Join(
		expectedRelationships,
		",",
	) {
		t.Fatalf(
			"relationship types=%v want=%v",
			relationshipTypes,
			expectedRelationships,
		)
	}

	service := &Service{
		repo: NewRepository(pool),
	}

	clonedShop, err :=
		service.CloneActiveVersion(
			ctx,
			CloneActiveVersionInput{
				NamespaceCode: NamespaceShop,
				Actor: Actor{
					ActorRef:   "system:section13b-final-clone",
					CommandRef: "section13b-final-clone",
				},
			},
		)

	if err != nil {
		t.Fatalf(
			"clone Shop: %v",
			err,
		)
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
		clonedShop.ID,
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
		clonedShop.ID,
	).Scan(&clonedAuthority); err != nil {
		t.Fatal(err)
	}

	if clonedTerms != 5 ||
		clonedAuthority != 17 {
		t.Fatalf(
			"cloned Shop terms=%d authority=%d",
			clonedTerms,
			clonedAuthority,
		)
	}
}
