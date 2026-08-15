# 12 — Migration Ownership

## Section 8H migrations

`000024_vendor_crm_intelligence_exports` creates four tables Section 8H owns outright — `crm_intelligence_evaluations`, `crm_intelligence_scope_states`, `crm_customer_intelligence_scores` and `crm_exports` — and changes no accepted table, so its rollback is a straight drop in foreign-key order. `000025_vendor_section8h_permissions` seeds six permissions and their role grants.

The constraints carry the section's invariants rather than restating them in prose. A CURRENT RFM row must hold all four RFM columns and no churn columns, a CURRENT churn row the reverse, and any non-CURRENT row none at all — so a score asserting "unavailable, composite 3" is unstorable. Every non-CURRENT row must carry at least one reason code, so "no score" always says why. A composite must equal the sum of its dimensions. `READY` requires a complete artifact, its checksum, its size, a completion instant and an expiry, and an expiry must follow completion. `BRANCH` scope requires an actual branch, because a branch-less branch score would be a business score wearing a branch label.

Two absences are asserted directly: no floating-point column anywhere in the score table, so nothing can be read as a probability; and no URL, link or token column in `crm_exports`, because a stored download link would be a credential with no expiry that survives every check the row exists to enforce.

Verified against real Postgres: fresh replay, Section 8G to 8H upgrade, rollback of the 8H pair and reapply, with explicit assertions that Sections 5-8G tables and permissions are undamaged, that no PENDING appears in any status vocabulary, and that an unavailable score with a risk number is refused by the database rather than merely unwritten.

## Section 8G migrations

`000022_vendor_crm_corporate_customers` creates seven tables — code counters, profiles, branch scopes, locations, cost centres, contacts, memberships — and **additively** extends the Section 8C interaction and task tables with a nullable corporate subject under `crm_interaction_subject_ck` / `crm_task_subject_ck`, which permit exactly one subject per row. `000023_vendor_section8g_permissions` seeds the five permissions and their role grants.

The down migration is the part that needed care, because this section is the first to alter a table it does not own. It deletes corporate-subject Section 8C rows first, drops the constraints, foreign keys, indexes and columns it added, restores `customer_profile_id`/`original_profile_id` to `NOT NULL`, and only then drops its own tables in foreign-key order. The migration test asserts each of those directly: no corporate column survives, both columns are `NOT NULL` again, and Sections 5-8F tables and permissions are undamaged — a section may only ever drop the schema it owns, and may only ever restore what it changed.

Verified against real Postgres: fresh replay, Section 8F to 8G upgrade, rollback of the 8G pair and reapply, plus direct assertions on the controlled vocabularies, the single-subject checks, twenty-six tenant-composite foreign keys, the four uniqueness rules the domain depends on, and fifteen supporting indexes. The absence checks matter as much: no personal or financial column on any corporate table, and no corporate order, invoice, credit, budget, contract, quotation, payment or wallet table anywhere in the schema.

## Section 8E migrations

`000018_vendor_crm_campaign_intelligence` creates `crm_campaigns`, `crm_campaign_versions`, `crm_campaign_experiment_variants`, `crm_campaign_audience_snapshots`, `crm_campaign_targets` and `crm_campaign_target_channels`. `000019_vendor_section8e_permissions` seeds five permissions and their role mappings. Tip moves from `000017` to `000019`. No accepted migration was edited.

The down file drops the `crm_campaigns` -> snapshot pointer constraint first, because `crm_campaigns` references the snapshot table and the snapshot table references `crm_campaigns`; without breaking that cycle neither table can be dropped. The rollback test walks down to `000018` and asserts both that every Section 8E table is gone and that Sections 5-8D are undamaged, then reapplies.

