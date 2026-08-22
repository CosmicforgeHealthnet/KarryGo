package section13security

// Section 13I exclusions and privacy certification.
//
// Section 13 has a list of things it must NOT have become, and an absence is
// harder to certify than a presence: nothing fails when an unwanted capability
// quietly appears, because the tests that exist all test something else. This
// file is the check that fails.
//
// Two kinds of assertion are made. The first is static: the Section 13 source
// and migrations are read and searched for the markers of an excluded
// capability - a vector column, an external search client, an advertising
// ledger, an order or payment call. The second is schema-level: the database is
// asked directly whether Section 13 created anything that stores customer
// identity or search history.
//
// The static half is deliberately narrow-scoped to Section 13's own files.
// Scanning the whole service would trip over Section 8's real CRM tables and
// Section 12's real inventory reservations, which are legitimate elsewhere and
// only excluded from Section 13.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// section13Sources are the files Section 13 owns. The exclusion scan reads
// these and nothing else.
var section13Sources = []string{
	"../features/taxonomy",
	"../features/searchprojection",
	"../section13security",
}

// section13Migrations are the migration numbers Section 13 introduced.
var section13Migrations = []string{
	"000047", "000048", "000049", "000050", "000051", "000052", "000053",
	"000054", "000055", "000056", "000057", "000058", "000059",
}

// excludedMarker is one capability Section 13 must not have introduced,
// expressed as the tokens that would appear in code if it had.
type excludedMarker struct {
	capability string
	why        string
	tokens     []string
}

// The exclusion list from the Section 13 specification. Each entry names why
// the capability is excluded, because "we did not build X" is only useful to a
// future reader alongside "and here is the boundary X would have crossed".
var excludedCapabilities = []excludedMarker{
	{
		capability: "vector / embedding search",
		why:        "Section 13 ranks with bounded, inspectable weights. An embedding is not inspectable, and pgvector is not an enabled extension.",
		// Deliberately precise. A loose "vector(" token matches
		// PostgreSQL's own to_tsvector, which is native full-text search and
		// exactly what Section 13 is built on - flagging it would make this
		// gate cry wolf on the correct implementation.
		tokens: []string{
			"pgvector", "USING ivfflat", "USING hnsw", "embedding",
			"cosine_similarity", "l2_distance", "<=>",
		},
	},
	{
		capability: "external search engine deployment",
		why:        "PostgreSQL is the search backend. An adapter seam is documented, but no OpenSearch or Elasticsearch client is wired.",
		tokens:     []string{"elasticsearch", "opensearch", "olivere/elastic", "elastic.NewClient"},
	},
	{
		capability: "LLM / semantic generation",
		why:        "Suggestions come from the canonical taxonomy and projected names. Nothing is generated.",
		tokens:     []string{"openai", "anthropic", "completion(", "gpt-", "llm."},
	},
	{
		capability: "advertising billing and campaign authority",
		why:        "Ranking has two bounded commercial weights and no billing. Campaign authority belongs to Section 8.",
		tokens:     []string{"ad_campaign", "ad_budget", "auction", "bid_amount", "cost_per_click", "billing_account"},
	},
	{
		capability: "order creation and checkout authority",
		why:        "A search hit is a pointer back to the authoritative catalogue. Checkout revalidates; search never authorises it.",
		tokens:     []string{"CreateOrder", "PlaceOrder", "checkout_session", "CreateCheckout"},
	},
	{
		capability: "payment and wallet execution",
		why:        "Money is payment-wallet-service's authority and is untouched by discovery.",
		tokens:     []string{"walletclient", "PayFromWallet", "CreatePaymentIntent", "paystack"},
	},
	{
		capability: "logistics execution",
		why:        "Search reports serviceability as a signal. Dispatching a rider is the logistics services' authority.",
		tokens:     []string{"dispatchclient", "CreateDelivery", "AssignRider", "haulingclient"},
	},
	{
		capability: "stock reservation from search",
		why:        "SEARCH RESULT != STOCK RESERVATION. Reservation is Section 12B's authority, behind its own contract.",
		tokens:     []string{"ReserveStock", "CreateReservation", "CommitReservation", "inventory_reservations"},
	},
	{
		capability: "customer PII profiling",
		why:        "Search stores no customer identity. Long-term behavioural analytics is analytics-service's authority.",
		tokens: []string{
			"customer_search_history", "customer_search_profile", "user_embedding",
			"behavioral_profile", "personalization_vector", "customer_pii",
		},
	},
	{
		capability: "dark-store search ownership",
		why:        "Vendor Service indexes vendor-owned catalogues. KarryGo dark stores are not a Vendor Service concern.",
		tokens:     []string{"dark_store", "darkstore"},
	},
}

