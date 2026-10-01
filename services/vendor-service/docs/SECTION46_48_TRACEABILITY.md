# Sections 46–48 traceability

This is a repository-evidence report, not a production-GO certificate. Vendor PostgreSQL is authoritative. Redis is rebuildable cache/coordination only. Wallet owns payment, refund and settlement; the logistics services own mission execution; Verification, Customer, Media and OTP/QR authorities remain external. Search and analytics are projections. Medicine is out of scope. Order Guarantee remains `FUTURE_DISABLED` with no route, worker, event or Wallet compensation call.

## Configuration and topology

| Flag/capability | Before | After | Default | Dependency | Evidence |
|---|---|---|---|---|---|
| Required 14 logical families | Present; RFQ named `COMMERCE_BULK_RFQ_ENABLED` | Catalogued and validated | false | service/domain chain | `internal/operational/catalogue.go`, `internal/config` |
| Global kill switch | Handler checks; workers could still start | all new worker claims suppressed; reads/durable rows preserved | false | none | `cmd/main.go` |
| Process deployment | API and workers coupled | `VENDOR_PROCESS_MODE=api|worker|all`; worker has no listener | all | PostgreSQL | `cmd/main.go`, `shared/go/serviceapp` |
| Order Guarantee | not implemented | explicitly future-disabled | false/no runtime switch | future contract | operational catalogue/test |

`api` runs the public/internal HTTP surfaces and passes an empty worker set. `worker` uses the same schema/domain wiring and runs without a public listener. `all` is the backward-compatible local mode. Production should deploy separate API and worker units, not `all`. Startup rejects an unknown mode and invalid values for the master service flag, worker flag, migration control, and global kill switch. Existing feature prerequisites and credential pairs remain fail-closed.

The API Gateway repository contains no Vendor mapping. No proxy was added because the current gateway is a partner API with its own auth model, and blindly proxying Vendor would risk changing authentication and exposing root-mounted internal routes. Public ingress is therefore `DEPENDENCY_GATED`; `/internal/...` and `/api/v1/internal/...` must never be forwarded.

## Operational readiness

| Requirement | Before | After | Status | Evidence |
|---|---|---|---|---|
| Structured logs/redaction | shared JSON safe-field logger | retained; opaque and named secret fields redact | IMPLEMENTED | `shared/go/logging` tests |
| Metrics/alerts | domain metrics and durable worker health | catalogue defined; no monitoring vendor claimed | PARTIAL | worker/event catalogues; readiness JSON |
| Readiness | DB and Redis both hard | PostgreSQL hard; Redis degradation no longer fails `/ready` | IMPLEMENTED | `cmd/main.go` |
| API/worker split | coupled | separate modes | IMPLEMENTED | process-mode tests |
| Gateway | no Vendor mapping | unchanged, explicitly gated | DEPENDENCY_GATED | gateway audit |
| Backup/restore | no Vendor provisioning evidence | procedure and ownership requirements documented | DEPLOYMENT_DEPENDENCY | below and runbooks |
| Search/cache | versioned build-beside-live, atomic activation, TTL cache | retained | IMPLEMENTED | Section 13 tests/code |
| Hard external authorities | flags require credentials; path failures fail closed | retained | IMPLEMENTED_WITH_DEPENDENCIES | config/integration tests |

Readiness states are `READY`, `DEGRADED`, `BLOCKED`, `DISABLED`, and `BACKLOGGED`. PostgreSQL failure is always `BLOCKED`. Wallet is hard only for paid commerce; Dispatch/Hauling/Taxi and OTP/QR are hard for the selected path. Verification is hard for activation/reverification. Notification, Analytics, and Redis degrade applicable capabilities without rewriting core truth. A dead secure-handover authority blocks completion; there is no insecure fallback.

Machine-observable alert conditions are: oldest outbox/job age above its SLO; dead-letter count greater than zero; Wallet/logistics UNKNOWN age above reconciliation SLO; stock reservation past expiry; settlement lag above SLO; verification expiry spike; secure-handover availability below one; search generation age/lag above SLO; and readiness `BLOCKED`. Alert rules still require a deployment monitoring system.

