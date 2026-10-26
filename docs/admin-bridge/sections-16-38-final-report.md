# Karryam Secure Admin — Sections 16–38 final report (26 October 2026)

## A. Overall status

**BACKEND_INTEGRATION_READY** for every technically implementable KarryGo-side capability in
Sections 16–38, subject to the mandatory CI results recorded in section D. All new capabilities
default to disabled. **Not** END_TO_END_CERTIFIED and **not** PRODUCTION_READY: no Centralized
Admin, separate-server run, production key, signed policy, staff grant or rollout approval exists.

Work was done directly on `staging` from `e58ab7d0b17c5f30ad7a4d67f0715feda9976a59`. No branch,
worktree, PR, `main` change, force push, deployment, production migration or Headquarters
repository change.

## B. Section-by-section result

### Phase A — owner integrations

**16 Support (support-dispute-service)** — `IMPLEMENTED`, integration-tested.
Owner resources `cases`, `sos-cases`, `disputes`, `case-messages`, `case-evidence` (redacted
≤240-rune labels; no bodies, names, phones, coordinates). Nine commands on a real state machine
(assign, reply, transition, escalate, close, attach evidence, SOS escalation, resolve dispute,
record remedy) with version fencing, native grants and `complaint_events`. Replies are idempotent
by `client_message_id`, ordered by a per-case sequence; notification delivery is tracked
separately from the stored message. Dispute remedy: Support records it only for
FAVOUR_COMPLAINANT/SPLIT, once; the Bridge first requires a CONFIRMED Wallet `REQUEST_REFUND`
(same dispute, same amount) and CONFIRMED `APPROVE_REFUND` in the same market/organization.
`support.case.changed.v1` events in the same transaction; HMAC snapshot route for backfill.
Migration `009_secure_admin_boundary.sql`. Blocked: SLA policy (`POLICY_UNAVAILABLE`), locale
(not recorded). Note: one enum value (`complainant_type='admin'`) cannot be removed by rollback.

**17 Notification campaigns** — `IMPLEMENTED`, integration-tested. Customer-service is now the
marketing consent authority (migration `020_marketing_consent.sql`, per-channel opt-in/withdrawal
with append-only history, audience and send-time check routes for notification-service only,
privacy withdrawal covers marketing). Notification-service accepts versioned approved intents
(migration `009_campaigns.sql`), plans recipients from consent, re-checks consent at send, honours
quiet hours, rolling 24 h frequency cap, message cap, dedupe, cancellation and revision
supersession. Truthful results: logging providers are `LOGGED_ONLY` (own counter), never sent;
`delivered_count` is null and `delivery_confirmation=UNAVAILABLE` (no provider receipts). Also
fixed: `notification_messages.status` was never updated. Blocked: SMS/WhatsApp, delivery
receipts, emergency broadcasts, zone targeting (no zone data).

**18 Verification** — `IMPLEMENTED`, integration-tested. One state machine shared by the decision
route, admin commands and an expiry sweep; decision-route defects fixed (unknown case 404,
idempotency conflict 409 instead of 500, illegal transition 409, reasons/future expiry required,
human decider recorded). Automation records findings (document presence/format, duplicate
document across businesses, Media object state, external authority) and a recommendation but
never decides; external KYC is a disabled adapter, so `AUTO_ELIGIBLE` is unreachable. Appeals via
`REOPEN_CASE` with a second reviewer. Append-only history, findings and appeals. Migration
`000004_secure_admin_boundary`. Blocked: external KYC authority, address consistency (no data).

**19 Media** — `IMPLEMENTED` (security fix), integration-tested. **Found and fixed:** the legacy
`GET /api/v1/media-files/:id/download` served any stored file — including identity documents,
delivery proofs and signatures — without authentication. Now only profile photos are public;
everything else needs the authenticated owner route or a private grant
(`MEDIA_LEGACY_PUBLIC_SENSITIVE_DOWNLOAD_ENABLED`, default false). Also: SVG never inline,
owner-id path validation, high-risk purposes fail closed without a SAFE scan, admin case grants
single-use, immutable revocation on closed cases, private legacy-mapping lookup (exposed on the
Bridge as `hq.media.legacy-mapping`), adopted legacy assets stop serving, tombstones honour holds.
Migration `000009_secure_admin_section19`. **Consumer impact:** clients fetching Dispatch/Hauling/
Taxi proof or document URLs anonymously now get 404; without a deployed scanner high-risk
documents cannot be downloaded. Blocked: ClamAV certification; document/signature legacy mapping.

**20 Gateway** — `IMPLEMENTED`. Two real bypass classes fixed: websocket upgrades were carried on
any matched route (now an explicit allowlist per upstream), and the client Host was relayed as
`X-Forwarded-Host`. Full encoding/case/method/host/absolute-form/prefix-confusion matrix pinned by
tests; explicit nginx denies in the ingress template.

### Phase B — shared framework

**21 Registry** — 435 capabilities (388 baseline + 10 Support/8 Verification resource rows +
12 command rows + 17 `hq.*`). Discovery reports AVAILABLE only when the client, flag, signed rule,
role and scopes all exist. Deterministic OpenAPI/schemas/checksums; backward compatibility checked
against the starting SHA; traceability register generated by `scripts/admin-bridge/traceability.py`.

**22 Events and projections** — `shared/go/adminevents` (strict envelope, compiled allowlisted
adapters with closed field kinds; restricted classes cannot be registered). Bridge migration
`007_hq_integration.sql`: deduplicated inbox, hash-only quarantine, version-fenced projections,
gap detection and repair, append-only HQ outbox with monotonic acks, backfill and reconciliation
runs with PARTIAL_COVERAGE on any failure, missing-at-source recorded not deleted, truthful
freshness (UNKNOWN when no data, never zero). Producers: Support, Verification, Notification.
Other owners keep their existing contracts (no new producer events invented).

