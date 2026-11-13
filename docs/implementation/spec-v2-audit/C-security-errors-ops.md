# Audit C: Security, errors and operations (fleet-partner-service)

Scope: Sections 28, 29, 56, 63, 64, 65 and Appendix C of `the specification text`, audited against
`services/fleet-partner-service` (FPS), `shared/go`, `deploy/`, `.github/workflows/`, `web/fleet`, and `apps/*`.
Every claim below was checked by reading code or grepping. The service's own Section 28/29 docs
(`docs/section28-security-privacy-audit.md`, `docs/section29-*.md`) were used only as pointers. In a few
places they are now stale; those places are noted.

Paths are relative to `services/fleet-partner-service/` unless they start with `shared/`, `deploy/`, `web/`, `apps/` or `.github/`.

## Summary

| Section | Implemented | Partial | Missing | Notes |
|---|---|---|---|---|
| 28 Security, privacy and audit | 12 | 7 | 0 | Authentication, object-level authz, rate limiting, redaction and closure are solid. Gaps: step-up is enforced on only a few actions, sessions are not revoked on org suspension, `fieldcrypto` is never used, audit rows have no `outcome`, and only 4 retention executors honour legal holds. |
| 29 Reliability, observability, readiness | 9 | 5 | 0 | Liveness/readiness, timeouts, classified retries, circuit breakers and degraded modes are real. Weak spots: metrics are sum/count only (no histograms), tracing is log-based with no exporter, backups are nightly, and there is no PITR. |
| 56 Security, privacy, audit, retention, legal holds | 18 | 8 | 7 | `privacy_requests`, `session_risk_records`, `GET /security-events`, the 56.4 events and provider-departure minimisation are absent. Paths differ from the spec (`/audit-logs`, `/auth/step-up/*`, `/retention-runs`, `DELETE /sessions/:id`). |
| 63 Stable error code catalogue (58 codes) | 7 | 4 | 47 | FPS uses its own feature-prefixed catalogue (`internal/platform/errorcodes`) plus generic shared codes (`bad_request`, `not_found`, `unauthorized`, `rate_limited`, `service_unavailable`). Most spec strings never appear. |
| Appendix C error catalogue (25 codes) | 8 | 5 | 12 | Better match because Appendix C is closer to the Sections 8-12 vocabulary. Two status mismatches: `validation_error` returns 400, not 422, and `decline_reason_required` returns 400, not 422. |
| 64 Security, privacy, visibility, audit controls | 17 | 22 | 1 | The authz evaluation order is mostly real. The field-visibility matrix is partly structural and partly permission-only. No KMS or envelope encryption. The audit-minimum fields are only partly captured. No privacy case object. |
| 65 Observability, SLOs, runbooks | 7 | 28 | 18 | No `/version` or `/internal/v1/status/dependencies` endpoint, no SLO definitions, no FPS alert rules, and no histogram buckets, so the P95 SLOs cannot be measured. 5 of the 12 required runbooks are missing. RPO is about 24h (nightly) against a 5-minute target. |

Counting rule: one row per numbered or bulleted spec requirement, or per table row. For the catalogues, one row per code.
"Partial" for a code means the exact string exists but the HTTP status differs, or the code is defined and never returned.

---

## Section 28. Security, privacy and audit (spec line 1605)

### 28.1 Authentication and sessions

| Req | Status | Evidence / gap |
|---|---|---|
| Short-lived access tokens, rotating refresh sessions | IMPLEMENTED | `internal/config/config.go:871-873` (access 15m, refresh 720h, step-up 5m); validation `config.go:970-973` caps access at 1h. Refresh rotation, one-time use and family reuse revocation: `internal/features/auth/usecases/session_flows.go:148,179-189`, `internal/features/auth/repositories/sessions.go:147,238`. |
| Session revocation on org/provider suspension and credential reset | PARTIAL | Membership suspend/remove revokes the user's org sessions (`internal/features/staff/usecases/membership_flows.go:107`). Closure revokes all org sessions (`internal/features/organizations/usecases/closure.go:270`). A refresh against an inactive org or membership revokes that session (`session_flows.go:116-129`). Every request re-checks session status (`internal/middleware/auth.go:142-158`) and org status (`internal/middleware/organization_lifecycle.go:70-112`). **Gap:** org suspension blocks access per request but never revokes sessions (`RevokeForOrganization` is called only from closure). The phone/email change flow (`internal/features/auth/usecases/phone_change.go`) does not revoke other sessions. Provider suspension belongs to the driver services (EXTERNAL). |
| MFA/step-up for sensitive company actions where configured | PARTIAL | The step-up flow exists: `POST /auth/step-up/start|verify` (`internal/features/auth/http/handlers.go:110-111`), port `internal/platform/stepup/stepup.go`, adapter `internal/app/stepup_adapter.go`. It is **enforced** for legal-hold create/release, security-settings update, API-client and webhook create/rotate/revoke/replay (`internal/features/integrations/service.go:122-138,159,272,343,392,455,513`), retention execution (`internal/features/retentionexecution/http/handlers.go:99,150`), and phone change (`phone_change.go:30-35`). **Not enforced** for approval decisions (`approvals` stores `step_up_reference` but never verifies it), wallet allocations/withdrawals (`companywallet`), payroll finalisation (`financeops`), sensitive exports (`exports`), role/permission changes (`staff`, `accesscontrol`), or closure approve/execute. |
| Service-to-service auth with separate credentials and scope | PARTIAL | Per-caller HMAC v2 plus durable nonce replay guard (`internal/middleware/auth.go:43-103`, `internal/platform/noncestore`). Per-route/method/event scoping exists in `internal/middleware/service_scope.go:43-80` (versioned secrets allow a rotation overlap), but it is **opt-in and off by default**: `FLEET_PARTNER_SERVICE_CREDENTIAL_SCOPING_ENABLED=false` (`config.go:560`), wired at `cmd/main.go` ~L290-315. With the flag off, the fallback is the all-or-nothing caller allowlist. Note: `docs/section28-security-privacy-audit.md` is stale and still says scoping does not exist. |
| Rate limits by IP, user, org and endpoint risk | IMPLEMENTED | `internal/middleware/ratelimit.go` (layered keys; 429 plus `Retry-After` at L103-107), route risk registry `internal/platform/riskregistry/registry.go`, mounted per route class in `internal/app/app.go:200,218,269,297`. `SetTrustedProxies` at `app.go:120`. Caveat: `FLEET_PARTNER_REDIS_REQUIRED=false` by default (`config.go:551`), so without Redis the limiter is per instance. |

### 28.2 Authorization

| Req | Status | Evidence / gap |
|---|---|---|
| Object-level authorization on every ID lookup | IMPLEMENTED | Tenant-scoped SQL on every repository path, plus the object-auth registry `internal/platform/objectauth/registry.go`, checked by `internal/section28contract` coverage tests and the readiness `objectauth.SelfCheck` (`cmd/main.go` ready check `object_authorization_registry`). 22 test files contain cross-tenant/IDOR assertions. |
| Field-level response shaping for provider, dispatcher, finance and support | PARTIAL | `internal/platform/fieldvisibility` registry. By its own admission, many resources are protected `MechanismPermissionOnly`: raw structs or `row_to_json` maps whose only protection is that no role currently holds the permission. Only some resources use shaped DTOs. |
| Provider cannot access customer price or wallet settlement | IMPLEMENTED | Shaped provider views (marketplace `ProviderOfferView`/`ProviderAssignmentSummaryView`, companywallet `ProviderStatement`/`SubWalletView`), rule in `docs/provider-data-minimization.md`. |
| Company users cannot access KarryGo internal admin | IMPLEMENTED | `/internal/fleet-partner/v1` accepts only HMAC; bearer tokens are rejected (`internal/middleware/auth.go:43-56`, `app.go:285-302`). |
| Independent providers need active membership for org endpoints | IMPLEMENTED | Provider reads are re-scoped through the membership join (`internal/features/marketplace/http/internal_handlers.go:120-140`, mounted `cmd/marketplace_routes.go:64`). Marketplace eligibility returns `provider_not_eligible` when the membership is missing (`internal/features/marketplace/usecases/eligibility.go:127,144`). |