## Backup, restore, and SLOs

No repository evidence identifies a configured encrypted Vendor backup schedule, retention, RPO/RTO owner, or completed restore drill. Production GO is blocked until infrastructure supplies these. Restore drill order: disable writes and claims; restore Vendor PostgreSQL into isolation; verify migration version and business/order/inventory/outbox/inbox/job/audit/idempotency/privacy rows; reconcile Wallet UNKNOWN/pending operations; reconcile logistics missions; reconcile Verification projections; rebuild search/read models; restore consumers; resume workers; enable writes only after readiness passes. Never treat stale external projections as authority.

Staging targets (not achieved measurements): discovery p50/p95/p99 100/300/750 ms and <1% errors; checkout quote 250/750/1500 ms and <1%; order creation 250/750/1500 ms and <0.5%; payment orchestration 500/1500/3000 ms and <0.5% definitive failures (UNKNOWN measured separately); fulfilment quote 500/1500/3000 ms and <2%. Window: 15 minutes after warm-up, bounded 25 concurrent clients, non-production isolated dataset. No staging latency run was executed in this task, so measured p50/p95/p99 remain `NOT_EXECUTED`.

## Quality-gate conclusion

Unit/config/process topology tests, standard Go test/vet/build, formatting, route/event/worker parity, security checks and diff checks are recorded in `SECTION48_QUALITY_GATES.json`. Real PostgreSQL suites are repository-backed but require the isolated test environment. Gateway deployment, backup/restore drill, staging credentials, alert deployment and bounded staging load run remain production-GO blockers. No frontend implementation or migration was introduced by Sections 46–48.

## Final closure audit — 2026-09-10

Local isolated runtime probes now prove all three process modes. API mode served health/readiness/meta, created no worker lease, and rejected unsigned internal access. Worker mode runs without an HTTP listener and now fails startup when PostgreSQL cannot be reached; its probe uncovered and closed a nil-server panic plus the missing startup database gate. All mode preserved health/readiness behavior. This is local process evidence, not proof that separate staging deployment units exist.

No staging integration variables, caller allowlists, deployed ingress, backup provider, restore artifact, monitoring stack, alert rules, or confirmed staging load target were accessible. The current repository architecture remains direct Vendor Service exposure with API Gateway described as future routing; TLS and public/private network separation are therefore unverified. See `FINAL_PRODUCTION_GO_CERTIFICATION.md` for exact operator actions. Production GO remains blocked.

An empty-database application-start migration attempt reached migration 78 before its existing 30-second context expired. The purpose-built isolated runner remains the certified migration mechanism and replayed all 116 migrations. This limitation must not be misrepresented as a failed historical migration.

## DevOps execution audit — 2026-09-11

GitHub metadata and existing workflows identify the platform backend deployment convention as immutable GHCR images deployed over SSH to Docker Compose below `/opt/karrygo` on a VPS. The current API Gateway remains a partner-auth boundary, so Vendor's repository deployment selects direct ingress through the existing VPS proxy rather than adding a semantically incorrect Gateway mapping.

Repository-controlled deployment gaps are closed by `services/vendor-service/Dockerfile`, `deploy/vendor-service/docker-compose.staging.yml`, `.github/workflows/deploy-vendor-service.yml`, and the dedicated `tools/migrate` binary. Compose validation passed, the production image built, and the packaged migration job acquired its PostgreSQL advisory lock and verified all 116 migration records. The workflow order is migration, API readiness, then Worker. API publishes only to VPS loopback; Worker publishes no port.

No VPS shell/context, Vendor staging environment file, public hostname, PostgreSQL provider, monitoring stack, or downstream staging credentials/allowlists were accessible. Consequently none of the live ingress, backup/restore, alert, black-box, failure-injection, or load/SLO gates changed to PASS. Exact VPS/provider/operator commands are in `deploy/vendor-service/README.md`; staging certification and production GO remain blocked.
