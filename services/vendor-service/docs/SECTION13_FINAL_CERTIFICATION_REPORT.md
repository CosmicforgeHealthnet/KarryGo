# Section 13 Final Certification Report

**Catalogue, Taxonomy and Search (13A–13I)**

Every result below was produced by executing the named test against real
PostgreSQL through the isolated Vendor test runner
(`tools/vendor-test-runner`), which gives each package a freshly migrated
database cloned from a common template. Nothing in this report is claimed from
reading code.

---

## SECTION 13 RESULT: GO

| Subsection | Scope | Result |
|---|---|---|
| 13A | Canonical taxonomy foundation | **PASS** |
| 13B | Food/Shop seeds and Nigerian vocabulary | **PASS** |
| 13C | Mapping, tags, dietary claims, governance | **PASS** |
| 13D | Separate Food/Shop search projections | **PASS** |
| 13E | Search, ranking, filters, facets, suggestions | **PASS** |
| 13F | Operational search signals | **PASS** |
| 13G | Redis cache and incremental projection | **PASS** |
| 13H | Generations, rebuild and recovery | **PASS** |
| 13I | APIs, IAM/HMAC, security, scale, OpenAPI | **PASS** |

---

## 1. Key facts

| Fact | Value |
|---|---|
| Database migration tip | **000059** (`000059_vendor_taxonomy_search_permissions`) |
| Section 13 migrations | 000047–000059 (13 migrations, all paired up/down) |
| Total live Vendor routes after 13I | **556** |
| Section 13 routes added | **60** (10 vendor bearer + 50 internal HMAC-v2) |
| FOOD scale dataset | **24,000** searchable documents |
| SHOP scale dataset | **24,000** searchable documents |
| New authentication mechanisms | **none** — existing `shared/go/serviceauth` HMAC-v2 reused |
| PostgreSQL extensions enabled by Section 13 | **none** |
| Full-rebuild pipeline | **operational** - authoritative source, workers started, BUILDING to ACTIVE end-to-end |

---

## 2. Hard gates executed

Every Section 13 hard gate previously deferred was run in 13I. None remains
deferred.

| Gate | Test | Result |
|---|---|---|
| 13A foundation | `TestSection13AHardGate` | PASS |
| 13B final vocabulary | `internal/features/taxonomy` Section 13B suite | PASS |
| 13C mapping/governance | `mapping_test.go`, `mapping_engine_test.go`, `governance_test.go` | PASS |
| 13D Food/Shop isolation | `TestSection13DHardGate` | PASS |
| 13E search/ranking/facets/suggestions | `TestSection13EHardGate` | PASS |
| 13F stale search vs authoritative inventory | `TestSection13FStaleSearchCannotAuthorizeInventory` | PASS |
| 13F unknown signals | `TestSection13FUnknownSignalsRemainUnknown`, `TestSection13FUnknownReliabilityHasNoPerfectScore` | PASS |
| 13G event idempotency and ordering | `TestSection13GEventIdempotencyAndOrdering` | PASS |
| 13G Redis unavailable | `TestSection13GRedisUnavailableFallsBackToOrigin` | PASS |
| 13G stale cache revision | `TestSection13GStaleRedisRevisionCannotWin` | PASS |
| 13G FOOD/SHOP key isolation | `TestSection13GFoodShopKeyIsolation` | PASS |
| 13G bounded TTL | `TestSection13GTTLAlwaysBounded` | PASS |
| 13H active generation reads | `TestSection13HQueriesUseActiveGeneration` | PASS |
| 13H failure recovery and atomic activation | `TestSection13HFailureRecoveryAndAtomicActivation` | PASS |
| 13H competing activation | `TestSection13HCompetingGenerationsCannotBothActivate` | PASS |
| 13H worker lease exclusivity | `TestSection13HWorkerLeaseIsExclusive` | PASS |
| 13I HMAC-v2 | `internal/features/taxonomy/section13i_hmac_test.go` | PASS |
| 13I IAM separation | `internal/section13security/iam_test.go` | PASS |
| 13I isolation | `internal/section13security/isolation_test.go` | PASS |
| 13I search API | `internal/section13security/search_api_test.go` | PASS |
| 13I scale and query plans | `internal/section13security/scale_test.go` | PASS |
| 13I exclusions and privacy | `internal/section13security/exclusions_test.go` | PASS |
| 13I route/OpenAPI parity | `internal/section13contract`, `internal/crmcontract` | PASS |