### 28.3 Secrets and sensitive data

| Req | Status | Evidence / gap |
|---|---|---|
| No secrets, tokens, bank details or ID documents in logs/events | IMPLEMENTED | Central key redaction `internal/platform/redact/redact.go:22`, used by `internal/observability/logging.go:23` and the audit recorder. Refresh tokens are stored only as HMAC (`migrations/000003_corporate_authentication.up.sql:91-92`). |
| Encrypt sensitive columns; tokenised refs for bank/identity | PARTIAL | Bank and identity data are held as opaque references into owning services (convention). `internal/platform/fieldcrypto` (AES-GCM, versioned keys) **is imported nowhere**. There is no KMS. The sensitive-column inventory (`docs/section28-sensitive-column-inventory.json`) exists, but remediation was never done. |
| Redact request/error payloads before logging | IMPLEMENTED | `RequestLogger` logs metadata only, never bodies (`internal/middleware/middleware.go:195-250`). Only safe error codes are logged (`safeErrorCode`, L241-245). |
| Signed expiring file URLs; malware/scan status from media | IMPLEMENTED (authority EXTERNAL) | `internal/features/documents`: signed URLs are never persisted, scan status comes from Media, quarantined files cannot be downloaded, and a restricted-download permission exists. |

### 28.4 Audit log

| Req | Status | Evidence / gap |
|---|---|---|
| Append-only, with org, branch, actor, action, resource, before/after, reason, IP/device, request ID, timestamp; high-risk rows also carry approval and idempotency refs | PARTIAL | Table `migrations/000001_*.up.sql:98-132` (append-only trigger) plus `000069_section28_audit_chain.up.sql:38-47` (`approval_reference`, `idempotency_key_reference`, `step_up_reference`, HMAC hash chain). Verify route `POST /audit/verify-chain`. **Gaps:** `network_meta` is populated in only one place (integrations auth, which stores a raw `sourceIp` rather than a hash: `internal/features/integrations/auth.go:77`). `approval_reference`/`idempotency_key_reference` are set only by legal-hold, security-settings, retention and integrations; companywallet allocation/withdrawal audits do not populate them. |

### 28.5 Privacy and retention

| Req | Status | Evidence / gap |
|---|---|---|
| Org retention bounded by platform minimums | IMPLEMENTED | `internal/features/retentionpolicy` (14 classes; raise-only enforced in the usecase). Routes at `internal/features/retentionpolicy/http/handlers.go:39-41`. |
| Legal hold prevents deletion/anonymisation | PARTIAL | `internal/features/legalhold` (handlers L46-49). Consulted only by the 4 retention executors (`internal/platform/retentionexec/model.go:41,82-84`; executors `executor_expired_export.go:104`, `executor_org_contact.go:83`, `executor_support_metadata.go:181`, `executor_superseded_projection.go:158`) and, for files, by Media document retention. Retention execution is off by default (`config.go:561`). No other domain deletion path checks `IsUnderHold`. |
| Account closure keeps immutable evidence | IMPLEMENTED | `internal/features/organizations/usecases/closure.go` does no hard deletes and uses fail-closed blocker checkers. |
| Providers can access own assignment/decline history | IMPLEMENTED | `GET /providers/{id}/offers|assignments|memberships` (internal, membership-scoped). Requires driver-service proxies; see the cross-service section. |

---

## Section 29. Reliability, observability and operational readiness (spec line 1655)

| Area | Status | Evidence / gap |
|---|---|---|
| Structured logs (requestId, correlationId, organizationId, actorId, safe error code) | IMPLEMENTED | `internal/middleware/middleware.go:218-246`, `internal/observability/logging.go:14-26` (JSON with timestamp, level, service, environment). |
| Metrics (HTTP latency/errors, DB pool, event lag, job backlog, lease recovery, assignment response, declines by category, external-client errors) | PARTIAL | Present: `fleet_partner_http_*`, `fleet_partner_db_pool_*`, `fleet_partner_outbox_*`, `fleet_partner_job_*` (incl. `job_lease_lost_total`), `fleet_partner_assignment_{accepts,declines,expiries}_total`, `fleet_partner_client_calls_total/_duration`, `fleet_partner_circuit_breaker_state`. **Gaps:** durations render as `_seconds_count`/`_seconds_sum` only, with no buckets (`shared/go/observability/metrics.go:99-110,131-135`), so latency percentiles cannot be computed. No inbox/event-lag gauge. No assignment-response latency. |
| Tracing across gateway and clients | PARTIAL | W3C `traceparent` is extracted inbound and propagated on platform client calls (`internal/platform/tracing/tracing.go:1-37,92-125`). Spans are emitted as log lines and metrics only (`StartSpan` at `tracing.go:198`; used in `internal/platformclients/base.go:333`, `internal/jobs/worker.go:144`, `internal/events/publisher.go:84`, `internal/events/stream_consumer.go:200`). No exporter or collector, no DB spans. |
| Health: liveness without deps; readiness = DB, config, critical worker | IMPLEMENTED | `/health` and `/live` touch no dependencies (`app.go:135-147`). `/ready` checks Postgres, migrations, Redis (if required), audit persistence, registries, job leases, dead-letter backlog, outbox backlog and age, and per-feature client config (`cmd/main.go:207-410`), plus `WorkerReady` (`app.go:166-168`). Note: the compose healthcheck uses `/ready` (`deploy/platform/compose/fleet-partner-service.yml` healthcheck). |
| Timeouts on all external calls | IMPLEMENTED | `internal/platformclients/base.go:200-201` (default 10s), `:419` `context.WithTimeout`. `FLEET_PARTNER_HTTP_CLIENT_TIMEOUT` and `DISPATCH_SERVICE_TIMEOUT_SECONDS` in config. |
| Retries: only classified transient failures; writes need idempotency plus read-back | IMPLEMENTED | Outcome classification `base.go:64-100` (`OutcomeUnknown` is not retried blindly). Durable instructions use the same idempotency key and read back (wallet, execution start). Retry counter `fleet_partner_client_retry_attempts_total`. |
| Circuit breaking with metrics and degraded mode | IMPLEMENTED | `internal/platform/circuitbreaker`, wired at `base.go:191,227`. Gauge `fleet_partner_circuit_breaker_state`. Kill switches `config.go:126,177,196`. |
| Backups: DB backup, restore test, retention documented | PARTIAL | `scripts/backup.sh` and `scripts/restore-verify.sh`, plus platform `deploy/platform/bin/backup.sh` (nightly 02:30, `KEEP_DAYS=7`). DB dumps are compressed but **not encrypted**; only settings files are encrypted. `.github/workflows/certify-staging-restore.yml` records operator evidence only. |
| Disaster recovery objectives, replay, rebuild documented and tested | PARTIAL | `docs/section29-disaster-recovery.md` covers outbox replay, inbox dedupe, projection rebuild, lease recovery and a checklist. It admits RPO is not continuous (§1) and that no DR schedule exists (§16). |
| Capacity: index, pagination, batch sizing | PARTIAL | `docs/section29-reliability-observability.md` §13-15 reports local load profiles (30 orgs x 50 jobs). Nothing at production scale. |
| 29.1 Wallet unavailable: no invented balances | IMPLEMENTED | Durable instruction plus staleness gates (`wallet_projection_stale`, `wallet_operation_unknown` 503: `internal/platform/errorcodes/errorcodes.go:236-238`). |
| 29.1 Execution unavailable: no local accept without remote confirmation | IMPLEMENTED | `internal/app/private_acceptance.go`, `execution_start_reconciliation_required`, `source_outcome_unknown` (503). |
| 29.1 Notification unavailable: persist intent | IMPLEMENTED | Job-framework intents; test `TestNotificationDegradedMode_*` (per `docs/section29-reliability-observability.md` §8-11). |
| 29.1 Analytics unavailable: outbox queues | IMPLEMENTED | Transactional outbox, plus backlog gauges `fleet_partner_outbox_backlog` and `_oldest_pending_age_seconds`. |

