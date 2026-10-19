# Incremental owner-read and Dispatch safety runbook

This is a source implementation runbook, not authorization to deploy or enable.
Keep the PR12/PR13 stack in draft and retain their private-network/cutover gates.

## Configuration reference — names only

| Name | Owner / purpose |
|---|---|
| `ADMIN_REMOTE_ENABLED` | Bridge remote ingress gate; existing default disabled |
| `ADMIN_TAXI_READ_ENABLED` | Bridge Taxi read gate; default disabled |
| `ADMIN_TAXI_ORIGIN` | Fixed private bare owner origin, provisioned by Platform |
| `ADMIN_TAXI_SECRET_FILE` | Dedicated base64url owner-HMAC secret file |
| `ADMIN_TAXI_READ_ROLES` | Comma-separated exact approved roles; no default grant |
| `ADMIN_ACCESS_POLICY_FILE` / `ADMIN_ACCESS_POLICY_KEYS_FILE` | Signed approval envelope / pinned governance public keys |
| `TAXI_ADMIN_READ_ENABLED` | Taxi owner read gate; default disabled |
| `TAXI_ADMIN_READ_SECRET_FILE` | Corresponding dedicated base64url secret file |
| `TAXI_ADMIN_ACCESS_POLICY_FILE` / `TAXI_ADMIN_ACCESS_POLICY_KEYS_FILE` | Independently verified identical owner policy revision / pinned keys |
| `ANALYTICS_ADMIN_TEST_DATABASE_URL` | Disposable Analytics privacy-journal PostgreSQL database |
| `ADMIN_TAXI_TEST_BINARY` | Test-only path to built actual Taxi main binary |
| `ADMIN_BRIDGE_TEST_DATABASE_URL` | Disposable Admin PostgreSQL test database |
| `TAXI_ADMIN_TEST_DATABASE_URL` | Disposable Taxi PostgreSQL test database |
| `DISPATCH_TEST_DATABASE_URL` | Disposable Dispatch PostgreSQL database with full migrations |
| `DISPATCH_TRIP_TEST_DATABASE_URL` | Disposable trip regression database |
| `DISPATCH_ADMIN_TEST_REDIS_ADDR` | Real test-only Redis endpoint |
| `ADMIN_BRIDGE_CONTRACT_CAPTURE_DIR` | Test-only actual wire capture directory |

`ADMIN_ANALYTICS_READ_ENABLED`, `ADMIN_CUSTOMER_SENSITIVE_READ_ENABLED`,
`ADMIN_CUSTOMER_WRITES_ENABLED`, `ADMIN_TAXI_WRITES_ENABLED`,
`ADMIN_DISPATCH_RECOVERY_ENABLED`, `ADMIN_HAULING_RECOVERY_ENABLED` and the separate
`ADMIN_REMOTE_WRITES_ENABLED` all default false. They request rollout only.
No configuration can enable unsupported HQ commands. Existing trust/TLS
and migration/runtime-role requirements remain in `runbook.md`.

## Migration and release order

1. Review this change and the full blocker matrix; resolve any additional CI
   regressions. Approve the exact role policy separately from code review.
2. Keep remote flags disabled. Do not change public routing or live senders.
3. Apply Admin migrations through 003 with the dedicated Admin migration role
   using the existing `--migrate` entry point. They add audit zone and policy
   revision pins (schema version 3). Reapply the updated runtime grants.
4. Apply Taxi migrations through 024 through Taxi's migration runner/owner role.
   Its nonce, policy revision and append-only read-audit tables belong to Taxi.
   Enabling owner reads without this schema refuses startup.
   Analytics migration 005 creates its own approval pins, query budgets and
   immutable disclosure/partition/audit journal; no reporting route is enabled.
5. Apply Dispatch migration 070 through Dispatch's existing migration process.
   Coordinate relay consumers: old binaries ignore the new fence and can resend
   unknown work. Do not run old and new relay consumers together during a change.
   Preserve the original decision, idempotency key and all existing operation rows.
6. Provision a reviewed private path from Admin to the Taxi listener and matching
   dedicated secret mounts. The shipped bridge Compose file has only its private
   DB network; this change does not attach it to a public/shared owner network.
   Do not open domain databases to Admin or expose owner routes through Gateway.
   Review the three DRAFT policy documents before creating any signed PRODUCTION
   envelope. Enroll independent governance public keys and coordinate the exact
   policy ID/version/checksum at both services. Revision rollback and same-version
   rewrites are refused. The `admin_policy_test` build tag exists only for test
   binaries with ephemeral TEST_ONLY policies; never use it for release builds.
7. Verify roles, permission, explicit market/zone/tenant delegation, scope denial,
   replay, audit persistence and owner downtime in the target environment before
   enabling this read subset. Independently test public-edge isolation there.
8. Complete the separate PR13 callback/worker cutover and target-environment gates
   only after explicit operator approval; code tests do not authorize cutover.

## Failure and rollback