### 13D expectations proved

Jollof Rice is discoverable through Food. Raw Rice 50kg is discoverable through
Shop. Neither appears in the other's results despite both matching "rice".
Re-certified independently at the API layer by
`TestSearchNamespacesNeverCrossOver`.

### 13E expectations proved

`jolof` → Jollof Rice · `shwarma` → Chicken Shawarma · `moin moin` → Moi Moi ·
`gari` → Garri · `paint beans` → the beans listing measured in paint ·
`50kg rice` → the matching Shop 50kg rice listing.

Also proved: a heavily promoted but irrelevant result cannot defeat textual
relevance; ordering and pagination are stable; facets are bounded; suggestions
are bounded; distance sorting requires a legitimate distance context.

### 13F expectation proved

Projection reports `IN_STOCK` → authoritative Shop inventory drops to zero →
checkout revalidation rejects the request. Inventory does not go negative.
Search does not authorize inventory consumption. `UNKNOWN` source state stays
`UNKNOWN` and is never upgraded to an artificial positive signal.

### 13G expectations proved

Duplicate event → applied once, no duplicate cache-revision bump. Newer revision
then older event → older marked `STALE`, cannot overwrite. Redis unavailable →
PostgreSQL search still correct. Stale Redis revision → newer PostgreSQL
revision wins. FOOD and SHOP cache keys remain isolated.

### 13H expectations proved

Generation 4 `ACTIVE`, generation 5 `BUILDING`, generation 5 fails halfway →
generation 4 remains `ACTIVE`. Retry → generation 5 `READY` → atomic activation
→ generation 5 `ACTIVE`, generation 4 `RETIRED`. Two workers cannot hold the
same lease. Two competing `READY` siblings cannot both activate. Incremental
workers remain revision-safe.

---

## 3. Security status

### HMAC-v2 — PASS

The Section 13 internal surfaces reuse `shared/go/serviceauth` unchanged. No
HMAC-v3, no service JWT, no API key, no Section 13 token type was introduced.
Requests are signed in tests with `serviceauth.SignRequest`; only negative cases
construct a signature by hand.

| Attack | Result |
|---|---|
| Missing `X-Service-Name` | rejected |
| Missing signature | 401 |
| Missing nonce | 401 |
| Missing timestamp | 401 |
| Invalid signature | 401 |
| Signature computed with another service's secret | 401 |
| Timestamp 30 minutes stale | 401 |
| Timestamp 30 minutes in the future | 401 |
| Authentic caller absent from the allowlist | 403 |
| Unknown caller with an unconfigured secret | rejected |
| Body altered after signing | 401 |
| HTTP method altered after signing | rejected |
| Path altered after signing | 401 |
| Query string altered after signing (RequestURI is signed) | 401 |
| Valid signed request | **200** |

### Nonce replay protection — PASS

A signed request is accepted once; the nonce row is then verified to exist in
`vendor_service_nonces`, and a re-signed request reusing that nonce is rejected
with 401. The row is read directly from PostgreSQL rather than inferred from the
status code, because "the second call failed" would also be true of an
in-process guard that a restart would forget.

### IAM separation — PASS

| Assertion | Result |
|---|---|
| Six Section 13 capabilities catalogued with the expected risk classification | PASS |
| `vendor.taxonomy.platform.admin` granted to no vendor role | PASS |
| `vendor.search.ranking.admin` granted to no vendor role | PASS |
| `vendor.search.projection.admin` granted to no vendor role | PASS |
| Owner and General Manager hold all three vendor-scoped capabilities and none of the platform ones | PASS |
| No vendor staff member (Owner, GM, Branch Manager, Analyst) can reach any platform capability | PASS |
| `taxonomy.read` does not imply `taxonomy.mapping.manage` | PASS |
| `search.read` does not imply ranking or projection administration | PASS |
| Mapping management does not imply taxonomy publication | PASS |
| Deny-by-default when a permission is absent | PASS |
| A custom role named "Owner" with no permissions authorizes nothing | PASS |
| Branch-scoped grant cannot reach another branch or a business-wide resource | PASS |

### FOOD/SHOP isolation — PASS