// readSection13Source returns every Section 13 Go file with its path.
func readSection13Source(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, dir := range section13Sources {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			out[path] = string(content)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	if len(out) == 0 {
		t.Fatal("no Section 13 source was found; the exclusion scan would be vacuous")
	}
	return out
}

func readSection13Migrations(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	for _, entry := range entries {
		for _, number := range section13Migrations {
			if !strings.HasPrefix(entry.Name(), number) {
				continue
			}
			content, readErr := os.ReadFile(filepath.Join("../../migrations", entry.Name()))
			if readErr != nil {
				t.Fatal(readErr)
			}
			out[entry.Name()] = string(content)
		}
	}
	if len(out) == 0 {
		t.Fatal("no Section 13 migration was found; the exclusion scan would be vacuous")
	}
	return out
}

// TestSection13IntroducedNoExcludedCapability is the exclusion gate.
func TestSection13IntroducedNoExcludedCapability(t *testing.T) {
	sources := readSection13Source(t)
	migrations := readSection13Migrations(t)

	// This file names every excluded token by definition, so scanning it would
	// report itself.
	delete(sources, filepath.Join("../section13security", "exclusions_test.go"))
	for path := range sources {
		if strings.HasSuffix(path, "exclusions_test.go") {
			delete(sources, path)
		}
	}

	scan := func(kind string, files map[string]string) {
		for path, content := range files {
			lower := strings.ToLower(content)
			for _, excluded := range excludedCapabilities {
				for _, token := range excluded.tokens {
					if !strings.Contains(lower, strings.ToLower(token)) {
						continue
					}
					// A token inside a comment is documentation of the
					// boundary, not an implementation of it. Only flag it when
					// it appears outside comment lines.
					if !appearsOutsideComments(content, token) {
						continue
					}
					t.Errorf("Section 13 %s %s contains %q, which suggests %s was introduced.\nWhy it is excluded: %s",
						kind, path, token, excluded.capability, excluded.why)
				}
			}
		}
	}

	scan("source", sources)
	scan("migration", migrations)
}

// appearsOutsideComments reports whether a token occurs on a line that is not
// a Go or SQL comment. Section 13's files legitimately DISCUSS the boundaries
// they do not cross, and flagging that prose would make the gate unusable.
func appearsOutsideComments(content, token string) bool {
	lowerToken := strings.ToLower(token)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "--") ||
			strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
			continue
		}
		if strings.Contains(strings.ToLower(trimmed), lowerToken) {
			return true
		}
	}
	return false
}

