# Corporate Logistics Backend Release 1 readiness and rollout

This is the consolidation index for the Release 1 general-availability evidence. Detailed executable coverage remains in `docs/section31-test-coverage.json`, `docs/master-spec-release1-coverage.json`, and the Go contract/integration tests; this document does not replace them.

## General-availability evidence

- API and events: `docs/openapi/`, `docs/asyncapi/`, `docs/api-conventions.md`, and `docs/section55-organization-api-clients-and-webhooks.md`.
- Configuration and flags: `.env.example`, `internal/config/config.go`, and `docs/configuration.md`.
- Security/privacy: `docs/section28-security-privacy-audit.md`, sensitive-column inventory, legal-hold/retention tests, and Section 55 credential/SSRF/signature tests.
- Reliability/observability: `docs/section29-reliability-observability.md`, durable outbox/inbox/jobs, readiness checks, bounded metrics, tracing, and runbooks under `docs/runbooks/`.
- Capacity/performance: Section 29 load/capacity tests and Section 31 mandatory coverage.
- Migrations/backward compatibility: paired additive migrations, migration inventory/replay/rollback/reapply tests, `docs/section30-database-migrations-backward-compatibility.md`, and compatibility contract tests.
- Backup/restore/DR: `docs/section29-disaster-recovery.md`, `docs/section29-reliability-observability.md`, and the restore drill evidence referenced by Section 31.
- Dependency/secret scanning and CI: `.github/workflows/section31-mandatory.yml`.
- Explicit scope exceptions: `docs/adr/ADR-001-release1-taxi-deferral.md` and `docs/adr/ADR-002-release1-ai-automation-deferral.md`.

## Staged rollout

Every stage requires the prior stage to remain healthy. “Rollback” means disable the relevant additive feature gate or restore the prior binary; do not delete tenant/provider data, rewrite Wallet ledger entries, discard outbox events, erase webhook history/audit, or bypass legal holds.

| Stage | Prerequisite | Metric/health gate | Rollback trigger | Rollback action |
|---|---|---|---|---|
| R0 | Backup verified; additive migrations reviewed | Migration current; API ready with new write flags off | Migration/readiness error | Restore prior binary; retain additive schema; rollback migration only before any dependent write |
| R1 | R0 | `/live`, `/ready`, protected `/metrics`, traces healthy | Probe/telemetry regression | Restore prior binary/config |
| R2 | R1; internal test organization | Onboarding errors/latency and audit/outbox normal | Tenant/auth/onboarding invariant failure | Disable onboarding writes for cohort |
| R3 | R2; controlled Dispatch/Hauling providers | Provisioning saga/reconciliation healthy | Unknown outcomes or membership mismatch | Stop new provisioning; reconcile existing sagas |
| R4 | R3; Wallet approval and controlled funds | Wallet read-back/sub-wallet reconciliation clean | Ledger/read-back mismatch | Stop new Wallet commands; never mutate local balances |
| R5 | R4 | Projection lag/backlog within threshold | Stale/incorrect marketplace projection | Return to projection-disabled/read-only posture |
| R6 | R5; controlled organizations | Reservation/acceptance/rollback metrics healthy | Acceptance conflict or source mismatch | Disable organization acceptance; release/reconcile reservations |
| R7 | R6 | Provider Accept and Decline both work; no auto-accept | Missing/hidden Decline or rights regression | Disable new assignments; preserve existing decisions |
| R8 | R7; execution clients configured | Dispatch/Hauling execution and read-back healthy | Unknown outcome/circuit-breaker threshold | Stop new execution starts; reconcile source service |
| R9 | R8; Wallet reconciliation sign-off | Settlement projections reconcile to Wallet | Financial mismatch | Disable settlement commands; keep Wallet authoritative |
| R10 | R9 | Private-operations module metrics within SLO | Module-specific correctness/safety failure | Disable only affected capability |
| R11 | R10; client hash key configured | Client auth success/failure/rate-limit/audit healthy | Auth, scope, IP, or secret-handling defect | Disable `FLEET_PARTNER_ORGANIZATION_API_CLIENTS_ENABLED`; credentials remain stored but unusable |
| R12 | R11; webhook encryption key and worker configured | Backlog/age/retry/dead-letter/latency within threshold | SSRF/signature leak, backlog, or failure spike | Disable webhook writes and/or delivery worker; retain subscriptions, jobs, outbox, history |
| R13 | R12; operational/security sign-off | Full mandatory gate green; SLOs and runbooks accepted | Any release-blocking regression | Roll back affected cohort/flag or prior binary without destructive schema changes |

Taxi remains disabled at every stage. AI and automation execution remain disabled at every stage.

## Rollback boundaries

- API clients: disable the client-auth feature; do not delete credentials/audit. Revocation remains an explicit management action.
- Webhook creation: disable webhook writes while retaining subscription configuration.
- Webhook delivery: stop the worker/fan-out flag; pending outbox/jobs and delivery history remain durable for controlled recovery.
- Corporate provider, marketplace acceptance, and private operations: use their existing fail-closed capability/kill switches and reconcile any unknown external outcome before retry.
- Migrations: prefer application rollback with additive schema retained. The paired down migration is for a proven pre-use rollback only because dropping integration tables after writes would destroy history.