| Cross-boundary attack | Result |
|---|---|
| SHOP version read through FOOD, and the reverse | refused |
| SHOP node read through FOOD, and the reverse | refused |
| FOOD node under a SHOP version id | refused |
| FOOD mapping onto a SHOP node | refused by the service **and** by PostgreSQL |
| SHOP mapping onto a FOOD node | refused |
| `FOOD_ITEM` source addressed through SHOP, and the reverse | refused |
| Unknown namespace defaulting to FOOD | refused, never defaulted |
| FOOD generation read/activated/retired through SHOP | refused; FOOD generation unchanged |
| FOOD ranking profile read/updated through SHOP | refused; weights unchanged |
| Food search returning a Shop document | never |
| Shop search returning a Food document | never |
| Shop filter block on the Food search path, and the reverse | 400 |

### Cross-vendor isolation — PASS

Another vendor's Food item, another vendor's second branch, another vendor's
Shop product: all refused as **404**, never 403, so no status code discloses
existence. A mapping another tenant attached to the same `source_ref` is never
returned. `business_ref` or `branch_ref` in a command body is rejected with 400
rather than silently ignored.

### Ranking integrity — PASS

Promotion weight above the non-text ceiling, commercial weight above the
ceiling, and the ceiling raised to exact-name relevance are all refused, and the
stored profile is verified unchanged after each attempt. An in-bounds adjustment
is accepted, proving the refusals are about the bound rather than about updates
failing generally.

### Privacy — PASS

No Section 13 table has a customer, user, history, personalization or embedding
column — asserted against `information_schema` across all 24 Section 13 tables.
No search analytics, search event, search history or impression table exists.
The suggestion API has no field in which a customer identity could be supplied.
Search responses expose no projected-document internals (`search_text`,
`search_tokens`, `normalized_name`, hint payloads and canonical taxonomy are all
absent from the response body, asserted).

### Exclusions — PASS

A static scan over Section 13's own source and migrations (comment-aware, so
documentation of a boundary is not mistaken for a breach of it) finds no marker
of: vector/embedding search, pgvector, OpenSearch, Elasticsearch, LLM/semantic
generation, advertising billing or campaign authority, order creation, checkout
authority, payment/wallet execution, logistics execution, stock reservation,
customer PII profiling, or dark-store search ownership.

No Section 13 migration executes `CREATE EXTENSION`, and no vector extension is
installed in the certification database.

---

## 4. Scale and query-plan findings

Dataset: **24,000 FOOD documents and 24,000 SHOP documents** at one generation,
built with `generate_series`, plus derived TOKEN and PHRASE term rows, then
`ANALYZE`d so every plan is chosen from real statistics.

### Observed timings

Recorded as evidence, not asserted as thresholds. Single run, developer machine,
PostgreSQL in Docker.

| Query | Namespace | Observed |
|---|---|---|
| exact dish | FOOD | ~247 ms |
| prefix | FOOD | ~225 ms |
| typo tolerance | FOOD | ~226 ms |
| synonym phrase | FOOD | ~645 ms |
| cuisine filter | FOOD | ~130 ms |
| dish filter | FOOD | ~336 ms |
| availability filter | FOOD | ~382 ms |
| facets (4 families) | FOOD | ~299 ms |
| exact listing | SHOP | ~399 ms |
| prefix | SHOP | ~220 ms |
| measurement term | SHOP | ~298 ms |
| brand filter | SHOP | ~202 ms |
| category filter | SHOP | ~203 ms |
| stock filter | SHOP | ~830 ms |
| facets (4 families) | SHOP | ~805 ms |

Suggestions returned the full bounded set of 10 in both namespaces.

### Query-plan gate

The gate is **not** "no plan may contain a Seq Scan" — a small lookup table is
correctly scanned sequentially, and asserting otherwise pins a planner decision
rather than an architectural property. Two narrower things were asserted:

1. **Index presence.** All twelve required indexes exist: GIN on the generated
   `tsvector` for both namespaces; B-tree on `(generation, active, branch_ref)`,
   on source identity, and on exact/prefix/typo-candidate term lookup for both
   namespaces.

2. **Index capability.** With `SET LOCAL enable_seqscan = off`, PostgreSQL
   planned every one of the ten real search queries **without** a sequential
   scan of any 24,000-row search table. Queries covered: full-text candidate
   scan, exact term lookup, prefix term lookup, typo candidate lookup,
   branch-scoped generation scan, and document hydration by id — for FOOD and
   SHOP.

