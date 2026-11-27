# Fleet Sections 21-33: implementation report

All work is on `staging` in `services/fleet-partner-service` (plus repo-root OpenAPI files under
`docs/openapi/`). No other service was changed. Layout follows the service convention
`features/<feature>/{models,repositories,usecases,http}`; use cases depend on repository interfaces
declared in the use-case package (architecture test FLEET-5.1b).

## Commits

| Commit | Work package | Content |
|---|---|---|
| `a1fc7371` | WP20 | Section 21/51 warehouse receipts, leased put-away and pick tasks, waves, packing, gate events |
| `0c1cbab2` | WP21 | Section 22/52 payroll fingerprint, receivables ageing, profitability, cost allocation, invoice submit |
| `421b70a2` | Section 26 | Produce the seven Section 26.2 core events that were registered but never emitted |
| `19607d23` | WP22 | Section 23/53 operational exceptions, SLA breaches, notification preferences |
| (this push) | WP23, WP24 | Section 24/54 analytics, reports, recommendations, automation; Section 28/56 security events and privacy requests; docs |

## WP20 - Warehouse operations (Section 21 / 51)

- Receipt states `EXPECTED -> ARRIVED -> RECEIVING -> (EXCEPTION ->) RECEIVED -> PUTAWAY_IN_PROGRESS -> COMPLETED`,
  or `CANCELLED`. Receiving posts accepted quantity (received minus damaged minus quarantined, enforced by a
  CHECK) to the stock ledger in the same transaction; a line whose count differs from expected opens a
  discrepancy that blocks completion until resolved with a reason.
- Put-away and pick tasks are claimed with a lease (default 5 minutes, `Options.TaskLease`). Completion
  requires the scanned bin / item to match the task. An expired lease is released by the warehouse worker
  so another operator can take the task.
- Pick waves group released fulfilment orders; a short pick is recorded and holds the order
  (limitation: no automatic replan).
- Packing quantity can never exceed picked quantity. Gate events are append-only and ordered.

## WP21 - Finance analysis (Section 22 / 52)

- `calculation_hash` on payroll periods: a SHA-256 over the sorted calculation inputs. Approval
  recomputes it and refuses with 409 `payroll_inputs_changed` if anything moved since calculation, so a
  period cannot be approved against figures nobody reviewed.
- Receivables ageing per currency (currencies are never summed together) and per customer.
- Profitability read model and append-only snapshots stamped with the method version.
- Cost allocations (MANUAL or EVEN), append-only, exact in minor units.
- Invoice `submit` command; cancel event renamed from the misspelt `invoice.canceld.v1` to `invoice.cancelled.v1`.
- No ledger was added: payment-wallet-service remains the money authority.

## Section 26 dependency fix

`internal/events/canonical.go` derives `organization.status_changed.v1`, `organization.provider.provisioned.v1`,
`organization.provider.membership_changed.v1`, `organization.asset.blocked.v1`, `corporate.order.confirmed.v1`,
`corporate.shipment.status_changed.v1` and `corporate.invoice.issued.v1` from the domain events already being
written, inside the same transaction, so they are produced exactly when the underlying change commits.

## WP22 - Exceptions, SLA, preferences (Section 23 / 53)

- Exceptions dedupe on (organization, source type, source id) while live; commands are optimistic
  (expected version). A durable job `SECTION23_EXCEPTION_SLA` escalates exceptions past their SLA and
  queues a notification; notification failure never loses the escalation.
- Notification preferences narrow channels for a user or organization, but mandatory channels for
  critical templates cannot be switched off. Preferences are applied inside `QueueIntentTx`.

## WP23 - Analytics, reports, recommendations, automation (Section 24 / 54)

- New feature `features/analytics`; `features/reports` and `features/automation` remain boundary
  markers pointing to it. The old stub `automation/section27_runner.go` was removed.
- Dashboards are synchronous, bounded and branch-scoped; unknown query parameters are refused.
- Report runs are asynchronous (`REPORT_RUN_GENERATE`), freeze the requester's branch permissions at
  request time, and expire (`ANALYTICS_HOUSEKEEPING`). Scheduled definitions use cron.
- Recommendations are deterministic rules (maintenance due, fuel/odometer anomaly, stale live trip)
  against a seeded model registry (`fleet_partner_model_versions`). Each carries its inputs, explanation
  and model version; a human accepts or rejects with an expected version; undecided ones expire.
- Automation rules: draft -> submit -> approve by a different person -> active -> disabled. Every
  status change writes an append-only version. The evaluator (`AUTOMATION_EVALUATE`) reads committed
  outbox events after a per-organization cursor; a unique (rule, trigger) key makes each execution
  happen once; events produced by automation are skipped so rules cannot trigger each other in a loop.
  Actions are limited to raising an exception and queueing a notification.

## WP24 - Governance (Section 28 / 56)

- `GET /security-events`: the organization's own authentication and authorization security events.
- Privacy requests (EXPORT, ERASURE, RECTIFICATION, RESTRICTION) with a response deadline
  (default 30 days), decision by someone other than the submitter, and completion blocked while an
  active legal hold covers the subject.

## Configuration

New switches, all default off (`.env.example`): `ANALYTICS_REPORTS_ENABLED`,
`FEATURE_AI_RECOMMENDATIONS`, `FEATURE_AUTOMATION`, `SECTION24_WORKERS_ENABLED`,
`PRIVACY_REQUESTS_ENABLED`, `SECURITY_EVENTS_ENABLED`, `OPERATIONAL_EXCEPTIONS_ENABLED`,
`NOTIFICATION_PREFERENCES_ENABLED`. Deploying the code therefore changes no behaviour until a switch is
turned on.

