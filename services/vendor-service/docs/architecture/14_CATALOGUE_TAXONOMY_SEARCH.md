# 14. Catalogue, Taxonomy and Search (Section 13)

> **Numbering note.** This is the Section **13** master document. It is filed as
> `14_` because `13_CAMPAIGN_EXECUTION_MODEL.md` already occupies that slot from
> Section 8F — architecture documents in this directory are numbered by
> creation order, not by product section. Nothing else in Section 13 depends on
> the file number.

This is the single end-to-end reference for Section 13A–13I. It replaces the
per-subsection notes rather than sitting alongside nine of them.

---

## 1. Purpose and ownership

Section 13 answers one question for the platform: **given what a person typed,
which vendor Food items and which vendor Shop listings should they see, in what
order?**

It owns three things and nothing else:

1. **The canonical taxonomy** — the platform's shared vocabulary for classifying
   what vendors sell. Versioned, governed, published.
2. **Two search projections** — derived, denormalised read models of the Food
   and Shop catalogues, built for query speed.
3. **The search engine over them** — normalisation, matching, ranking,
   filtering, faceting and suggestion.

It owns none of the following, and the boundary is enforced, not merely stated:

| Not owned by Section 13 | Owner | Why the boundary matters |
|---|---|---|
| Food catalogue truth | Section 9 (`food`) | A projection that disagrees with the catalogue is stale, not authoritative. |
| Shop catalogue and inventory truth | Section 12 (`shop`) | Search reports stock; it never holds it. |
| Orders, cart, checkout | future commerce sections | A search hit is a pointer, not a promise. |
| Payment and wallet movement | `payment-wallet-service` | Discovery touches no money. |
| Logistics execution | dispatch / taxi / hauling services | Serviceability is a signal, not a dispatch. |
| Long-term behavioural analytics | `analytics-service` | Vendor Service is not an analytics warehouse. |

### The authority direction

```
   Food authoritative domain            Shop authoritative domain
   (Section 9: menus, items,            (Section 12: products, variants,
    availability, pricing)               listings, inventory, pricing)
             |                                       |
             v                                       v
   food_search_documents                  shop_search_documents
   food_search_terms                      shop_search_terms
             |                                       |
             v                                       v
      Food search engine                     Shop search engine
             |                                       |
             +------------------+--------------------+
                                v
                        Redis (cache only)
```

Authority flows **downward only**. Redis may be wrong or absent; PostgreSQL may
be stale relative to the catalogue; neither may ever be treated as the truth
that a catalogue, an inventory level, a price or a verification decision is read
from. Every arrow in that diagram is a one-way derivation.

---

## 2. Namespaces: FOOD and SHOP are not one system

The single most consequential structural decision in Section 13 is that Food and
Shop are **two parallel systems that share a design, not one system with a type
column**.

They have separate canonical namespaces, separate taxonomy versions, separate
projection tables, separate term tables, separate ranking profiles, separate
generations, separate cache key spaces and separate engines.

The reason is concrete. Both catalogues contain "rice". A Jollof Rice dish and a
50kg sack of raw rice match the same text and mean entirely different things to
a customer: one is dinner in twenty minutes, the other is a month of groceries.
Merging them would produce a result list that is useless in both directions, and
no amount of filtering added afterwards reliably unmerges it.

Because the separation is structural rather than a filter, the failure mode it
prevents cannot be reintroduced by forgetting a `WHERE` clause. Certification
attacks it from both directions (§13 of this document).

---

## 3. Canonical taxonomy (13A, 13B, 13C)

### 3.1 Versioning (13A)

A namespace holds an ordered series of versions. Exactly one may be `PUBLISHED`.

```
DRAFT ──publish──> PUBLISHED ──(next publish)──> RETIRED
```

- Version numbers are allocated under a lock on the namespace row, so two
  concurrent drafts cannot claim the same number.
- Every command is idempotent on a caller-supplied command reference; a replay
  returns the original result rather than allocating a second version.
- Each mutation writes taxonomy state, change-history evidence and an outbox
  event in **one transaction**. There is no window where the vocabulary changed
  but the evidence did not.

Facets are the axes of classification (cuisine, dish type, category); nodes are
the values on an axis. A facet is `FLAT` or `HIERARCHICAL`; a node in a flat
facet may not have a parent, and a node may not outlive its parent or facet as
active. `validateDraftForPublication` enforces the whole structural rule set,
and the Section 13I `POST .../validate` route runs **that same predicate** in a
rolled-back transaction — a separate validator would be free to drift from the
one that actually gates publication.