---

## Section 56. Security, privacy, audit, retention and legal holds (spec lines 3743-3796)

### 56.1 Persistent aggregates

| Aggregate | Status | Evidence |
|---|---|---|
| audit_events | IMPLEMENTED | `fleet_partner_audit_records` (000001, 000069). |
| security_events | PARTIAL | `fleet_partner_auth_security_events` (`migrations/000003...up.sql:148-168`, append-only, ip/ua hashes) covers auth only. There is no generic security event store and no read API. |
| data_access_events | PARTIAL | `fleet_partner_document_access_audit` and `fleet_partner_sensitive_access_audit` tables. They cover documents and sensitive reads only, not export downloads. |
| retention_policies | IMPLEMENTED | `fleet_partner_retention_policies` (000068). |
| retention_actions | PARTIAL | `fleet_partner_retention_runs` and `_run_records`. Only 4 executable classes (expired export, org contact, support metadata, superseded projection), and the feature flag defaults off. |
| legal_holds | IMPLEMENTED | `fleet_partner_legal_holds` (000066). 10 scope types; documents are delegated to Media. |
| privacy_requests | MISSING | No table, model or route. |
| session_risk_records | MISSING | Sessions carry `ip_hash`/`user_agent_hash`/`device_label` (`000003...:85-101`) but there is no risk model. |

### 56.2 Binding business rules

| Rule | Status | Evidence / gap |
|---|---|---|
| Deny-by-default authz (session, membership, tenant, permission, branch, state, approval, restriction) | IMPLEMENTED | `ResolveTenant` → `RequireOperationalOrganization` → `accesscontrol.Authorizer.Require` → tenant SQL. |
| Step-up for role/secret changes, high-value finance approvals, payroll finalisation, sensitive exports, closure | PARTIAL | Only secrets (API clients/webhooks) are enforced from this list. See 28.1. |
| Logs, errors, metrics and events never contain secrets or unredacted financial data | IMPLEMENTED | `internal/platform/redact`. Metric-label cardinality guard in `shared/go/observability/cardinality.go`. |
| Audit events: actor, action, resource, **outcome**, reason, request/correlation ids, redacted metadata | PARTIAL | There is no `outcome` column in the audit table. Denials are audited only for documents. |
| Category-based retention; financial/safety/compliance/audit never hard-deleted via ordinary endpoints; legal hold overrides | PARTIAL | The policy model is category-based and the audit append-only trigger exists. The legal-hold override is honoured only by the retention executors. |
| Provider departure minimises org-held personal data | MISSING | No departure or minimisation code in `providers`/`managedmembers`/`workforce` (grep found no hits). |

### 56.3 API surface (`/fleet-partner/v1` prefix)

| Spec route | Status | Actual |
|---|---|---|
| GET /sessions | IMPLEMENTED | `internal/features/auth/http/handlers.go:113`. Lists only the caller's own sessions; no org-admin view. |
| POST /sessions/{id}/revoke | PARTIAL | `DELETE /sessions/:sessionId` (`handlers.go:114`), self-service only. `security.sessions.revoke` is seeded (000067) but **enforced nowhere**, and there is no admin revoke of another user. |
| POST /security/step-up | IMPLEMENTED (path differs) | `POST /auth/step-up/start` and `/auth/step-up/verify` (`handlers.go:110-111`). |
| GET /audit-events | IMPLEMENTED (path differs) | `GET /audit-logs`, `/audit-logs/:auditId` (`internal/features/audit/http/handlers.go:51-52`), plus `POST /audit/verify-chain`. |
| GET/POST /retention-policies | IMPLEMENTED | GET list, GET/PUT by class (`retentionpolicy/http/handlers.go:39-41`). |
| POST /retention-actions | PARTIAL | `POST /retention-runs`, `GET /retention-runs/:runId[/records]` (`retentionexecution/http/handlers.go:46-48`). 4 classes only, flag default off. |
| GET/POST /legal-holds | IMPLEMENTED | `legalhold/http/handlers.go:46-49` (plus release). |
| GET/POST /privacy-requests | MISSING | Not present. |
| GET /security-events | MISSING | Not present. Security events are write-only. |

### 56.4 Events

| Event | Status | Evidence |
|---|---|---|
| security.session_revoked, risk_detected; audit.recorded; legal_hold.applied/released; privacy_request.*; retention_action.completed/failed | MISSING | None of these is an outbox event. `legal_hold.created/released` and `retention.run.initiated` are audit **actions** only (`legalhold/usecases/service.go:164,235`; `retentionexec/admin.go:145`). |

### 56.5 Mandatory tests

| Test | Status | Evidence |
|---|---|---|
| Tenant IDOR | IMPLEMENTED | 22 test files (for example `internal/integration/audit_verify_chain_http_test.go`, `documents_external_access_test.go`). The object-auth registry coverage test is in `internal/section28contract`. |
| Branch ABAC | IMPLEMENTED | Branch filters in audit/legal-hold repositories. 35 integration test files exercise branches. |
| Token/session replay | IMPLEMENTED | Refresh reuse (`internal/features/auth/http/browser_cookie_test.go`), nonce replay (noncestore), session status (`internal/middleware/auth_test.go`). |
| Step-up | IMPLEMENTED | `TestSection28LegalHold_StepUp*`. |
| Audit append-only | IMPLEMENTED | `internal/integration/integration_test.go`, `audit_chain_test.go`. |
| Log redaction | IMPLEMENTED | `internal/platform/redact/redact_test.go`, `observability/metrics_test.go`. |
| Retention blocker / legal hold | IMPLEMENTED | `internal/integration/retention_execution_test.go` (`SKIPPED_LEGAL_HOLD`). |
| Privacy export/delete scope | MISSING | There is no privacy-request feature to test. |

---

## Section 63. Stable error code catalogue (spec lines 5698-5821)