Constraints carry the section's invariants rather than leaving them to application code: the funnel arithmetic and the cohort partition are `CHECK`s on the snapshot row; a `BRANCH` campaign must name exactly one branch and a `BUSINESS` campaign none; quiet hours must be wholly present or wholly absent, and so must a frequency cap; an experiment's stored allocation total must be exactly 10000 when enabled and 0 when not; at most one control cohort per version, via a partial unique index; one target per relationship per snapshot; and `send_time_recheck_required` carries `CHECK (send_time_recheck_required = true)`, which is what makes "preparing is not permission to send" unstorable-otherwise rather than merely conventional.

Every child reaches its parent through the composite `(id, business_id)` key, so PostgreSQL — not only the HTTP layer — refuses a campaign pointing at another business's segment, branch, profile or snapshot. The snapshot additionally carries a composite foreign key onto `crm_segment_evaluations (business_id, segment_id, generation)`, which pins the exact source generation; `MATCH SIMPLE` means a null generation (a static source, which has no evaluation run) correctly skips the check.

## Section 8D migrations

`000016_vendor_crm_loyalty` creates the five loyalty tables; `000017_vendor_section8d_permissions` seeds four permissions and their role mappings. Both ship with down files, and a real-Postgres test proves the pair rolls back to the accepted Section 8C baseline and reapplies without damaging anything behind it.

The invariants that keep a points balance honest live in the schema, not only in Go: a non-negative balance, an aggregate that must equal credited minus debited, a non-zero delta on every entry, credit types that can only credit and debit types that can only debit, remaining points that cannot exceed issued, restored points that cannot exceed consumed, and an expiry policy that must agree with its window. Uniqueness carries the concurrency design — one programme per business, one account per relationship, one sequence position per account, one reversal per entry, one command key per business, and one producer event per business. Every foreign key is tenant-composite, so a loyalty row can never reference another business's account, entry or lot.

## Section 8C migrations

`000014_vendor_crm_interactions_tasks` owns four tables: `crm_interactions`, `crm_interaction_revisions`, `crm_tasks`, and `crm_task_history`. `000015_vendor_section8c_permissions` owns the six interaction/task permissions and role mappings. The pair uses composite tenant-safe foreign keys, controlled check constraints, terminal timestamp integrity, and query-supporting customer/branch/type/assignee/status/due indexes. Tests cover fresh replay, 8B-to-8C upgrade, rollback, reapply, and preservation of earlier sections; accepted migrations `000001`-`000013` were not mutated.

## Runner convention chosen, and why

Three migration mechanisms coexist in the platform (see [00](00_REPOSITORY_AUDIT_BASELINE.md)). Vendor Service adopts `payment-wallet-service`'s exact pattern: paired `NNNNNN_name.up.sql`/`NNNNNN_name.down.sql` files, a service-local tracking table (`vendor_service_schema_migrations`), and a self-contained Go runner (`internal/database/migrate.go`) that is:

- **Deploy-safe without an external script** — unlike `driver-dispatch-delivery-service`, which has no in-binary migration runner at all and depends on an external `psql`-driven bootstrap script that its own documentation admits produces files that are "not all re-run-safe."
- **Query-driven for rollback, not count-based** — `RollbackLastMigration` determines the most recently applied migration by querying `vendor_service_schema_migrations` directly, never by a caller-supplied step count. This deliberately avoids a real, repeated bug class in `fleet-partner-service`'s own history: multiple migration-replay tests there hardcoded "roll back the last N migrations," which silently under-rolled-back every time a later session added a new migration on top, and in one case left a shared test database mid-rollback for whatever test ran next in the same process. `ListMigrations` also hard-fails on a duplicate numeric prefix, so two migrations can never silently race for the same slot.

## Sequencing (per the specification's requested domain order)