### 3.2 Seeds and Nigerian vocabulary (13B)

Seed bundles are fingerprinted and applied idempotently, recording append-only
evidence in `canonical_taxonomy_seed_runs` (facet, node, term and authority
counts, actor, version, timestamps).

The vocabulary is deliberately Nigerian-first: aliases and synonyms carry the
spellings people actually type. `jolof` reaches Jollof Rice, `shwarma` reaches
Chicken Shawarma, `moin moin` reaches Moi Moi, `gari` reaches Garri, and "paint"
is a real measurement term for beans rather than a typo to be corrected away.
This is why the ranking model treats alias and synonym matches as first-class
signals with their own weights rather than as fuzzy fallbacks.

Re-running a seed is a destructive publication act — it rewrites the vocabulary
every vendor is classified against — so it is **not exposed as an HTTP command**.
The internal surface reads seed evidence and status; application goes through
the seed applier's own path.

### 3.3 Mapping, semantic tags, dietary claims, governance (13C)

- **Mapping** binds a source resource (a Food item, a Shop product or variant)
  to a canonical node. Mappings may only target a **published** version, are
  typed (`PRIMARY`/`SECONDARY`/`DERIVED`) and carry a confidence basis.
- **Facet-mapping replacement** is the command a vendor actually uses: it
  replaces every mapping a resource holds inside one facet, atomically and
  idempotently, at `SERIALIZABLE` isolation.
- **Semantic tags** are governed attribute/discovery vocabulary.
- **Dietary claims** carry a verification policy (`DECLARATION`,
  `EVIDENCE_REQUIRED`, `PLATFORM_VERIFIED`) and a risk level. An assignment
  moves through declared → evidence → verified/rejected/expired. A declaration
  is never silently upgraded into a verified claim.
- **Governance cases** open against a taxonomy subject and move through a review
  workflow with revision-guarded transitions.

---

## 4. Projections (13D)

`food_search_documents` and `shop_search_documents` are denormalised, generation-
keyed read models. Each row carries its identity refs, its display name, its
normalised name, token/alias/synonym arrays, canonical taxonomy, operational
hints, the revisions it was derived from, and a generated `tsvector`.

Normalisation (13D) is Unicode-aware and deliberately conservative: it lowercases
and folds, but it does not strip the punctuation and word breaks Nigerian terms
depend on. "Moi Moi" must not become "moimoi" if that stops "moi moi" matching.

**A defect found and fixed in 13I.** The document builders bound Go `nil` slices
to `NOT NULL` array columns, so any document whose optional vocabulary was empty
was rejected outright by the database. A Food item created but not yet placed on
a menu has no menu refs; a Shop variant without a barcode has no barcode terms.
Both are ordinary states, and both silently failed to become discoverable. The
builders now normalise every text array to non-nil, so the invariant holds for
every backend rather than only the one that happened to be tested.

---

## 5. Search engine (13E)

Query handling, in order:

1. **Normalise** the query with the same normaliser the documents were built
   with. Raw search syntax is never preserved into SQL.
2. **Gather candidates** by union of: full-text match on the generated
   `tsvector`; exact term match; prefix term match; and bounded typo candidates
   (same first character, length within ±2, then bounded Levenshtein).
3. **Filter** on the namespace's own filter set.
4. **Score** with the ranking profile.
5. **Sort, cursor-paginate, facet.**

### The ranking bound that keeps search honest

A ranking profile is a vector of bounded weights (0–10000) plus three limits:
`max_non_text_boost`, `max_typo_distance` (0–2) and `max_candidates` (50–2000).

Two of the weights are commercial: `commercial_weight` and `promotion_weight`.
Both are capped by `max_non_text_boost`, and `max_non_text_boost` is required to
be **strictly less than `exact_name_weight`**.

That single inequality is the guarantee that a promoted result cannot outrank a
genuinely relevant one. It is enforced three times over — by database `CHECK`
constraints, by `validateRankingProfile` when the engine loads a profile, and by
the Section 13I administration path before a weight is stored — because it is
the one ranking rule whose violation would be invisible in normal operation and
corrosive to every result on the platform.

Sponsored placement is represented explicitly (`ORGANIC` / `SPONSORED`) and
travels in the response. There is no covert boost.