How this was checked: `grep "\"<code>\""` over FPS `internal/`, `cmd/` and `shared/go/apperrors` (non-test), then usages of the matching constant.
FPS sends the canonical envelope with codes from `internal/platform/errorcodes/errorcodes.go` and generic shared codes
`bad_request|validation_failed|unauthorized|forbidden|not_found|conflict|rate_limited|service_unavailable|internal_error`
(`shared/go/apperrors/errors.go:11-19`).

| Code | Spec HTTP | Exact string in FPS? | Returned? | Status | Closest actual FPS code |
|---|---|---|---|---|---|
| authentication_required | 401 | no | - | MISSING | `unauthorized` (shared bearer middleware) |
| invalid_token | 401 | no | - | MISSING | `unauthorized`, `session_expired`, `session_revoked` (403 at `session_flows.go:125`) |
| mfa_required | 401 | no | - | MISSING | `step_up_required` / `step_up_expired`, returned as **403** (`session_flows.go:420-436`) |
| forbidden | 403 | yes (shared) | yes, via `apperrors.Forbidden` everywhere | IMPLEMENTED | - |
| organization_inactive | 403 | no | - | MISSING | `organization_verification_pending`, `organization_changes_required`, `organization_onboarding_incomplete`, `organization_not_eligible` (`middleware/organization_lifecycle.go:100-112`) |
| branch_scope_forbidden | 403 | no | - | MISSING | generic `forbidden` / `not_found` |
| resource_not_found | 404 | no | - | MISSING | `not_found` plus about 25 `*_not_found` feature codes |
| validation_error | 422 | yes (`errorcodes.ValidationFailed`, L162) | yes, 20 sites, but via `errorcodes.BadRequest` = **400**; shared `apperrors.Validation` returns `validation_failed` 422 in 22 files | PARTIAL | - |
| unsupported_service_type | 422 | no | - | MISSING | `service_catalog_category_disabled`, `bad_request` |
| idempotency_key_required | 400 | no | - | MISSING | `bad_request` "Idempotency-Key header is required." (`internal/platform/idempotency/http.go:48`, `idempotency.go:79`) |
| idempotency_conflict | 409 | yes (L210) | yes, 13 sites, 409 | IMPLEMENTED | - |
| version_conflict | 409 | literal only | yes, `privateoperations/usecases/service.go:257,352,483` (409) and `backgroundops/http/internal.go:283,301` (raw JSON, not the envelope) | PARTIAL | everywhere else `expected_version_mismatch` and `*_version_conflict` |
| state_transition_invalid | 409 | no | - | MISSING | `order_invalid_transition`, `opportunity_invalid_transition`, `*_invalid_state` |
| organization_limit_reached | 409 | no | - | MISSING | none |
| provider_identity_conflict | 409 | yes (L101) | yes, `providers/usecases/provider_service.go:219` | IMPLEMENTED | - |
| provider_already_linked | 409 | no | - | MISSING | `provider_already_managed` |
| provider_membership_inactive | 409 | no | - | MISSING | `provider_membership_not_active` (409, 1 site) |
| provider_not_eligible | 422 | yes (L185) | used as an eligibility **reason code** (`marketplace/usecases/eligibility.go:127,144,194`; `assignment_service.go:699`). HTTP mapping not confirmed as 422 | PARTIAL | - |
| provider_assignment_required | 409 | no | - | MISSING | `assignment_reassignment_required` |
| assignment_response_forbidden | 403 | no | - | MISSING | `assignment_not_owned_by_provider` (403) |
| assignment_response_expired | 409 | yes (L172) | yes, `assignment_service.go:585` | IMPLEMENTED | - |
| assignment_already_responded | 409 | no | - | MISSING | `assignment_already_resolved` (409) |
| assignment_acceptance_precondition_failed | 409 | no | - | MISSING | `eligibility_blocked`, `provider_acceptance_*` |
| offer_expired | 409 | no | - | MISSING | `marketplace_offer_expired` (409) |
| offer_already_resolved | 409 | no | - | MISSING | `marketplace_offer_already_resolved` |
| source_request_unavailable | 409 | no | - | MISSING | `request_not_assignable` |
| source_operation_pending | 202 | no | - | MISSING | `source_outcome_unknown` returned as **503 error** (spec wants a 202 operation, not an error) |
| pricing_not_configured | 422 | no | - | MISSING | none |
| commercial_data_hidden | 403 | no | - | MISSING | none; provider views omit the fields instead |
| wallet_not_ready | 409 | no | - | MISSING | `organization_wallet_not_ready`, `provider_subwallet_not_ready` |
| wallet_operation_pending | 202 | no | - | MISSING | `wallet_operation_unknown` (**503**) |
| wallet_insufficient_funds | 409 | no | - | MISSING | `insufficient_organization_wallet_funds` is defined but **never returned** |
| currency_mismatch | 422 | no | - | MISSING | none |
| duplicate_financial_source | 409 | no | - | MISSING | `provider_allocation_duplicate` |
| asset_not_eligible | 422 | no | - | MISSING | assets use generic `bad_request` / `conflict` (18 / 22 sites) |
| asset_unavailable | 409 | no | - | MISSING | generic `conflict` |
| asset_operationally_restricted | 409 | no | - | MISSING | `vehicle_safety_blocked` is defined but never returned |
| inspection_incomplete | 422 | no | - | MISSING | generic |
| return_to_service_forbidden | 409 | no | - | MISSING | generic |
| document_not_ready | 409 | no | - | MISSING | `document_not_downloadable`, `document_not_verifiable`, `media_authority_unavailable` |
| compliance_blocked | 409 | no | - | MISSING | generic `conflict`; `provider_restriction_active` is defined but never returned |
| proof_requirement_missing | 422 | no | - | MISSING | generic `bad_request` (16 sites in `proof`) |
| proof_immutable | 409 | no | - | MISSING | generic `conflict` |
| inventory_insufficient | 409 | no | - | MISSING | generic `conflict` (warehouse, 43 sites) |
| inventory_version_conflict | 409 | no | - | MISSING | generic |
| dock_slot_conflict | 409 | no | - | MISSING | generic |
| credit_hold | 409 | no | - | MISSING | `customer_credit_hold` (409) |
| approval_required | 409 | yes (L63) | **never returned**; features use `quotation_approval_required`, `contract_approval_required`, `order_approval_required`, `provider_*_approval_required` | PARTIAL | - |
| approval_expired | 409 | yes (L64) | yes, `approvals/usecases/service.go:992,1143` | IMPLEMENTED | - |
| segregation_of_duties_violation | 403 | no | - | MISSING | `approval_self_decision_forbidden`, `approval_separation_of_duties` (403) |
| import_validation_failed | 422 | no | - | MISSING | `import_invalid_file`, `import_schema_version_unsupported`, `import_not_validated` (`bulkimports/usecases/generic.go:150-258`) |
| export_not_ready | 409 | yes (L363) | yes, `exports/usecases/service.go:305` (409); the default branch at L307 returns it as 403 | IMPLEMENTED | - |
| rate_limit_exceeded | 429 | no | - | MISSING | `rate_limited` (`middleware/ratelimit.go:107`), `authentication_rate_limited` |
| downstream_unavailable | 503 | no | - | MISSING | `service_unavailable`, `fleet_service_unavailable`, `source_service_unavailable`, `media_authority_unavailable` |
| downstream_timeout | 504 | no | - | MISSING | none; timeouts map to `unknown_outcome` and 503 |
| event_version_gap | 409 | no | - | MISSING | `marketplace_projection_gap` (409), `stale_source_version` |
| schema_version_unsupported | 422 | no | - | MISSING | `import_schema_version_unsupported` (imports only) |
| internal_error | 500 | yes (shared L19) | yes, via `apperrors.Internal` | IMPLEMENTED | - |