### Bounded behaviour

Page size clamped to 50; suggestion limit clamped to 10; facet families
truncated to 8; a ninth facet family refused; malformed cursor refused; empty
query refused; over-long query refused; filter value lists capped at 32.

Cursor pagination walked six pages of 25 with **no document appearing twice and
none dropped**.

### No N+1

Transaction cost measured at page size 1 and page size 50: **4 and 4**. Cost
that grows with hit count is the shape of an N+1; measured growth was zero.

---

## 5. OpenAPI and route parity

| Check | Result |
|---|---|
| Every live Section 13 route appears in a specification | PASS |
| Every documented Section 13 operation exists in the router | PASS (no phantom) |
| Section 13 route family counts (10 / 33 / 6 / 11) | PASS |
| No internal route mounted under the public vendor base | PASS |
| Public Section 13 surface is classification-only | PASS |
| Specification security scheme matches the mount point | PASS |
| Every internal operation documents the four HMAC headers | PASS |
| Whole-service route inventory (556) matches production registration | PASS |
| Every live route appears in `API_ROUTE_REGISTER.md` | PASS |

Specifications: `section13-taxonomy.openapi.json` (10 operations, bearer),
`section13-taxonomy-admin.openapi.json` (33, HMAC-v2),
`section13-search.openapi.json` (6, HMAC-v2),
`section13-search-projection.openapi.json` (11, HMAC-v2).

---

## 6. Defects found and fixed during 13I

Three genuine defects were found by running the previously-deferred gates. Each
was fixed at its root rather than worked around.

**1. Projection rejected documents with empty optional vocabulary (13D, product
defect).** The Food and Shop document builders bound Go `nil` slices to
`NOT NULL` array columns. A Food item created but not yet placed on a menu, or a
Shop variant with no barcode, failed projection outright and silently never
became discoverable — both ordinary catalogue states. Fixed in the builders, so
the invariant holds for every backend rather than only the tested one.

**2. Concurrent generation activation surfaced as an opaque internal error
(13H, product defect).** Activation runs at `SERIALIZABLE` and takes its
snapshot at the advisory-lock statement, before a racing winner commits, so the
loser could be aborted with SQLSTATE 40001 instead of returning the intended
domain conflict. The invariant held — only one generation activated — but the
administration API reported a 500 for a routine race, and the fact that the
invariant held was hidden. Serialization failures now map to
`ErrGenerationActivationConflict`. The isolation level was deliberately left at
`SERIALIZABLE`, because not every generation mutation takes the advisory lock.

**3. The 13D hard gate could not run (test defect).** Its projection reset used
`TRUNCATE` without `CASCADE` and predated the Section 13E term tables that hold
a foreign key onto the documents, so the gate had been failing at setup.

---

## 7. Full isolated Vendor regression

Executed with `go run ./tools/vendor-test-runner` (the authoritative isolated
runner), which builds one migrated template database and clones a fresh
database per package. No package shares a database with another, and no
destructive test runs against a shared one.

```
PASS: test database safety guard
PASS: template ready (59 migrations, 43 measurement definitions)
Test-bearing packages: 35
Packages passed: 35/35
VENDOR ISOLATED REGRESSION: GO
```

**35/35 packages pass**, re-run after the production rebuild closure, covering
Sections 5 through 13 — every feature package,
every per-section contract package, the whole-service route inventory, the
migration suite, and both new Section 13 packages
(`internal/section13contract`, `internal/section13security`).

### One pre-existing test updated, and why it is not a hidden regression

The first full run after Section 13I failed one package:

```
FAIL: internal/database
--- FAIL: TestSection5SchemaExists
    migrate_test.go:215: expected Owner to hold every permission (127), got 124
```

That assertion was true while every catalogued permission was a vendor
capability. Section 13I deliberately introduces three **platform** capabilities
that no vendor role may hold — requirement 6 of the section, not an accident.
The three the Owner does not hold are exactly
`vendor.taxonomy.platform.admin`, `vendor.search.ranking.admin` and
`vendor.search.projection.admin`.

The assertion was **split rather than relaxed**: Owner must now hold every
vendor-scoped permission **and** exactly zero platform-scoped ones. That is a
stronger statement than the original count comparison. It fails if a future
migration forgets to grant Owner a new vendor capability, and it also fails if
one sweeps a platform capability into a vendor role — a failure mode the
original assertion would have actively rewarded.

