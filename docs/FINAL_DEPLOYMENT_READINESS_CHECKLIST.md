# ART-005 — Deployment readiness checklist and runbook

Phase baseline `9ebad225ea9ec25397778a4dc8d3ff842f2938a3`.
This is the OPS-014 runbook. No secret value, hostname or key appears here.

**Read this first.** Commands below are marked **[VERIFIED]** when they were
actually executed during certification and **[NOT RUN]** when they were not.
Nothing hypothetical is presented as tested. The deployment system itself is
described in `FINAL_DEPLOYED_STAGING_EVIDENCE.md`; the short version is that
only three of twelve services have a deploy path today and none of them is on
the corporate money path, so most of this runbook could not be exercised.

## 0. The actual deployment pipeline

```
GitHub Actions  →  docker build (commit stamped via BUILD_COMMIT_SHA)
                →  ghcr.io/cosmicforgehealthnet/<service>
                →  SSH to the VPS (secrets VPS_HOST / VPS_USER / VPS_SSH_KEY)
                →  docker compose under /opt/karrygo/<service>
```

Services with a deploy workflow: **api-gateway, media-file-service,
vendor-service**. Every other service must be deployed by a workflow that does
not exist yet.

Per-service environment files (`/opt/karrygo/<service>/<service>.env`, mode
600) are operator-provisioned and are **not** in the repository. Vendor's
deploy currently fails because its file has never been created.

## 1. Startup order

Dependencies fail closed, so a service started before its dependency refuses
rather than degrades. Start in this order:

1. PostgreSQL and Redis for every service.
2. `payment-wallet-service` — it is the money authority and everything
   settlement-related read-backs against it.
3. `notification-service`, `media-file-service`,
   `verification-compliance-service` — supporting services with no upstream
   dependency on the others.
4. `fleet-partner-service` — corporate authority.
5. `driver-taxi-service`, `driver-dispatch-delivery-service`,
   `driver-hauling-service` — execution authorities.
6. `vendor-service` — reads capability from the execution authorities.
7. `support-dispute-service` — resolves identity from the execution services.

A service started out of order is not broken: Fleet unreachable yields a
retryable 503 on corporate operations, never an independent downgrade.

## 2. Migrations

| Service | Runner | Tracking table | Re-run safe |
|---|---|---|---|
| driver-taxi-service | self-migrating on boot (`TAXI_MIGRATION=true`) | `taxi_schema_migrations` | yes |
| driver-hauling-service | self-migrating (`HAULING_MIGRATION=true`) | `hauling_schema_migrations` | yes |
| payment-wallet-service | self-migrating | `schema_migrations` | yes |
| fleet-partner-service | self-migrating (`MIGRATION=true`) | `fleet_partner_schema_migrations` | yes |
| vendor-service | self-migrating (`VENDOR_MIGRATION=true`) | `vendor_service_schema_migrations` | yes |
| driver-dispatch-delivery-service | **no in-service runner** — golang-migrate files applied externally | whatever the external tool records | depends on the tool |

Five of the six own a tracking table and skip what is already applied, so a
restart re-runs nothing and a partially-applied chain resumes where it stopped.
Dispatch is the exception: its migrations are golang-migrate files with no
in-service runner, so re-run safety is a property of whatever applies them, and
the local bootstrap script maintains its own marker table for exactly that
reason.

**The tracking table is the safety mechanism, not a bookkeeping detail.**
Applying the `.sql` files with raw `psql` looks equivalent and is not: the
tracker stays empty, and the service's own runner then tries to apply the whole
chain again on boot and fails on the first `CREATE TABLE`. This bit the Vendor
certification run in this very session, which is why it is written down here
rather than left to be rediscovered during a deployment.

Run migrations before starting the service that owns them. Never run two
migration runners against one database concurrently — `CREATE TABLE IF NOT
EXISTS` is not safe against concurrent execution from two processes.

## 3. Health and version verification

**[VERIFIED]** — executed against a locally booted, linker-stamped binary this
phase; see `FINAL_DEPLOYED_STAGING_EVIDENCE.md` §5.

```
curl -fsS https://<service>/health     # process alive, and reports commit_sha
curl -fsS https://<service>/ready      # database and Redis reachable
curl -fsS https://<service>/api/v1/<name>/meta   # version, commit_sha, build_time, environment
```

Version check (this is OPS-002):

```
EXPECTED_SHA=$(git rev-parse origin/staging)
RUNNING_SHA=$(curl -fsS https://<service>/health | jq -r .commit_sha)
test "$RUNNING_SHA" = "$EXPECTED_SHA" || echo "STALE: $RUNNING_SHA"
```

A service reporting `unknown` was built outside the pipeline — that is a
different problem from running the wrong commit, and is deliberately
distinguishable rather than both looking like an empty field.

### Legacy health block (kept for reference)

```
curl -fsS https://<service>/health     # process alive
curl -fsS https://<service>/ready      # database and Redis reachable
curl -fsS https://<service>/api/v1/<base>/meta
```

`/health` must not be gated on an optional dependency. Hauling serves health
with Redis unreachable by design; `/ready` is where a dependency failure shows.

## 4. Metrics

| Service | Route | Token |
|---|---|---|
| fleet | `GET /metrics` | `FLEET_PARTNER_METRICS_AUTH_TOKEN` |
| taxi | `GET <api>/outbox-metrics` | `TAXI_METRICS_AUTH_TOKEN` |
| hauling | `GET <api>/metrics` | `HAULING_METRICS_AUTH_TOKEN` |
| wallet | `GET <api>/metrics` | `PAYMENT_WALLET_METRICS_AUTH_TOKEN` |