**23 Command orchestration** — existing lifecycle preserved; extended with the Wallet-confirmed
remedy gate and campaign orchestration (approval consumed atomically, one owner submission,
UNKNOWN_OUTCOME resolved only by read-back).

**24 Unified support** — `GET /hq/support/cases` (composite `karryam:<source>:<case>` identity,
emergency first) plus Section 16 owner reads/commands; simulated HQ consumer tests.

**25 Marketing** — intent vs result kept separate; HQ two-person approval bound to the exact
digest; Bridge attaches requester/approval identities; owner validation and truthful results.

### Phase C

**26 Finance** — Wallet `GET /internal/payment-wallet/v1/admin/finance-reports` (one REPEATABLE
READ snapshot, per currency, exact source fields, null-not-zero, no revenue claim) and
`hq.finance.report`. Blocked: revenue recognition policy; corporate `finance_*` ledger not in
measures; `provider_earned` is gross of reversals.

**27 Observability** — loopback-only `/metrics` (denials, audit failures, ingress outcomes,
quarantine, gaps, lag, consumer lag, unknown outcomes, open SOS and age); structured logs carry
service, product and build SHA, never bodies/tokens. SLO thresholds not invented.

**28 Threat model** — see `a-z-final-security-and-release-gates.md` §1 (16 threats, controls,
negative tests). Also fixed a Referral defect: command history was always denied because a
`FOR SHARE` grant check ran in a read-only transaction.

**29 Deployment** — hardened admin compose, inert ingress network, internal DB, non-root (CI now
asserts image uid matches compose `user`), dry-run-by-default port closing incl. 8110, nftables
candidates, external scan and host-evidence scripts. Not applied anywhere.

**30 Privacy** — classification map (`data-classification-map.md`), digest-only privacy
register with per-owner dispositions (customer-service fulfilment accepted only with a CONFIRMED
owner operation; other holders POLICY_REQUIRED; legal hold), approved AES-256-GCM single-use
15-minute exports with download audit, projection tombstones.

**31 Source obligations** — changes confined to the owning services; HMAC semantics unchanged.

### Phase D

**32–38** — phase assessment, gates, decision register and STOP triggers in
`a-z-final-security-and-release-gates.md`; runbooks in `sections-16-38-runbook.md`;
traceability in `a-z-final-capability-traceability.json`.

## C. Test certification (local, disposable PostgreSQL 16 + Redis 7)

| Suite | Result |
|---|---|
| Bridge + Taxi adminread + Analytics adminprivacy + analyticsadmin + adminpolicy, `-race`, testgate | 806 passed, 0 failed, 0 skipped |
| Owners: Support, Notification, Customer, Verification, Media, Wallet, Referral, shared admin packages, `-race -p 1` | 856 passed in the gated run; 4 environment-gated tests (customer closure migration, wallet isolated outbox, two wallet load tests) passed separately with their variables set; now enabled in CI |
| Gateway + partner-web + serviceauth, `-race`, gateway testgate | 305 passed, 0 skipped |
| Deployment-safety unit tests | 37 run; the Docker restore cycle needs a Docker daemon (CI) |
| Contracts | OpenAPI 3.1 + 98 schemas validated; deterministic regeneration; backward compatibility vs `e58ab7d` |

Simulated HQ scenarios (Section 33.4 #1, 2, 6–12) pass over a real TLS 1.3 mTLS listener; this is
single-host simulation, not inter-VPS evidence.

## D. Git and CI evidence

Code commits: `e58ab7d..1af0c495d37601d88f8777b29201cdf660614139` (shared contracts, Support,
Notification/Customer, Verification, Media, Gateway/deploy, Wallet, Referral fix, Bridge/registry/CI).
This report, the runbooks, the gates document and the traceability register are committed on top
in a documentation-only commit; that commit is the certified SHA. Its mandatory workflow run IDs
and conclusions (Admin Bridge 2–5, Admin Owner Closure, Section 31, Customer V4, Vendor 46–48,
Partner Gateway split, Deployment Safety, Admin Sections 16–38) are reported in the handover
message, because recording them here would require a further, untested commit.

First-attempt failure, disclosed: on `cc500c9` the new "Admin Sections 16–38" workflow (run
40298958085) failed because its Bridge step did not build the Taxi owner binary, so
`TestTaxiOwnerProcessPostgresMTLS` skipped and the zero-skip gate rejected the run. The test itself
passed on that SHA in "Admin bridge Sections 2-5" (run 40298957956). The workflow was corrected
(build the binary, provision its disposable database) in the next commit, not rerun.

## E. Remaining external requirements

- **BLOCKED_DOMAIN:** Pricing quote issue/bind/reconcile; Wallet `finance.job.settlement_reversed.v1`
  producer; global commerce closure; Referral appeals/campaigns/rankings; verification address data;
  legacy document/signature media association.
- **BLOCKED_POLICY:** signed access policies, owner contexts, staff grants; approval thresholds;
  revenue recognition; retention/erasure schedules; SOS SLA/staffing; marketing legal bases and
  age rules; KYC lawful basis; emergency broadcast authority.
- **BLOCKED_INFRA:** private network/VPN and firewall application; certificates and keys; ClamAV;
  SMS/WhatsApp and delivery-receipt providers; external scans; production migrations, backups and
  rollout; real Centralized Admin.
- **NOT_IMPLEMENTED (software):** none identified within the repository's authority.

## F. Production safety

No production deployment, migration, Admin activation, key installation, Headquarters repository
change, public Gateway exposure, `main` change, force push or destructive reset.