// TestSection13CreatedNoCustomerIdentityStorage asks the database rather than
// the source: did Section 13 create any table that could hold a customer
// search dossier?
//
// The check is on real columns of real Section 13 tables, so it catches a
// column added later without touching the excluded-token list.
func TestSection13CreatedNoCustomerIdentityStorage(t *testing.T) {
	pool, ctx := testPool(t)

	// Every table Section 13 owns.
	section13Tables := []string{
		"canonical_taxonomy_namespaces", "canonical_taxonomy_versions",
		"canonical_taxonomy_facets", "canonical_taxonomy_nodes",
		"canonical_taxonomy_node_terms", "canonical_taxonomy_mappings",
		"canonical_taxonomy_tags", "canonical_taxonomy_tag_assignments",
		"canonical_taxonomy_dietary_claims", "canonical_taxonomy_dietary_claim_assignments",
		"canonical_taxonomy_governance_cases", "canonical_taxonomy_seed_runs",
		"canonical_taxonomy_authority_vocabulary",
		"food_search_documents", "shop_search_documents",
		"food_search_terms", "shop_search_terms",
		"search_ranking_profiles", "search_cache_revisions",
		"search_projection_source_checkpoints", "search_projection_event_ledger",
		"search_projection_generations", "search_projection_generation_history",
		"search_projection_jobs",
	}

	// Column names that would indicate customer identity or search history
	// being stored inside a discovery table.
	forbiddenColumns := []string{
		"customer_id", "customer_ref", "customer_email", "customer_phone",
		"user_id", "user_ref", "end_user_id",
		"search_history", "query_history", "viewed_at", "clicked_at",
		"personalization", "embedding", "user_vector", "profile_vector",
	}

	for _, table := range section13Tables {
		rows, err := pool.Query(ctx,
			`SELECT column_name FROM information_schema.columns
              WHERE table_schema='public' AND table_name=$1`, table)
		if err != nil {
			t.Fatal(err)
		}
		var columns []string
		for rows.Next() {
			var column string
			if err = rows.Scan(&column); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			columns = append(columns, column)
		}
		rows.Close()

		if len(columns) == 0 {
			t.Errorf("Section 13 table %s does not exist; the privacy scan would skip it silently", table)
			continue
		}

		for _, column := range columns {
			for _, forbidden := range forbiddenColumns {
				if strings.Contains(column, forbidden) {
					t.Errorf("Section 13 table %s has column %q, which stores customer identity or behaviour in a discovery table",
						table, column)
				}
			}
		}
	}
}

// TestSection13EnabledNoPostgreSQLExtension proves certification did not turn
// one on, and that no Section 13 migration contains a CREATE EXTENSION.
//
// Extensions are a deployment-wide decision with operational consequences
// (replication, upgrade compatibility, managed-service availability); a
// section quietly enabling one is exactly the kind of change that is invisible
// until a production upgrade fails.
func TestSection13EnabledNoPostgreSQLExtension(t *testing.T) {
	for name, content := range readSection13Migrations(t) {
		// Comment-aware, because migration 000054 states in prose that no
		// extension is created - an assertion of the boundary, not a breach
		// of it, and a substring scan cannot tell the two apart.
		if appearsOutsideComments(content, "CREATE EXTENSION") {
			t.Errorf("Section 13 migration %s executes CREATE EXTENSION", name)
		}
	}

	pool, ctx := testPool(t)
	rows, err := pool.Query(ctx, `SELECT extname FROM pg_extension ORDER BY extname`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		switch name {
		case "plpgsql":
			// Present in every PostgreSQL database by default.
		case "vector", "pgvector":
			t.Errorf("extension %q is installed; Section 13 must not depend on vector search", name)
		default:
			t.Logf("pre-existing extension present: %s", name)
		}
	}
}

// TestSearchAnalyticsPayloadsStayBounded documents and pins the analytics
// boundary.
//
// Section 13 may emit bounded operational search events. It may not become an
// analytics warehouse: no event table with per-customer rows exists, and the
// outbox is the only place a search event could be produced.
func TestSearchAnalyticsPayloadsStayBounded(t *testing.T) {
	pool, ctx := testPool(t)

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables
          WHERE table_schema='public'
            AND (table_name LIKE '%search_analytics%'
              OR table_name LIKE '%search_event%'
              OR table_name LIKE '%search_history%'
              OR table_name LIKE '%search_impression%')`,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("%d search analytics tables exist; long-term behavioural analytics belongs to analytics-service", count)
	}

	// search_projection_event_ledger is a projection idempotency ledger, not a
	// customer analytics table. Confirm it holds no customer column - if that
	// ever changed, the name would still look innocent.
	rows, err := pool.Query(ctx,
		`SELECT column_name FROM information_schema.columns
          WHERE table_schema='public' AND table_name='search_projection_event_ledger'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var column string
		if err = rows.Scan(&column); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(column, "customer") || strings.Contains(column, "user") {
			t.Errorf("search_projection_event_ledger has column %q", column)
		}
	}
}
