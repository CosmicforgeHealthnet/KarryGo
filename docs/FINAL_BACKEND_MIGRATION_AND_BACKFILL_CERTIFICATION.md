# ART-001 — Migration and backfill certification

Phase baseline `3007d6d42b60bf256578cf2688fe9e330a4a2798`.

The finding that shapes this whole document: **Section 19 needed almost no new
schema.** The durable models it asks for already exist, built by Sections 6–18.
Adding a second operating-mode column or a second completed-work table would
have created two sources of truth for the same fact, which is the failure this
section exists to prevent. **Zero migrations were added in this phase.** Every
column, index and constraint the section asks for was verified present before
anything was written, including the indexes the new observability queries read
(`idx_hauling_outbox_target_ready`, `idx_hauling_outbox_reconciliation`, the
matching Taxi pair, and `idx_finance_outbox_due`) — so no index was duplicated
either.

## 1. DB-001 … DB-009

| ID | Outcome | Where it actually lives |
|---|---|---|
| DB-001 operating mode | ALREADY SATISFIED | `fleet_partner_organization_settings.operating_mode`, `NOT NULL DEFAULT 'CENTRALIZED'`, `CHECK (… IN ('CENTRALIZED','DELEGATED','HYBRID'))`, with `version` for optimistic concurrency and a FK to the organization. Read and written through `fleetpartner` settings; enforcement gated by `FLEET_OPERATING_MODE_ENFORCEMENT_ENABLED` (default **true**). |
| DB-002 compensation plans | ALREADY SATISFIED | `fleet_partner_compensation_plans` (id, organization, currency, status, version, effective_from/to, created_by, approved_by) with `…_plan_rules`, `…_plan_versions`, `…_rule_versions` carrying the lineage. |
| DB-003 membership assignments | ALREADY SATISFIED | `fleet_partner_membership_compensation_assignments` binds membership → plan **and plan version**, with `service_context`, `currency`, `effective_from/to`, `status`, `assigned_by`, `approved_by`. Historical reproduction does not depend on the latest plan because the row pins the version. |
| DB-004 shift / work session | ALREADY SATISFIED | `fleet_partner_work_sessions` (organization, membership, source_service, started_at, ended_at, status, approved_at, approved_by, version) plus `fleet_partner_shifts` and `fleet_partner_shift_assignments` for planning. PER_SHIFT resolves against the approved session, not against jobs. No new shift table was added. |
| DB-005 completed work | ALREADY SATISFIED | `fleet_partner_completed_work_records` carries every required field including `source_version`, `idempotency_key`, `correlation_id`, `request_fingerprint` and `ingestion_status`. Two unique indexes: `(source_service, source_job_id, membership_id, source_version)` and `(organization_id, idempotency_key)`. |
| DB-006 settlement instruction | ALREADY SATISFIED | `fleet_partner_managed_work_settlement_instructions` with instruction id, version, compensation plan + resolution version, provider, organization, currency, the three authorized shares, `visibility_policy_snapshot`, `idempotency_key`, `request_fingerprint`, and a status whose CHECK includes `RECONCILIATION_REQUIRED`. Correlation lives on `fleet_partner_finance_operations`, which is the row that actually carries a posting attempt. |
| DB-007 execution corporate context | ALREADY SATISFIED | Equivalent semantics, deliberately not identical schemas. Taxi `taxi_bookings`: organization, membership, fleet_assignment_id, **fleet_assignment_version**, settlement_mode, source_version, fleet_correlation_id. Hauling `haulage_bookings`: the same set plus `execution_mode`. Dispatch `trips`: organization, settlement_organization, membership, fleet_assignment_id, execution_source, execution_mode, source_version, fleet_correlation_id — with `chk_trip_organization_managed_complete` refusing an ORGANIZATION_MANAGED trip that is missing any part of the corporate tuple. |
| DB-008 wallet idempotency | ALREADY SATISFIED | `uq_managed_settlements_instruction` on `(instruction_id, instruction_version)` and `uq_managed_settlements_idempotency` on `idempotency_key`. Database-level, not application-level. Concurrency certified: six concurrent sends of one instruction produce one journal (`TestConcurrentSendsOfOneInstructionPostOnce`), and a 400-request duplicate storm at concurrency 16 produces one journal (`TestLoadDuplicateSettlementStorm`). |
| DB-009 Vendor late-bound execution | ALREADY SATISFIED (organization dimension deliberately absent) | `commerce_platform_mission_refs` carries authority, mission_ref, link_state, status_projection and `status_source_version`; provider identity arrives late through the custody chain and `commerce_delivery_eta_current.provider_ref`; settlement references sit on `commerce_delivery_funding_operations.wallet_operation_ref`. Organization and membership are **not** stored: Vendor never computes the corporate split, and a second copy of Fleet's membership truth would drift without ever being consulted. |