An unset token makes the route refuse every request. Set the token and keep
`/metrics` off the public ingress: backlog and dead-letter counts describe
where the system is weak.

## 5. Feature-flag operations

Full inventory in `FINAL_FEATURE_FLAG_ROLLOUT_ROLLBACK.md`. Operationally:

- Flags are environment variables read at boot. Changing one requires a
  restart; there is no runtime flag service.
- Registration-time flags (`*_INTEGRATION_ENABLED` where the route group is
  conditional) unmount routes when off. Behaviour-time flags
  (`TAXI_FLEET_PROVIDER_WORK_ENABLED`) keep the route mounted and answer 503,
  which is what a provider app should see during a halt.
- Turning a flag **off** never destroys state. Work already accepted continues
  through its lifecycle.

## 6. Reconciliation

| Service | Where to look | What to do |
|---|---|---|
| taxi | `GET <api>/internal/fleet/completed-work?state=RECONCILIATION_REQUIRED` | inspect, then `POST …/:id/requeue` |
| hauling | same routes | same |
| dispatch | `GET /api/v1/internal/dispatch/v1/operations/outbox?status=DEAD_LETTER` | `POST …/:id/requeue` |
| fleet | `/finance/reconciliation-cases`, `/operations/backlogs` | `…/replay`, `…/read-back` |
| wallet | `/operations/:operationType/:id/replay`, `/reconciliation` | replay; never hand-edit a journal |
| vendor | `/projection-jobs/:jobId/replay` | replay |

A requeue is a **redelivery**: payload, hash and aggregate version are
untouched and the repair is counted. It is safe against a destination that may
already hold the event.

## 7. Dead-letter recovery

1. List dead letters on the owning service.
2. Read `lastError` and `attemptCount`. A dead letter always has a cause.
3. If the cause is fixed, requeue. The destination dedupes on the preserved
   identity.
4. If the cause is a permanent rejection, do **not** requeue in a loop: raise
   it, because the destination is telling you the event is wrong.
5. A requeued row that dead-letters again with a rising `requeueCount` is an
   escalation, not a retry.

## 8. Incident response

| Symptom | First check | Action |
|---|---|---|
| Providers cannot accept corporate work | Fleet `/ready`; corporate-context 503 rate | If Fleet is down, this is correct fail-closed behaviour. Restore Fleet; nothing to repair. |
| Settlements not posting | `reconciliation_backlog_operations{state="RECONCILIATION_REQUIRED"}` | Replay through Wallet's operator route. Never insert a journal by hand. |
| Outbox age climbing | `outbox_oldest_pending_age_seconds` | Is the worker running? Is the destination up? Requeue only after the destination is healthy. |
| Deliveries stuck at pickup | Credential dependency health | A protected handover never completes without verification. Use the support recovery path, never a bypass. |
| Provider sees 503 on corporate routes | The service's corporate flag | Expected during a halt. Confirm it is deliberate. |

## 9. Financial rollback rules

**Never delete or reverse a Wallet journal row.** A posted journal is a
historical fact. To undo a financial effect:

1. Use the explicit reversal path, which writes a compensating journal and
   leaves the original intact (`TestReversalCompensatesWithoutErasingHistory`).
2. For a Commerce delivery, use the refund and dispute-reversal path, which
   attributes the recovery across the parties that received it.
3. If the correct outcome is unknown, leave the settlement in
   `RECONCILIATION_REQUIRED` and resolve it with evidence. Reconciliation must
   never force a terminal state without it.

Rolling back a **deployment** does not roll back money. The two are separate
operations and the second one is always a compensating journal.

## 10. Secret rotation (SEC-011)

`serviceauth.ParseSecrets` accepts a `caller=secret` map and the verifier
accepts any entry, so a callee can trust two secrets for one caller at once.
Signing uses a single secret, so the caller's restart is the cut-over.

1. Add the new secret alongside the old in the **callee's** `*_SERVICE_SECRETS`.
2. Restart the callee. It now accepts both.
3. Switch the **caller's** signing secret to the new value.
4. Restart the caller.
5. Remove the old entry from the callee and restart it.

Steps 2 and 4 are the only windows, and at no point is a valid signature
rejected. Provider and customer token secrets follow the same pattern where
the service accepts a secret list; where it accepts only one, rotation
requires a controlled restart and session re-issue, which must be scheduled.

## 11. Pre-launch checklist

Tick every line before enabling a corporate or commerce flag in staging.

- [ ] Migrations applied; chain matches the deployed commit.
- [ ] Deployed commit SHA verified against the intended release — **blocked
      today: `/meta` does not report one** (see ART-008 P1-1).
- [ ] `/health` and `/ready` green on every service.
- [ ] `outbox_oldest_pending_age_seconds` near zero on every service.
- [ ] `outbox_dead_letter_events` zero, or every entry triaged.
- [ ] `reconciliation_backlog_operations{state="RECONCILIATION_REQUIRED"}` zero.
- [ ] Metrics token set on every service and `/metrics` not publicly routable.
- [ ] Every internal caller allowlist populated with real staging secrets.
- [ ] Notification, Media, Verification and Support reachable and authenticated.
- [ ] Wallet payment dependency pointed at a sandbox, never production money.
- [ ] Backups enabled and a restore drill completed — **not yet done**.
- [ ] Alert rules created and one synthetic alert delivered — **not yet done**.
- [ ] Rollback path confirmed: which flag to turn off, and who turns it off.