Totals: 7 implemented, 4 partial, 47 missing.

## Appendix C. Error code catalogue (spec lines 2355-2410)

| Code | Spec HTTP | In FPS? | Returned? | Status | Notes / closest actual |
|---|---|---|---|---|---|
| invalid_request | 400 | no | - | MISSING | `bad_request` |
| validation_error | 422 | yes | yes, as 400 | PARTIAL | status mismatch; see §63 |
| unauthenticated | 401 | only as the downstream outcome label `OutcomeUnauthenticated` (`platformclients/base.go:70`), not as an error code | - | MISSING | `unauthorized` |
| forbidden | 403 | yes | yes | IMPLEMENTED | - |
| resource_not_found | 404 | no | - | MISSING | `not_found` / `*_not_found` |
| organization_not_active | 403 | no | - | MISSING | `organization_verification_pending` etc. |
| membership_not_active | 403 | no | - | MISSING | `provider_membership_not_active` (409); staff gets `forbidden` "You do not have an active organization membership." (`middleware/auth.go` ResolveTenant) |
| provider_identity_conflict | 409 | yes | yes | IMPLEMENTED | - |
| provider_already_managed | 409 | yes | yes, `provider_service.go:227-228,345` | IMPLEMENTED | - |
| assignment_already_resolved | 409 | yes | yes, `assignment_service.go:240,412,581` | IMPLEMENTED | - |
| assignment_response_expired | 409 | yes | yes | IMPLEMENTED | - |
| request_already_claimed | 409 | yes | yes, `dispatcher.go:223,249` | IMPLEMENTED | - |
| provider_not_eligible | 422 | yes | as an eligibility reason code | PARTIAL | HTTP status not confirmed |
| vehicle_not_eligible | 422 | yes | as an eligibility reason code (`eligibility.go:199`) | PARTIAL | HTTP status not confirmed |
| decline_reason_required | 422 | yes | yes, but via `errorcodes.BadRequest` = **400** (`assignment_service.go:348`, `offer_response.go:267`) | PARTIAL | status mismatch |
| expected_version_mismatch | 409 | yes | yes, 11 sites | IMPLEMENTED | - |
| idempotency_conflict | 409 | yes | yes | IMPLEMENTED | - |
| approval_required | 409 | yes | never returned | PARTIAL | feature-prefixed variants |
| external_service_unavailable | 503 | no | - | MISSING | `service_unavailable`, `fleet_service_unavailable` |
| external_outcome_unknown | 503 | no | - | MISSING | `source_outcome_unknown`, `wallet_operation_unknown` (both 503) |
| wallet_operation_rejected | 422 | no | - | MISSING | `wallet_operation_conflict` |
| insufficient_organization_balance | 422 | no | - | MISSING | `insufficient_organization_wallet_funds` is defined, never returned |
| asset_safety_blocked | 409 | no | - | MISSING | `vehicle_safety_blocked` is defined, never returned |
| compliance_restriction | 409 | no | - | MISSING | `provider_restriction_active` is defined, never returned |
| rate_limit_exceeded | 429 | no | - | MISSING | `rate_limited` |

Totals: 8 implemented, 5 partial, 12 missing.

**Catalogue observations**
1. The two spec catalogues disagree with each other. Appendix C says `unauthenticated`, `invalid_request` and `external_*`; Section 63 says `authentication_required`, `invalid_token` and `downstream_*`. Appendix C has 422 `insufficient_organization_balance` where Section 63 has 409 `wallet_insufficient_funds`. One catalogue has to be chosen before any rename.
2. FPS has no catalogue contract test and no OpenAPI error enum tying codes to statuses.
3. Several FPS constants are defined but never returned: `approval_required`, `insufficient_organization_wallet_funds`, `vehicle_safety_blocked`, `provider_restriction_active`.
4. Pending-outcome semantics are wrong for the spec. FPS returns 503 errors (`source_outcome_unknown`, `wallet_operation_unknown`), while the spec wants 202 operation-status responses with a polling link and no error envelope.
5. The asset, inspection, maintenance, compliance, proof and warehouse features return only generic `bad_request`/`conflict` codes. Clients cannot branch on them.

---

## Section 64. Security, privacy, visibility and audit controls (spec lines 5822-5943)

### 64.1 Identity and session

| Req | Status | Evidence / gap |
|---|---|---|
| Web users: short-lived access, rotating refresh, device/session records, revocation. Providers use their owning service's auth and get membership via a trusted exchange | IMPLEMENTED (provider part EXTERNAL) | FPS owns corporate auth (`internal/features/auth`). The session row holds ip/ua hash and a device label. Providers are authenticated by driver services, which call FPS's corporate-context and membership internal routes. |
| JWT validation: signature, iss, aud, exp, nbf, token/session version, subject status; algorithm allowlist | PARTIAL | `shared/go/auth/token.go:78-107` verifies only the HMAC signature and `exp`. The header `alg` is ignored (HS256 is hardcoded at L63), so algorithm confusion is structurally impossible. There are **no `iss`/`aud`/`nbf` claims**; role and service are checked by `BearerMiddleware` instead. Session status and membership are checked per request (`middleware/auth.go:142-158`). |
| Mandatory step-up for role/permission changes, approval decisions, wallet allocations/withdrawals, payroll finalisation, API secret rotation, sensitive exports, closure | PARTIAL | Enforced only for API secret rotation (and legal holds, security settings, retention). See 28.1. |
| S2S: mTLS or signed tokens with audience/scope, short lifetime, key rotation; internal endpoints reject end-user tokens | PARTIAL | HMAC with a 5-minute skew, nonce, and versioned-secret overlap. Scoping is opt-in, off by default (`config.go:560`). End-user tokens are rejected (`auth.go:43-56`). No mTLS. |
| Org API credentials hashed, shown once, scoped, rotatable, revocable, IP-restricted, rate-limited | IMPLEMENTED | `internal/features/integrations/service.go:100,145-286` (HMAC hash key of at least 32 bytes, rotation overlap), `auth.go:30-53` (current/previous hash, allowlist). Separate limiter at `app.go:212-220`. |

### 64.2 Authorization evaluation order

| Step | Status | Evidence |
|---|---|---|
| 1 Authenticate principal and active session | IMPLEMENTED | `CorporateBearer` plus session status (`auth.go:35,156`). |
| 2 Resolve org and membership from trusted context | IMPLEMENTED | `ResolveTenant` ignores client-supplied org IDs. |
| 3 Permission version plus org status | IMPLEMENTED | Role-version cache in the authorizer; `RequireOperationalOrganization` (`organization_lifecycle.go:70`). |
| 4 Org-scoped load, 404 not 403 | IMPLEMENTED | Tenant SQL. `docs/security.md:40` states cross-tenant returns 404 (verified by tests). |
| 5 Branch, dept, ownership, service type, field scope | PARTIAL | Branch is implemented (`PermittedBranchIDs`). Field scope is partly permission-only (fieldvisibility). |
| 6 Financial limits, step-up recency, approval ref | PARTIAL | Approval references are implemented (approvals engine). Step-up is limited. No generic per-actor transaction-limit check was found outside the approval policies. |
| 7 State transition / preconditions | IMPLEMENTED | Usecase state guards plus `expected_version_mismatch`. |
| 8 Audit denied high-risk attempts | PARTIAL | Only documents audit denials (`documents/usecases/external_access.go:21`, `documents/repositories/documents.go:1085`) and approval refusals (`fleet_partner_approval_refusal_write_failures_total`). |