Distance sorting requires a legitimate distance context and is refused without
one, rather than silently degrading to relevance order.

---

## 6. Operational signals (13F)

Documents carry availability, serviceability, trust and reliability hints, plus
the source revisions they were derived from.

The rule that governs all of them: **a signal is a hint for ordering and
filtering, never an authorisation.** An `IN_STOCK` Shop result is what the
projection last observed. Checkout revalidates against authoritative inventory
and rejects the purchase if stock has since gone; inventory never goes negative
because of something search said.

`UNKNOWN` stays `UNKNOWN`. A missing signal is never upgraded into a positive
one, because "we do not know whether this is available" and "this is available"
lead to different customer outcomes and only one of them is honest.

---

## 7. Cache and incremental projection (13G)

Redis is a latency cache in front of the PostgreSQL projection, and nothing more.

Cache keys are built from `(namespace, class, generation, cache revision, query
identity)`. Because the generation and the cache revision are part of the key, a
stale entry cannot be served after either changes — it simply never matches.
FOOD and SHOP occupy separate key spaces.

Incremental projection consumes source events through
`search_projection_event_ledger`, which gives:

- **Idempotency** — a duplicate event is recognised and applied once. It does
  not bump the cache revision a second time.
- **Ordering safety** — an event carrying an older source revision than the
  document already holds is `STALE` and is discarded. Late delivery cannot
  overwrite newer state.

If Redis is unavailable, every engine reads through to PostgreSQL and returns
correct results more slowly. This is why the health endpoint reports cache
availability **separately** from search health: an operator who cannot tell the
two apart will fail over a search tier that is answering correctly.

---

## 8. Generations, rebuilds and recovery (13H)

```
BUILDING ──validate──> READY ──activate──> ACTIVE ──superseded──> RETIRED
    │
    └── failure ──> FAILED ──retry──> BUILDING
```

- A rebuild allocates a **new** generation and builds alongside the live one.
  The `ACTIVE` generation keeps serving for the whole rebuild.
- Only `READY` may become `ACTIVE`. A `FAILED` or half-built generation can
  never replace the live one.
- Activation is atomic: the new generation becomes `ACTIVE`, the generation it
  was built from becomes `RETIRED`, and the cache revision bumps, in one
  transaction. A unique partial index enforces at most one `ACTIVE` generation
  per namespace at the schema level.
- Two sibling generations built from the same base cannot both activate. The
  loser is refused by a compare-and-swap on the base generation.
- Workers claim jobs under leases with `SKIP LOCKED`; a worker that dies lets
  its lease lapse for another to adopt. Retries back off, and exhausted jobs
  dead-letter rather than spinning.

**A defect found and fixed in 13I.** Activation runs at `SERIALIZABLE` and takes
its snapshot at the advisory-lock statement, i.e. *before* a racing winner
commits. The loser could therefore be aborted by PostgreSQL with SQLSTATE 40001
instead of returning the intended domain conflict — surfacing a routine race as
an opaque 500 on the administration API, and hiding the fact that the invariant
actually held. A serialization failure always aborts its transaction, so it now
maps to `ErrGenerationActivationConflict`. The isolation level was deliberately
left at `SERIALIZABLE` rather than lowered: not every generation mutation takes
the advisory lock, so it is doing real work for the paths that do not.

### The rebuild source: where a rebuilt document actually comes from

`internal/searchrebuild` implements `SearchFullRebuildSource` against the
authoritative Vendor domain. It reads Food items, menus, prices and availability
from Section 9; Shop products, variants, listings, sell units, identifiers and
inventory balances from Section 12; canonical classification and vocabulary from
Sections 13A-13C (published versions only); business verification from Section 7;
and branch geography from Section 6.

It reads **no search document, no search term table and no Redis key.** That is
the property that makes a rebuild a recovery mechanism rather than a copy: if
the projection were its own source, a corrupted projection would rebuild into an
identically corrupted one, and the mechanism that exists to recover from
corruption could never recover from it. The certification gate proves this by
corrupting the live projection before rebuilding and confirming the result
matches the catalogue instead.

The package sits outside `internal/features` deliberately. Rebuilding one Food
document touches five features at once, so it belongs to no single feature - and
it could not go through the owning features' repositories anyway, because
`internal/architecture_test.go` forbids one feature importing another's
repositories and those repositories expose per-entity reads rather than the bulk
keyset scans a rebuild needs.