```text
Foundation                          -> 000001_vendor_service_foundation
Durability (outbox/inbox)           -> 000002_events_outbox_inbox
Durability (idempotency)            -> 000003_idempotency_keys
Authentication and IAM              -> 000004, 000005
Vendor identity/business/branches   -> 000006, 000007
Onboarding/verification/activation  -> 000008, 000009
Branches/staff                      -> (future)
CRM                                 -> (future)
Catalogues                          -> (future)
Inventory                           -> (future)
Purchasing                          -> (future)
Production                          -> (future)
Commerce Orders                     -> (future)
Fulfilment                          -> (future)
Vendor delivery                     -> (future)
Advanced features                   -> (future)
```

No single giant migration exists or is planned; each future domain gets its own additive, sequentially-numbered migration when that feature section is implemented.

## What exists today

- `000001_vendor_service_foundation` — `pgcrypto` extension, a singleton `vendor_service_identity` marker row. No business tables — this is scaffolding to give the runner and its tests something real to exercise, not a business migration.
- `000002_events_outbox_inbox` — `vendor_outbox`, `vendor_inbox_processed_events`.
- `000003_idempotency_keys` — `vendor_idempotency_keys`.
- `000004`-`000005` — authentication/IAM tables and permissions.
- `000006`-`000007` — Vendor business graph and Section 6 permissions.
- `000008_vendor_section7_onboarding` — onboarding aggregate, representative, opaque documents, verification projection/history/requirements, masked bank projection, durable operations and callback nonces.
- `000009_vendor_section7_permissions` — six Section 7 permissions and role mappings.

## Verification performed (not merely asserted)