### 64.3 Field visibility matrix

| Data | Status | Evidence / gap |
|---|---|---|
| Customer fare / contract rate (provider never; omitted, not null-masked) | IMPLEMENTED | Shaped provider DTOs; `docs/provider-data-minimization.md` §1 (omission rule). |
| KarryGo commission / platform settlement | IMPLEMENTED | Not in provider views. The managed settlement split is not exposed to providers. |
| Org margin / profitability (analytics row/column security) | PARTIAL | Reports are permission-gated. Column security relies on permission only. |
| Internal provider payout (provider only if policy exposes) | PARTIAL | `ProviderStatement` view exists. A policy-driven exposure switch was not verified. |
| Customer full contact (masked before acceptance / after retention) | PARTIAL / EXTERNAL | FPS masks CRM contacts (`crm/http/dto.go:40-68`). Trip-stage masking belongs to the execution services. |
| Provider personal / employment (HR roles; medical most restricted) | PARTIAL | Permission-gated workforce and providers. No separate medical/safety-notes tier. |
| Wallet master balance (finance only, separate endpoint) | IMPLEMENTED | companywallet views plus permissions. |
| Provider sub-wallet (own only, membership-scoped) | IMPLEMENTED | `SubWalletView`, membership join. |
| Government / licence documents (signed purpose URL) | IMPLEMENTED | documents restricted tiers (`documents.download_restricted`). |
| Safety incident evidence (legal hold, access audit) | PARTIAL | Safety references plus legal hold scope `INCIDENT`. Evidence access audit exists only through documents. |
| Audit events (auditor/owner; export audited) | PARTIAL | `audit.read` / `audit.read_sensitive`. There is no audit export dataset, so audited export is not possible. |

### 64.4 Cryptography and secrets

| Req | Status | Evidence / gap |
|---|---|---|
| TLS in transit; encryption at rest; envelope encryption with KMS and key versions | PARTIAL / EXTERNAL | TLS is terminated by nginx (`deploy/platform/nginx`). `fieldcrypto` exists but is unused. No KMS. DB backups are unencrypted (`deploy/platform/bin/backup.sh:11-16`). |
| Passwords adaptive-hashed; transaction PINs never in corporate storage | IMPLEMENTED | Auth is passwordless (`docs/passwordless-authentication.md`). No `pin` column in migrations; `pin_hash` appears only in the classifier keyword list (`internal/section28contract/sensitivecolumns.go:62`). |
| API/webhook secrets: CSPRNG, hashed, shown once, rotation overlap | IMPLEMENTED | `integrations/service.go` (`RotateAPIClient` / `RotateWebhookSecret` with a bounded overlap). |
| Redact auth headers, cookies, tokens, OTPs, secrets, bank/gov IDs, biometrics, free text | IMPLEMENTED | `internal/platform/redact/redact.go:22` (key-based). Free-text evidence is not logged. |
| Short-lived signed file URLs; no public URLs for private docs | IMPLEMENTED (EXTERNAL authority) | documents feature plus Media. |

### 64.5 Retention and privacy classes

| Req | Status | Evidence / gap |
|---|---|---|
| Class-based retention (10 spec classes) | IMPLEMENTED | 14 classes in `retentionpolicy/models` (a superset with different names). |
| Retention jobs check disputes, investigations, claims, tax periods, legal holds and cross-service ownership | PARTIAL | Executors check legal holds and owning-service authority (support metadata via support-dispute-service, exports via Media). No dispute/claim/tax-period checks. Only 4 classes are executable. |
| Deletion requests produce a traceable privacy case and completion report | MISSING | No privacy request feature. |

### 64.6 Audit event minimums

| Category | Status | Evidence / gap |
|---|---|---|
| Authentication / security (principal, session/device, result, IP hash, UA hash, request/correlation IDs) | PARTIAL | `fleet_partner_auth_security_events` has ip/ua hashes. The main audit table has no outcome and no hashed network metadata. |
| Access administration (before/after permission hashes, approvers, branch scope, step-up ref) | PARTIAL | Staff and role changes are audited with summaries. No permission hashes, and no step-up ref because step-up is not enforced there. |
| Provider management | PARTIAL | Audited actions exist (provider provisioning, membership). Source identity operation refs were not verified per row. |
| Assignment response (exact accept/decline, reason, deadline, device/session, source refs) | PARTIAL | `assignment.accepted/declined/acknowledged`, `offer.provider_accepted/declined` audit actions. Device/session are not captured. |
| Financial (amount, refs, operation/idempotency IDs, approvals, terminal state) | PARTIAL | `wallet.provider_allocation.*` and `wallet.provider_withdrawal.*` actions, but the dedicated `idempotency_key_reference`/`approval_reference` columns are not populated by companywallet. |
| Safety / compliance | PARTIAL | Audited via feature summaries. No evidence-access record outside documents. |
| Data access / export (scope, row estimate, file ID/expiry, download event) | PARTIAL | Export download counter plus document access audit. Row-estimate and query-scope capture not verified. |
| Administrative override (original state, reason, authority, expiry, compensating action) | PARTIAL | `internal/features/admininvestigations` and `admin_owner_read_audit` tables. Expiry and post-review are not modelled. |

---

## Section 65. Observability, reliability, SLOs and runbooks (spec lines 5944-6069)

### 65.1 Required endpoints

| Endpoint | Status | Evidence |
|---|---|---|
| GET /healthz | PARTIAL | Served as `/health` and `/live` (`internal/app/app.go:145-146`). Behaviour matches; the name differs. |
| GET /readyz (DB/config; event/wallet degradation **reported**) | PARTIAL | `/ready` (`app.go:150-176`). It fails hard on backlog thresholds and has no "degraded but ready" reporting. Failures expose only `{dependency, status}`. |
| GET /metrics (authenticated) | IMPLEMENTED | `app.go:178-182`; `MetricsAuth` bearer, constant-time compare (`internal/middleware/auth.go:240-253`). |
| GET /version (commit, build time, schema compat, version) | MISSING | No route. `/meta` returns only service/route-class context. |
| GET /internal/v1/status/dependencies | MISSING | No route. Breaker state appears only as a metric. |

### 65.2 SLOs

No SLO is defined anywhere for FPS: no SLO documents, recording rules or error-budget alerts. Measurability of each objective:

| Objective | Status | Notes |
|---|---|---|
| Read API availability 99.9% | PARTIAL | `fleet_partner_http_requests_total{status}` gives 5xx counts. No read/command split, no SLO rule. |
| Command API availability 99.9% | PARTIAL | Same. |
| P95 reads < 400ms | MISSING | No histogram buckets (`shared/go/observability/metrics.go:99-110,131-135`). |
| P95 commands < 800ms | MISSING | Same. |
| Accept/Decline P95 < 500ms | MISSING | Counters only; no latency for the assignment response command. |
| Offer/assignment projection 99% < 5s | MISSING | No consumer-lag or occurredAt-delta metric. Freshness is a row status only (`GAP_DETECTED`). |
| Outbox publication 99.9% < 30s | PARTIAL | `fleet_partner_outbox_oldest_pending_age_seconds` exists. No alert. |
| Wallet projection freshness 99% < 60s with stale marker | PARTIAL | Stale marker exists (`wallet_projection_stale`). No freshness metric. |
| Critical notification enqueue 99% < 5s | MISSING | No intent-to-acceptance latency metric. |
| RPO <= 5 min | MISSING | Nightly dumps (`deploy/platform/bin/backup.sh:2-16`). No WAL/PITR (`docs/section29-disaster-recovery.md` §1). |
| RTO <= 60 min, quarterly exercise | PARTIAL | Mechanical restore is measured (`scripts/restore-verify.sh`, DR doc §3). No quarterly exercise. |
| Tenant isolation: zero exposure | PARTIAL | Tests plus registries. No runtime anomaly metric. |