## 2. BF-001 … BF-006

| ID | Outcome | Detail |
|---|---|---|
| BF-001 operating mode | ALREADY SATISFIED | Legacy organizations receive `CENTRALIZED` from the column default. That is the safe direction: CENTRALIZED means the organization assigns work explicitly, so nothing is auto-offered to a provider on behalf of an organization that never chose delegation. No historical intent is inferred from unrelated activity. Provenance is the migration that introduced the column with its default. |
| BF-002 compensation | ALREADY SATISFIED | No backfill exists, and that is the decision. There is no historical plan or binding for pre-Section-12 work, so none is invented. Work without an assignment resolves to no managed payout rather than a fabricated one. |
| BF-003 completed work | ALREADY SATISFIED | No historical completed work is synthesised. Records are created only by an execution service submitting its own authoritative completion through the canonical contract, which carries provider, assignment, membership and source version. `ingestion_status` already distinguishes `RECORDED`, `DUPLICATE`, `RECONCILIATION_REQUIRED` and `INVALID`, so an uncertain record has somewhere to go that is not money. |
| BF-004 settlement replay | ALREADY SATISFIED | A backfill cannot repost: posting is keyed on `(instruction_id, instruction_version)` at the database level, and `PostManagedSettlement` refuses a job the legacy path already settled (`TestManagedSettlementRefusesAJobAlreadySettledLegacy`). Nothing in the schema produces a queued financial command that a worker would read as new. |
| BF-005 backfill idempotency | ALREADY SATISFIED | Every migration in this phase and the ones before it is written with `IF NOT EXISTS` / `ON CONFLICT` semantics, and the runners are re-run-safe. Verified by reapplying the full chain for every service in this phase. |
| BF-006 migration recovery | ALREADY SATISFIED (vacuously, plus verified) | No migration was added in this phase, so there is no new irreversible step to document. The requirement is nonetheless exercised: the most recent migration of the phase before this one (`dispatch-delivery-service/000057_outbox_operator_repair`) was re-verified apply → rollback → reapply, including that its down migration drops only the columns it added and leaves the outbox table and its undelivered rows intact. |

## 3. Migration inventory

| Service | Latest before this phase | Added in this phase | Apply | Rollback | Reapply |
|---|---|---|---|---|---|
| fleet-partner-service | `000078_work_settled_components` | — | PASS (78 files) | n/a | n/a |
| driver-taxi-service | `012_fleet_managed_execution` | — | PASS (self-migrating, empty DB) | n/a | n/a |
| driver-dispatch-delivery-service | `000057_outbox_operator_repair` | — | PASS (61 files) | PASS (`000057` down) | PASS |
| driver-hauling-service | `018_fleet_completed_work` | — | PASS (self-migrating, empty DB) | n/a | n/a |
| payment-wallet-service | `000016_commerce_managed_delivery_recovery` | — | PASS (self-migrating, empty DB) | n/a | n/a |
| vendor-service | `000116_sections40_42_security_recovery` | — | PASS (116 files) | n/a | n/a |
| support-dispute-service | `007_commerce_case_integration` | — | PASS | n/a | n/a |
| media-file-service | `000004_data_export_artifacts` | — | PASS | n/a | n/a |
| notification-service | `002_commerce_semantic_notifications` | — | PASS | n/a | n/a |
| verification-compliance-service | `000002_vendor_verification_cases` | — | PASS | n/a | n/a |

Replay conditions exercised: empty database (Taxi, Hauling, Wallet self-migrate
and require one); database already at the previous release (Fleet, Dispatch,
Vendor migrated forward from the committed chain); repeated runner invocation
(every chain applied more than once across this phase's runs).

## 4. Protected migrations

Unmodified, verified by `git diff` against the phase baseline showing zero
changed lines in each:

- `fleet-partner-service/migrations/000077_fleet_partner_foundation`
- `driver-taxi-service/migrations/012_fleet_managed_execution`
- `driver-hauling-service/migrations/018_fleet_completed_work`
- `payment-wallet-service/migrations/000013_managed_work_settlement`
- `dispatch-delivery-service/migrations/000057_outbox_operator_repair`