`internal/database/migrate_test.go` is gated on `VENDOR_SERVICE_TEST_DATABASE_URL` (skips cleanly, like every other service's real-Postgres-gated test, when unset). It was run live against a throwaway database on the repository's existing `karrygo-fleet-partner-test-db` Postgres container during this architecture session (created and dropped as part of verification, no lasting change to that container):

1. `ApplyMigrations` — all three migrations applied; `MigrationsCurrent` confirms.
2. Confirmed `vendor_service_identity` row and `vendor_idempotency_keys` table exist.
3. `RollbackLastMigration` — rolled back `000003`; confirmed `vendor_idempotency_keys` was actually dropped (not just un-tracked) and `MigrationsCurrent` correctly reports it missing.
4. `ApplyMigrations` again — reapplied cleanly; confirmed `vendor_idempotency_keys` exists again and the tracker is consistent.

Result: **pass**. This is the "migration replay" test layer required by the task's test-architecture instruction, exercised for real rather than only described.

## Section 6 migrations

Section 6 adds only vendor-service migrations: `000006_vendor_business_brands_capabilities_branches` and `000007_vendor_section6_permissions`. Both have down migrations. `000006` is additive to Section 5: existing opaque IAM branch assignments are not retroactively foreign-keyed; new production assignments are validated through a service port. Migration replay always discovers the latest version dynamically and therefore covers these additions without hardcoded positioning.

## Existing-service migration changes

Section 7 adds `verification-compliance-service` migration `000002_vendor_verification_cases`, including Vendor cases/history and additive delivery state on its existing outbox. Payment Wallet and Media reuse existing schemas. The change register records ownership, rollback and deployment config.

## Section 8A migrations

- `000010_vendor_crm_foundation` — `vendor_customer_profiles`, `crm_customer_identity_links`, `crm_customer_branch_relations`, `crm_customer_order_facts`, `crm_consent_projections`, `crm_privacy_projections`, `crm_privacy_processing_history`, plus one additive nullable `payload_fingerprint` column on `vendor_inbox_processed_events`.
- `000011_vendor_section8a_crm_permissions` — five CRM permissions and their role mappings.

No accepted Section 5–7 migration was modified. The inbox column was added rather than duplicating inbox infrastructure: the existing table deduped by event id but could not detect the same id carrying a different body, which Section 8A requires.

Verified against real Postgres: fresh replay, Section 7 → 8A upgrade, rollback of the 8A pair, and reapply. The pre-existing `TestSection7MigrationsRollbackAndReapplyAsAPair` hardcoded "the last two migrations are Section 7's"; it now steps back past 8A first, preserving exactly what it originally proved.

Tenancy is enforced in the schema, not only in middleware: every CRM table carries `business_id` and reaches its parent through a composite `(id, business_id)` foreign key, so a child row cannot be attached across tenants even if application code were wrong.

## Section 8B migrations

- `000012_vendor_section8b_tags_segments` — `crm_tags`, `crm_customer_tag_memberships`, `crm_segments`, `crm_segment_versions`, `crm_segment_evaluations`, `crm_segment_memberships`, `crm_segment_exclusions`.
- `000013_vendor_section8b_permissions` — five permissions and their role mappings.

No accepted Section 5-8A migration was modified, and no Section 8A table was reused for a Section 8B purpose: profiles, consent projections, privacy projections and order facts are read, never duplicated.

Verified against real Postgres: fresh replay, Section 8A → 8B upgrade, rollback of the 8B pair and reapply. The rollback test additionally asserts that Section 8A's tables and permissions are **undamaged** by an 8B rollback — a section may only ever drop the schema it owns.

The Section 7 and Section 8A rollback tests previously stepped past later sections by naming them, a list that goes stale silently. They now walk down to their own first version through a shared helper, so a future section never has to edit an older section's test to keep it honest.

Tenancy is enforced in the schema: all seven tables carry `business_id` and reach their parent through a composite `(id, business_id)` foreign key, so a tag cannot be attached to another business's customer even if application code were wrong. Eleven such constraints and thirteen supporting indexes are asserted directly by the migration test.

## Section 9A migrations

- `000029_vendor_food_catalogue` — `food_menus`, `food_menu_sections`,
  `food_items`, `food_item_media`, `food_item_variants`,
  `food_modifier_groups`, `food_modifiers`, `food_item_modifier_groups`.
- `000030_vendor_section9a_permissions` — three permissions and their role
  mappings.

## Section 9B migrations

- `000031_vendor_food_operations` — schedules, availability, branch overrides,
  immutable price versions, prep/logistics profiles, the inventory seam and
  catalogue revisions.
- `000032_vendor_section9b_permissions` — pricing and operations permissions
  with the role mappings that keep Kitchen out of unrestricted pricing.

No accepted Section 5-8 migration was modified and no existing table was reused
for a Section 9A purpose: businesses, branches and branch capabilities are read
through foreign keys, never duplicated.

Tenancy is enforced in the schema. Every table carries `business_id` and reaches
its parent through a composite foreign key — `(branch_id, business_id)`,
`(menu_id, business_id)`, `(menu_id, business_id, branch_id)`,
`(section_id, menu_id)`, `(item_id, business_id)` and
`(modifier_group_id, business_id)`. A section addressed through the wrong menu,
an item claiming a branch its menu does not belong to, and a modifier group
borrowed from another business are therefore unstorable rather than merely
refused by application code.

Ordering is enforced in the schema too. Each ordered collection has a
`UNIQUE (parent, position) DEFERRABLE INITIALLY DEFERRED` constraint, which is
what allows a single atomic statement to permute positions while the committed
state remains provably unique. A non-deferrable constraint would fail on the
first swapped row; no constraint at all would make deterministic ordering a
convention rather than a guarantee.

The capability CHECK admits `RESTAURANT` and `FAST_FOOD` only. Widening it for a
later food capability is an additive `ALTER ... DROP/ADD CONSTRAINT`, not a
destructive change, which is why the boundary is expressed as a CHECK rather
than as a lookup table this section would then own.

Verified against real Postgres: fresh replay from `000001`, application on the
Section 8 tip, and rollback of the 9A pair followed by reapply. The rollback
test asserts that Section 8's tables and permissions are **undamaged** — a
section may only ever drop the schema it owns. The structural test additionally
proves the database refuses a non-food capability, a cross-business branch, a
negative price, a negative modifier delta, an impossible selection range, an
unknown lifecycle value and an archived row without `archived_at`.