### 65.3 Mandatory metrics and alerts

| Family | Status | Actual metrics |
|---|---|---|
| http_requests_total / duration / in_flight | PARTIAL | `fleet_partner_http_requests_total{method,route,status}`, `fleet_partner_http_request_duration` (sum/count only), `fleet_partner_http_active_requests`. No org label (good). |
| db_pool_* / query_duration | PARTIAL | 9 `fleet_partner_db_pool_*` series (`internal/observability/dbpool.go`). No query duration. |
| outbox_pending / oldest_age / publish_failures | PARTIAL | `fleet_partner_outbox_backlog`, `_oldest_pending_age_seconds`, `_publish_failures_total`, `_stale_backlog`. No `event family` label. |
| inbox_lag / duplicates / gap_detected | PARTIAL | `fleet_partner_stream_consumer_{processed,payload_conflicts,decode_errors,...}_total`. No lag or gap metric. |
| durable_jobs {type,status} | PARTIAL | `fleet_partner_jobs{status}` (`internal/jobs/worker.go:102`), `job_{claims,retries,lease_lost,succeeded}_total`. No `type` label. Dead-letter count appears only in readiness. |
| assignment_responses | PARTIAL | `fleet_partner_assignment_{accepts,declines,expiries}_total`, `offer_provider_*`. No latency. |
| source_integration {service,operation,outcome} | IMPLEMENTED | `fleet_partner_client_calls_total`, `_client_call_duration`, `fleet_partner_source_{command_errors,unknown_outcomes}_total`, `source_reconciliation_backlog`. |
| wallet_reconciliation {status,currency} | MISSING | None. Only the config flag `WALLET_RECONCILIATION_ENABLED` exists. |
| projection_freshness | MISSING | None. |
| security_denials {reason,route} | PARTIAL | `fleet_partner_rate_limit_denied_total{risk_class}` only. No authz/IDOR denial counter. |
| imports/exports/reports | PARTIAL | Exports have counters. Imports and reports have none. |
| compliance/maintenance | PARTIAL | `fleet_partner_maintenance_{expired,errors}_total`. No compliance expiry backlog. |
| webhook_delivery | IMPLEMENTED | `fleet_partner_webhook_{deliveries_total,delivery_backlog,disabled_subscriptions,dead_letter_count,...}`. |
| resource_utilization (CPU, mem, goroutines, FDs) | MISSING | No Go runtime/process collector in `shared/go/observability`. |
| **Alerts** | MISSING | The only alert rules in the repo are `deploy/monitoring/customer-v4/alerts.rules.yml`. There are no FPS rules, no Prometheus scrape config and no dashboards. `docs/FINAL_OBSERVABILITY_AND_ALERT_MATRIX.md:107-108` proposes two PromQL expressions that are not committed as rules. |

### 65.4 Logging and tracing

| Req | Status | Evidence / gap |
|---|---|---|
| JSON logs: timestamp, severity, service, env, requestId, correlationId, trace/span, route template, actor type, org, resource type/ID, result, error code | PARTIAL | All present (`middleware.go:218-246`, `logging.go:14-21`) except resource type/ID. |
| No raw bodies by default | IMPLEMENTED | `RequestLogger` logs no bodies. |
| Traces cover edge, DB spans, downstream, outbox, workers | PARTIAL | Edge, downstream, outbox and worker spans exist as log/metric spans. No DB spans, no exporter. |
| Every critical workflow traceable by correlationId | IMPLEMENTED | correlationId is carried through jobs, outbox envelopes, finance operations and audit rows (`docs/FINAL_OBSERVABILITY_AND_ALERT_MATRIX.md:24`, `tracing.go:24-37`). |

### 65.5 Required runbooks

Existing FPS runbooks (`docs/runbooks/`): auth-delivery-failure, contract-activation-failure, contract-expiry, crm-dead-letter-replay, customer-credit-hold, organization-activation, organization-closure, provider-provisioning-reconciliation, quotation-acceptance-conflict, quotation-delivery-failure, sla-unknown-outcome. Also `docs/rollback.md` and `docs/section29-disaster-recovery.md`.

| Spec runbook | Status | Coverage |
|---|---|---|
| Database unavailable | PARTIAL | DR doc §3 restore and §13 checklist. No step for "stop write workers / keep liveness". |
| Event consumer lag/gap | PARTIAL | DR doc §5-7 (outbox replay, inbox dedupe, projection rebuild). |
| Source execution unknown | PARTIAL | `runbooks/sla-unknown-outcome.md`, `provider-provisioning-reconciliation.md`. Nothing assignment- or execution-specific. |
| Wallet discrepancy (finance kill switch, dual approval) | PARTIAL | The kill switch exists (`FinanceExecutionKillSwitch`, `config.go:177`) and DR doc §9 covers reconciliation. No runbook. |
| Mass notification failure | MISSING | - |
| Cross-tenant suspicion | MISSING | - |
| Compromised org user / API key | MISSING | Tooling exists (revoke API client, self session revoke) but there is no runbook, and admins cannot revoke other users' sessions. |
| Unsafe asset reported | MISSING | - |
| Worker dead-letter growth | PARTIAL | `runbooks/crm-dead-letter-replay.md` (CRM only), DR doc §5. |
| Bad deployment/migration | PARTIAL | `docs/rollback.md`, `deploy/platform/bin/rollback.sh`. |
| Object storage/media outage | MISSING | - |
| Regional disaster | PARTIAL | DR doc §11-14. Single region; no event recovery across regions. |

### 65.6 Backup and DR

| Req | Status | Evidence / gap |
|---|---|---|
| PITR, encrypted backups, restore verification; include audit, idempotency, outbox/inbox, jobs | PARTIAL | Full logical dumps include all tables. No PITR. DB dumps are not encrypted. Restore verification exists (`scripts/restore-verify.sh`, `backup.sh --test-restore`). |
| Object files backed up per media policy; FPS keeps IDs and hashes only | IMPLEMENTED (EXTERNAL) | FPS stores file references only (`000001` `fleet_partner_file_references`). |
| Redis is ephemeral; authoritative state in Postgres | IMPLEMENTED | DR doc §4. The outbox is Postgres-backed. |
| Quarterly isolated recovery exercises | MISSING | Not scheduled (DR doc §16). `certify-staging-restore.yml` only records evidence. |
| Idempotency/external reconciliation before high-risk writes resume | PARTIAL | Documented in the DR checklist §13. No enforced "writes disabled until reconciled" gate. |

---

## 1. Cross-service work required