**Batching.** Keyset pagination over immutable primary keys (`food_items.id`,
`shop_branch_listings.id`), never OFFSET - an OFFSET scan over a mutable
catalogue silently skips and duplicates rows as data shifts beneath a long
rebuild. The ceiling is 200 documents per batch. Every enrichment is a bulk
`= ANY($1)` over the batch, so query count per batch is constant in batch size.
The checkpoint is the last id of the previous batch and is persisted by the
worker runtime, so a worker that dies resumes rather than restarting.

**Unknown stays unknown.** Reliability has no authoritative source anywhere in
Vendor Service, so every rebuilt document carries reliability `UNKNOWN` with no
basis points. There is no authoritative promotion source either, so a rebuild
never promotes anything.

### Workers

Two `ProjectionWorker`s run through the existing `jobs.Job` runtime, started in
`cmd/main.go`. Section 13 introduced no second scheduler.

| Worker | Job types | Lease | Claim batch | Poll |
|---|---|---|---|---|
| `search-projection-rebuild` | `FULL_REBUILD`, `SCOPED_REBUILD` | 2 min | 1 | 10 s |
| `search-projection-validate-activate` | `GENERATION_VALIDATE_ACTIVATE` | 30 s | 5 | 5 s |

They are two workers rather than one because `ProjectionWorker` takes a single
handler for the job types it claims, and the two stages are genuinely different
work: a rebuild streams a catalogue for minutes, an activation performs one
short guarded transition. Separate leases mean a long rebuild never delays
activating a generation that has already finished.

Both are gated on `VENDOR_SEARCH_ADMIN_ENABLED`. Standing them down freezes
rebuilds; discovery keeps answering from the ACTIVE generation.

**The handoff between stages is transactional.** `CompleteGenerationBuild`
commits the completion, its evidence and the follow-on
`GENERATION_VALIDATE_ACTIVATE` job together. Before this was so, a crash between
"build completed" and "enqueue validation" left the generation `BUILDING` with
nothing left to drive it - the rebuild had finished, no job existed to validate
it, and it sat there permanently. The generation does **not** become `READY`
there: that only queues the validation, and `MarkGenerationReady` refuses unless
the documents actually present in the table match the count the worker reported
processing.

---

## 9. APIs (13I)

Section 13 exposes **60 routes** across two authority models.

### 9.1 Vendor taxonomy — 10 routes, `/api/v1/vendor/taxonomy/...`

Bearer authentication plus Vendor IAM. This surface answers "how is MY catalogue
classified, and how do I change that". Nine reads and one command
(`PUT .../mappings`).

The isolation chain every source-addressed operation walks:

```
session -> staff -> vendor/business -> permission scope -> branch -> source resource
```

`business_ref` and `branch_ref` are **never** read from the request. They are
resolved from the owning catalogue table by `VendorAccess.ResolveSource`, which
also binds each source type to exactly one namespace. A body carrying either
field is rejected with 400, not silently ignored — silently ignoring it would be
safe but dishonest, leaving the caller believing it had written into another
scope.

Guessing another vendor's `source_ref` returns **404, never 403**: a
distinguishable "forbidden" would be a cross-tenant enumeration oracle.

### 9.2 Platform taxonomy administration — 33 routes, `/internal/vendor/v1/taxonomy/...`

serviceauth HMAC-v2, caller allowlist, durable nonce. Versions, drafts, facets,
nodes, vocabulary, publication, governance, platform-scope mapping and semantic
assignment, and seed evidence.

### 9.3 Internal Food/Shop search — 6 routes, `/internal/vendor/v1/search/...`

serviceauth HMAC-v2 with its **own** allowlist. Search, facets and suggestions,
per namespace, with no combined endpoint.

Callers cannot select a generation: the request shape has no such field, and the
generation-aware layer fills in the `ACTIVE` one. Being able to name a
generation would let a caller pin a retired or half-built projection.

### 9.4 Ranking and projection administration — 11 routes

serviceauth HMAC-v2. Ranking profile inspection and bounded updates; rebuild
request, generation inspection, retry, activation, retirement, dead-letter
replay, and bounded health.

### 9.5 Why search is internal rather than public

Its consumers are other KarryGo services, and its responses are cross-vendor. A
vendor authenticated against their own business has no business reading a ranked
list of their competitors. The customer app reaches discovery through a service
that is on the allowlist.

