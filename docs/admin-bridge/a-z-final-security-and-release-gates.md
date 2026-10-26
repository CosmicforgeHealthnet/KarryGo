# Karryam Secure Admin — Sections 16–38 security and release gates

This file is the Section 28 threat register, the Section 32 K0–K10 phase assessment,
the Section 33 acceptance-gate list, the Section 36 definition of done and the Section 37
decision register and STOP triggers. It records evidence produced in this repository on
`staging` only. Nothing here is a production certification, a penetration test or an
inter-VPS end-to-end result.

Evidence labels: **TESTED-LOCAL** = run against disposable PostgreSQL 16/Redis in this
session; **CI** = enforced by a GitHub workflow on the final SHA (see the final report);
**ARTIFACT** = configuration/tooling validated syntactically but not applied; **EXTERNAL** =
needs real infrastructure, a third party or a signed approval.

## 1. Section 28 — threat model and controls

| # | Threat | Software control in this repository | Negative test evidence | Residual risk / external requirement |
|---|---|---|---|---|
| T1 | Compromised HQ browser | No browser path to the Bridge: TLS 1.3 mTLS with a SPIFFE workload URI pinned in the trust file, HMAC v2 with a dedicated HQ key and a short-lived Ed25519 staff assertion bound to the exact request digest; R3 actions need a strong ACR and a second staff approval | `TestRealMTLSBoundary`, `TestNonceAndJTIReplay`, `TestPostgresHQBridgeRoutesEndToEnd` (self-approval 409) | HQ session hardening (HttpOnly, step-up UX) lives in `centralize-admin` — EXTERNAL |
| T2 | Stolen staff assertion | Audience/product/action binding, `request_digest` of method+URI+body, ≤300 s lifetime, durable JTI and nonce claims, staff/session revocation table | `TestNonceAndJTIReplay`, `TestPostgresSimulatedHQScenarios` (#11 revoked staff 401) | Revocation propagation latency depends on HQ issuer — EXTERNAL |
| T3 | Stolen workload credential | Certificate fingerprint enrolled in `admin_remote_principals` with expiry/revocation; wrong SPIFFE identity rejected | `TestRealMTLSBoundary/wrong_principal`, simulator scenario 2 | Certificate issuance/rotation by the chosen private CA — EXTERNAL |
| T4 | Stolen HMAC key | Per-audience keys (HQ, each owner resource/command/read-back, campaign, finance, snapshot, event-ingress source, export key) refused at start-up if any two are equal or equal an HQ key | `TestOwnerCommand…`, `LoadHQ` distinct-key checks, `TestScopedAnalyticsConfigurationNeedsBothFlagsAndDistinctCredential` | Key custody and rotation drill — EXTERNAL (runbook §2) |
| T5 | Public admin endpoint discovery | Gateway never registers the admin upstream, denies `/api/v1/admin` and any `internal` segment after up to 8 decoding layers, case-folding, dot-segment and backslash normalisation; nginx denies before proxying; Bridge listens only on the private bind IP | Gateway `TestPublicAdminIsolation`, `TestPublicOwnerAdministrativePathsNeverReachOwners`, Section 20 additions; `validate.py` nginx ordering | External black-box scan of the real host — EXTERNAL (`scripts/admin-bridge/exposure/external_scan.sh`) |
| T6 | SSRF through adapters | Every owner client has a compiled origin and fixed path dictionary; redirects disabled; no request field selects host, path, method, SQL or service | `TestPrivateMediaCaseClientUsesFixedPathsAndRejectsUnboundOrRawResponses`, `NewSnapshotClient` refuses paths/queries | — |
| T7 | Cross-tenant data | Five-scope delegation (market, zone, organization, tenant + product) intersected with signed owner contexts; owner-native grants; HQ projections filtered by scope; exports, approvals and privacy rows scope-bound | `TestOwnerResourcesDelegation`, `TestPostgresSupportRemedyRequiresWalletConfirmedRefund` (cross-market), Support/Verification secure-admin tests | Production scope mappings unsigned — BLOCKED_POLICY |
| T8 | Duplicate financial action | Owner idempotency + immutable decision ledger; Bridge operation slot uniqueness; UNKNOWN_OUTCOME never resubmitted; Support remedy requires Bridge-observed CONFIRMED Wallet refund operations | `TestPostgresOwnerCommandUncertainOutcomeReconcilesWithoutResubmit`, `TestPostgresSupportRemedyRequiresWalletConfirmedRefund`, campaign UNKNOWN_OUTCOME read-back in `TestPostgresHQBridgeRoutesEndToEnd` | — |
| T9 | Self or forged approval | Database CHECK `approver_ref <> requester_ref` on both approval tables; approval consumed with the operation in one transaction; Bridge attaches approvals, HQ cannot send an approval object | `TestPostgresOwnerCommandTwoPersonApprovalIsBoundSingleUseAndRequesterOnly`, HQ e2e (self-approve, foreign submit) | — |
| T10 | Event poisoning | Source service bound to HMAC caller; compiled adapters with closed field kinds; unknown/extra fields, free text, foreign product and cross-owner events quarantined (hash only); version fencing | `TestPostgresEventInboxDedupeVersionFenceGapAndQuarantine`, `TestPostgresSignedIngressReplayAndCallerBinding`, `TestUnknownFieldAndFreeTextQuarantine` | — |
| T11 | Analytics/HQ data leakage | Adapters cannot carry RESTRICTED_PII, CREDENTIAL or HEALTH classes (registration panics); Support events exclude subject, description, names, phones, coordinates; Verification events exclude document refs; finance report aggregate-only | `TestRegistryNeverExportsRestrictedClasses`, `TestPostgresFeedCursorScopeAndMonotonicAck`, owner event tests | Small-cell suppression values — BLOCKED_POLICY |
| T12 | Lateral movement / tunnel key theft | Admin compose: dedicated non-routed ingress network (no masquerade, no ICC), DB internal-only, non-root, read-only, cap_drop ALL, no-new-privileges; nftables candidate allows only the HQ peer to 8110 | `validate.py` compose assertions, `test_network.py`, `scripts/release` deployment tests | Real firewall/VPN application — EXTERNAL |
| T13 | Compromised Bridge workload | Bridge holds no owner database credentials; owners re-authorize every call with their own grants/contexts; runtime DB role validated non-privileged; evidence tables append-only | `TestRuntimeRoleGuardAndGrants`, `TestOwnerCommandRuntimeRoleGrantsAndEvidenceImmutability` | — |
| T14 | Supply-chain compromise | Pinned Go toolchain (`go1.26.10`), `govulncheck`, Trivy image scan, credential grep in CI | admin-bridge.yml, section31-mandatory.yml | Signed image provenance and SBOM attestation in the release pipeline — EXTERNAL |
| T15 | Unsafe rollback / operator error | Global `ADMIN_REMOTE_WRITES_ENABLED` kill switch and per-family flags; migrations forward-only for evidence tables; durable cursors and original-identity read-back | `TestHQRoutesDefaultDisabledAndWritesKillSwitch`, `TestMigrationFailureRollsBackAndDownIsIrreversible` | Rehearsed restore on real backups — EXTERNAL |
| T16 | Public legacy media URL (found and fixed) | Media now serves only profile photos publicly; documents, proofs and signatures need authenticated owner or private grant; SVG never inline; high-risk purposes fail closed without a SAFE scan | Media `section19_test.go` (10 tests) | ClamAV production certification — EXTERNAL |

No independent third-party penetration test has been performed. These controls are not a
guarantee of invulnerability.

## 2. Section 32 — K0–K10 phase assessment

| Phase | Software | Tested | Staging-integrated | Blocked by infrastructure | Requires approval |
|---|---|---|---|---|---|
| K0 audit | Gap matrix and source register (`sections-16-38-final-report.md`, `a-z-final-capability-traceability.json`) | n/a | — | — | — |
| K1 network isolation | Gateway denial, nginx denial, compose topology, nftables candidate, close-ports tooling, external scan script | TESTED-LOCAL + CI | — | Firewall/VPN application, external scan | Network topology owner |
| K2 bridge authentication | mTLS + HMAC v2 + delegated staff + five-scope policy + replay + audit | TESTED-LOCAL + CI | — | Live certificates | HQ issuer/JWKS approval |
| K3 owner clients | Typed fixed clients for all owners incl. Support, Verification, Notification campaigns, Wallet finance, Media mapping, owner snapshots | TESTED-LOCAL + CI | — | Live credentials | — |
| K4 read models | Event inbox, projections, HQ outbox, backfill, reconciliation, freshness/coverage | TESTED-LOCAL + CI | — | Owner relays enabled with real secrets | — |
| K5 visibility | Owner resource families for 16 boundaries (435 capabilities) | TESTED-LOCAL + CI | — | — | Signed owner contexts and policies |
| K6 restricted commands | Owner command dictionaries incl. Support (9) and Verification (6) | TESTED-LOCAL + CI | — | — | Staff grants, approval thresholds |
| K7 money/pricing/referral | Existing Wallet/Pricing/Referral boundaries preserved; finance reporting added; Referral history defect fixed | TESTED-LOCAL + CI | — | — | Revenue recognition, rate books, reversal producer |
| K8 HQ departments | Support queue, campaign intent/result, finance exchange, privacy register, exports | TESTED-LOCAL + CI (simulated HQ) | — | Real Centralized Admin | Marketing/consent/retention policy |
| K9 security/reliability | Threat register above; negative suites; race tests | TESTED-LOCAL + CI | — | Load/soak, DR rehearsal, third-party pentest | Security sign-off |
| K10 rollout | Checklists, runbooks, scan/capture scripts, migration register | ARTIFACT | — | Everything live | All production sign-offs |

## 3. Section 33 — acceptance gates

Mandatory, runnable here and in CI: Go build/vet for every module; gofmt of changed files;
`govulncheck`; credential grep; OpenAPI 3.1/JSON-Schema validation and deterministic
regeneration with checksums; backward-compatibility diff against the starting SHA;
PostgreSQL-backed suites with zero skips for the Bridge, Support, Verification, Notification,
Customer, Media, Wallet and Referral; `-race` for the Bridge and owner Admin packages;
migration up/replay/rollback tests where the owner provides them; gateway public-denial
suites; deployment-safety unit tests. A test skipped because a database URL was absent is
not counted as a pass (testgates fail on skips).

Simulated (single host, real mTLS listener, disposable PostgreSQL): Section 33.4 scenarios
1, 2, 6, 7, 8, 9, 10, 11, 12 in `TestPostgresSimulatedHQScenarios`; scenarios 4 and 5 in the
Support and Notification owner suites. **Not executed:** separate-VPS connectivity, real
provider delivery receipts, real KYC providers, external port scans, backup restore on
production-like data.

## 4. Section 36 — definition of done

The highest label this repository can reach is **BACKEND_INTEGRATION_READY**: all
technically implementable KarryGo-side software is complete, default-off and tested,
mandatory CI is green on the final SHA, and only policy, infrastructure and external-domain
dependencies remain. **END_TO_END_CERTIFIED** and **PRODUCTION_CERTIFIED** require the real
Centralized Admin, separate-server runs and signed approvals and are not claimed.

## 5. Section 37 — decision register

| Decision | Owner | Approval needed | Affected capabilities | Safe default | Verification | Unblocking evidence |
|---|---|---|---|---|---|---|
| Private transport/topology | Platform/network | Signed topology record | All Bridge ingress, event ingress | Bridge bound to loopback/private IP; remote disabled | `external_scan.sh`, `capture_host_evidence.sh` | Scan + host evidence from staging |
| Staff roles, markets, tenants, purposes | HQ governance + each owner | Signed access policy and owner contexts | Every delegated read/command | `blocked_policy` | Policy signature + owner context import | Signed policy revision pinned in `admin_policy_revisions` |
| HQ workload identity / issuer | Security | Issuer, JWKS and SPIFFE URI | All | Trust file required at start | `TestRealMTLSBoundary` on staging certs | Enrolled principal row |
| Signing keys, pinning, revocation | Security | Key ceremony | All HMAC/Ed25519 | Start-up refuses missing/duplicate keys | Rotation drill (runbook §2) | Drill record |
| Approval thresholds / two-person finance | Finance + Security | Threshold policy | Wallet, Support remedy, campaigns, exports | Every declared action requires a second approver | Owner tests | Signed threshold policy |
| Production rate books / price cutover | Pricing owner | Four-eye policy | Pricing quote features | Quote cutover disabled | Pricing suites | Approved rate book |
| Global commerce/market closure | Commerce owner | Domain semantics | Vendor/market controls | Not implemented | — | Approved closure contract |
| Notification channels/providers | Marketing + Legal | Contracted providers | Campaign delivery | Logging providers count as LOGGED_ONLY, never SENT; delivered UNAVAILABLE | Notification suite | Provider contracts + receipt integration |
| Verification/KYC authority | Compliance + Legal | Lawful basis + provider contract | Verification automation | `DisabledAuthority`; AUTO_ELIGIBLE unreachable | Verification suite | Contract + certification |
| Malware scanner | Security | ClamAV certification | Media high-risk reads | Fail closed (no SAFE → no read) | Media suite | Certified scanner deployment |
| Consent, marketing, retention rules | Legal/DPO | Retention schedule, legal bases, age rules | Campaigns, privacy register | POLICY_REQUIRED dispositions; no deletion | Privacy register tests | Approved schedule |
| Revenue recognition | Finance | Recognition policy | Finance exchange | `SOURCE_FIGURE_RECOGNITION_POLICY_PENDING` | Wallet finance suite | Approved policy |
| SOS staffing/escalation SLA | Operations/Safety | SLA policy | Support SOS | `sla_state=POLICY_UNAVAILABLE` | Support suite | Approved SLA |
| Production CI/test credentials | Platform | Secured CI secrets | Staging certification | CI uses disposable containers | Workflows | Staging secrets in CI |
| Production migrations/backups/rollout | Platform + owners | Change approval | All | Nothing applied | Runbooks | Signed change record |

### STOP triggers (no production promotion if any holds)

Admin publicly reachable; delegated identity bypassable; cross-tenant authorization fails;
any money/verification/custody owner check bypassed; sensitive data in HQ projections;
stale/unknown figures shown as zero; approval or replay race fails; mandatory database tests
skipped; a private peer reaches unrelated products; backup restore or rollback unproven.
Unrelated safe software work may continue; only the unsafe promotion stops.