**shared/go**
- `shared/go/auth`: add `iss`, `aud` and `nbf` claims, a token or session version claim, and strict validation (64.1). This affects every service that uses `TokenSigner`.
- `shared/go/apperrors`: decide on one catalogue, Section 63 or Appendix C. Add the canonical generic codes (`resource_not_found`, `rate_limit_exceeded`, `idempotency_key_required`, `authentication_required`/`invalid_token`, `downstream_unavailable`/`downstream_timeout`, `version_conflict`, `state_transition_invalid`) or an alias layer, and make `validation_error` 422 consistently. Add a 202 "operation pending" response helper (not an error).
- `shared/go/observability`: histogram buckets for durations, Go runtime/process collector (goroutines, memory, FDs), HELP/TYPE lines.
- Tracing: a shared OTLP exporter behind `StartSpan`, plus DB span instrumentation.

**api-gateway**: originate `traceparent` and correlation IDs at the edge, apply edge rate limiting with a 429 `Retry-After` that matches the catalogue code, terminate TLS, and pass `/version`.

**payment-wallet-service**: return typed rejection and insufficient-funds codes that FPS can map to `wallet_operation_rejected` / `insufficient_organization_balance`. Expose wallet reconciliation status/currency data for the `wallet_reconciliation` metric. Keep PIN handling out of FPS (already true).

**driver-dispatch-delivery-service / driver-hauling-service / driver-taxi-service**:
- Proxy provider-facing history (`/providers/{id}/offers|assignments|memberships`) to the provider apps.
- Relay FPS assignment error codes unchanged (`assignment_response_expired`, `assignment_already_resolved`, `decline_reason_required`, `provider_membership_not_active`).
- Revoke provider sessions when FPS membership is suspended (28.1).
- Implement provider-departure minimisation, and provider-side privacy export/delete, in the owning identity stores (56.2).
- Mask customer contact by trip stage (64.3).

**media-file-service**: signed URLs bound to user, purpose and resource; object versioning and backup policy (65.6); scan status (already authoritative). Supply the outage signal for the "object storage/media outage" degraded mode.

**notification-service**: security notifications (session revoked, risk detected, legal hold applied) and an admin/security recipient audience for cross-tenant and compromise incidents. Intent-to-acceptance latency for the notification SLO.

**support-dispute-service / verification-compliance-service**: dispute, claim and investigation blockers for retention execution (64.5). Privacy request fan-out (export/delete) across the services that own customer and provider data.

**customer-service**: privacy-request participation for customer contacts that FPS mirrors into CRM.

**admin-backoffice-service**: platform-operator access to org audit and legal holds; cross-tenant incident tooling; security event review.

**deploy / infra (`deploy/platform`, `.github/workflows`)**:
- Continuous WAL/PITR (RPO 5 minutes) and encrypted DB dumps.
- Prometheus scrape config for FPS `/metrics` with the token.
- Alert rules for every 65.3 family, plus SLO recording rules and dashboards.
- KMS for `fieldcrypto` envelope keys.
- Set `FLEET_PARTNER_SERVICE_CREDENTIAL_SCOPING_ENABLED=true` and `FLEET_PARTNER_REDIS_REQUIRED=true` in production; ensure `FLEET_PARTNER_METRICS_AUTH_TOKEN` and `FLEET_PARTNER_AUDIT_CHAIN_SECRET` are set.
- A scheduled quarterly DR exercise; optional mTLS.
- The compose healthcheck should probe `/live`, not `/ready`, if it ever drives restarts.

**fleet-partner-service itself** (for completeness):
- privacy_requests, session_risk_records and `/security-events`.
- Admin session revocation.
- The 56.4 outbox events.
- Step-up on approvals, allocations/withdrawals, payroll, exports, role changes and closure.
- An audit `outcome` column, hashed IP/UA capture, and population of approval/idempotency refs in finance audits.
- `/version` and the dependency status route.
- Readiness that reports degraded state.
- Revoke sessions on org suspension and on credential change.
- Domain deletion paths that call `IsUnderHold`.
- `fieldcrypto` usage.
- 5 missing runbooks.
- Error-code alignment and a catalogue contract test.

## 2. Frontend work required

**web/fleet (Next.js)**. The security settings pages exist but are mocked. Verified files: `src/app/(dashboard)/settings/sessions/page.tsx` ("MOCK_SESSIONS"), `settings/security/page.tsx` ("(mock)" toasts at L186/191), `settings/retention/page.tsx` (L156 "(mock)"), and `settings/legal-holds`, `settings/audit`, `settings/audit/verification`, `settings/access-history` all contain MOCK markers. Work needed:
- **Sessions**: wire `GET /sessions` and `DELETE /sessions/:id`. Show device label, last seen and the current session. Add an admin view of other users' sessions once FPS builds it.
- **Step-up**: a reusable modal for the `POST /auth/step-up/start` then `/verify` OTP flow. Pass `stepUpReference` in the bodies of legal-hold, security-settings, API-client/webhook and retention-run requests. Handle `step_up_required` / `step_up_expired` (403) by re-prompting and retrying. Extend this to approvals, wallet allocations/withdrawals, payroll, exports, role changes and closure as FPS enforces them. `src/lib/types/auth.ts:53` already has a `stepUp` type.
- **Session / auth errors**: on `session_revoked` / `session_expired` / `refresh_token_reused`, force sign-out with a message. Keep the existing lifecycle-refusal event (`src/lib/api/client.ts:65-68`) in sync with `organization_*` codes.
- **Error handling**: one central code→message/action map following whichever catalogue is chosen. Show `field` violations from 422. Back off and show a countdown using `Retry-After` on 429 (`rate_limited`; today only auth handles it, in `src/lib/auth/authErrors.ts:132`). Poll the resource for `*_outcome_unknown` / 202 pending operations. Ask the user to refresh on `expected_version_mismatch` / `version_conflict`. Show approval-request links for `*_approval_required`.
- **Audit**: real audit-log list and detail. Show sensitive fields only when the user holds the permission. Verify-chain page. Export once an AUDIT dataset exists.
- **Privacy / retention**: real retention policies (raise-only validation), retention runs (initiate with step-up, run records), legal holds (create/release with step-up, version guard), and privacy requests and security events once FPS builds them.
- **Security settings**: real GET/PUT with `expectedVersion` and step-up when MFA is required.
- **API clients and webhooks**: show the secret once, rotate with an overlap window, IP allowlist, all with step-up.
- **Wallet views**: render "unavailable, last updated at" when the projection is stale (29.1). Never show invented balances.

**Provider apps (`apps/dispatch_provider`, `apps/truck_provider`, `apps/taxi_provider`)**. A grep found almost no corporate-membership handling: `taxi_provider` has 0 matches, and the other two match only in auth models.
- Organization membership indicator and states (active, suspended), including forced logout or degraded mode on `provider_membership_not_active`.
- Screens for own assignment, acceptance and decline history, plus membership history (28.5 / 56), through each owning service's proxy.
- Accept/decline UX: a mandatory decline reason (`decline_reason_required`), and handling for `assignment_response_expired` / `assignment_already_resolved` / `assignment_not_owned_by_provider` with a refresh-state action.
- Never render commercial fields. A client-side test should assert that provider DTOs carry no customer fare or commission keys.
- Rate-limit (429) and `Retry-After` handling, plus generic `service_unavailable` / outcome-unknown polling for accept actions whose outcome is unknown.
- Privacy: export/delete request entry points once the owning services support them. Show the provider their own audit/access events if policy exposes them (64.3).