---

# Final-closure repairs (2026-11-27)

Every change below came out of the read-only re-audit (01) and is the smallest correction found. Nothing
was rebuilt; no new service, ledger, session store or audit history was introduced.

| Commit | Change | Spec | Why it was needed |
|---|---|---|---|
| `f38b8305` | Order-level PICK refuses wave orders; CANCEL recovers a PICKING order once no pick task is open, releasing reservations with a balancing RELEASE movement | 51.2 | A short-picked order dead-ended, and the old PICK path recorded stock as picked twice |
| `2b820123` | `report_run.ready/failed.v1` and `legal_hold.applied/released.v1` published in the transaction of the change | 54.4, 56.4 | Required events were never produced |
| `f4fe0df9` | `GET /security/sessions`, `POST /security/sessions/:id/revoke`; aliases `GET /audit-events`, `POST /sessions/:id/revoke`; migration `000105` (`security.manage`) | 59.10, 56.3 | Organization session management was missing; the spec's audit and revoke paths were absent |
| `ad478ab2` | Fleet runtime image upgrades OpenSSL to 3.5.9-r2 with a build-time floor check | 28.3, 33.2 | CVE-2026-124709; no gate scanned the fleet image |
| `c85fb5be` | Same patch in the shared `services/Dockerfile` (owner-authorized) | 33.2 | Admin Bridge Trivy gate |
| `b4f0c77a` | media-file-service OpenTelemetry v1.45.0 -> v1.47.0 (owner-authorized) | 31, 33.2 | GO-2026-6974, hidden behind the first failing module |
| `d5409a21`, `befb4c33` | gorilla/websocket v1.5.3 -> v1.5.4-0.20240701034025-d67f41855da4 in Dispatch and Hauling (owner-authorized); new Hauling tracking-socket test | 31, 33.2 | GO-2026-6278 |
| `b92ecf79` | Staging capability switches for Sections 21-28 | 33.1 | Completed features were off on staging |

## Organization session management, in detail

- Governance owns the routes; it reaches the auth feature's `SessionRepository` through a
  `SessionAuthority` port adapted in the composition root (`internal/app/governance_sessions.go`).
- Listing returns the organization's sessions only, newest first, never token or network hashes, and marks
  the caller's own session. Branch-scoped callers are refused (staff sessions are organization-level).
- Revocation locks the session family (the same advisory lock refresh rotation takes), revokes the
  family's active credentials **scoped to this organization only**, appends session history, writes an
  audit entry carrying the human reason and publishes `security.session_revoked.v1`, all in one
  transaction. A repeat is a no-op. A session of another organization answers 404.
- Access tokens stop working on the next request: the auth middleware already checks session status.
- `SECURITY_SESSIONS_ENABLED` gates the routes. `security.manage` is granted to owners and administrators.

## Cross-service changes and their blast radius

| Change | Services affected | How it reached staging |
|---|---|---|
| Shared Dockerfile OpenSSL | 9 shared-image services auto-deployed (media, wallet, verification, customer, taxi, hauling, support, referral, pricing); api-gateway, partner-web, admin, dispatch and analytics stay manual-deploy | `deploy-staging-auto.yml`, each with a pre-deploy database backup and automatic rollback; all healthy |
| Media OpenTelemetry | media-file-service | same run |
| Websocket | Hauling (auto-deployed); Dispatch (manual-deploy: the fix is in CI, not yet on the Dispatch staging runtime) | same run for Hauling |

Not changed, recorded for the owners: notification-service has its own Dockerfile on the same `alpine:3.22`
base (same OpenSSL), outside what was authorized.

## Final closure, round 2 (2026-11-27, after the owner's product decisions)

Starting commit `ba1959bd`. Commits: `9ad46a91` (Support/Dispute organization messages), `0102c77f`
(Fleet privacy settings, support-case messages, bounded notification delivery), `283a7b24` (OpenSSL in
the notification and vendor images), `73ab7ff9` (staging pairing, privacy switch, backlog evidence),
`dd9f8681` (Section 22/23 workers on staging), `0cbe08e4` (gofmt of two new files).

| Gap | What already existed (reused) | What was added |
|---|---|---|
| `GET/PATCH /privacy-settings` | Retention-policy store with platform minimums, retention executors, legal holds | Governance routes over that store through a port (no new table); only executor-enforced classes writable; version = sum of class row versions; strict merge patch; event `privacy_settings.updated.v1` |
| `POST /support-cases/{id}/messages` | Fleet support-case projections and the signed Support/Dispute client; Support/Dispute chat messages with a per-case client-id index (009) | Support/Dispute organization-message create and read-back (migration 011); Fleet submission ledger without text (000106), media checks, read-back on unknown outcome, reconcile in the existing support job; event `support_message.created.v1` |
| Notification retries without limit | Intent table already had `DEAD_LETTERED`; notification-service read-back by idempotency key | Attempt cap and maximum age in the delivery worker (read back before closing anything that may have been sent); notification-service templates for Fleet's three keys |
| Fleet support on staging | `staging-pairings.txt` mechanism | Fleet -> Support/Dispute pair with a guard against replacing a built-in caller list |