---

## 8. Static quality gates

| Gate | Result |
|---|---|
| `go build ./...` | **PASS** |
| `go vet ./...` | **PASS** |
| `gofmt` on all Section 13 code | **PASS** |
| `git diff --check` | **PASS** |
| Migration numbering contiguous, no duplicates | **PASS** (000047–000059) |
| Every migration has a paired `.down.sql` | **PASS** |
| No `CREATE EXTENSION` in any Section 13 migration | **PASS** |
| No secret material committed | **PASS** — allowlists and secrets are environment configuration |
| No raw customer PII search storage | **PASS** |
| No frontend or mobile change | **PASS** — no file outside `services/vendor-service` was modified |
| Full isolated Vendor regression | **PASS** — 35/35 packages |

---

## 9. Production rebuild closure

The previous edition of this report recorded one non-blocking limitation:
full-rebuild workers were not started, because Section 13H's
`SearchFullRebuildSource` port had no concrete implementation. **That limitation
is resolved and is removed from section 10 below.**

A privileged rebuild request now progresses on its own:

```
POST /internal/vendor/v1/search/projections/{FOOD|SHOP}/rebuilds   (HMAC-v2)
  -> BUILDING     new generation allocated; previous stays ACTIVE and serving
  -> rebuild worker streams the AUTHORITATIVE catalogue in batches of <= 200
  -> validation   document_count must equal processed_count
  -> READY
  -> activation   atomic: new ACTIVE, base RETIRED, cache revision bumped
  -> ACTIVE
```

### 9.1 Authoritative sources

`internal/searchrebuild` reads the domain that owns each value:

| Projected value | Authoritative source |
|---|---|
| Food item, menu, provider capability, price, status | `food_items`, `food_menus`, `food_branch_item_overrides` |
| Food availability | `food_item_availability` |
| Food catalogue revision | `food_catalogue_revisions` |
| Shop product, brand, class, category | `shop_products`, `shop_categories` |
| Shop variant, SKU, pack, temperature zone, price | `shop_variants` |
| Shop listing, eligibility, operating model, policies | `shop_branch_listings` |
| Sell units, measure names, local aliases | `shop_listing_sell_units`, `shop_sell_units`, `shop_measure_definitions`, `shop_measure_aliases` |
| Identifiers and barcodes | `shop_product_identifiers` |
| Fresh attributes, handling flags | `shop_fresh_attributes`, `shop_variant_handling_flags` |
| Inventory signal | `shop_inventory_balances` |
| Shop catalogue / inventory revisions | `shop_catalogue_revisions`, `shop_inventory_revisions` |
| Canonical classification and vocabulary | `canonical_taxonomy_mappings`, `_nodes`, `_facets`, `_node_terms` (PUBLISHED versions only) |
| Semantic tags, dietary claims | `canonical_taxonomy_tag_assignments`, `_dietary_claim_assignments` |
| Trust | `vendor_verification_projections` (BUSINESS dimension) |
| Serviceability | `vendor_branches`, `vendor_branch_service_areas` |
| Reliability | none exists — projected as `UNKNOWN` |
| Commercial / promotion | none exists — no promotion applied |

It reads **no** search document, **no** search term table and **no** Redis key.

### 9.2 Batching and checkpointing

Keyset pagination over immutable primary keys, never OFFSET. Batch ceiling
remains the certified 200. Every enrichment is one bulk `= ANY($1)` per concern
per batch, so query count is constant in batch size. The checkpoint is the last
id of the previous batch and is persisted by the worker runtime, so a worker
that dies resumes rather than restarting.

### 9.3 Workers

Two `ProjectionWorker`s, started through the existing `jobs.Job` runtime in
`cmd/main.go` — no second scheduler was introduced:

| Worker | Job types | Lease | Claim batch | Poll |
|---|---|---|---|---|
| `search-projection-rebuild` | `FULL_REBUILD`, `SCOPED_REBUILD` | 2 min | 1 | 10 s |
| `search-projection-validate-activate` | `GENERATION_VALIDATE_ACTIVATE` | 30 s | 5 | 5 s |

Both are gated on `VENDOR_SEARCH_ADMIN_ENABLED`. Standing them down freezes
rebuilds; discovery keeps answering from the ACTIVE generation.