Disable the bridge Taxi-read flag and owner Taxi-read flag to withdraw the new
read subset. The global bridge remote gate can withdraw all HQ ingress without
changing public Customer/Provider traffic. Keep audit evidence and the new
columns/tables in place; no down migration erases read evidence.

Dispatch migration 070 deliberately refuses automated down migration. Rolling
back to the previous relay implementation can replay uncertain decisions even
with the new column present. Prefer a forward fix. Before any operator-managed
rollback, obtain authoritative Fleet outcomes, resolve/retain fenced operations,
and prove there are no in-flight consumers or unresolved decisions. Never clear
`readback_required`, change status to PENDING or replay a legacy dead letter as a
substitute for an authoritative receipt. There is no new HQ API to do this.

Monitor UNKNOWN_OUTCOME/readback-required and legacy dead-letter counts, pending
age, audit failures, denied reads and owner latency. Unresolved Fleet decisions
need investigation; service availability alone is not proof of their outcome.

## Retention and privacy

Nonces are stored as hashes and retained for at least their full eleven-minute
acceptance protection window. Operators may prune only expired nonce rows in
bounded batches using the expiry index after the window; no audit rows are part
of that cleanup. No new audit or evidence retention duration is invented here.
Apply a separately approved retention/legal-hold policy before any evidence
archival/deletion process. Cursors contain only scoped references, timestamps
and technical bounds; they are signed and expire after ten minutes.

Do not purge Analytics disclosure or partition records as cache cleanup. They
are privacy fences against resampling/reconstruction across users and reports;
policy changes preserve old fences. Approve retention and migration semantics
before any archival, scope hierarchy or privacy-domain change. No production
minimum cohort, rounding, query budget or freshness value is supplied here.

`admin-metrics-dictionary.json` documents source semantics and null approved
policy fields. It does not approve Analytics disclosure or reuse advisory
minimum samples as an HQ suppression policy.

## Reproduction

Use the exact commands in `admin-owner-closure.yml` for database installation,
real owner binary build, race tests and required no-skip gates. The Admin workflow
also validates actual wire samples, old-v1 compatibility, dependency scanning
and image/deployment checks. Do not mark a skipped database test as passed.

Real HQ, deployed edge/firewall checks, target private owner networking,
Notification delivery, protected Media grants and real-device handover remain
separate target-environment certification requirements.

## Source-owned resource and custody reads

Use the fixed five-owner configuration described in
[owner-resource-contexts.md](owner-resource-contexts.md). All switches default
false. Enabling a switch supplies no policy grant: read access requires both an
approved governance rule and a current independently signed owner context bound
to live canonical source facts. Preparation/publication uses a separate owner
workload; HQ cannot self-approve or choose a replacement tenant.

Dispatch and Hauling custody details require their respective
`dispatch.admin-custody.v2` and `hauling.admin-custody.v2` contracts. Do not reuse
v1 approvals. Every collection is bounded at 100 with an explicit `truncated`
indicator. Truncated data cannot establish absence, completion or permission to
intervene. Detail ETags are read-model fingerprints, never native command versions.
Legacy missing/stale manifest versions fail closed.

Provider decision readbacks cover original standard/private assignment and
marketplace offer actions. Follow
[provider-decision-readback.md](provider-decision-readback.md); a historical
receipt never grants present execution permission. Preserve immutable receipt,
readback and owner audit evidence during rollback. Unsupported historical proof
must remain fenced. No HQ recovery or override command is enabled.

Hauling truck capacity/equipment metadata now requires
`hauling.admin-truck-capability.v2` owner and governance approval; a v1 rollout
cannot silently enable the larger disclosure. Recorded capability counters are
partial evidence, not full truck mutation versions. Null facts cannot authorize
cargo matching, and known flags alone establish no eligibility or compliance.
Hauling recovery reads require their own `hauling.recovery.read` permission and
exact native booking/commerce-mission ownership. No new schema migration is
needed for these reads; native mutation and cargo-fit guards remain unchanged.

## Customer owner review intake

See [customer-review-intake.md](customer-review-intake.md) before configuring
migration 016 or the new owner endpoints. This records a pending review work item
only. Its version guards the new native review aggregate, not Customer profile
standing. Account/lifecycle/verification/consent and Trip/Wallet changes are not
supported by this action. HQ operation orchestration remains unconnected.

Both `CUSTOMER_ADMIN_REVIEW_WRITES_ENABLED` and
`CUSTOMER_ADMIN_REVIEW_READBACK_ENABLED` default false. Enabling either requires
the existing source resource/context/policy boundary and normal migration 016.
Mount distinct write/readback secrets, each separate from the resource key.
Keep write/readback controls independent so uncertain accepted requests can be
read back after writes are disabled. Never treat a missing receipt as proof that
the original request failed; never issue a new key or repeat a POST to reconcile.

Read and retain immutable request/audit evidence. No automatic down migration
or destructive cleanup is supplied. Owner approval of work-item handling,
closure, retention, permissions and exact access scopes remains BLOCKED_POLICY.