### 9.6 The search read stack

```
GenerationAware -> Cached -> Engine -> PostgreSQL backend
```

Each layer sits where it does for a reason:

- **GenerationAware** is outermost because it resolves the `ACTIVE` generation.
- **Cached** sits below it so the resolved generation is part of the cache key.
  Caching above generation resolution would let a key survive a generation
  switch and serve retired documents.
- **Engine** resolves the ranking profile and scores. Below the cache, a profile
  change applies on the next miss — and the cache revision a profile change
  bumps is what forces those misses.
- **PostgreSQL** is the only layer touching durable state, and the one that
  stays correct when Redis is gone.

---

## 10. Security model (13I)

### HMAC-v2 — reused, not reinvented

Both internal surfaces use the existing certified `shared/go/serviceauth`
contract: caller identity, configured allowlist, timestamp freshness, nonce,
body SHA-256, method binding, RequestURI binding, HMAC-SHA256, existing
`X-Service-*` headers, and the existing `vendor_service_nonces` table.

No HMAC-v3, no service JWT, no API key, no Section 13 token type was created.

Nonce consumption is factored into `internal/platform/servicenonce` rather than
inlined for a fifth and sixth time. Uniqueness is the database's: `ON CONFLICT
DO NOTHING` means two concurrent replays race in PostgreSQL and exactly one
wins, which is what makes the guard correct across several service instances
rather than only inside one process.

### IAM — permission-based, deny-by-default

Migration `000059` catalogues six capabilities:

| Capability | Scope | Granted to |
|---|---|---|
| `vendor.taxonomy.read` | vendor | Owner, GM, Branch Manager, Kitchen/Pastry, Shop Inventory, Marketing, Procurement, Analyst |
| `vendor.taxonomy.mapping.manage` | vendor | Owner, GM, Branch Manager, Kitchen/Pastry, Shop Inventory |
| `vendor.search.read` | vendor | same read set as taxonomy.read |
| `vendor.taxonomy.platform.admin` | **platform** | **no role** |
| `vendor.search.ranking.admin` | **platform** | **no role** |
| `vendor.search.projection.admin` | **platform** | **no role** |

> **VENDOR STAFF IS NOT GLOBAL PLATFORM TAXONOMY ADMIN.**

The three platform capabilities are granted to no vendor role — not even Owner.
Two independent things make that hold:

1. Migration `000005`'s "Owner: every permission" grant is a **one-time snapshot**
   taken when it ran, so a permission introduced later is never swept in
   retroactively. Migration `000059` adds no grant that would undo this.
2. There is no bearer route to exercise them on. The platform surfaces are
   root-mounted and HMAC-only.

Authorization is never keyed on a role's display name. A custom vendor role
literally named "Owner" with no permissions attached authorizes nothing, and the
certification suite creates exactly that role to prove it.

---

## 11. Privacy

Section 13 stores no customer identity and builds no search profile.

There is no customer search-history table, no behavioural identity graph, no ML
user embedding, no personalized vector and no secret per-user ranking profile.
The suggestion API has nowhere to put a customer id — the request shape has no
such field — and suggestions come only from canonical taxonomy, aliases,
synonyms, brands and projected names.

Search responses expose stable references and freshness evidence. They do not
expose the projected document: search text, token lists, normalised vocabulary
and raw operational hint payloads stay inside, because shipping them would let a
consumer rebuild a shadow catalogue out of search responses.

Bounded operational events (`search.performed`, `search.zero_results`,
`search.result_selected`) are permitted with non-sensitive payloads.
Long-term behavioural analytics remains `analytics-service`'s authority.

---

## 12. Scale

Certified at **24,000 Food documents and 24,000 Shop documents** (Section 13I
requires at least 20,000 each), built with `generate_series` rather than through
HTTP.

No millisecond SLA is asserted. A number measured once on one machine is
evidence, not a threshold. Observed timings are recorded in the certification
report.

The query-plan gate is deliberately **not** "no plan may contain a Seq Scan". A
small lookup table is correctly scanned sequentially, and asserting otherwise
pins a planner decision rather than an architectural property. Two narrower
things are asserted instead:

1. The indexes the large paths depend on exist (GIN on the tsvector; B-tree on
   generation/active/branch, source identity, exact term, prefix term and typo
   candidate).
2. With `SET LOCAL enable_seqscan = off`, the planner can satisfy every real
   search query **without** a sequential scan of a 24,000-row search table —
   only possible if the access path is index-capable.

An N+1 gate compares transaction cost at page size 1 against page size 50. Cost
that grows with hit count is the shape of an N+1; measured growth was zero.

---

## 13. Certification: the attacks that must fail

| Axis | Attack | Result |
|---|---|---|
| HMAC | missing name / signature / nonce / timestamp | 401 |
| HMAC | invalid signature, or signature from another service's key | 401 |
| HMAC | timestamp 30 min stale or 30 min future | 401 |
| HMAC | body, method, path or query altered after signing | 401 |
| HMAC | authentic caller absent from the allowlist | 403 |
| HMAC | replayed nonce (durably recorded) | 401 |
| IAM | any vendor staff member reaching a platform capability | forbidden |
| IAM | read capability implying a write capability | forbidden |
| IAM | role named "Owner" with no permissions | forbidden |
| IAM | branch-scoped grant reaching another branch | 404 |
| Namespace | SHOP version/node/facet through FOOD, and the reverse | refused |
| Namespace | FOOD mapping onto a SHOP node, and the reverse | refused by service **and** by PostgreSQL |
| Namespace | Food source type addressed through SHOP, and the reverse | refused |
| Namespace | FOOD generation activated/retired through SHOP | refused, FOOD generation unchanged |
| Namespace | FOOD ranking profile read/updated through SHOP | refused, weights unchanged |
| Namespace | Food search returning a Shop document, and the reverse | never |
| Tenant | another vendor's source_ref, mapping, or branch resource | 404 |
| Tenant | `business_ref`/`branch_ref` smuggled into a command body | 400 |
| Ranking | commercial or promotional weight above the non-text ceiling | refused, profile unchanged |
| Ranking | non-text ceiling raised to exact-name relevance | refused |
| Bounds | page size 5000, query 300 chars, empty query, 10 facet families, malformed cursor | clamped or refused |

---

## 14. Failure modes and what happens

| Failure | Behaviour |
|---|---|
| Redis unavailable | Search stays correct, reads through to PostgreSQL, health reports degraded cache separately |
| Stale Redis entry | Never served — generation and cache revision are part of the key |
| Duplicate source event | Applied once; no second cache-revision bump |
| Out-of-order source event | Older revision marked `STALE` and discarded |
| Rebuild fails halfway | Generation `FAILED`; `ACTIVE` untouched; retry resumes |
| Two workers race for a job | Lease + `SKIP LOCKED`; one claims it |
| Two READY siblings race to activate | One activates; the other gets a conflict |
| Worker dies mid-job | Lease lapses; another worker adopts |
| Job exhausts retries | Dead-lettered; replayable through the admin API |
| Taxonomy administration disabled | Routes unregistered entirely |
| Search administration disabled | Administration returns 503, rebuild workers stand down; **discovery keeps answering** |
| Rebuild source unreachable mid-build | Generation `FAILED` with `REBUILD_SOURCE_FAILED`; `ACTIVE` untouched; retry resumes |
| Worker crashes mid-rebuild | Lease lapses; another worker adopts the job and resumes from its persisted checkpoint |
| Build finishes but validation fails | Generation stays `BUILDING`; it never becomes `READY`, so it can never activate |

---

## 15. Adapter seam for a future external search engine

`FoodSearchOrigin`/`ShopSearchOrigin` and the `*SearchBackend` interfaces are
the seam. A future OpenSearch or Elasticsearch backend would implement the
backend interface and slot in below the cache, leaving generation resolution,
cache identity, ranking bounds and the API contract unchanged.

No such client is wired, no such deployment exists, and none is planned as part
of Section 13. The seam is documented so that adding one later is a substitution
rather than a rewrite — not as a promise that it will be added.

---

## 16. Explicit exclusions

Section 13 introduced none of the following, and
`internal/section13security/exclusions_test.go` scans Section 13's own source
and migrations to keep it that way:

AI recommendation engine · personalized ML ranking · vector embeddings ·
pgvector · semantic LLM search · OpenSearch deployment · Elasticsearch
deployment · advertising billing · campaign authority · order creation ·
checkout authority · payment/wallet execution · logistics execution · customer
PII profiling · dark-store search ownership.

No PostgreSQL extension was enabled by any Section 13 migration.