### 9.4 Closure gates executed

| Gate | Test | Result |
|---|---|---|
| Real Food rebuild end-to-end | `TestRealFoodRebuildEndToEnd` | **PASS** |
| Real Shop rebuild end-to-end | `TestRealShopRebuildEndToEnd` | **PASS** |
| Rebuild reads the domain, not the projection | `TestRebuildReadsTheDomainNotTheProjection` | **PASS** |
| Failure preserves ACTIVE; retry completes; base retired | `TestRebuildFailurePreservesActiveGeneration` | **PASS** |
| Unvalidated generation is never activated | `TestUnvalidatedGenerationIsNeverActivated` | **PASS** |
| Bounded keyset batching, no repeats, checkpoint advances | `TestRebuildBatchingIsBoundedAndKeysetPaged` | **PASS** |
| No per-document query growth | `TestRebuildBatchQueriesDoNotScaleWithBatchSize` | **PASS** |
| Rebuild runs with no Redis at all | `TestRebuildRunsWithoutRedis` | **PASS** |
| HMAC-v2 request drives BUILDING → ACTIVE through the real API | `TestHMACRebuildRequestDrivesGenerationToActive` | **PASS** |
| Unsigned rebuild rejected and queues nothing | `TestHMACRebuildRequestRequiresSignature` | **PASS** |
| Scale: 1,507 documents in bounded batches | `TestRebuildAtScaleStaysBatchedAndBounded` | **PASS** |
| Rebuild keyset scan is index-capable | `TestRebuildSourceQueryPlansAreIndexCapable` | **PASS** |

Observed: 1,507 Food documents rebuilt in ~9.8 s over batches of at most 200,
with `document_count == processed_count` and every document traceable to a
catalogue row. Query cost measured at batch size 1 and batch size 7: identical.

### 9.5 Three further defects found and fixed by these gates

**1. A completed build could never become READY (product defect, 13H).**
`CompleteGenerationBuild` recorded the completion but enqueued nothing, so a
finished rebuild sat at `BUILDING` forever with no job left to validate it. This
was the real cause of the symptom the previous limitation described, and it
would have persisted even after a source was implemented. Completion, its
evidence and the follow-on `GENERATION_VALIDATE_ACTIVATE` job now commit in one
transaction; the job is deduplicated on `(namespace, generation)`.

**2. A signal with unknown reliability could not be stored (product defect,
13F).** `ApplyOperationalSignals` bound nullable basis-point parameters whose
only appearances were inside `jsonb_build_object` and `COALESCE`, so PostgreSQL
could not infer their type and the statement failed with "could not determine
data type" whenever they were NULL. NULL basis points are the *normal* shape for
reliability — Section 13F requires unknown to stay unknown — so the one case the
section mandates was the one case that could not be written. Every parameter in
both signal statements is now explicitly typed.

**3. Signals could not be applied alongside a rebuild.** Signals are keyed by
document id, which only the projector knew how to derive.
`FoodSearchDocumentID`/`ShopSearchDocumentID` now export that derivation so a
caller producing a document and its signals in one pass uses the same function
rather than reimplementing the hash.

---

## 10. Remaining known limitations (non-blocking)

**1. Both internal surfaces are configuration-gated and off by default.**
`VENDOR_TAXONOMY_ENABLED` and `VENDOR_SEARCH_ENABLED` default to false, and each
requires its own caller allowlist plus HMAC secrets before any consumer can call
it. Startup validation refuses an enabled surface with an empty allowlist rather
than silently answering 403 to everyone.

**2. No platform operator console exists.** The platform taxonomy and search
administration surfaces are internal APIs only. UI requirements are documented
in `UI_UX_REQUIREMENTS.md`; no UI was built, and none is in scope for Section 13.

**3. Repository-wide CRLF checkout.** `core.autocrlf=true` means the working
copy stores CRLF while git stores LF, so `gofmt -l` over the whole repository
flags files that are clean as committed. This predates Section 13 and affects
217 files. Section 13's own files, and the LF-normalized content of every file
this section modified, are gofmt-clean.

**4. `13I_CONTEXT.txt` is implementation scratch.** It is untracked and must not
be committed as product documentation. So is the pre-existing
`vendor-test-runner.exe` build artifact.
