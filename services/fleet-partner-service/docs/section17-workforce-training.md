# Section 17 - Workforce, shifts, training, and performance

Section 17 is implemented inside `fleet-partner-service`. Its 37 operations are registered from `workforce.TenantRoutes` below `/fleet-partner/v1`; `docs/openapi/section17-workforce-training.openapi.json` is the machine-readable contract and is parity-tested against that catalogue.

## Security and request contract

- Every operation requires an authenticated corporate actor, trusted organization context, the route's semantic permission, and branch scope where the resource is branch-owned.
- Create and command operations require `Idempotency-Key`. Replays return the stored status and response; reuse with a different request fingerprint conflicts.
- Mutable state transitions require `expectedVersion` or `If-Match` and fail on stale versions.
- IDs are always queried with `organization_id`; branch-owned profiles and shifts are filtered by permitted branch IDs.
- Qualification and training evidence stores Media File Service IDs only. Evidence validation fails closed when Media authority is unavailable.

## API groups

| Group | Operations | State ownership |
| --- | ---: | --- |
| Workforce profiles | 5 | membership-linked profile, supervisor, role, lifecycle |
| Shifts and assignments | 11 | draft/published/cancelled shifts; assignment check-in/out lifecycle |
| Leave | 6 | requested/approved/rejected/cancelled/completed leave |
| Qualifications | 9 | qualification definitions and provider verification lifecycle |
| Training | 5 | course definitions and append-only completions |
| Performance | 1 | immutable provider performance snapshots |

Shift assignment and leave approval serialize by organization/member and reject overlapping active work or approved leave. Attendance actions enforce self/supervisor authority. Eligibility rejects inactive profiles, active leave, expired or missing qualifications, and expired or missing mandatory training; it is invoked both while selecting Section 16 dispatch candidates and again at Section 15 provider acceptance.

Performance snapshots are immutable and deterministic. Each snapshot records its source versions and formula version. Declines are categorized as safety, operational, availability, personal, technical, or other. A decline can inform a snapshot, but never directly creates suspension, dismissal, wallet mutation, deduction, access reduction, ranking, or automatic discipline.

## Flags and workers

`WORKFORCE_ENABLED`, `SHIFTS_ENABLED`, `LEAVE_ENABLED`, `QUALIFICATIONS_ENABLED`, `TRAINING_ENABLED`, and `PERFORMANCE_SNAPSHOTS_ENABLED` default to false. `SECTION17_WORKERS_ENABLED` also defaults false and requires the shared worker runtime. The global Fleet kill switch fails closed.

The Section 17 maintenance job uses the shared durable queue with leasing, heartbeat, retry, dead-letter, and replay behavior. It activates and completes shifts, completes elapsed leave, detects no-shows, expires qualifications and training, publishes audit/outbox events, and generates the previous UTC day's performance snapshot idempotently for each active provider profile.

For rollout, apply migrations first, deploy with all flags disabled, enable read/write capabilities by organization policy, and enable workers last. Rollback begins by disabling workers and capability flags; code can then be rolled back without deleting authoritative history. Migrations `000038` and `000039` have tested down/up paths for controlled schema rollback.
